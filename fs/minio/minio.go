package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/minio/internal/errs"
	"github.com/jmgilman/go/fs/minio/internal/pathutil"
	"github.com/jmgilman/go/fs/minio/internal/types"
	"github.com/jmgilman/go/fs/minio/internal/walk"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

// MinioFS implements core.FS for MinIO/S3-compatible storage.
// Note: The name follows the package naming convention (LocalFS, MemoryFS, etc.)
// used throughout the fs library to distinguish between different implementations.
//
//nolint:revive // MinioFS name is intentional to match naming pattern across fs implementations
type MinioFS struct {
	client             *minio.Client
	bucket             string
	prefix             string // Optional prefix for all keys
	multipartThreshold int64  // Threshold for multipart uploads
	renameConcurrency  int    // Max concurrent operations for directory rename
}

// NewMinIO creates a MinIO-backed filesystem.
// Returns error if configuration is invalid or connection fails.
func NewMinIO(cfg Config) (*MinioFS, error) {
	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	var client *minio.Client
	var err error

	// Use provided client or create new one
	if cfg.Client != nil {
		client = cfg.Client
	} else {
		// Create new MinIO client
		client, err = minio.New(cfg.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.UseSSL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create minio client: %w", err)
		}
	}

	// Normalize prefix: use forward slashes, trim trailing slash
	prefix := pathutil.NormalizePrefix(cfg.Prefix)

	// Set default multipart threshold if not specified
	multipartThreshold := cfg.MultipartThreshold
	if multipartThreshold == 0 {
		multipartThreshold = 5 * 1024 * 1024 // 5MB default
	}

	// Set default rename concurrency if not specified
	renameConcurrency := cfg.MaxRenameConcurrency
	if renameConcurrency == 0 {
		renameConcurrency = 10 // Default to 10 concurrent operations
	}

	return &MinioFS{
		client:             client,
		bucket:             cfg.Bucket,
		prefix:             prefix,
		multipartThreshold: multipartThreshold,
		renameConcurrency:  renameConcurrency,
	}, nil
}

// joinPath joins the filesystem prefix with the given name.
// It handles empty prefix correctly and uses forward slashes.
func (m *MinioFS) joinPath(name string) string {
	return pathutil.JoinPath(m.prefix, name)
}

// Stub implementations for core.FS interface
// These will be implemented in subsequent tasks

// Open opens the named file for reading.
// Returns a streaming file that doesn't buffer the entire object in memory.
// The returned file supports Seek and ReadAt via HTTP range requests.
func (m *MinioFS) Open(name string) (fs.File, error) {
	key := m.joinPath(name)
	return newStreamingFile(context.Background(), m, key, name)
}

// Stat returns file information for the named file.
func (m *MinioFS) Stat(name string) (fs.FileInfo, error) {
	key := m.joinPath(name)
	ctx := context.Background()

	// Use StatObject to get object metadata
	info, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, errs.PathError("stat", name, errs.Translate(err))
	}

	return types.NewFileInfo(
		filepath.Base(name),
		info.Size,
		info.LastModified,
		0644,
	), nil
}

// ReadDir reads the directory named by name and returns a list of directory entries.
func (m *MinioFS) ReadDir(name string) ([]fs.DirEntry, error) {
	key := m.joinPath(name)
	ctx := context.Background()

	// Ensure key ends with "/" for directory listing (unless it's root)
	if key != "" && !strings.HasSuffix(key, "/") {
		key = key + "/"
	}

	var entries []fs.DirEntry

	// List objects with delimiter to get directory structure
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    key,
		Recursive: false, // Use delimiter for directory-like listing
	}) {
		if object.Err != nil {
			return nil, errs.PathError("readdir", name, errs.Translate(object.Err))
		}

		// Skip the directory itself if it appears as an object
		if object.Key == key {
			continue
		}

		// Get the name relative to the directory
		relName := strings.TrimPrefix(object.Key, key)

		// Check if this is a subdirectory (ends with /)
		isDir := strings.HasSuffix(object.Key, "/")
		if isDir {
			relName = strings.TrimSuffix(relName, "/")
		}

		// Skip empty names
		if relName == "" {
			continue
		}

		// For files, use object metadata
		entries = append(entries, types.NewS3DirEntry(
			relName,
			isDir,
			object.Size,
			object.LastModified,
		))
	}

	// Sort entries by name to enforce fs.ReadDir contract
	// MinIO typically returns results sorted by key, but we enforce it for strict compliance
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

// ReadFile reads the named file and returns the contents.
func (m *MinioFS) ReadFile(name string) ([]byte, error) {
	key := m.joinPath(name)
	ctx := context.Background()

	// Get size first to pre-allocate exact buffer size
	info, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, errs.PathError("readfile", name, errs.Translate(err))
	}

	// Stream object directly into pre-allocated buffer
	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errs.PathError("readfile", name, errs.Translate(err))
	}
	defer func() {
		_ = obj.Close()
	}()

	// Pre-allocate buffer with exact size for single allocation
	buf := make([]byte, info.Size)
	_, err = io.ReadFull(obj, buf)
	if err != nil {
		return nil, errs.PathError("readfile", name, err)
	}

	return buf, nil
}

// Exists reports whether the named file or directory exists.
func (m *MinioFS) Exists(name string) (bool, error) {
	_, err := m.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Create creates the named file for writing.
func (m *MinioFS) Create(name string) (core.File, error) {
	key := m.joinPath(name)
	return newFileWrite(m, key, name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC), nil
}

// OpenFile opens the named file with the specified flags and permissions.
// Supported flags: O_RDONLY, O_WRONLY, O_CREATE, O_TRUNC.
// Unsupported flags: O_RDWR, O_APPEND, O_EXCL, O_SYNC (returns ErrUnsupported).
func (m *MinioFS) OpenFile(name string, flag int, _ fs.FileMode) (core.File, error) {
	// Check for unsupported flags
	if flag&os.O_RDWR != 0 {
		return nil, errs.PathErrorf("open", name, "%w: O_RDWR not supported in S3", core.ErrUnsupported)
	}
	if flag&os.O_APPEND != 0 {
		return nil, errs.PathErrorf("open", name, "%w: O_APPEND not supported in S3", core.ErrUnsupported)
	}
	if flag&os.O_EXCL != 0 {
		return nil, errs.PathErrorf("open", name, "%w: O_EXCL not supported in S3", core.ErrUnsupported)
	}
	if flag&os.O_SYNC != 0 {
		return nil, errs.PathErrorf("open", name, "%w: O_SYNC not supported in S3", core.ErrUnsupported)
	}

	key := m.joinPath(name)

	// Handle write modes
	if flag&(os.O_WRONLY|os.O_CREATE) != 0 {
		return newFileWrite(m, key, name, flag), nil
	}

	// Handle read mode (O_RDONLY or no flags) - use streaming file
	return newStreamingFile(context.Background(), m, key, name)
}

// WriteFile writes data to the named file.
func (m *MinioFS) WriteFile(name string, data []byte, _ fs.FileMode) error {
	file, err := m.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = file.Write(data)
	if err != nil {
		return errs.PathError("writefile", name, err)
	}

	if err := file.Close(); err != nil {
		return errs.PathError("writefile", name, err)
	}

	return nil
}

// Mkdir creates a new directory with the specified name and permissions.
// In S3, directories are virtual, so this is a no-op that always succeeds.
func (m *MinioFS) Mkdir(name string, _ fs.FileMode) error {
	// S3 has virtual directories - no need to create them explicitly
	// Just validate the path is normalized
	_ = m.joinPath(name)
	return nil
}

// MkdirAll creates a directory path, including any necessary parents.
// In S3, directories are virtual, so this is a no-op that always succeeds.
func (m *MinioFS) MkdirAll(path string, _ fs.FileMode) error {
	// S3 has virtual directories - no need to create them explicitly
	// Just validate the path is normalized
	_ = m.joinPath(path)
	return nil
}

// Remove removes the named file or directory.
func (m *MinioFS) Remove(name string) error {
	key := m.joinPath(name)
	ctx := context.Background()

	err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return errs.PathError("remove", name, errs.Translate(err))
	}

	return nil
}

// RemoveAll removes path and any children it contains.
func (m *MinioFS) RemoveAll(path string) error {
	key := m.joinPath(path)
	ctx := context.Background()

	// Ensure key ends with "/" for recursive listing (unless it's root)
	if key != "" && !strings.HasSuffix(key, "/") {
		key = key + "/"
	}

	// Create channel for objects to delete
	objectsCh := make(chan minio.ObjectInfo, 100)

	// Launch lister goroutine
	var listErr error
	go func() {
		defer close(objectsCh)
		for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
			Prefix:    key,
			Recursive: true,
		}) {
			if object.Err != nil {
				listErr = object.Err
				return
			}
			objectsCh <- object
		}
	}()

	// Use RemoveObjects batch API for efficient deletion
	errorCh := m.client.RemoveObjects(ctx, m.bucket, objectsCh, minio.RemoveObjectsOptions{})

	// Collect errors from deletion
	var errList []error
	for err := range errorCh {
		if err.Err != nil {
			errList = append(errList, err.Err)
		}
	}

	// Check for list error first
	if listErr != nil {
		return errs.PathError("removeall", path, errs.Translate(listErr))
	}

	// Return first delete error if any
	if len(errList) > 0 {
		return errs.PathError("removeall", path, errs.Translate(errList[0]))
	}

	return nil
}

// Rename renames (moves) oldpath to newpath.
// In S3/MinIO, this is implemented as parallel copy + batch delete.
//
// IMPORTANT: This operation is NOT atomic. If an error occurs during
// the copy phase, some objects may have been copied. If an error occurs
// during the delete phase, objects will exist at both old and new paths.
//
// For directories, this uses a bounded worker pool for parallel copies
// followed by batch deletion. The default concurrency is 10 workers.
func (m *MinioFS) Rename(oldpath, newpath string) error {
	oldKey := m.joinPath(oldpath)
	newKey := m.joinPath(newpath)
	ctx := context.Background()

	// First, try to stat the oldpath to see if it's a file
	_, err := m.Stat(oldpath)
	if err == nil {
		// It's a file, do simple copy
		return m.renameFile(oldKey, newKey, oldpath)
	}

	// Check if it's a virtual directory
	dirPrefix := oldKey
	if dirPrefix != "" && !strings.HasSuffix(dirPrefix, "/") {
		dirPrefix = dirPrefix + "/"
	}

	newPrefix := newKey
	if newPrefix != "" && !strings.HasSuffix(newPrefix, "/") {
		newPrefix = newPrefix + "/"
	}

	// Parallel copy all objects
	copied, err := m.parallelCopy(ctx, dirPrefix, newPrefix)
	if err != nil {
		return errs.PathError("rename", oldpath, errs.Translate(err))
	}

	if len(copied) == 0 {
		return errs.PathError("rename", oldpath, fs.ErrNotExist)
	}

	// Batch delete old objects
	toDelete := make(chan minio.ObjectInfo, len(copied))
	go func() {
		defer close(toDelete)
		for _, key := range copied {
			toDelete <- minio.ObjectInfo{Key: key}
		}
	}()

	errorCh := m.client.RemoveObjects(ctx, m.bucket, toDelete, minio.RemoveObjectsOptions{})
	for err := range errorCh {
		if err.Err != nil {
			// Copy succeeded but delete failed - partial state
			return errs.PathError("rename", oldpath, errs.Translate(err.Err))
		}
	}

	return nil
}

// renameFile renames a single file (helper method).
// This is used by Rename() for simple file-to-file renames.
func (m *MinioFS) renameFile(oldKey, newKey, oldpath string) error {
	ctx := context.Background()

	src := minio.CopySrcOptions{
		Bucket: m.bucket,
		Object: oldKey,
	}
	dst := minio.CopyDestOptions{
		Bucket: m.bucket,
		Object: newKey,
	}

	_, err := m.client.CopyObject(ctx, dst, src)
	if err != nil {
		return errs.PathError("rename", oldpath, errs.Translate(err))
	}

	// Remove old object
	err = m.client.RemoveObject(ctx, m.bucket, oldKey, minio.RemoveObjectOptions{})
	if err != nil {
		return errs.PathError("rename", oldpath, errs.Translate(err))
	}

	return nil
}

// Walk walks the file tree rooted at root, calling walkFn for each file or directory.
// For S3/MinIO, this handles virtual directories that don't exist as objects.
func (m *MinioFS) Walk(root string, walkFn fs.WalkDirFunc) error {
	rootKey := m.joinPath(root)
	ctx := context.Background()

	// For S3, we need special handling of the root since it might be a virtual directory
	// First, check if root is a file
	_, err := m.Stat(root)
	if err == nil {
		// Root is a file, use standard WalkDir
		if walkErr := fs.WalkDir(m, root, walkFn); walkErr != nil {
			return fmt.Errorf("walk %s: %w", root, walkErr)
		}
		return nil
	}

	// Root might be a virtual directory - check if any objects exist with this prefix
	prefix := rootKey
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// Check if any objects exist with this prefix
	objectsCh := m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
		MaxKeys:   1, // We only need to know if ANY object exists
	})

	// Read the first object (or error) from the channel
	firstObject, ok := <-objectsCh
	if !ok {
		// Channel closed without any objects - directory doesn't exist
		return errs.PathError("walk", root, fs.ErrNotExist)
	}

	if firstObject.Err != nil {
		return fmt.Errorf("walk %s: %w", root, errs.Translate(firstObject.Err))
	}

	// At least one object exists with this prefix - it's a valid directory

	// Virtual directory exists, manually walk it
	return m.walkDir(root, rootKey, walkFn)
}

// walkDir recursively walks a directory tree.
// This is a helper method for Walk() that handles virtual directories in S3.
func (m *MinioFS) walkDir(name, key string, walkFn fs.WalkDirFunc) error {
	ctx := context.Background()

	// Create a synthetic directory entry for the root
	rootEntry := types.NewS3DirEntry(filepath.Base(name), true, 0, time.Time{})

	// Call walkFn for the directory itself
	if err := walkFn(name, rootEntry, nil); err != nil {
		if errors.Is(err, fs.SkipDir) {
			return nil
		}
		return err
	}

	// List immediate children
	prefix := key
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	var entries []fs.DirEntry
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	}) {
		if object.Err != nil {
			return errs.Translate(object.Err)
		}

		// Skip the directory marker itself
		if object.Key == prefix {
			continue
		}

		relName := strings.TrimPrefix(object.Key, prefix)
		isDir := strings.HasSuffix(object.Key, "/")
		if isDir {
			relName = strings.TrimSuffix(relName, "/")
		}

		if relName == "" {
			continue
		}

		entries = append(entries, types.NewS3DirEntry(
			relName,
			isDir,
			object.Size,
			object.LastModified,
		))
	}

	// Walk each entry
	for _, entry := range entries {
		if err := walk.ProcessEntry(name, key, entry, walkFn, m.walkDir); err != nil {
			return err
		}
	}

	return nil
}

// Chroot returns a new filesystem rooted at dir.
func (m *MinioFS) Chroot(dir string) (core.FS, error) {
	// Verify the directory exists (optional - could skip for S3 virtual dirs)
	// For now, we'll skip the check since S3 has virtual directories

	// Create new filesystem with extended prefix
	newPrefix := m.joinPath(dir)

	return &MinioFS{
		client:             m.client,
		bucket:             m.bucket,
		prefix:             newPrefix,
		multipartThreshold: m.multipartThreshold,
		renameConcurrency:  m.renameConcurrency,
	}, nil
}

// parallelCopy copies objects from old to new prefix using a worker pool.
// Returns the list of successfully copied object keys for cleanup.
func (m *MinioFS) parallelCopy(ctx context.Context, oldPrefix, newPrefix string) ([]string, error) {
	// Create errgroup with concurrency limit
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(m.renameConcurrency)

	// Track copied objects for deletion
	var copiedMu sync.Mutex
	var copied []string

	// Stream objects and copy in parallel
	for object := range m.client.ListObjects(egCtx, m.bucket, minio.ListObjectsOptions{
		Prefix:    oldPrefix,
		Recursive: true,
	}) {
		if object.Err != nil {
			return copied, object.Err
		}

		objectKey := object.Key
		eg.Go(func() error {
			// Calculate new key
			relPath := strings.TrimPrefix(objectKey, oldPrefix)
			newKey := newPrefix + relPath

			// Copy object
			src := minio.CopySrcOptions{Bucket: m.bucket, Object: objectKey}
			dst := minio.CopyDestOptions{Bucket: m.bucket, Object: newKey}

			_, err := m.client.CopyObject(egCtx, dst, src)
			if err != nil {
				return fmt.Errorf("copy object %s to %s: %w", objectKey, newKey, err)
			}

			// Track for deletion
			copiedMu.Lock()
			copied = append(copied, objectKey)
			copiedMu.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return copied, fmt.Errorf("parallel copy failed: %w", err)
	}

	return copied, nil
}

// Compile-time interface check.
var _ core.FS = (*MinioFS)(nil)
