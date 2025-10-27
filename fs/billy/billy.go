package billy

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/jmgilman/go/fs/core"
)

// LocalFS wraps billy's osfs for local filesystem access.
// It provides a thin adapter that implements core.FS while maintaining
// access to the underlying billy.Filesystem for go-git integration.
type LocalFS struct {
	bfs billy.Filesystem
}

// MemoryFS wraps billy's memfs for in-memory filesystem access.
// It provides a thin adapter that implements core.FS while maintaining
// access to the underlying billy.Filesystem for go-git integration.
type MemoryFS struct {
	bfs billy.Filesystem
}

// Option configures filesystem creation.
// Reserved for future extensibility.
type Option func(*config)

type config struct {
	// Reserved for future options
}

// NewLocal creates a go-billy-backed local filesystem.
// The returned filesystem is rooted at the filesystem root ("/").
func NewLocal(_ ...Option) *LocalFS {
	return &LocalFS{
		bfs: osfs.New("/"),
	}
}

// NewMemory creates a go-billy-backed in-memory filesystem.
// The filesystem is initially empty.
func NewMemory(_ ...Option) *MemoryFS {
	return &MemoryFS{
		bfs: memfs.New(),
	}
}

// Unwrap returns the underlying billy.Filesystem for go-git integration.
// This allows passing the filesystem to go-git APIs that require billy.Filesystem.
func (lfs *LocalFS) Unwrap() billy.Filesystem {
	return lfs.bfs
}

// Unwrap returns the underlying billy.Filesystem for go-git integration.
// This allows passing the filesystem to go-git APIs that require billy.Filesystem.
func (mfs *MemoryFS) Unwrap() billy.Filesystem {
	return mfs.bfs
}

// normalize converts paths to use forward slashes consistently.
// This is a simplified path normalization since billy handles security.
func normalize(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

// dirEntry wraps fs.FileInfo to implement fs.DirEntry.
type dirEntry struct {
	info fs.FileInfo
}

func (d *dirEntry) Name() string               { return d.info.Name() }
func (d *dirEntry) IsDir() bool                { return d.info.IsDir() }
func (d *dirEntry) Type() fs.FileMode          { return d.info.Mode().Type() }
func (d *dirEntry) Info() (fs.FileInfo, error) { return d.info, nil }

// LocalFS ReadFS interface implementation

// Open opens the named file for reading.
// Returns a File that also implements fs.File.
func (lfs *LocalFS) Open(name string) (fs.File, error) {
	name = normalize(name)
	f, err := lfs.bfs.Open(name)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: lfs.bfs, name: name}, nil
}

// Stat returns file metadata for the named file.
func (lfs *LocalFS) Stat(name string) (fs.FileInfo, error) {
	return lfs.bfs.Stat(normalize(name))
}

// ReadDir reads the directory named by dirname and returns
// a list of directory entries sorted by filename.
func (lfs *LocalFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// Billy's ReadDir returns []fs.FileInfo, we need to convert to []fs.DirEntry
	infos, err := lfs.bfs.ReadDir(normalize(name))
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = &dirEntry{info: info}
	}
	return entries, nil
}

// ReadFile reads the named file and returns its contents.
func (lfs *LocalFS) ReadFile(name string) ([]byte, error) {
	name = normalize(name)
	f, err := lfs.bfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// Exists reports whether the named file or directory exists.
func (lfs *LocalFS) Exists(name string) (bool, error) {
	name = normalize(name)
	_, err := lfs.bfs.Stat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// LocalFS WriteFS interface implementation

// Create creates or truncates the named file for writing.
// Returns a File that also implements fs.File.
func (lfs *LocalFS) Create(name string) (core.File, error) {
	name = normalize(name)
	f, err := lfs.bfs.Create(name)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: lfs.bfs, name: name}, nil
}

// OpenFile opens a file with the specified flags and permissions.
func (lfs *LocalFS) OpenFile(name string, flag int, perm fs.FileMode) (core.File, error) {
	name = normalize(name)
	f, err := lfs.bfs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: lfs.bfs, name: name}, nil
}

// WriteFile writes data to the named file, creating it if necessary.
func (lfs *LocalFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	name = normalize(name)
	f, err := lfs.bfs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(data)
	return err
}

// Mkdir creates a new directory with the specified name and permission bits.
// Unlike MkdirAll, this will fail if the parent directory does not exist.
func (lfs *LocalFS) Mkdir(name string, perm fs.FileMode) error {
	name = normalize(name)
	// Check if directory already exists
	if _, err := lfs.bfs.Stat(name); err == nil {
		return os.ErrExist
	}
	// Check if parent exists (unless it's root)
	parent := filepath.Dir(name)
	if parent != "." && parent != "/" {
		if _, err := lfs.bfs.Stat(parent); err != nil {
			return err // Parent doesn't exist
		}
	}
	// Create the directory (MkdirAll won't create parents since we verified parent exists)
	return lfs.bfs.MkdirAll(name, perm)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (lfs *LocalFS) MkdirAll(path string, perm fs.FileMode) error {
	return lfs.bfs.MkdirAll(normalize(path), perm)
}

// LocalFS ManageFS interface implementation

// Remove removes the named file or empty directory.
func (lfs *LocalFS) Remove(name string) error {
	return lfs.bfs.Remove(normalize(name))
}

// RemoveAll removes path and any children it contains.
func (lfs *LocalFS) RemoveAll(path string) error {
	path = normalize(path)
	// Billy doesn't have RemoveAll, implement via recursive removal
	info, err := lfs.bfs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // RemoveAll returns nil if path doesn't exist
		}
		return err
	}

	if !info.IsDir() {
		return lfs.bfs.Remove(path)
	}

	// Remove directory contents recursively
	entries, err := lfs.bfs.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := normalize(filepath.Join(path, entry.Name()))
		if err := lfs.RemoveAll(entryPath); err != nil {
			return err
		}
	}

	// Remove the directory itself
	return lfs.bfs.Remove(path)
}

// Rename renames (moves) oldpath to newpath.
func (lfs *LocalFS) Rename(oldpath, newpath string) error {
	return lfs.bfs.Rename(normalize(oldpath), normalize(newpath))
}

// LocalFS WalkFS interface implementation

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root.
func (lfs *LocalFS) Walk(root string, walkFn fs.WalkDirFunc) error {
	return lfs.walkDir(root, walkFn)
}

func (lfs *LocalFS) walkDir(root string, walkFn fs.WalkDirFunc) error {
	root = normalize(root)
	info, err := lfs.bfs.Stat(root)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = lfs.walk(root, &dirEntry{info: info}, walkFn)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func (lfs *LocalFS) walk(path string, d fs.DirEntry, walkFn fs.WalkDirFunc) error {
	if err := walkFn(path, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			err = nil
		}
		return err
	}

	entries, err := lfs.bfs.ReadDir(path)
	if err != nil {
		err = walkFn(path, d, err)
		if err != nil {
			return err
		}
	}

	for _, entry := range entries {
		newPath := normalize(filepath.Join(path, entry.Name()))
		if err := lfs.walk(newPath, &dirEntry{info: entry}, walkFn); err != nil {
			if errors.Is(err, fs.SkipDir) {
				continue
			}
			return err
		}
	}
	return nil
}

// LocalFS ChrootFS interface implementation

// Chroot returns a filesystem scoped to the given directory.
func (lfs *LocalFS) Chroot(dir string) (core.FS, error) {
	dir = normalize(dir)
	chrootFS, err := lfs.bfs.Chroot(dir)
	if err != nil {
		return nil, err
	}
	return &LocalFS{bfs: chrootFS}, nil
}

// Type returns FSTypeLocal for local filesystem implementations.
func (lfs *LocalFS) Type() core.FSType {
	return core.FSTypeLocal
}

// MemoryFS ReadFS interface implementation

// Open opens the named file for reading.
// Returns a File that also implements fs.File.
func (mfs *MemoryFS) Open(name string) (fs.File, error) {
	name = normalize(name)
	f, err := mfs.bfs.Open(name)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: mfs.bfs, name: name}, nil
}

// Stat returns file metadata for the named file.
func (mfs *MemoryFS) Stat(name string) (fs.FileInfo, error) {
	return mfs.bfs.Stat(normalize(name))
}

// ReadDir reads the directory named by dirname and returns
// a list of directory entries sorted by filename.
func (mfs *MemoryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// Billy's ReadDir returns []fs.FileInfo, we need to convert to []fs.DirEntry
	infos, err := mfs.bfs.ReadDir(normalize(name))
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = &dirEntry{info: info}
	}
	return entries, nil
}

// ReadFile reads the named file and returns its contents.
func (mfs *MemoryFS) ReadFile(name string) ([]byte, error) {
	name = normalize(name)
	f, err := mfs.bfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// Exists reports whether the named file or directory exists.
func (mfs *MemoryFS) Exists(name string) (bool, error) {
	name = normalize(name)
	_, err := mfs.bfs.Stat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// MemoryFS WriteFS interface implementation

// Create creates or truncates the named file for writing.
// Returns a File that also implements fs.File.
func (mfs *MemoryFS) Create(name string) (core.File, error) {
	name = normalize(name)
	f, err := mfs.bfs.Create(name)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: mfs.bfs, name: name}, nil
}

// OpenFile opens a file with the specified flags and permissions.
func (mfs *MemoryFS) OpenFile(name string, flag int, perm fs.FileMode) (core.File, error) {
	name = normalize(name)
	f, err := mfs.bfs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &File{file: f, fs: mfs.bfs, name: name}, nil
}

// WriteFile writes data to the named file, creating it if necessary.
func (mfs *MemoryFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	name = normalize(name)
	f, err := mfs.bfs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(data)
	return err
}

// Mkdir creates a new directory with the specified name and permission bits.
// Unlike MkdirAll, this will fail if the parent directory does not exist.
func (mfs *MemoryFS) Mkdir(name string, perm fs.FileMode) error {
	name = normalize(name)
	// Check if directory already exists
	if _, err := mfs.bfs.Stat(name); err == nil {
		return os.ErrExist
	}
	// Check if parent exists (unless it's root)
	parent := filepath.Dir(name)
	if parent != "." && parent != "/" {
		if _, err := mfs.bfs.Stat(parent); err != nil {
			return err // Parent doesn't exist
		}
	}
	// Create the directory (MkdirAll won't create parents since we verified parent exists)
	return mfs.bfs.MkdirAll(name, perm)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (mfs *MemoryFS) MkdirAll(path string, perm fs.FileMode) error {
	return mfs.bfs.MkdirAll(normalize(path), perm)
}

// MemoryFS ManageFS interface implementation

// Remove removes the named file or empty directory.
func (mfs *MemoryFS) Remove(name string) error {
	return mfs.bfs.Remove(normalize(name))
}

// RemoveAll removes path and any children it contains.
func (mfs *MemoryFS) RemoveAll(path string) error {
	path = normalize(path)
	// Billy doesn't have RemoveAll, implement via recursive removal
	info, err := mfs.bfs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // RemoveAll returns nil if path doesn't exist
		}
		return err
	}

	if !info.IsDir() {
		return mfs.bfs.Remove(path)
	}

	// Remove directory contents recursively
	entries, err := mfs.bfs.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := normalize(filepath.Join(path, entry.Name()))
		if err := mfs.RemoveAll(entryPath); err != nil {
			return err
		}
	}

	// Remove the directory itself
	return mfs.bfs.Remove(path)
}

// Rename renames (moves) oldpath to newpath.
func (mfs *MemoryFS) Rename(oldpath, newpath string) error {
	return mfs.bfs.Rename(normalize(oldpath), normalize(newpath))
}

// MemoryFS WalkFS interface implementation

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root.
func (mfs *MemoryFS) Walk(root string, walkFn fs.WalkDirFunc) error {
	return mfs.walkDir(root, walkFn)
}

func (mfs *MemoryFS) walkDir(root string, walkFn fs.WalkDirFunc) error {
	root = normalize(root)
	info, err := mfs.bfs.Stat(root)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = mfs.walk(root, &dirEntry{info: info}, walkFn)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

func (mfs *MemoryFS) walk(path string, d fs.DirEntry, walkFn fs.WalkDirFunc) error {
	if err := walkFn(path, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			err = nil
		}
		return err
	}

	entries, err := mfs.bfs.ReadDir(path)
	if err != nil {
		err = walkFn(path, d, err)
		if err != nil {
			return err
		}
	}

	for _, entry := range entries {
		newPath := normalize(filepath.Join(path, entry.Name()))
		if err := mfs.walk(newPath, &dirEntry{info: entry}, walkFn); err != nil {
			if errors.Is(err, fs.SkipDir) {
				continue
			}
			return err
		}
	}
	return nil
}

// MemoryFS ChrootFS interface implementation

// Chroot returns a filesystem scoped to the given directory.
func (mfs *MemoryFS) Chroot(dir string) (core.FS, error) {
	dir = normalize(dir)
	chrootFS, err := mfs.bfs.Chroot(dir)
	if err != nil {
		return nil, err
	}
	return &MemoryFS{bfs: chrootFS}, nil
}

// Type returns FSTypeMemory for in-memory filesystem implementations.
func (mfs *MemoryFS) Type() core.FSType {
	return core.FSTypeMemory
}

// Compile-time interface checks.
var (
	_ core.FS = (*LocalFS)(nil)
	_ core.FS = (*MemoryFS)(nil)
)
