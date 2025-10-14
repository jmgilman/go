package fstest

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/core"
)

// TestMetadataFS tests metadata operations (Lstat, Chmod, Chtimes).
// Uses type assertion - skips if fs doesn't implement core.MetadataFS.
// Uses POSIXTestConfig() by default.
func TestMetadataFS(t *testing.T, filesystem core.FS) {
	TestMetadataFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestMetadataFSWithConfig tests metadata operations with behavior configuration.
func TestMetadataFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Type assert to check if filesystem supports MetadataFS
	mfs, ok := filesystem.(core.MetadataFS)
	if !ok {
		t.Skip("MetadataFS not supported")
		return
	}

	// Run all subtests
	t.Run("Lstat", func(t *testing.T) {
		testMetadataFSLstat(t, filesystem, mfs)
	})
	t.Run("Chmod", func(t *testing.T) {
		testMetadataFSChmod(t, filesystem, mfs)
	})
	t.Run("Chtimes", func(t *testing.T) {
		testMetadataFSChtimes(t, filesystem, mfs)
	})
}

// testMetadataFSLstat tests Lstat() operation.
// Per testing philosophy (lines 297-313), we test that the method succeeds,
// not backend-specific behavior.
func testMetadataFSLstat(t *testing.T, filesystem core.FS, mfs core.MetadataFS) {
	// Setup: Create a test file
	testData := []byte("test file for Lstat")
	if err := filesystem.WriteFile("lstat-test.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(lstat-test.txt): setup failed: %v", err)
	}

	// Test Lstat on regular file
	info, err := mfs.Lstat("lstat-test.txt")
	if err != nil {
		t.Errorf("Lstat(lstat-test.txt): got error %v, want nil", err)
		return
	}

	// Verify basic properties of returned FileInfo
	if info.IsDir() {
		t.Errorf("Lstat(lstat-test.txt): IsDir() = true, want false")
	}
	if info.Name() != "lstat-test.txt" {
		t.Errorf("Lstat(lstat-test.txt): Name() = %q, want %q", info.Name(), "lstat-test.txt")
	}
	if info.Size() != int64(len(testData)) {
		t.Errorf("Lstat(lstat-test.txt): Size() = %d, want %d", info.Size(), len(testData))
	}

	// Test Lstat on directory
	if err := filesystem.Mkdir("lstat-dir", 0755); err != nil {
		t.Fatalf("Mkdir(lstat-dir): setup failed: %v", err)
	}

	dirInfo, err := mfs.Lstat("lstat-dir")
	if err != nil {
		t.Errorf("Lstat(lstat-dir): got error %v, want nil", err)
		return
	}

	if !dirInfo.IsDir() {
		t.Errorf("Lstat(lstat-dir): IsDir() = false, want true")
	}
	if dirInfo.Name() != "lstat-dir" {
		t.Errorf("Lstat(lstat-dir): Name() = %q, want %q", dirInfo.Name(), "lstat-dir")
	}

	// Test Lstat on non-existent file
	_, err = mfs.Lstat("nonexistent-lstat.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(nonexistent-lstat.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Note: We do NOT test symlink-specific behavior here (that's for TestSymlinkFS).
	// We only test that Lstat succeeds on regular files and directories.
}

// testMetadataFSChmod tests Chmod() operation.
// Per testing philosophy (lines 297-313), we test that the method succeeds,
// not that it actually changes permissions on disk. Different providers handle
// permissions differently (S3 ignores them, local applies them).
func testMetadataFSChmod(t *testing.T, filesystem core.FS, mfs core.MetadataFS) {
	// Setup: Create a test file
	testData := []byte("test file for Chmod")
	if err := filesystem.WriteFile("chmod-test.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(chmod-test.txt): setup failed: %v", err)
	}

	// Test Chmod to change permissions
	err := mfs.Chmod("chmod-test.txt", 0600)
	if err != nil {
		t.Errorf("Chmod(chmod-test.txt, 0600): got error %v, want nil", err)
	}

	// Verify file still exists and is accessible
	data, err := filesystem.ReadFile("chmod-test.txt")
	if err != nil {
		t.Errorf("ReadFile(chmod-test.txt) after Chmod: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(chmod-test.txt) after Chmod: got %q, want %q", data, testData)
	}

	// Test Chmod on directory
	if err := filesystem.Mkdir("chmod-dir", 0755); err != nil {
		t.Fatalf("Mkdir(chmod-dir): setup failed: %v", err)
	}

	err = mfs.Chmod("chmod-dir", 0700)
	if err != nil {
		t.Errorf("Chmod(chmod-dir, 0700): got error %v, want nil", err)
	}

	// Verify directory still exists and is accessible
	info, err := filesystem.Stat("chmod-dir")
	if err != nil {
		t.Errorf("Stat(chmod-dir) after Chmod: got error %v, want nil", err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Stat(chmod-dir) after Chmod: IsDir() = false, want true")
	}

	// Test Chmod on non-existent file
	err = mfs.Chmod("nonexistent-chmod.txt", 0644)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Chmod(nonexistent-chmod.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Note: We do NOT verify that actual permissions were changed on disk.
	// This is per testing philosophy - we test interface contract, not backend behavior.
}

// testMetadataFSChtimes tests Chtimes() operation.
// Per testing philosophy (lines 297-313), we test that the method succeeds,
// not that it actually changes times on disk. Different providers handle
// times differently (S3 may only support mtime, not atime).
func testMetadataFSChtimes(t *testing.T, filesystem core.FS, mfs core.MetadataFS) {
	// Setup: Create a test file
	testData := []byte("test file for Chtimes")
	if err := filesystem.WriteFile("chtimes-test.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(chtimes-test.txt): setup failed: %v", err)
	}

	// Test Chtimes with specific times
	// Use time package which is already imported (verified in existing code)
	atime := testTime{year: 2024, month: 1, day: 15, hour: 10, minute: 30, second: 0}
	mtime := testTime{year: 2024, month: 2, day: 20, hour: 14, minute: 45, second: 30}

	err := mfs.Chtimes("chtimes-test.txt", atime.toTime(), mtime.toTime())
	if err != nil {
		t.Errorf("Chtimes(chtimes-test.txt): got error %v, want nil", err)
	}

	// Verify file still exists and is accessible
	data, err := filesystem.ReadFile("chtimes-test.txt")
	if err != nil {
		t.Errorf("ReadFile(chtimes-test.txt) after Chtimes: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(chtimes-test.txt) after Chtimes: got %q, want %q", data, testData)
	}

	// Test Chtimes on directory
	if err := filesystem.Mkdir("chtimes-dir", 0755); err != nil {
		t.Fatalf("Mkdir(chtimes-dir): setup failed: %v", err)
	}

	err = mfs.Chtimes("chtimes-dir", atime.toTime(), mtime.toTime())
	if err != nil {
		t.Errorf("Chtimes(chtimes-dir): got error %v, want nil", err)
	}

	// Verify directory still exists and is accessible
	info, err := filesystem.Stat("chtimes-dir")
	if err != nil {
		t.Errorf("Stat(chtimes-dir) after Chtimes: got error %v, want nil", err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Stat(chtimes-dir) after Chtimes: IsDir() = false, want true")
	}

	// Test Chtimes on non-existent file
	err = mfs.Chtimes("nonexistent-chtimes.txt", atime.toTime(), mtime.toTime())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Chtimes(nonexistent-chtimes.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Note: We do NOT verify that actual times were changed on disk.
	// This is per testing philosophy - we test interface contract, not backend behavior.
}

// testTime is a simple struct to hold time components for testing.
type testTime struct {
	year   int
	month  int
	day    int
	hour   int
	minute int
	second int
}

// toTime converts testTime to time.Time.
func (tt testTime) toTime() time.Time {
	return time.Date(tt.year, time.Month(tt.month), tt.day, tt.hour, tt.minute, tt.second, 0, time.UTC)
}
