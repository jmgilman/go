package fstest

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestChrootFS tests scoped filesystem views and boundary enforcement.
// Verifies chroot prevents path traversal attacks.
// Uses POSIXTestConfig() by default.
func TestChrootFS(t *testing.T, filesystem core.FS) {
	TestChrootFSWithConfig(t, filesystem, POSIXTestConfig())
}

// TestChrootFSWithConfig tests scoped filesystem views with behavior configuration.
func TestChrootFSWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Run all subtests
	t.Run("ChrootToSubdirectory", func(t *testing.T) {
		testChrootFSBasic(t, filesystem, config)
	})
	t.Run("PathTraversalPrevention", func(t *testing.T) {
		testChrootFSPathTraversal(t, filesystem, config)
	})
	t.Run("ChrootOnChroot", func(t *testing.T) {
		testChrootFSNested(t, filesystem, config)
	})
	t.Run("SpecialCharactersAndNormalization", func(t *testing.T) {
		testChrootFSSpecialChars(t, filesystem, config)
	})
}

// testChrootFSBasic tests Chroot() to subdirectory and verifies operations stay within boundary.
func testChrootFSBasic(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create directory structure
	// root/
	//   chroot-dir/
	//     inside.txt
	//   outside.txt
	if err := filesystem.Mkdir("chroot-dir", 0755); err != nil {
		t.Fatalf("Mkdir(chroot-dir): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("chroot-dir/inside.txt", []byte("inside content"), 0644); err != nil {
		t.Fatalf("WriteFile(chroot-dir/inside.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("outside.txt", []byte("outside content"), 0644); err != nil {
		t.Fatalf("WriteFile(outside.txt): setup failed: %v", err)
	}

	// Chroot to subdirectory
	chrootFS, err := filesystem.Chroot("chroot-dir")
	if err != nil {
		t.Fatalf("Chroot(chroot-dir): got error %v, want nil", err)
	}

	// Verify we can access files inside the chroot
	data, err := chrootFS.ReadFile("inside.txt")
	if err != nil {
		t.Errorf("chrootFS.ReadFile(inside.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("inside content")) {
		t.Errorf("chrootFS.ReadFile(inside.txt): got %q, want %q", data, "inside content")
	}

	// Verify we cannot access files outside the chroot
	_, err = chrootFS.ReadFile("outside.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("chrootFS.ReadFile(outside.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Verify write operations stay within boundary
	err = chrootFS.WriteFile("new-inside.txt", []byte("new content"), 0644)
	if err != nil {
		t.Errorf("chrootFS.WriteFile(new-inside.txt): got error %v, want nil", err)
	}

	// Verify the file was created in the correct location (relative to root FS)
	data, err = filesystem.ReadFile("chroot-dir/new-inside.txt")
	if err != nil {
		t.Errorf("filesystem.ReadFile(chroot-dir/new-inside.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("new content")) {
		t.Errorf("filesystem.ReadFile(chroot-dir/new-inside.txt): got %q, want %q", data, "new content")
	}

	// Verify we cannot write outside the chroot via absolute path
	// (attempting to write to root from chrootFS should fail)
	err = chrootFS.WriteFile("/escape.txt", []byte("escape"), 0644)
	if err == nil {
		// Check if file was created outside chroot (security breach)
		if _, statErr := filesystem.Stat("escape.txt"); statErr == nil {
			t.Errorf("chrootFS.WriteFile(/escape.txt): allowed write outside chroot boundary (security issue)")
		}
	}
	// Note: The behavior here depends on implementation - some may error, some may treat "/" as relative
}

// testChrootFSPathTraversal tests that path traversal attacks via ".." fail securely.
func testChrootFSPathTraversal(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create directory structure with sensitive file outside
	// root/
	//   sandbox/
	//     allowed.txt
	//   sensitive.txt
	if err := filesystem.Mkdir("sandbox", 0755); err != nil {
		t.Fatalf("Mkdir(sandbox): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("sandbox/allowed.txt", []byte("allowed"), 0644); err != nil {
		t.Fatalf("WriteFile(sandbox/allowed.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("sensitive.txt", []byte("secret data"), 0644); err != nil {
		t.Fatalf("WriteFile(sensitive.txt): setup failed: %v", err)
	}

	// Chroot to sandbox
	chrootFS, err := filesystem.Chroot("sandbox")
	if err != nil {
		t.Fatalf("Chroot(sandbox): got error %v, want nil", err)
	}

	// Test 1: Try to read file outside via ".."
	_, err = chrootFS.ReadFile("../sensitive.txt")
	if err == nil {
		t.Errorf("chrootFS.ReadFile(../sensitive.txt): should fail, but succeeded (path traversal vulnerability)")
	}
	// The error should be either ErrNotExist or some other error preventing access
	// The important thing is that we should NOT be able to read the sensitive file

	// Test 2: Try more aggressive path traversal "../../../etc/passwd" style
	_, err = chrootFS.ReadFile("../../../sensitive.txt")
	if err == nil {
		t.Errorf("chrootFS.ReadFile(../../../sensitive.txt): should fail, but succeeded (path traversal vulnerability)")
	}

	// Test 3: Try path traversal in the middle of a path
	_, err = chrootFS.ReadFile("allowed.txt/../../sensitive.txt")
	if err == nil {
		t.Errorf("chrootFS.ReadFile(allowed.txt/../../sensitive.txt): should fail, but succeeded (path traversal vulnerability)")
	}

	// Test 4: Verify we can still read allowed files
	data, err := chrootFS.ReadFile("allowed.txt")
	if err != nil {
		t.Errorf("chrootFS.ReadFile(allowed.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("allowed")) {
		t.Errorf("chrootFS.ReadFile(allowed.txt): got %q, want %q", data, "allowed")
	}

	// Test 5: Try to write outside via ".."
	err = chrootFS.WriteFile("../escape.txt", []byte("escaped"), 0644)
	if err == nil {
		// Check if file was created outside sandbox (security breach)
		if _, statErr := filesystem.Stat("escape.txt"); statErr == nil {
			t.Errorf("chrootFS.WriteFile(../escape.txt): allowed write outside chroot (security issue)")
		}
	}
	// Should either error or safely contain within chroot

	// Test 6: Try to create directory outside via ".."
	err = chrootFS.Mkdir("../escape-dir", 0755)
	if err == nil {
		// Check if directory was created outside sandbox (security breach)
		if info, statErr := filesystem.Stat("escape-dir"); statErr == nil && info.IsDir() {
			t.Errorf("chrootFS.Mkdir(../escape-dir): allowed mkdir outside chroot (security issue)")
		}
	}
}

// testChrootFSNested tests that chroot on chroot works correctly.
func testChrootFSNested(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create nested directory structure
	setupNestedChrootStructure(t, filesystem, config)

	// Test first level chroot
	chroot1 := testNestedChrootLevel1(t, filesystem)

	// Test second level chroot
	chroot2 := testNestedChrootLevel2(t, chroot1)

	// Test third level chroot and verify write operations
	testNestedChrootLevel3(t, filesystem, chroot2)
}

// setupNestedChrootStructure creates the directory structure for nested chroot tests.
func setupNestedChrootStructure(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// root/
	//   level1/
	//     level2/
	//       level3/
	//         deep.txt
	//       mid.txt
	//     shallow.txt
	if err := filesystem.MkdirAll("level1/level2/level3", 0755); err != nil {
		t.Fatalf("MkdirAll(level1/level2/level3): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("level1/shallow.txt", []byte("shallow"), 0644); err != nil {
		t.Fatalf("WriteFile(level1/shallow.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("level1/level2/mid.txt", []byte("mid"), 0644); err != nil {
		t.Fatalf("WriteFile(level1/level2/mid.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("level1/level2/level3/deep.txt", []byte("deep"), 0644); err != nil {
		t.Fatalf("WriteFile(level1/level2/level3/deep.txt): setup failed: %v", err)
	}
}

// testNestedChrootLevel1 tests the first level chroot and returns it.
func testNestedChrootLevel1(t *testing.T, filesystem core.FS) core.FS {
	chroot1, err := filesystem.Chroot("level1")
	if err != nil {
		t.Fatalf("Chroot(level1): got error %v, want nil", err)
	}

	// Verify we can access shallow.txt from chroot1
	data, err := chroot1.ReadFile("shallow.txt")
	if err != nil {
		t.Errorf("chroot1.ReadFile(shallow.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("shallow")) {
		t.Errorf("chroot1.ReadFile(shallow.txt): got %q, want %q", data, "shallow")
	}

	return chroot1
}

// testNestedChrootLevel2 tests the second level chroot and returns it.
func testNestedChrootLevel2(t *testing.T, chroot1 core.FS) core.FS {
	chroot2, err := chroot1.Chroot("level2")
	if err != nil {
		t.Fatalf("chroot1.Chroot(level2): got error %v, want nil", err)
	}

	// Verify we can access mid.txt from chroot2
	data, err := chroot2.ReadFile("mid.txt")
	if err != nil {
		t.Errorf("chroot2.ReadFile(mid.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("mid")) {
		t.Errorf("chroot2.ReadFile(mid.txt): got %q, want %q", data, "mid")
	}

	// Verify we cannot access shallow.txt from chroot2 (it's outside this chroot)
	_, err = chroot2.ReadFile("shallow.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("chroot2.ReadFile(shallow.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Verify we cannot escape chroot2 to access shallow.txt via ".."
	_, err = chroot2.ReadFile("../shallow.txt")
	if err == nil {
		t.Errorf("chroot2.ReadFile(../shallow.txt): should fail (path traversal from nested chroot)")
	}

	return chroot2
}

// testNestedChrootLevel3 tests the third level chroot and verifies write operations.
func testNestedChrootLevel3(t *testing.T, filesystem core.FS, chroot2 core.FS) {
	chroot3, err := chroot2.Chroot("level3")
	if err != nil {
		t.Fatalf("chroot2.Chroot(level3): got error %v, want nil", err)
	}

	// Verify we can access deep.txt from chroot3
	data, err := chroot3.ReadFile("deep.txt")
	if err != nil {
		t.Errorf("chroot3.ReadFile(deep.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("deep")) {
		t.Errorf("chroot3.ReadFile(deep.txt): got %q, want %q", data, "deep")
	}

	// Verify we cannot access mid.txt from chroot3
	_, err = chroot3.ReadFile("mid.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("chroot3.ReadFile(mid.txt): got error %v, want fs.ErrNotExist", err)
	}

	// Verify write operations in nested chroot are correctly scoped
	err = chroot3.WriteFile("nested-file.txt", []byte("nested content"), 0644)
	if err != nil {
		t.Errorf("chroot3.WriteFile(nested-file.txt): got error %v, want nil", err)
	}

	// Verify the file was created in the correct location (from root FS perspective)
	data, err = filesystem.ReadFile("level1/level2/level3/nested-file.txt")
	if err != nil {
		t.Errorf("filesystem.ReadFile(level1/level2/level3/nested-file.txt): got error %v, want nil", err)
	} else if !bytes.Equal(data, []byte("nested content")) {
		t.Errorf("filesystem.ReadFile(level1/level2/level3/nested-file.txt): got %q, want %q", data, "nested content")
	}
}

// testChrootFSSpecialChars tests special characters and path normalization.
func testChrootFSSpecialChars(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create directory structure with various path scenarios
	setupSpecialCharsStructure(t, filesystem, config)

	// Chroot to testdir
	chrootFS, err := filesystem.Chroot("testdir")
	if err != nil {
		t.Fatalf("Chroot(testdir): got error %v, want nil", err)
	}

	// Run various path normalization tests
	testChrootRedundantSlashes(t, chrootFS)
	testChrootDotPaths(t, chrootFS)
	testChrootDotDotSafety(t, chrootFS)
	testChrootBasicPath(t, chrootFS)
	testChrootTrailingSlash(t, chrootFS)
}

// setupSpecialCharsStructure creates the directory structure for special chars tests.
func setupSpecialCharsStructure(t *testing.T, filesystem core.FS, config FSTestConfig) {
	if err := filesystem.MkdirAll("testdir/subdir", 0755); err != nil {
		t.Fatalf("MkdirAll(testdir/subdir): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("testdir/file.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile(testdir/file.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("testdir/subdir/nested.txt", []byte("nested"), 0644); err != nil {
		t.Fatalf("WriteFile(testdir/subdir/nested.txt): setup failed: %v", err)
	}
}

// testChrootRedundantSlashes tests path with redundant slashes.
func testChrootRedundantSlashes(t *testing.T, chrootFS core.FS) {
	// Note: Behavior may vary - some implementations normalize, some don't
	data, err := chrootFS.ReadFile("subdir//nested.txt")
	if err == nil && !bytes.Equal(data, []byte("nested")) {
		t.Errorf("chrootFS.ReadFile(subdir//nested.txt): got %q, want %q", data, "nested")
	}
}

// testChrootDotPaths tests paths with "." components.
func testChrootDotPaths(t *testing.T, chrootFS core.FS) {
	// Test single dot at start
	data, err := chrootFS.ReadFile("./file.txt")
	if err == nil && !bytes.Equal(data, []byte("content")) {
		t.Errorf("chrootFS.ReadFile(./file.txt): got %q, want %q", data, "content")
	}

	// Test dots in the middle
	data, err = chrootFS.ReadFile("./subdir/./nested.txt")
	if err == nil && !bytes.Equal(data, []byte("nested")) {
		t.Errorf("chrootFS.ReadFile(./subdir/./nested.txt): got %q, want %q", data, "nested")
	}
}

// testChrootDotDotSafety verifies ".." is properly blocked or safely handled.
func testChrootDotDotSafety(t *testing.T, chrootFS core.FS) {
	// This should either:
	// a) Successfully read file.txt (if normalized to "file.txt" safely within chroot)
	// b) Fail with an error (if ".." is blocked)
	// What it should NOT do is allow access outside the chroot
	data, err := chrootFS.ReadFile("subdir/../file.txt")
	if err == nil && !bytes.Equal(data, []byte("content")) {
		// If content is different, may have escaped chroot
		t.Errorf("chrootFS.ReadFile(subdir/../file.txt): path normalization may be unsafe")
	}
}

// testChrootBasicPath tests normal path handling.
func testChrootBasicPath(t *testing.T, chrootFS core.FS) {
	_, err := chrootFS.ReadFile("subdir/nested.txt")
	if err != nil {
		t.Errorf("chrootFS.ReadFile(subdir/nested.txt): got error %v, want nil", err)
	}
}

// testChrootTrailingSlash tests trailing slash handling on directory operations.
func testChrootTrailingSlash(t *testing.T, chrootFS core.FS) {
	entries, err := chrootFS.ReadDir("subdir/")
	if err == nil && len(entries) == 0 {
		t.Errorf("chrootFS.ReadDir(subdir/): got empty directory, expected files")
	}
}
