package minio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/minio/internal/errs"
	"github.com/jmgilman/go/fs/minio/internal/types"
	"github.com/minio/minio-go/v7"
)

// File represents a MinIO object handle.
// Behavior differs based on open mode (read vs write).
type File struct {
	fs   *MinioFS
	key  string // Full S3 key (including prefix)
	name string // Original name provided to Open/Create
	mode int    // Open flags (O_RDONLY, O_WRONLY, etc.)

	// Read mode fields
	// reader wraps downloaded object data. We use interface{} to hold a type
	// that implements both io.ReadSeeker and io.ReaderAt (like *bytes.Reader).
	reader interface {
		io.ReadSeeker
		io.ReaderAt
	}
	size    int64     // Object size
	modTime time.Time // Last modified time

	// Write mode fields
	buffer       *bytes.Buffer  // Accumulates writes for small files
	pipeW        *io.PipeWriter // Streaming writer once threshold exceeded
	putRes       chan error     // Result from background PutObject when streaming
	bytesWritten int64          // Total bytes written (for Stat in write mode)
	closed       bool           // Prevent double-close
}

// newFileWrite creates a File in write mode with an empty buffer.
func newFileWrite(mfs *MinioFS, key, name string, flag int) *File {
	return &File{
		fs:           mfs,
		key:          key,
		name:         name,
		mode:         flag,
		buffer:       new(bytes.Buffer),
		pipeW:        nil,
		putRes:       nil,
		bytesWritten: 0,
		closed:       false,
	}
}

// Read reads up to len(p) bytes into p. It returns the number of bytes read
// and any error encountered. At end of file, Read returns 0, io.EOF.
// Read is only supported in read mode (O_RDONLY).
func (f *File) Read(p []byte) (int, error) {
	if f.mode&os.O_WRONLY != 0 {
		return 0, errs.PathError("read", f.name, fs.ErrInvalid)
	}
	if f.reader == nil {
		return 0, errs.PathError("read", f.name, fs.ErrInvalid)
	}
	n, err := f.reader.Read(p)
	if err == nil || errors.Is(err, io.EOF) {
		return n, err
	}
	return n, errs.PathError("read", f.name, err)
}

// getEffectiveThreshold returns the threshold for transitioning to streaming writes.
// If the threshold is 0 or negative, returns the default 5MB.
func (f *File) getEffectiveThreshold() int64 {
	threshold := f.fs.multipartThreshold
	if threshold <= 0 {
		threshold = 5 * 1024 * 1024 // 5MB default
	}
	return threshold
}

// transitionToStreaming transitions from buffered writes to streaming writes.
// It creates a pipe, starts a background upload goroutine, and flushes any existing buffer.
// Returns the number of bytes from p that were written and any error.
// nolint:contextcheck // Background upload by design; io.Writer.Write cannot accept context
func (f *File) transitionToStreaming(p []byte) (int, error) {
	pr, pw := io.Pipe()
	f.pipeW = pw
	f.putRes = make(chan error, 1)

	go func() {
		_, err := f.fs.client.PutObject(
			context.Background(),
			f.fs.bucket,
			f.key,
			pr,
			-1,
			minio.PutObjectOptions{
				ContentType: "application/octet-stream",
			},
		)
		_ = pr.Close()
		f.putRes <- errs.Translate(err)
		close(f.putRes)
	}()

	// Flush existing buffered data into the pipe, then discard buffer
	if f.buffer != nil && f.buffer.Len() > 0 {
		if _, err := f.pipeW.Write(f.buffer.Bytes()); err != nil {
			return 0, errs.PathError("write", f.name, err)
		}
		f.buffer = nil
	}

	n, err := f.pipeW.Write(p)
	f.bytesWritten += int64(n)
	if err != nil {
		return n, errs.PathError("write", f.name, err)
	}
	return n, nil
}

// Write writes len(p) bytes from p to the underlying data stream.
// It returns the number of bytes written and any error encountered.
// Write is only supported in write mode (O_WRONLY, O_CREATE).
// nolint:contextcheck // io.Writer.Write signature cannot accept a context parameter
func (f *File) Write(p []byte) (int, error) {
	if f.closed {
		return 0, errs.PathError("write", f.name, fs.ErrClosed)
	}
	if f.mode&(os.O_WRONLY|os.O_RDWR) == 0 {
		return 0, errs.PathError("write", f.name, fs.ErrInvalid)
	}

	// If already streaming, write directly to pipe
	if f.pipeW != nil {
		n, err := f.pipeW.Write(p)
		f.bytesWritten += int64(n)
		if err != nil {
			return n, errs.PathError("write", f.name, err)
		}
		return n, nil
	}

	threshold := f.getEffectiveThreshold()

	// If buffer plus incoming payload stays under threshold, keep buffering
	if f.buffer != nil && int64(f.buffer.Len()+len(p)) <= threshold {
		n, err := f.buffer.Write(p)
		f.bytesWritten += int64(n)
		if err != nil {
			return n, errs.PathError("write", f.name, err)
		}
		return n, nil
	}

	// If client or bucket is not configured (e.g., unit tests), fall back to buffering
	if f.fs == nil || f.fs.client == nil || f.fs.bucket == "" {
		if f.buffer == nil {
			f.buffer = new(bytes.Buffer)
		}
		n, err := f.buffer.Write(p)
		f.bytesWritten += int64(n)
		return n, err
	}

	// Transition to streaming
	return f.transitionToStreaming(p)
}

// Seek sets the offset for the next Read operation. It returns the new offset
// and an error, if any. Seek is only supported in read mode.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.mode&os.O_WRONLY != 0 {
		return 0, errs.PathError("seek", f.name, core.ErrUnsupported)
	}
	if f.reader == nil {
		return 0, errs.PathError("seek", f.name, fs.ErrInvalid)
	}
	pos, err := f.reader.Seek(offset, whence)
	if err != nil {
		return pos, errs.PathError("seek", f.name, err)
	}
	return pos, nil
}

// ReadAt reads len(p) bytes from the File starting at byte offset off.
// It returns the number of bytes read and any error encountered.
// ReadAt is only supported in read mode.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	if f.mode&os.O_WRONLY != 0 {
		return 0, errs.PathError("readat", f.name, core.ErrUnsupported)
	}
	if f.reader == nil {
		return 0, errs.PathError("readat", f.name, fs.ErrInvalid)
	}
	n, err := f.reader.ReadAt(p, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, errs.PathError("readat", f.name, err)
	}
	return n, err
}

// Stat returns the FileInfo structure describing the file.
// In read mode, returns the size and modTime from the downloaded object.
// In write mode, returns the current buffer size and current time.
func (f *File) Stat() (fs.FileInfo, error) {
	if f.mode&os.O_WRONLY != 0 {
		// Write mode: return bytes written so far
		return types.NewFileInfo(f.name, f.bytesWritten, time.Now(), 0644), nil
	}
	// Read mode: return downloaded object info
	return types.NewFileInfo(f.name, f.size, f.modTime, 0644), nil
}

// Close closes the file, releasing any resources.
// In write mode, Close uploads the buffer contents to S3.
// In read mode, Close is a no-op.
func (f *File) Close() error {
	if f.closed {
		return nil // Already closed, idempotent
	}
	f.closed = true

	// If in write mode, finalize upload depending on mode
	if f.mode&(os.O_WRONLY|os.O_RDWR) != 0 {
		// Streaming mode
		if f.pipeW != nil && f.putRes != nil {
			_ = f.pipeW.Close()
			if err := <-f.putRes; err != nil {
				return errs.PathError("close", f.name, err)
			}
			return nil
		}
		// Buffered small file
		if f.buffer != nil {
			return f.sync(context.Background())
		}
	}

	return nil
}

// Sync commits the current contents of the file to S3 storage.
// In write mode, uploads the buffer contents via PutObject.
// In read mode, Sync is a no-op.
// Sync can be called multiple times (idempotent).
func (f *File) Sync() error {
	if f.mode&(os.O_WRONLY|os.O_RDWR) != 0 {
		// Streaming mode: data is being uploaded asynchronously
		if f.pipeW != nil {
			return nil
		}
		if f.buffer != nil {
			return f.sync(context.Background())
		}
	}
	return nil
}

// sync is the internal implementation that performs the actual upload.
func (f *File) sync(ctx context.Context) error {

	// Upload the buffer contents
	reader := bytes.NewReader(f.buffer.Bytes())
	_, err := f.fs.client.PutObject(
		ctx,
		f.fs.bucket,
		f.key,
		reader,
		int64(f.buffer.Len()),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		},
	)
	if err != nil {
		return errs.Translate(err)
	}

	return nil
}

// Name returns the name of the file as provided to Open or Create.
func (f *File) Name() string {
	return f.name
}

// streamingFile provides streaming reads without buffering entire objects.
// This type is used for read operations to minimize memory usage.
type streamingFile struct {
	fs     *MinioFS
	key    string
	name   string
	obj    *minio.Object
	info   minio.ObjectInfo
	offset int64 // Current read position for Seek implementation
	closed bool
}

// newStreamingFile creates a streaming file handle for reading.
// It opens the object for streaming without downloading the entire content.
func newStreamingFile(ctx context.Context, mfs *MinioFS, key, name string) (*streamingFile, error) {
	// First get metadata using StatObject (doesn't open a stream)
	info, err := mfs.client.StatObject(ctx, mfs.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, errs.PathError("open", name, errs.Translate(err))
	}

	// Then get the object for streaming
	obj, err := mfs.client.GetObject(ctx, mfs.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errs.PathError("open", name, errs.Translate(err))
	}

	return &streamingFile{
		fs:     mfs,
		key:    key,
		name:   name,
		obj:    obj,
		info:   info,
		offset: 0,
		closed: false,
	}, nil
}

// Read reads up to len(p) bytes into p from the streaming object.
func (f *streamingFile) Read(p []byte) (int, error) {
	if f.closed {
		return 0, errs.PathError("read", f.name, fs.ErrClosed)
	}
	n, err := f.obj.Read(p)
	f.offset += int64(n)

	// Normalize EOF behavior: if we read any data, return nil error
	// Only return EOF when no data is read
	if n > 0 && errors.Is(err, io.EOF) {
		return n, nil
	}
	return n, err
}

// Close closes the streaming file and releases resources.
func (f *streamingFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	return f.obj.Close()
}

// Stat returns file information for the streaming file.
func (f *streamingFile) Stat() (fs.FileInfo, error) {
	return types.NewFileInfo(
		filepath.Base(f.name),
		f.info.Size,
		f.info.LastModified,
		0644,
	), nil
}

// Name returns the name of the file.
func (f *streamingFile) Name() string {
	return f.name
}

// Write is not supported for read-only streaming files.
func (f *streamingFile) Write(_ []byte) (int, error) {
	return 0, errs.PathError("write", f.name, fs.ErrInvalid)
}

// Seek sets the read position for the next Read operation.
// It reopens the object with a range request starting at the new offset.
func (f *streamingFile) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, errs.PathError("seek", f.name, fs.ErrClosed)
	}

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		newOffset = f.info.Size + offset
	default:
		return 0, errs.PathError("seek", f.name, fs.ErrInvalid)
	}

	if newOffset < 0 {
		return 0, errs.PathError("seek", f.name, fs.ErrInvalid)
	}

	// If seeking to current position, no need to reopen
	if newOffset == f.offset {
		return newOffset, nil
	}

	// Close current object
	_ = f.obj.Close()

	// Reopen with range starting at new offset
	opts := minio.GetObjectOptions{}
	if newOffset > 0 {
		if err := opts.SetRange(newOffset, 0); err != nil {
			return 0, errs.PathError("seek", f.name, err)
		}
	}

	// nolint:contextcheck // fs.File.Seek cannot accept context; using background context
	obj, err := f.fs.client.GetObject(context.Background(), f.fs.bucket, f.key, opts)
	if err != nil {
		return 0, errs.PathError("seek", f.name, errs.Translate(err))
	}

	f.obj = obj
	f.offset = newOffset
	return newOffset, nil
}

// ReadAt reads len(p) bytes from the file starting at byte offset off.
// It uses HTTP range requests for efficient random access.
func (f *streamingFile) ReadAt(p []byte, off int64) (int, error) {
	if f.closed {
		return 0, errs.PathError("readat", f.name, fs.ErrClosed)
	}

	if off < 0 {
		return 0, errs.PathError("readat", f.name, fs.ErrInvalid)
	}

	// Use a dedicated range request for ReadAt (doesn't affect main stream position)
	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(off, off+int64(len(p))-1); err != nil {
		return 0, errs.PathError("readat", f.name, err)
	}

	// nolint:contextcheck // fs.File.ReadAt cannot accept context; using background context
	obj, err := f.fs.client.GetObject(context.Background(), f.fs.bucket, f.key, opts)
	if err != nil {
		return 0, errs.PathError("readat", f.name, errs.Translate(err))
	}
	defer func() {
		_ = obj.Close()
	}()

	return io.ReadFull(obj, p)
}

// Compile-time interface checks.
var (
	_ core.File   = (*File)(nil)
	_ fs.File     = (*File)(nil)
	_ io.Seeker   = (*File)(nil)
	_ io.ReaderAt = (*File)(nil)
	_ core.Syncer = (*File)(nil)

	_ core.File   = (*streamingFile)(nil)
	_ fs.File     = (*streamingFile)(nil)
	_ io.Seeker   = (*streamingFile)(nil)
	_ io.ReaderAt = (*streamingFile)(nil)
)
