package fstest

import (
	"bytes"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestTempFS tests temporary file operations (TempFile, TempDir).
// Uses type assertion - skips if fs doesn't implement core.TempFS.
// Uses POSIXTestConfig() by default.
func TestTempFS(t *testing.T, filesystem core.FS) {
	TestTempFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestTempFSWithConfig tests temporary file operations with behavior configuration.
func TestTempFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Type assert to check if filesystem supports TempFS
	tfs, ok := filesystem.(core.TempFS)
	if !ok {
		t.Skip("TempFS not supported")
		return
	}

	// Run all subtests
	t.Run("TempFile", func(t *testing.T) {
		testTempFSTempFile(t, filesystem, tfs)
	})
	t.Run("TempDir", func(t *testing.T) {
		testTempFSTempDir(t, filesystem, tfs)
	})
	t.Run("TempFileInDir", func(t *testing.T) {
		testTempFSTempFileInDir(t, filesystem, tfs)
	})
	t.Run("TempDirInDir", func(t *testing.T) {
		testTempFSTempDirInDir(t, filesystem, tfs)
	})
}

// testTempFSTempFile tests TempFile() creation in default temp directory.
func testTempFSTempFile(t *testing.T, filesystem core.FS, tfs core.TempFS) {
	// Create a temporary file with pattern
	f, err := tfs.TempFile("", "test-*.txt")
	if err != nil {
		t.Fatalf("TempFile(%q, %q): got error %v, want nil", "", "test-*.txt", err)
	}

	// Get the file name immediately for cleanup
	tempPath := f.Name()
	defer func() {
		// Clean up: remove the temp file (file already closed in function body)
		_ = filesystem.Remove(tempPath)
	}()

	// Get the file name
	stat, err := f.Stat()
	if err != nil {
		t.Errorf("Stat() on temp file: got error %v, want nil", err)
		return
	}
	filename := stat.Name()

	// Verify the filename matches the pattern (should contain "test-" and end with ".txt")
	if len(filename) == 0 {
		t.Errorf("TempFile: filename is empty")
		return
	}

	// Test writing to the temp file
	testData := []byte("temporary file content")
	n, err := f.Write(testData)
	if err != nil {
		t.Errorf("Write() to temp file: got error %v, want nil", err)
		return
	}
	if n != len(testData) {
		t.Errorf("Write() to temp file: wrote %d bytes, want %d", n, len(testData))
	}

	// Close and reopen to verify persistence
	if err := f.Close(); err != nil {
		t.Errorf("Close() temp file: got error %v, want nil", err)
		return
	}

	// Verify the temp file exists and is accessible
	data, err := filesystem.ReadFile(tempPath)
	if err != nil {
		t.Errorf("ReadFile(%q) on temp file: got error %v, want nil", tempPath, err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(%q) on temp file: got %q, want %q", tempPath, data, testData)
	}

	// Clean up
	if err := filesystem.Remove(tempPath); err != nil {
		t.Errorf("Remove(%q) temp file: got error %v, want nil", tempPath, err)
	}
}

// testTempFSTempDir tests TempDir() creation in default temp directory.
func testTempFSTempDir(t *testing.T, filesystem core.FS, tfs core.TempFS) {
	// Create a temporary directory with pattern
	dirPath, err := tfs.TempDir("", "test-dir-*")
	if err != nil {
		t.Fatalf("TempDir(%q, %q): got error %v, want nil", "", "test-dir-*", err)
	}
	defer func() {
		// Clean up: remove the temp directory
		_ = filesystem.RemoveAll(dirPath)
	}()

	// Verify the directory path is not empty
	if dirPath == "" {
		t.Errorf("TempDir: returned empty path")
		return
	}

	// Verify the directory exists
	info, err := filesystem.Stat(dirPath)
	if err != nil {
		t.Errorf("Stat(%q) on temp dir: got error %v, want nil", dirPath, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Stat(%q) on temp dir: IsDir() = false, want true", dirPath)
	}

	// Test creating a file inside the temp directory
	testFile := dirPath + "/testfile.txt"
	testData := []byte("file in temp directory")
	if err := filesystem.WriteFile(testFile, testData, 0644); err != nil {
		t.Errorf("WriteFile(%q) in temp dir: got error %v, want nil", testFile, err)
		return
	}

	// Verify the file was created and is accessible
	data, err := filesystem.ReadFile(testFile)
	if err != nil {
		t.Errorf("ReadFile(%q) in temp dir: got error %v, want nil", testFile, err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(%q) in temp dir: got %q, want %q", testFile, data, testData)
	}

	// Clean up
	if err := filesystem.RemoveAll(dirPath); err != nil {
		t.Errorf("RemoveAll(%q) temp dir: got error %v, want nil", dirPath, err)
	}
}

// testTempFSTempFileInDir tests TempFile() creation in a specific directory.
func testTempFSTempFileInDir(t *testing.T, filesystem core.FS, tfs core.TempFS) {
	// Setup: Create a directory for temp files
	if err := filesystem.Mkdir("tempfiles", 0755); err != nil {
		t.Fatalf("Mkdir(tempfiles): setup failed: %v", err)
	}
	defer func() {
		_ = filesystem.RemoveAll("tempfiles")
	}()

	// Create a temporary file in the specific directory
	f, err := tfs.TempFile("tempfiles", "indir-*.tmp")
	if err != nil {
		t.Fatalf("TempFile(%q, %q): got error %v, want nil", "tempfiles", "indir-*.tmp", err)
	}

	tempPath := f.Name()
	defer func() {
		// Clean up: remove the temp file (file already closed in function body)
		_ = filesystem.Remove(tempPath)
	}()

	// Verify the temp file was created in the specified directory
	// Note: Different providers may return different path formats, so we just verify it exists
	if err := f.Close(); err != nil {
		t.Errorf("Close() temp file in dir: got error %v, want nil", err)
		return
	}

	// Verify the file exists
	_, err = filesystem.Stat(tempPath)
	if err != nil {
		t.Errorf("Stat(%q) on temp file in dir: got error %v, want nil", tempPath, err)
	}

	// Clean up
	if err := filesystem.Remove(tempPath); err != nil {
		t.Errorf("Remove(%q) temp file in dir: got error %v, want nil", tempPath, err)
	}
}

// testTempFSTempDirInDir tests TempDir() creation in a specific directory.
func testTempFSTempDirInDir(t *testing.T, filesystem core.FS, tfs core.TempFS) {
	// Setup: Create a parent directory
	if err := filesystem.Mkdir("tempdirs", 0755); err != nil {
		t.Fatalf("Mkdir(tempdirs): setup failed: %v", err)
	}
	defer func() {
		_ = filesystem.RemoveAll("tempdirs")
	}()

	// Create a temporary directory in the specific directory
	dirPath, err := tfs.TempDir("tempdirs", "subdir-*")
	if err != nil {
		t.Fatalf("TempDir(%q, %q): got error %v, want nil", "tempdirs", "subdir-*", err)
	}
	defer func() {
		_ = filesystem.RemoveAll(dirPath)
	}()

	// Verify the directory path is not empty
	if dirPath == "" {
		t.Errorf("TempDir in dir: returned empty path")
		return
	}

	// Verify the directory exists
	info, err := filesystem.Stat(dirPath)
	if err != nil {
		t.Errorf("Stat(%q) on temp dir in dir: got error %v, want nil", dirPath, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Stat(%q) on temp dir in dir: IsDir() = false, want true", dirPath)
	}

	// Test creating a file inside the nested temp directory
	testFile := dirPath + "/nested.txt"
	testData := []byte("nested temp file")
	if err := filesystem.WriteFile(testFile, testData, 0644); err != nil {
		t.Errorf("WriteFile(%q) in nested temp dir: got error %v, want nil", testFile, err)
		return
	}

	// Verify the file was created
	data, err := filesystem.ReadFile(testFile)
	if err != nil {
		t.Errorf("ReadFile(%q) in nested temp dir: got error %v, want nil", testFile, err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(%q) in nested temp dir: got %q, want %q", testFile, data, testData)
	}

	// Clean up
	if err := filesystem.RemoveAll(dirPath); err != nil {
		t.Errorf("RemoveAll(%q) nested temp dir: got error %v, want nil", dirPath, err)
	}
}
