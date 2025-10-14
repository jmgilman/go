package fstest

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestSymlinkFS tests symlink operations (Symlink, Readlink).
// Uses type assertion - skips if fs doesn't implement core.SymlinkFS.
// Uses POSIXTestConfig() by default.
func TestSymlinkFS(t *testing.T, filesystem core.FS) {
	TestSymlinkFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestSymlinkFSWithConfig tests symlink operations with behavior configuration.
func TestSymlinkFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Type assert to check if filesystem supports SymlinkFS
	sfs, ok := filesystem.(core.SymlinkFS)
	if !ok {
		t.Skip("SymlinkFS not supported")
		return
	}

	// Run all subtests
	t.Run("SymlinkCreate", func(t *testing.T) {
		testSymlinkFSCreate(t, filesystem, sfs)
	})
	t.Run("ReadlinkBasic", func(t *testing.T) {
		testSymlinkFSReadlink(t, filesystem, sfs)
	})
	t.Run("SymlinkToDirectory", func(t *testing.T) {
		testSymlinkFSDirectory(t, filesystem, sfs)
	})
	t.Run("BrokenSymlink", func(t *testing.T) {
		testSymlinkFSBroken(t, filesystem, sfs)
	})
}

// testSymlinkFSCreate tests Symlink() creation and basic verification.
func testSymlinkFSCreate(t *testing.T, filesystem core.FS, sfs core.SymlinkFS) {
	// Setup: Create a target file
	targetContent := []byte("target file content")
	if err := filesystem.WriteFile("target.txt", targetContent, 0644); err != nil {
		t.Fatalf("WriteFile(target.txt): setup failed: %v", err)
	}

	// Create a symlink pointing to the target
	err := sfs.Symlink("target.txt", "link.txt")
	if err != nil {
		t.Errorf("Symlink(target.txt, link.txt): got error %v, want nil", err)
		return
	}

	// Verify the symlink points to the correct target
	target, err := sfs.Readlink("link.txt")
	if err != nil {
		t.Errorf("Readlink(link.txt): got error %v, want nil", err)
		return
	}
	if target != "target.txt" {
		t.Errorf("Readlink(link.txt): got %q, want %q", target, "target.txt")
	}

	// Verify we can read through the symlink
	data, err := filesystem.ReadFile("link.txt")
	if err != nil {
		t.Errorf("ReadFile(link.txt) through symlink: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, targetContent) {
		t.Errorf("ReadFile(link.txt) through symlink: got %q, want %q", data, targetContent)
	}
}

// testSymlinkFSReadlink tests Readlink() to read symlink target.
func testSymlinkFSReadlink(t *testing.T, filesystem core.FS, sfs core.SymlinkFS) {
	// Setup: Create a target file and symlink
	if err := filesystem.WriteFile("readlink-target.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile(readlink-target.txt): setup failed: %v", err)
	}
	if err := sfs.Symlink("readlink-target.txt", "readlink-link.txt"); err != nil {
		t.Fatalf("Symlink(readlink-target.txt, readlink-link.txt): setup failed: %v", err)
	}

	// Test Readlink on the symlink
	target, err := sfs.Readlink("readlink-link.txt")
	if err != nil {
		t.Errorf("Readlink(readlink-link.txt): got error %v, want nil", err)
		return
	}
	if target != "readlink-target.txt" {
		t.Errorf("Readlink(readlink-link.txt): got %q, want %q", target, "readlink-target.txt")
	}

	// Test Readlink on a regular file (should fail)
	_, err = sfs.Readlink("readlink-target.txt")
	if err == nil {
		t.Errorf("Readlink(readlink-target.txt) on regular file: got nil error, want error")
	}

	// Test Readlink on non-existent file (should fail)
	_, err = sfs.Readlink("nonexistent-link.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Readlink(nonexistent-link.txt): got error %v, want fs.ErrNotExist", err)
	}
}

// testSymlinkFSDirectory tests symlink to directory.
func testSymlinkFSDirectory(t *testing.T, filesystem core.FS, sfs core.SymlinkFS) {
	// Setup: Create a directory with a file inside
	if err := filesystem.Mkdir("target-dir", 0755); err != nil {
		t.Fatalf("Mkdir(target-dir): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("target-dir/file.txt", []byte("dir content"), 0644); err != nil {
		t.Fatalf("WriteFile(target-dir/file.txt): setup failed: %v", err)
	}

	// Create a symlink pointing to the directory
	err := sfs.Symlink("target-dir", "link-dir")
	if err != nil {
		t.Errorf("Symlink(target-dir, link-dir): got error %v, want nil", err)
		return
	}

	// Verify the symlink points to the correct target
	target, err := sfs.Readlink("link-dir")
	if err != nil {
		t.Errorf("Readlink(link-dir): got error %v, want nil", err)
		return
	}
	if target != "target-dir" {
		t.Errorf("Readlink(link-dir): got %q, want %q", target, "target-dir")
	}

	// Verify we can access files through the symlinked directory
	data, err := filesystem.ReadFile("link-dir/file.txt")
	if err != nil {
		t.Errorf("ReadFile(link-dir/file.txt) through symlink: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, []byte("dir content")) {
		t.Errorf("ReadFile(link-dir/file.txt) through symlink: got %q, want %q", data, "dir content")
	}
}

// testSymlinkFSBroken tests broken symlinks (pointing to non-existent targets).
// Per design lines 256-257, broken symbolic links are valid.
func testSymlinkFSBroken(t *testing.T, filesystem core.FS, sfs core.SymlinkFS) {
	// Create a symlink pointing to a non-existent target
	err := sfs.Symlink("nonexistent-target.txt", "broken-link.txt")
	if err != nil {
		t.Errorf("Symlink(nonexistent-target.txt, broken-link.txt): got error %v, want nil", err)
		return
	}

	// Verify Readlink works on broken symlink
	target, err := sfs.Readlink("broken-link.txt")
	if err != nil {
		t.Errorf("Readlink(broken-link.txt): got error %v, want nil", err)
		return
	}
	if target != "nonexistent-target.txt" {
		t.Errorf("Readlink(broken-link.txt): got %q, want %q", target, "nonexistent-target.txt")
	}

	// Verify that trying to read through the broken symlink fails
	_, err = filesystem.ReadFile("broken-link.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(broken-link.txt) through broken symlink: got error %v, want fs.ErrNotExist", err)
	}

	// Note: Per design, broken symlinks should be detectable via Lstat (if supported).
	// We don't test Lstat here as that's covered by TestMetadataFS.
}
