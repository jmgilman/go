// Package types provides shared type definitions for the minio filesystem.
package types // nolint:revive // Internal package with clear purpose

import (
	"io/fs"
	"time"
)

// FileInfo implements fs.FileInfo for MinIO objects.
type FileInfo struct {
	FileName    string
	FileSize    int64
	FileModTime time.Time
	FileMode    fs.FileMode
}

// Name returns the name of the file.
func (fi *FileInfo) Name() string { return fi.FileName }

// Size returns the length in bytes for regular files.
func (fi *FileInfo) Size() int64 { return fi.FileSize }

// Mode returns the file mode bits.
func (fi *FileInfo) Mode() fs.FileMode { return fi.FileMode }

// ModTime returns the modification time.
func (fi *FileInfo) ModTime() time.Time { return fi.FileModTime }

// IsDir returns true if this describes a directory.
func (fi *FileInfo) IsDir() bool { return fi.FileMode&fs.ModeDir != 0 }

// Sys returns the underlying data source (always nil for S3).
func (fi *FileInfo) Sys() interface{} { return nil }

// NewFileInfo creates a new FileInfo with the given parameters.
func NewFileInfo(name string, size int64, modTime time.Time, mode fs.FileMode) *FileInfo {
	return &FileInfo{
		FileName:    name,
		FileSize:    size,
		FileModTime: modTime,
		FileMode:    mode,
	}
}

// S3DirEntry implements fs.DirEntry for S3 objects and virtual directories.
type S3DirEntry struct {
	EntryName    string
	EntryIsDir   bool
	EntrySize    int64
	EntryModTime time.Time
}

// Name returns the name of the entry.
func (e *S3DirEntry) Name() string {
	return e.EntryName
}

// IsDir reports whether the entry describes a directory.
func (e *S3DirEntry) IsDir() bool {
	return e.EntryIsDir
}

// Type returns the type bits for the entry.
func (e *S3DirEntry) Type() fs.FileMode {
	if e.EntryIsDir {
		return fs.ModeDir
	}
	return 0
}

// Info returns the FileInfo for the entry.
func (e *S3DirEntry) Info() (fs.FileInfo, error) {
	mode := fs.FileMode(0644)
	if e.EntryIsDir {
		mode = fs.ModeDir | 0755
	}
	return NewFileInfo(e.EntryName, e.EntrySize, e.EntryModTime, mode), nil
}

// NewS3DirEntry creates a new S3DirEntry with the given parameters.
func NewS3DirEntry(name string, isDir bool, size int64, modTime time.Time) *S3DirEntry {
	return &S3DirEntry{
		EntryName:    name,
		EntryIsDir:   isDir,
		EntrySize:    size,
		EntryModTime: modTime,
	}
}

// Compile-time interface checks.
var (
	_ fs.FileInfo = (*FileInfo)(nil)
	_ fs.DirEntry = (*S3DirEntry)(nil)
)
