package fstest

import (
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestWalkFS tests directory tree traversal with Walk.
// Verifies correct ordering and path handling.
// Uses POSIXTestConfig() by default.
func TestWalkFS(t *testing.T, filesystem core.FS) {
	TestWalkFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestWalkFSWithConfig tests directory tree traversal with behavior configuration.
func TestWalkFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Run all subtests
	t.Run("SimpleTree", func(t *testing.T) {
		testWalkFSSimpleTree(t, filesystem, config)
	})
	t.Run("WithSubdirectories", func(t *testing.T) {
		testWalkFSWithSubdirectories(t, filesystem, config)
	})
	t.Run("EmptyDirectory", func(t *testing.T) {
		testWalkFSEmptyDirectory(t, filesystem, config)
	})
	t.Run("PathHandling", func(t *testing.T) {
		testWalkFSPathHandling(t, filesystem, config)
	})
}

// testWalkFSSimpleTree tests Walk() on a simple directory tree.
func testWalkFSSimpleTree(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a simple directory with files
	if err := filesystem.Mkdir("walktest", 0755); err != nil {
		t.Fatalf("Mkdir(walktest): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("walktest/file1.txt", []byte("content1"), 0644); err != nil {
		t.Fatalf("WriteFile(walktest/file1.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("walktest/file2.txt", []byte("content2"), 0644); err != nil {
		t.Fatalf("WriteFile(walktest/file2.txt): setup failed: %v", err)
	}

	// Walk the directory and collect visited paths
	var visited []string
	err := filesystem.Walk("walktest", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited = append(visited, path)
		return nil
	})

	if err != nil {
		t.Fatalf("Walk(walktest): got error %v, want nil", err)
	}

	// Verify all paths were visited
	// For virtual directories (S3-like), directory prefixes may not be visited
	// Only verify files are present
	if config.VirtualDirectories {
		// Should visit: walktest/file1.txt, walktest/file2.txt (directory may be absent)
		if len(visited) < 2 {
			t.Errorf("Walk(walktest): visited %d paths, want at least 2. Visited: %v", len(visited), visited)
			return
		}
		// Verify files are in visited paths (order may vary)
		hasFile1 := false
		hasFile2 := false
		for _, path := range visited {
			if path == "walktest/file1.txt" {
				hasFile1 = true
			}
			if path == "walktest/file2.txt" {
				hasFile2 = true
			}
		}
		if !hasFile1 {
			t.Errorf("Walk(walktest): missing walktest/file1.txt in visited paths: %v", visited)
		}
		if !hasFile2 {
			t.Errorf("Walk(walktest): missing walktest/file2.txt in visited paths: %v", visited)
		}
	} else {
		// Walk should visit: walktest (dir), walktest/file1.txt, walktest/file2.txt
		// The order should be lexical: directory first, then files in lexical order
		expectedPaths := []string{"walktest", "walktest/file1.txt", "walktest/file2.txt"}
		if len(visited) != len(expectedPaths) {
			t.Errorf("Walk(walktest): visited %d paths, want %d. Visited: %v", len(visited), len(expectedPaths), visited)
			return
		}

		for i, expected := range expectedPaths {
			if visited[i] != expected {
				t.Errorf("Walk(walktest): path[%d] = %q, want %q", i, visited[i], expected)
			}
		}
	}
}

// testWalkFSWithSubdirectories tests Walk() with nested subdirectories.
func testWalkFSWithSubdirectories(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a directory tree with subdirectories
	if err := filesystem.MkdirAll("walkroot/subdir1", 0755); err != nil {
		t.Fatalf("MkdirAll(walkroot/subdir1): setup failed: %v", err)
	}
	if err := filesystem.MkdirAll("walkroot/subdir2", 0755); err != nil {
		t.Fatalf("MkdirAll(walkroot/subdir2): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("walkroot/root.txt", []byte("root"), 0644); err != nil {
		t.Fatalf("WriteFile(walkroot/root.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("walkroot/subdir1/file1.txt", []byte("file1"), 0644); err != nil {
		t.Fatalf("WriteFile(walkroot/subdir1/file1.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("walkroot/subdir2/file2.txt", []byte("file2"), 0644); err != nil {
		t.Fatalf("WriteFile(walkroot/subdir2/file2.txt): setup failed: %v", err)
	}

	// Walk the directory tree and collect visited paths
	var visited []string
	err := filesystem.Walk("walkroot", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited = append(visited, path)
		return nil
	})

	if err != nil {
		t.Fatalf("Walk(walkroot): got error %v, want nil", err)
	}

	// Verify all paths were visited
	// For virtual directories (S3-like), directory prefixes may not be visited
	if config.VirtualDirectories {
		// Should visit at least the 3 files (directories may be absent)
		if len(visited) < 3 {
			t.Errorf("Walk(walkroot): visited %d paths, want at least 3. Visited: %v", len(visited), visited)
			return
		}
		// Verify all files are present (order may vary)
		hasRootFile := false
		hasFile1 := false
		hasFile2 := false
		for _, path := range visited {
			if path == "walkroot/root.txt" {
				hasRootFile = true
			}
			if path == "walkroot/subdir1/file1.txt" {
				hasFile1 = true
			}
			if path == "walkroot/subdir2/file2.txt" {
				hasFile2 = true
			}
		}
		if !hasRootFile {
			t.Errorf("Walk(walkroot): missing walkroot/root.txt in visited paths: %v", visited)
		}
		if !hasFile1 {
			t.Errorf("Walk(walkroot): missing walkroot/subdir1/file1.txt in visited paths: %v", visited)
		}
		if !hasFile2 {
			t.Errorf("Walk(walkroot): missing walkroot/subdir2/file2.txt in visited paths: %v", visited)
		}
	} else {
		// Verify all paths were visited in lexical order
		// Expected order: walkroot (dir), walkroot/root.txt, walkroot/subdir1 (dir),
		// walkroot/subdir1/file1.txt, walkroot/subdir2 (dir), walkroot/subdir2/file2.txt
		expectedPaths := []string{
			"walkroot",
			"walkroot/root.txt",
			"walkroot/subdir1",
			"walkroot/subdir1/file1.txt",
			"walkroot/subdir2",
			"walkroot/subdir2/file2.txt",
		}

		if len(visited) != len(expectedPaths) {
			t.Errorf("Walk(walkroot): visited %d paths, want %d. Visited: %v", len(visited), len(expectedPaths), visited)
			return
		}

		for i, expected := range expectedPaths {
			if visited[i] != expected {
				t.Errorf("Walk(walkroot): path[%d] = %q, want %q", i, visited[i], expected)
			}
		}
	}
}

// testWalkFSEmptyDirectory tests Walk() on an empty directory.
func testWalkFSEmptyDirectory(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Skip if filesystem has virtual directories (S3-like)
	// Empty directories cannot be walked in S3 because they don't exist as objects
	if config.VirtualDirectories {
		t.Skip("Skipping empty directory Walk test - filesystem has virtual directories")
		return
	}

	// Setup: Create an empty directory
	if err := filesystem.Mkdir("emptydir", 0755); err != nil {
		t.Fatalf("Mkdir(emptydir): setup failed: %v", err)
	}

	// Walk the empty directory
	var visited []string
	err := filesystem.Walk("emptydir", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited = append(visited, path)
		return nil
	})

	if err != nil {
		t.Fatalf("Walk(emptydir): got error %v, want nil", err)
	}

	// Verify only the directory itself was visited
	if len(visited) != 1 {
		t.Errorf("Walk(emptydir): visited %d paths, want 1. Visited: %v", len(visited), visited)
		return
	}

	if visited[0] != "emptydir" {
		t.Errorf("Walk(emptydir): path[0] = %q, want %q", visited[0], "emptydir")
	}
}

// testWalkFSPathHandling tests Walk() verifies paths are correct.
func testWalkFSPathHandling(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test structure
	if err := filesystem.MkdirAll("pathtest/nested", 0755); err != nil {
		t.Fatalf("MkdirAll(pathtest/nested): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("pathtest/top.txt", []byte("top"), 0644); err != nil {
		t.Fatalf("WriteFile(pathtest/top.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("pathtest/nested/deep.txt", []byte("deep"), 0644); err != nil {
		t.Fatalf("WriteFile(pathtest/nested/deep.txt): setup failed: %v", err)
	}

	// Walk and verify each path can be accessed via Stat
	err := filesystem.Walk("pathtest", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// For virtual directories, skip Stat check on directory entries
		if config.VirtualDirectories && d.IsDir() {
			return nil
		}

		// Verify the path is accessible via Stat
		info, statErr := filesystem.Stat(path)
		if statErr != nil {
			t.Errorf("Walk provided path %q that cannot be accessed via Stat: %v", path, statErr)
			return nil // Continue walking
		}

		// Verify the DirEntry and FileInfo match
		if d.IsDir() != info.IsDir() {
			t.Errorf("Walk path %q: DirEntry.IsDir() = %v, FileInfo.IsDir() = %v (mismatch)",
				path, d.IsDir(), info.IsDir())
		}

		if d.Name() != info.Name() {
			t.Errorf("Walk path %q: DirEntry.Name() = %q, FileInfo.Name() = %q (mismatch)",
				path, d.Name(), info.Name())
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Walk(pathtest): got error %v, want nil", err)
	}
}
