package fstest_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/fstest"
)

// TestIntegration_RealFilesystem validates the test suite works correctly
// against a real filesystem using temporary directories.
//
// This is the primary integration test as it exercises real filesystem
// behavior including permissions, symlinks, metadata, and platform-specific
// edge cases.
func TestIntegration_RealFilesystem(t *testing.T) {
	t.Run("Suite", func(t *testing.T) {
		fstest.TestSuite(t, func() core.FS {
			return newTempDirFS(t)
		})
	})

	t.Run("OpenFileFlags", func(t *testing.T) {
		filesystem := newTempDirFS(t)

		// Real filesystems support all standard flags
		supportedFlags := []int{
			os.O_RDONLY, os.O_WRONLY, os.O_RDWR,
			os.O_CREATE, os.O_TRUNC, os.O_APPEND, os.O_EXCL,
			os.O_SYNC, // Real OS filesystems support synchronous I/O
		}

		fstest.TestOpenFileFlags(t, filesystem, supportedFlags)
	})
}

// TestIntegration_StandardLibraryFS validates the suite works with Go's
// standard library filesystem testing utilities.
func TestIntegration_StandardLibraryFS(t *testing.T) {
	// Run core tests - each gets its own fresh filesystem
	t.Run("ReadFS", func(t *testing.T) {
		filesystem := newTempDirFS(t)
		fstest.TestReadFS(t, filesystem)
	})

	t.Run("WriteFS", func(t *testing.T) {
		filesystem := newTempDirFS(t)
		fstest.TestWriteFS(t, filesystem)
	})

	t.Run("ManageFS", func(t *testing.T) {
		filesystem := newTempDirFS(t)
		fstest.TestManageFS(t, filesystem)
	})
}

// TestIntegration_OptionalInterfaces validates that optional interface
// detection and testing works correctly.
func TestIntegration_OptionalInterfaces(t *testing.T) {
	filesystem := newTempDirFS(t)

	t.Run("MetadataFS", func(t *testing.T) {
		// Real filesystems should support metadata operations
		fstest.TestMetadataFS(t, filesystem)
	})

	t.Run("SymlinkFS", func(t *testing.T) {
		// Skip on platforms that don't support symlinks
		if !supportsSymlinks() {
			t.Skip("Platform doesn't support symlinks")
		}
		fstest.TestSymlinkFS(t, filesystem)
	})

	t.Run("TempFS", func(t *testing.T) {
		// Real filesystems should support temp files
		fstest.TestTempFS(t, filesystem)
	})
}

// TestIntegration_FileCapabilities validates file capability detection
// and testing works correctly.
func TestIntegration_FileCapabilities(t *testing.T) {
	filesystem := newTempDirFS(t)

	// Real filesystems should support most file capabilities
	fstest.TestFileCapabilities(t, filesystem)
}

// TestIntegration_SecurityBoundaries validates chroot and path traversal
// prevention works correctly with real filesystem paths.
func TestIntegration_SecurityBoundaries(t *testing.T) {
	filesystem := newTempDirFS(t)

	fstest.TestChrootFS(t, filesystem)
}

// TestIntegration_DirectoryTraversal validates walk operations work correctly
// with real filesystem directory structures.
func TestIntegration_DirectoryTraversal(t *testing.T) {
	filesystem := newTempDirFS(t)

	fstest.TestWalkFS(t, filesystem)
}

// TestIntegration_ErrorHandling validates that proper errors are returned
// for invalid operations.
func TestIntegration_ErrorHandling(t *testing.T) {
	t.Run("NotExistErrors", func(t *testing.T) {
		filesystem := newTempDirFS(t)
		// Attempt to read non-existent file
		_, err := filesystem.ReadFile("does-not-exist.txt")
		if !os.IsNotExist(err) {
			t.Errorf("Expected os.IsNotExist error, got: %v", err)
		}

		// Attempt to stat non-existent file
		_, err = filesystem.Stat("does-not-exist.txt")
		if !os.IsNotExist(err) {
			t.Errorf("Expected os.IsNotExist error, got: %v", err)
		}
	})

	t.Run("ExistErrors", func(t *testing.T) {
		filesystem := newTempDirFS(t)
		// Create a file
		if err := filesystem.WriteFile("exists.txt", []byte("data"), 0644); err != nil {
			t.Fatalf("Setup failed: %v", err)
		}

		// Attempt to create directory with same name
		err := filesystem.Mkdir("exists.txt", 0755)
		if err == nil {
			t.Error("Expected error when creating directory with existing file name")
		}
	})
}

// TestIntegration_ConcurrentOperations validates the suite handles
// concurrent filesystem operations correctly.
func TestIntegration_ConcurrentOperations(t *testing.T) {
	// This test validates that the suite's tests work correctly even
	// when the filesystem might be accessed concurrently
	// (though each test gets its own fresh instance)

	t.Run("ParallelWrites", func(t *testing.T) {
		t.Parallel()
		for i := 0; i < 5; i++ {
			i := i
			t.Run("", func(t *testing.T) {
				t.Parallel()
				// Each parallel test gets its own filesystem
				fs := newTempDirFS(t)
				if err := fs.WriteFile("test.txt", []byte("data"), 0644); err != nil {
					t.Errorf("Write %d failed: %v", i, err)
				}
			})
		}
	})
}

// TestIntegration_LargeFiles validates the suite works with larger files
// (though still reasonable for testing).
func TestIntegration_LargeFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	filesystem := newTempDirFS(t)

	// Create a 1MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := filesystem.WriteFile("large.bin", data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read it back
	read, err := filesystem.ReadFile("large.bin")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(read) != len(data) {
		t.Errorf("Read %d bytes, expected %d", len(read), len(data))
	}
}

// TestIntegration_SpecialCharactersInPaths validates handling of special
// characters in filenames.
func TestIntegration_SpecialCharactersInPaths(t *testing.T) {
	filesystem := newTempDirFS(t)

	// Test various special characters (platform-dependent)
	testCases := []string{
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"file.multiple.dots.txt",
		"file(with)parens.txt",
		"file[with]brackets.txt",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			data := []byte("test content")
			if err := filesystem.WriteFile(name, data, 0644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			read, err := filesystem.ReadFile(name)
			if err != nil {
				t.Fatalf("ReadFile failed: %v", err)
			}

			if string(read) != string(data) {
				t.Errorf("Content mismatch")
			}
		})
	}
}

// newTempDirFS creates a new filesystem rooted in a temporary directory.
// The directory is automatically cleaned up when the test completes.
func newTempDirFS(t *testing.T) core.FS {
	t.Helper()
	tmpDir := t.TempDir() // Automatically cleaned up
	return &osFS{root: tmpDir}
}

// osFS is a simple wrapper around os package operations for testing.
// This provides a minimal core.FS implementation backed by the real filesystem.
type osFS struct {
	root string
}

// compile-time interface assertions.
var _ core.FS = (*osFS)(nil)
var _ core.MetadataFS = (*osFS)(nil)
var _ core.SymlinkFS = (*osFS)(nil)
var _ core.TempFS = (*osFS)(nil)

func (f *osFS) resolve(name string) string {
	// Clean the name to resolve any ../ or ./ components
	cleaned := filepath.Clean(name)

	// Reject absolute paths trying to escape
	if filepath.IsAbs(cleaned) {
		return filepath.Join(f.root, filepath.Base(cleaned))
	}

	// Join with root
	full := filepath.Join(f.root, cleaned)

	// Ensure the resolved path is still within root (prevents ../ escapes)
	if !isWithin(full, f.root) {
		// Return a path that will fail - still within root but non-existent
		return filepath.Join(f.root, ".invalid-path-traversal")
	}

	return full
}

// isWithin checks if path is within or equal to root.
func isWithin(path, root string) bool {
	// Convert both to absolute clean paths
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}

	// Check if path starts with root
	return absPath == absRoot || (len(absPath) > len(absRoot) && absPath[len(absRoot)] == filepath.Separator && absPath[:len(absRoot)] == absRoot)
}

// ReadFS methods.
func (f *osFS) Open(name string) (fs.File, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Open(f.resolve(name))
}

func (f *osFS) Stat(name string) (fs.FileInfo, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Stat(f.resolve(name))
}

func (f *osFS) ReadDir(name string) ([]fs.DirEntry, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.ReadDir(f.resolve(name))
}

func (f *osFS) ReadFile(name string) ([]byte, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.ReadFile(f.resolve(name))
}

func (f *osFS) Exists(name string) (bool, error) {
	_, err := os.Stat(f.resolve(name))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return false, err
}

// WriteFS methods.
func (f *osFS) Create(name string) (core.File, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Create(f.resolve(name))
}

func (f *osFS) OpenFile(name string, flag int, perm fs.FileMode) (core.File, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.OpenFile(f.resolve(name), flag, perm)
}

func (f *osFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.WriteFile(f.resolve(name), data, perm)
}

func (f *osFS) Mkdir(name string, perm fs.FileMode) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Mkdir(f.resolve(name), perm)
}

func (f *osFS) MkdirAll(path string, perm fs.FileMode) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.MkdirAll(f.resolve(path), perm)
}

// ManageFS methods.
func (f *osFS) Remove(name string) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Remove(f.resolve(name))
}

func (f *osFS) RemoveAll(path string) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.RemoveAll(f.resolve(path))
}

func (f *osFS) Rename(oldpath, newpath string) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Rename(f.resolve(oldpath), f.resolve(newpath))
}

// WalkFS methods.
func (f *osFS) Walk(root string, walkFn fs.WalkDirFunc) error {
	// Walk must provide paths relative to filesystem root, not absolute paths
	fullRoot := f.resolve(root)
	//nolint:wrapcheck // Test helper - must pass through filepath errors unchanged
	return filepath.WalkDir(fullRoot, func(path string, d fs.DirEntry, err error) error {
		// Convert absolute path to relative path from filesystem root
		relPath, relErr := filepath.Rel(f.root, path)
		if relErr != nil {
			//nolint:wrapcheck // Test helper - must pass through filepath errors unchanged
			return relErr
		}
		return walkFn(relPath, d, err)
	})
}

// ChrootFS methods.
func (f *osFS) Chroot(dir string) (core.FS, error) {
	newRoot := f.resolve(dir)

	// Verify directory exists
	info, err := os.Stat(newRoot)
	if err != nil {
		//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "chroot", Path: dir, Err: fs.ErrInvalid}
	}

	return &osFS{root: newRoot}, nil
}

// MetadataFS methods.
func (f *osFS) Lstat(name string) (fs.FileInfo, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Lstat(f.resolve(name))
}

func (f *osFS) Chmod(name string, mode fs.FileMode) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Chmod(f.resolve(name), mode)
}

func (f *osFS) Chtimes(name string, atime, mtime time.Time) error {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Chtimes(f.resolve(name), atime, mtime)
}

// SymlinkFS methods.
func (f *osFS) Symlink(oldname, newname string) error {
	// oldname is relative to the symlink, not to root
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Symlink(oldname, f.resolve(newname))
}

func (f *osFS) Readlink(name string) (string, error) {
	//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
	return os.Readlink(f.resolve(name))
}

// TempFS methods.
func (f *osFS) TempFile(dir, pattern string) (core.File, error) {
	resolvedDir := f.root
	if dir != "" {
		resolvedDir = f.resolve(dir)
	}
	file, err := os.CreateTemp(resolvedDir, pattern)
	if err != nil {
		//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
		return nil, err
	}
	// Wrap to return relative paths
	return &relativeTempFile{File: file, root: f.root}, nil
}

func (f *osFS) TempDir(dir, pattern string) (string, error) {
	resolvedDir := f.root
	if dir != "" {
		resolvedDir = f.resolve(dir)
	}
	name, err := os.MkdirTemp(resolvedDir, pattern)
	if err != nil {
		//nolint:wrapcheck // Test helper - must pass through os errors unchanged for error detection
		return "", err
	}
	// Return path relative to root
	rel, err := filepath.Rel(f.root, name)
	if err != nil {
		//nolint:wrapcheck // Test helper - must pass through filepath errors unchanged
		return "", err
	}
	return rel, nil
}

// relativeTempFile wraps os.File to return relative paths.
type relativeTempFile struct {
	*os.File
	root string
}

func (f *relativeTempFile) Name() string {
	absName := f.File.Name()
	rel, err := filepath.Rel(f.root, absName)
	if err != nil {
		return absName
	}
	return rel
}

// supportsSymlinks checks if the current platform supports symbolic links.
func supportsSymlinks() bool {
	// Windows requires developer mode or admin privileges for symlinks
	// This is a simplified check
	tmpDir, err := os.MkdirTemp("", "symlink-test")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	target := filepath.Join(tmpDir, "target")
	link := filepath.Join(tmpDir, "link")

	if err := os.WriteFile(target, []byte("test"), 0644); err != nil {
		return false
	}

	if err := os.Symlink(target, link); err != nil {
		return false
	}

	return true
}
