package billy

import (
	"io"
	"io/fs"

	"github.com/go-git/go-billy/v5"
	"github.com/jmgilman/go/fs/core"
)

// File wraps billy.File to implement both core.File and fs.File.
// It stores the filename since billy.File.Name() may return different formats
// depending on the backend implementation.
// It also stores a reference to the filesystem to support Stat() calls.
type File struct {
	file billy.File
	fs   billy.Basic // Need Basic for Stat() method
	name string
}

// Read implements io.Reader (required by fs.File).
// Delegates directly to the underlying billy.File.
func (f *File) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

// Write implements io.Writer (required by core.File).
// Delegates directly to the underlying billy.File.
func (f *File) Write(p []byte) (int, error) {
	return f.file.Write(p)
}

// Close implements io.Closer (required by fs.File).
// Delegates directly to the underlying billy.File.
func (f *File) Close() error {
	return f.file.Close()
}

// Stat implements fs.File.Stat.
// Since billy.File doesn't provide Stat(), we call the filesystem's Stat() method.
func (f *File) Stat() (fs.FileInfo, error) {
	return f.fs.Stat(f.name)
}

// Name returns the name provided to Open/Create.
// This is required by core.File and provides consistent behavior
// across different billy backends.
func (f *File) Name() string {
	return f.name
}

// Seek implements io.Seeker.
// Billy.File provides Seek, so we delegate directly.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

// Truncate implements core.Truncater.
// Delegates directly to the underlying billy.File.
func (f *File) Truncate(size int64) error {
	return f.file.Truncate(size)
}

// Sync implements core.Syncer.
// Billy.File may or may not provide Sync depending on the backend.
// For backends without Sync (e.g., memfs), this is a no-op.
func (f *File) Sync() error {
	// Type assert to check if the underlying billy.File supports Sync
	if syncer, ok := f.file.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}
	// No-op for backends that don't support sync (e.g., in-memory)
	return nil
}

// Compile-time interface checks.
var (
	_ core.File      = (*File)(nil)
	_ fs.File        = (*File)(nil)
	_ io.Seeker      = (*File)(nil)
	_ core.Truncater = (*File)(nil)
	_ core.Syncer    = (*File)(nil)
)
