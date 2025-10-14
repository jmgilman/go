package fstest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestReadFS tests read-only operations: Open, Stat, ReadDir, ReadFile.
// Assumes fs is pre-populated with test data structure.
// Uses POSIXTestConfig() by default.
func TestReadFS(t *testing.T, filesystem core.FS) {
	TestReadFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestReadFSWithConfig tests read-only operations with behavior configuration.
func TestReadFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create test data structure
	testContent := []byte("test file content")

	// Create a directory with a test file
	if err := filesystem.MkdirAll("testdir", 0755); err != nil {
		t.Fatalf("MkdirAll(testdir): setup failed: %v", err)
	}

	if err := filesystem.WriteFile("testdir/testfile.txt", testContent, 0644); err != nil {
		t.Fatalf("WriteFile(testdir/testfile.txt): setup failed: %v", err)
	}

	// Run all subtests
	t.Run("Open", func(t *testing.T) {
		testReadFSOpen(t, filesystem, testContent, config)
	})
	t.Run("StatFile", func(t *testing.T) {
		testReadFSStatFile(t, filesystem, testContent, config)
	})
	t.Run("StatDir", func(t *testing.T) {
		testReadFSStatDir(t, filesystem, config)
	})
	t.Run("ReadDir", func(t *testing.T) {
		testReadFSReadDir(t, filesystem, config)
	})
	t.Run("ReadFile", func(t *testing.T) {
		testReadFSReadFile(t, filesystem, testContent, config)
	})
	t.Run("OpenNotExist", func(t *testing.T) {
		testReadFSOpenNotExist(t, filesystem, config)
	})
	t.Run("ExistsFile", func(t *testing.T) {
		testReadFSExistsFile(t, filesystem, config)
	})
	t.Run("ExistsDir", func(t *testing.T) {
		testReadFSExistsDir(t, filesystem, config)
	})
	t.Run("ExistsNotExist", func(t *testing.T) {
		testReadFSExistsNotExist(t, filesystem, config)
	})
}

// testReadFSOpen tests Open() on existing file and reads contents.
func testReadFSOpen(t *testing.T, filesystem core.FS, testContent []byte, config FSTestConfig) {
	f, err := filesystem.Open("testdir/testfile.txt")
	if err != nil {
		t.Errorf("Open(%q): got error %v, want nil", "testdir/testfile.txt", err)
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Errorf("Close(): got error %v", closeErr)
		}
	}()

	// Read the file contents
	data := make([]byte, len(testContent))
	n, err := f.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("Read(): got error %v, want nil or EOF", err)
		return
	}
	if n != len(testContent) {
		t.Errorf("Read(): read %d bytes, want %d", n, len(testContent))
	}
	if !bytes.Equal(data, testContent) {
		t.Errorf("Read(): got %q, want %q", data, testContent)
	}
}

// testReadFSStatFile tests Stat() on file.
func testReadFSStatFile(t *testing.T, filesystem core.FS, testContent []byte, config FSTestConfig) {
	info, err := filesystem.Stat("testdir/testfile.txt")
	if err != nil {
		t.Errorf("Stat(%q): got error %v, want nil", "testdir/testfile.txt", err)
		return
	}
	if info.IsDir() {
		t.Errorf("Stat(%q): IsDir() = true, want false", "testdir/testfile.txt")
	}
	if info.Size() != int64(len(testContent)) {
		t.Errorf("Stat(%q): Size() = %d, want %d", "testdir/testfile.txt", info.Size(), len(testContent))
	}
}

// testReadFSStatDir tests Stat() on directory.
func testReadFSStatDir(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Skip if filesystem has virtual directories (S3-like)
	if config.VirtualDirectories {
		t.Skip("Skipping directory Stat test - filesystem has virtual directories")
		return
	}

	info, err := filesystem.Stat("testdir")
	if err != nil {
		t.Errorf("Stat(%q): got error %v, want nil", "testdir", err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Stat(%q): IsDir() = false, want true", "testdir")
	}
}

// testReadFSReadDir tests ReadDir() on directory with files.
func testReadFSReadDir(t *testing.T, filesystem core.FS, config FSTestConfig) {
	entries, err := filesystem.ReadDir("testdir")
	if err != nil {
		t.Errorf("ReadDir(%q): got error %v, want nil", "testdir", err)
		return
	}
	if len(entries) != 1 {
		t.Errorf("ReadDir(%q): got %d entries, want 1", "testdir", len(entries))
		return
	}
	if entries[0].Name() != "testfile.txt" {
		t.Errorf("ReadDir(%q): got entry name %q, want %q", "testdir", entries[0].Name(), "testfile.txt")
	}
	if entries[0].IsDir() {
		t.Errorf("ReadDir(%q): entry IsDir() = true, want false", "testdir")
	}
}

// testReadFSReadFile tests ReadFile() entire contents.
func testReadFSReadFile(t *testing.T, filesystem core.FS, testContent []byte, config FSTestConfig) {
	data, err := filesystem.ReadFile("testdir/testfile.txt")
	if err != nil {
		t.Errorf("ReadFile(%q): got error %v, want nil", "testdir/testfile.txt", err)
		return
	}
	if !bytes.Equal(data, testContent) {
		t.Errorf("ReadFile(%q): got %q, want %q", "testdir/testfile.txt", data, testContent)
	}
}

// testReadFSOpenNotExist tests Open() non-existent file returns fs.ErrNotExist.
func testReadFSOpenNotExist(t *testing.T, filesystem core.FS, config FSTestConfig) {
	_, err := filesystem.Open("nonexistent")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(%q): got error %v, want fs.ErrNotExist", "nonexistent", err)
	}
}

// testReadFSExistsFile tests Exists() returns true for existing file.
func testReadFSExistsFile(t *testing.T, filesystem core.FS, config FSTestConfig) {
	exists, err := filesystem.Exists("testdir/testfile.txt")
	if err != nil {
		t.Errorf("Exists(%q): got error %v, want nil", "testdir/testfile.txt", err)
		return
	}
	if !exists {
		t.Errorf("Exists(%q): got false, want true", "testdir/testfile.txt")
	}
}

// testReadFSExistsDir tests Exists() returns true for existing directory.
func testReadFSExistsDir(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Skip if filesystem has virtual directories (S3-like)
	if config.VirtualDirectories {
		t.Skip("Skipping directory Exists test - filesystem has virtual directories")
		return
	}

	exists, err := filesystem.Exists("testdir")
	if err != nil {
		t.Errorf("Exists(%q): got error %v, want nil", "testdir", err)
		return
	}
	if !exists {
		t.Errorf("Exists(%q): got false, want true", "testdir")
	}
}

// testReadFSExistsNotExist tests Exists() returns false for non-existent path.
func testReadFSExistsNotExist(t *testing.T, filesystem core.FS, config FSTestConfig) {
	exists, err := filesystem.Exists("nonexistent")
	if err != nil {
		t.Errorf("Exists(%q): got error %v, want nil", "nonexistent", err)
		return
	}
	if exists {
		t.Errorf("Exists(%q): got true, want false", "nonexistent")
	}
}
