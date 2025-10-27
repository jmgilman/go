package core

import (
	"io"
	"io/fs"
	"time"
)

// FSType represents the underlying type of filesystem implementation.
type FSType int

const (
	// FSTypeUnknown indicates the filesystem type is unknown or unspecified.
	FSTypeUnknown FSType = iota
	// FSTypeLocal indicates a local filesystem (e.g., disk-backed).
	FSTypeLocal
	// FSTypeMemory indicates an in-memory filesystem.
	FSTypeMemory
	// FSTypeRemote indicates a remote filesystem (e.g., S3, cloud storage).
	FSTypeRemote
)

// String returns a string representation of the FSType.
func (t FSType) String() string {
	switch t {
	case FSTypeLocal:
		return "local"
	case FSTypeMemory:
		return "memory"
	case FSTypeRemote:
		return "remote"
	default:
		return "unknown"
	}
}

// FS is the primary filesystem interface combining all core operations.
// FS explicitly embeds fs.FS for stdlib compatibility.
//
// All filesystem providers MUST implement this interface, which is composed
// of five sub-interfaces representing different categories of operations:
// ReadFS, WriteFS, ManageFS, WalkFS, and ChrootFS.
type FS interface {
	fs.FS // Ensures stdlib compatibility (provides Open returning fs.File)
	ReadFS
	WriteFS
	ManageFS
	WalkFS
	ChrootFS

	// Type returns the underlying filesystem type.
	// This allows callers to introspect whether the filesystem is
	// backed by a real disk, in-memory storage, or remote storage.
	Type() FSType
}

// ReadFS defines read-only filesystem operations.
// All backends MUST support this interface.
type ReadFS interface {
	// Open opens the named file for reading.
	// Returns fs.File for compatibility with io/fs package.
	// Callers can type-assert to File for write operations.
	//
	// The name must be a valid path relative to the filesystem root.
	// On success, methods on the returned file can be used for reading.
	// The returned file should be closed when no longer needed.
	Open(name string) (fs.File, error)

	// Stat returns file metadata.
	// It returns fs.FileInfo describing the named file.
	// If there is an error, it will be of type *fs.PathError.
	Stat(name string) (fs.FileInfo, error)

	// ReadDir reads the directory named by dirname and returns
	// a list of directory entries sorted by filename.
	//
	// If there is an error, it will be of type *fs.PathError.
	ReadDir(name string) ([]fs.DirEntry, error)

	// ReadFile reads the named file and returns its contents.
	// A successful call returns err == nil, not err == EOF.
	// Because ReadFile reads the whole file, it does not treat EOF
	// as an error to be reported.
	ReadFile(name string) ([]byte, error)

	// Exists reports whether the named file or directory exists.
	// It returns true if the path exists, false if it does not exist.
	// If an error occurs while checking (e.g., permission denied),
	// it returns false and the error.
	//
	// Note: For most use cases, callers should check the error.
	// A false result with a non-nil error indicates the existence
	// could not be determined, not that the file doesn't exist.
	Exists(name string) (bool, error)
}

// WriteFS defines write operations.
//
// Note: Not all providers support all flags. Providers should document
// which flags they support in their implementation documentation.
type WriteFS interface {
	// Create creates or truncates the named file for writing.
	// If the file already exists, it is truncated.
	// If the file does not exist, it is created with mode 0666 (before umask).
	// Returns File which also implements fs.File.
	//
	// The returned file must be closed when no longer needed.
	Create(name string) (File, error)

	// OpenFile opens a file with the specified flags and permissions.
	// The flags are a bitmask (O_RDONLY, O_WRONLY, O_RDWR, O_CREATE, O_TRUNC, etc.).
	//
	// Flag support varies by provider - see provider documentation for details.
	// For example, S3-based providers may not support all flag combinations.
	//
	// If the file is created, the permission mode perm is used (before umask).
	// Returns File which also implements fs.File.
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)

	// WriteFile writes data to the named file, creating it if necessary.
	// If the file already exists, WriteFile truncates it before writing.
	//
	// WriteFile is a convenience function that handles opening, writing,
	// and closing the file automatically. It is equivalent to opening the
	// file with O_WRONLY|O_CREATE|O_TRUNC, writing the data, and closing.
	WriteFile(name string, data []byte, perm fs.FileMode) error

	// Mkdir creates a new directory with the specified name and permission bits.
	// If the directory already exists, Mkdir returns an error (typically ErrExist).
	//
	// The permission bits perm are used for the new directory (before umask).
	// Note that some providers may not support permission bits.
	Mkdir(name string, perm fs.FileMode) error

	// MkdirAll creates a directory named path, along with any necessary parents.
	// If path is already a directory, MkdirAll does nothing and returns nil.
	//
	// The permission bits perm are used for all directories that MkdirAll creates.
	// If the path already exists and is a directory, MkdirAll does nothing.
	MkdirAll(path string, perm fs.FileMode) error
}

// ManageFS defines file and directory management operations.
//
// These operations modify the filesystem structure by removing or
// renaming files and directories.
type ManageFS interface {
	// Remove removes the named file or empty directory.
	// If the path does not exist, Remove returns an error (typically ErrNotExist).
	// If the path is a directory and is not empty, Remove returns an error.
	Remove(name string) error

	// RemoveAll removes path and any children it contains.
	// It removes everything it can but returns the first error it encounters.
	// If the path does not exist, RemoveAll returns nil (no error).
	RemoveAll(path string) error

	// Rename renames (moves) oldpath to newpath.
	// If newpath already exists and is not a directory, Rename replaces it.
	//
	// Note: S3 providers implement rename as copy+delete, which can be expensive
	// for large files or directories. Local filesystem providers use atomic rename.
	Rename(oldpath, newpath string) error
}

// WalkFS defines directory tree traversal operations.
type WalkFS interface {
	// Walk walks the file tree rooted at root, calling walkFn for each file or
	// directory in the tree, including root.
	//
	// All errors that arise visiting files and directories are filtered by walkFn.
	// The files are walked in lexical order, which makes the output deterministic
	// but requires Walk to read an entire directory into memory before processing
	// any of the files in that directory.
	//
	// Walk does not follow symbolic links (if supported by the provider).
	Walk(root string, walkFn fs.WalkDirFunc) error
}

// ChrootFS defines the ability to create scoped filesystem views.
//
// Chroot allows creating a restricted view of the filesystem where all
// operations are relative to a specific directory. This is useful for
// sandboxing operations and preventing directory traversal attacks.
type ChrootFS interface {
	// Chroot returns a filesystem scoped to the given directory.
	// All operations on the returned FS are relative to dir and cannot
	// access paths outside of dir.
	//
	// The directory dir must exist and be accessible, or Chroot returns an error.
	// The returned FS implements the same interfaces as the parent FS.
	Chroot(dir string) (FS, error)
}

// File represents an open file handle.
// File extends fs.File with write operations.
//
// All provider File types implement both File and fs.File, allowing them
// to be used with stdlib functions that accept fs.File while also supporting
// write operations through the io.Writer interface.
type File interface {
	fs.File // Embeds: Read([]byte) (int, error), Close() error, Stat() (fs.FileInfo, error)

	// Write writes len(p) bytes from p to the underlying data stream.
	// It returns the number of bytes written from p (0 <= n <= len(p))
	// and any error encountered that caused the write to stop early.
	// Write must return a non-nil error if it returns n < len(p).
	// Write must not modify the slice data, even temporarily.
	io.Writer

	// Name returns the name of the file as provided to Open or Create.
	// This is useful for debugging and error messages.
	Name() string
}

// Optional File capabilities (use type assertions):
//
// - io.Seeker: Seek(offset int64, whence int) (int64, error)
// - io.ReaderAt: ReadAt(p []byte, off int64) (n int, err error)
// - io.WriterAt: WriteAt(p []byte, off int64) (n int, err error)
// - Truncater: Truncate(size int64) error
// - Syncer: Sync() error
// - fs.ReadDirFile: ReadDir(n int) ([]fs.DirEntry, error)

// Truncater allows truncating a file to a specified size.
//
// Not all File implementations support truncation. Callers should use
// type assertion to check if this capability is available:
//
//	if t, ok := file.(Truncater); ok {
//	    err := t.Truncate(size)
//	}
type Truncater interface {
	// Truncate changes the size of the file.
	// It does not change the I/O offset.
	// If the file is larger than size, the extra data is discarded.
	// If the file is smaller than size, it is extended with null bytes.
	Truncate(size int64) error
}

// Syncer allows syncing file contents to stable storage.
//
// Not all File implementations support sync operations. Callers should use
// type assertion to check if this capability is available:
//
//	if s, ok := file.(Syncer); ok {
//	    err := s.Sync()
//	}
type Syncer interface {
	// Sync commits the current contents of the file to stable storage.
	// Typically, this means flushing the file system's in-memory copy
	// of recently written data to disk.
	Sync() error
}

// MetadataFS defines metadata operations (typically local and memory filesystems only).
//
// Use type assertion to check if a filesystem supports metadata operations:
//
//	if mfs, ok := filesystem.(MetadataFS); ok {
//	    err := mfs.Chmod("file.txt", 0600)
//	}
//
// Cloud storage providers typically do not support these operations.
type MetadataFS interface {
	// Lstat returns file info without following symbolic links.
	// If the file is a symbolic link, the returned FileInfo describes
	// the symbolic link itself, not the file it points to.
	Lstat(name string) (fs.FileInfo, error)

	// Chmod changes the mode/permissions of the named file.
	// The mode is a bitmask of permission bits (0644, 0755, etc.).
	//
	// Not all providers support all permission bits. Cloud storage
	// providers typically ignore permission changes.
	Chmod(name string, mode fs.FileMode) error

	// Chtimes changes the access and modification times of the named file.
	// A zero time value means to preserve the existing time.
	//
	// Not all providers support setting times. Cloud storage providers
	// may only support modification time, not access time.
	Chtimes(name string, atime, mtime time.Time) error
}

// SymlinkFS defines symbolic link operations (typically local filesystems only).
//
// Use type assertion to check if a filesystem supports symlink operations:
//
//	if sfs, ok := filesystem.(SymlinkFS); ok {
//	    err := sfs.Symlink("target", "linkname")
//	}
//
// Cloud storage providers typically do not support symbolic links.
type SymlinkFS interface {
	// Symlink creates a symbolic link named newname pointing to oldname.
	// If newname already exists, Symlink returns an error.
	//
	// The oldname path is not validated; it is stored as-is in the symlink.
	// Broken symbolic links are valid and detectable via Lstat.
	Symlink(oldname, newname string) error

	// Readlink returns the destination of the named symbolic link.
	// If the file is not a symbolic link, Readlink returns an error.
	Readlink(name string) (string, error)
}

// TempFS defines temporary file and directory creation operations
// (typically local and memory filesystems only).
//
// Use type assertion to check if a filesystem supports temp operations:
//
//	if tfs, ok := filesystem.(TempFS); ok {
//	    file, err := tfs.TempFile("", "prefix-")
//	}
//
// Cloud storage providers typically do not support temporary files.
type TempFS interface {
	// TempFile creates a new temporary file in the directory dir,
	// opens it for reading and writing, and returns the File.
	//
	// The filename is generated by adding a random string to the end of pattern.
	// If pattern includes a "*", the random string replaces the "*".
	// If dir is the empty string, TempFile uses the default directory for
	// temporary files (typically /tmp on Unix systems).
	//
	// The caller is responsible for removing the file when no longer needed.
	TempFile(dir, pattern string) (File, error)

	// TempDir creates a new temporary directory in the directory dir
	// and returns the pathname of the new directory.
	//
	// The directory name is generated by adding a random string to the end of pattern.
	// If pattern includes a "*", the random string replaces the "*".
	// If dir is the empty string, TempDir uses the default directory for
	// temporary files (typically /tmp on Unix systems).
	//
	// The caller is responsible for removing the directory when no longer needed.
	TempDir(dir, pattern string) (string, error)
}
