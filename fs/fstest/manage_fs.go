package fstest

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestManageFS tests file management: Remove, RemoveAll, Rename.
// Tests deletion and renaming of files and directories.
// Uses POSIXTestConfig() by default.
func TestManageFS(t *testing.T, filesystem core.FS) {
	TestManageFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestManageFSWithConfig tests file management with behavior configuration.
func TestManageFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Run all subtests
	t.Run("RemoveSingleFile", func(t *testing.T) {
		testManageFSRemoveFile(t, filesystem, config)
	})
	t.Run("RemoveEmptyDirectory", func(t *testing.T) {
		testManageFSRemoveEmptyDir(t, filesystem, config)
	})
	t.Run("RemoveAll", func(t *testing.T) {
		testManageFSRemoveAll(t, filesystem, config)
	})
	t.Run("RenameFile", func(t *testing.T) {
		testManageFSRenameFile(t, filesystem, config)
	})
	t.Run("RenameDirectory", func(t *testing.T) {
		testManageFSRenameDir(t, filesystem, config)
	})
	t.Run("RemoveNotExist", func(t *testing.T) {
		testManageFSRemoveNotExist(t, filesystem, config)
	})
}

// testManageFSRemoveFile tests Remove() single file deletion.
func testManageFSRemoveFile(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file
	testData := []byte("test file content")
	if err := filesystem.WriteFile("testfile.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(testfile.txt): setup failed: %v", err)
	}

	// Verify file exists before removal
	if _, err := filesystem.Stat("testfile.txt"); err != nil {
		t.Fatalf("Stat(testfile.txt): file should exist before removal: %v", err)
	}

	// Remove the file
	if err := filesystem.Remove("testfile.txt"); err != nil {
		t.Fatalf("Remove(testfile.txt): got error %v, want nil", err)
	}

	// Verify file no longer exists
	_, err := filesystem.Stat("testfile.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(testfile.txt) after Remove: got error %v, want fs.ErrNotExist", err)
	}
}

// testManageFSRemoveEmptyDir tests Remove() empty directory deletion.
func testManageFSRemoveEmptyDir(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create an empty directory
	if err := filesystem.Mkdir("emptydir", 0755); err != nil {
		t.Fatalf("Mkdir(emptydir): setup failed: %v", err)
	}

	// Skip Stat verification if filesystem has virtual directories
	if !config.VirtualDirectories {
		// Verify directory exists before removal (only for non-virtual dirs)
		info, err := filesystem.Stat("emptydir")
		if err != nil {
			t.Fatalf("Stat(emptydir): directory should exist before removal: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(emptydir): should be a directory")
		}
	}

	// Remove the empty directory
	if err := filesystem.Remove("emptydir"); err != nil {
		t.Fatalf("Remove(emptydir): got error %v, want nil", err)
	}

	// Verify directory no longer exists
	_, err := filesystem.Stat("emptydir")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(emptydir) after Remove: got error %v, want fs.ErrNotExist", err)
	}
}

// testManageFSRemoveAll tests RemoveAll() recursive deletion.
func testManageFSRemoveAll(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a directory tree with files
	if err := filesystem.MkdirAll("parent/child1", 0755); err != nil {
		t.Fatalf("MkdirAll(parent/child1): setup failed: %v", err)
	}
	if err := filesystem.MkdirAll("parent/child2", 0755); err != nil {
		t.Fatalf("MkdirAll(parent/child2): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("parent/file1.txt", []byte("content1"), 0644); err != nil {
		t.Fatalf("WriteFile(parent/file1.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("parent/child1/file2.txt", []byte("content2"), 0644); err != nil {
		t.Fatalf("WriteFile(parent/child1/file2.txt): setup failed: %v", err)
	}

	// Skip Stat verification if filesystem has virtual directories
	if !config.VirtualDirectories {
		// Verify parent directory exists before removal (only for non-virtual dirs)
		info, err := filesystem.Stat("parent")
		if err != nil {
			t.Fatalf("Stat(parent): directory should exist before removal: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(parent): should be a directory")
		}
	}

	// Remove the entire directory tree
	if err := filesystem.RemoveAll("parent"); err != nil {
		t.Fatalf("RemoveAll(parent): got error %v, want nil", err)
	}

	// Verify parent directory and all children no longer exist
	_, err := filesystem.Stat("parent")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(parent) after RemoveAll: got error %v, want fs.ErrNotExist", err)
	}

	// Note: Per design (line 112), RemoveAll on non-existent path should return nil
	// Test that RemoveAll on already-removed path doesn't error
	if err := filesystem.RemoveAll("parent"); err != nil {
		t.Errorf("RemoveAll(parent) on non-existent path: got error %v, want nil", err)
	}
}

// testManageFSRenameFile tests Rename() file.
func testManageFSRenameFile(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file
	testData := []byte("test file for rename")
	if err := filesystem.WriteFile("oldfile.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(oldfile.txt): setup failed: %v", err)
	}

	// Verify old file exists
	if _, err := filesystem.Stat("oldfile.txt"); err != nil {
		t.Fatalf("Stat(oldfile.txt): file should exist before rename: %v", err)
	}

	// Rename the file
	if err := filesystem.Rename("oldfile.txt", "newfile.txt"); err != nil {
		t.Fatalf("Rename(oldfile.txt, newfile.txt): got error %v, want nil", err)
	}

	// Verify old file no longer exists
	_, err := filesystem.Stat("oldfile.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(oldfile.txt) after Rename: got error %v, want fs.ErrNotExist", err)
	}

	// Verify new file exists with correct content
	data, err := filesystem.ReadFile("newfile.txt")
	if err != nil {
		t.Errorf("ReadFile(newfile.txt) after Rename: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(newfile.txt) after Rename: got %q, want %q", data, testData)
	}
}

// testManageFSRenameDir tests Rename() directory.
func testManageFSRenameDir(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a directory with a file inside
	if err := filesystem.Mkdir("olddir", 0755); err != nil {
		t.Fatalf("Mkdir(olddir): setup failed: %v", err)
	}
	testData := []byte("test file in directory")
	if err := filesystem.WriteFile("olddir/testfile.txt", testData, 0644); err != nil {
		t.Fatalf("WriteFile(olddir/testfile.txt): setup failed: %v", err)
	}

	// Skip Stat verification if filesystem has virtual directories
	if !config.VirtualDirectories {
		// Verify old directory exists (only for non-virtual dirs)
		info, err := filesystem.Stat("olddir")
		if err != nil {
			t.Fatalf("Stat(olddir): directory should exist before rename: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(olddir): should be a directory")
		}
	}

	// Rename the directory
	if err := filesystem.Rename("olddir", "newdir"); err != nil {
		t.Fatalf("Rename(olddir, newdir): got error %v, want nil", err)
	}

	// Verify old directory no longer exists
	_, err := filesystem.Stat("olddir")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(olddir) after Rename: got error %v, want fs.ErrNotExist", err)
	}

	// Skip new directory Stat if virtual directories
	if !config.VirtualDirectories {
		// Verify new directory exists (only for non-virtual dirs)
		info, err := filesystem.Stat("newdir")
		if err != nil {
			t.Errorf("Stat(newdir) after Rename: got error %v, want nil", err)
			return
		}
		if !info.IsDir() {
			t.Errorf("Stat(newdir) after Rename: IsDir() = false, want true")
		}
	}

	// Verify file inside renamed directory still exists with correct content
	data, err := filesystem.ReadFile("newdir/testfile.txt")
	if err != nil {
		t.Errorf("ReadFile(newdir/testfile.txt) after Rename: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("ReadFile(newdir/testfile.txt) after Rename: got %q, want %q", data, testData)
	}
}

// testManageFSRemoveNotExist tests error case: Remove non-existent file returns fs.ErrNotExist.
func testManageFSRemoveNotExist(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Skip if filesystem has idempotent delete (S3-like)
	if config.IdempotentDelete {
		t.Skip("Skipping Remove non-existent test - filesystem has idempotent delete")
		return
	}

	// Try to remove a non-existent file
	err := filesystem.Remove("nonexistent.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove(nonexistent.txt): got error %v, want fs.ErrNotExist", err)
	}
}
