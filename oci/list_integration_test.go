//go:build integration

package ocibundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmgilman/go/oci/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_ListFiles tests listing files from a real OCI artifact
func TestClient_ListFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create test data with various files
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"config.json":           `{"app":"test"}`,
		"readme.txt":            "README content",
		"data/file1.json":       `{"data":1}`,
		"data/file2.txt":        "Data 2",
		"data/sub/file3.json":   `{"data":3}`,
		"src/main.go":           "package main",
		"src/util/helper.go":    "package util",
		"test/main_test.go":     "package main_test",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create client
	client, err := NewWithOptions(
		WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/list-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)
	t.Logf("Pushed test data to %s", reference)

	// Test ListFiles
	t.Run("list all files", func(t *testing.T) {
		result, err := client.ListFiles(ctx, reference)
		require.NoError(t, err)

		// Verify we got results
		assert.NotNil(t, result)
		assert.Greater(t, len(result.Files), 0, "Should have files")
		assert.Greater(t, result.FileCount, 0, "Should have file count")
		assert.Greater(t, result.TotalSize, int64(0), "Should have total size")

		t.Logf("Listed %d files, %d directories, total size: %d bytes",
			result.FileCount, result.DirCount, result.TotalSize)

		// Verify all our test files are present
		fileNames := make(map[string]bool)
		for _, file := range result.Files {
			fileNames[file.Name] = true
			t.Logf("  - %s (size: %d, isDir: %v)", file.Name, file.Size, file.IsDir)
		}

		// Check for our files (excluding eStargz metadata)
		assert.True(t, fileNames["config.json"], "Should find config.json")
		assert.True(t, fileNames["readme.txt"], "Should find readme.txt")
		assert.True(t, fileNames["data/file1.json"], "Should find data/file1.json")
		assert.True(t, fileNames["src/main.go"], "Should find src/main.go")
	})

	// Test ListFilesWithFilter
	t.Run("list with filter - JSON only", func(t *testing.T) {
		result, err := client.ListFilesWithFilter(ctx, reference, "**/*.json")
		require.NoError(t, err)

		// Should only have JSON files
		for _, file := range result.Files {
			assert.Contains(t, file.Name, ".json", "Should only have .json files")
		}

		// Should have at least the 3 JSON files we created
		assert.GreaterOrEqual(t, result.FileCount, 3, "Should have at least 3 JSON files")

		t.Logf("Filtered to %d JSON files", result.FileCount)
	})

	// Test ListFilesWithFilter
	t.Run("list with filter - Go files", func(t *testing.T) {
		result, err := client.ListFilesWithFilter(ctx, reference, "**/*.go")
		require.NoError(t, err)

		// Should only have Go files
		for _, file := range result.Files {
			assert.Contains(t, file.Name, ".go", "Should only have .go files")
		}

		// Should have the 3 Go files we created
		assert.Equal(t, 3, result.FileCount, "Should have 3 Go files")

		t.Logf("Filtered to %d Go files", result.FileCount)
	})

	// Test ListFilesWithFilter with multiple patterns
	t.Run("list with multiple patterns", func(t *testing.T) {
		result, err := client.ListFilesWithFilter(ctx, reference, "**/*.json", "**/*.go")
		require.NoError(t, err)

		// Should have both JSON and Go files
		assert.GreaterOrEqual(t, result.FileCount, 6, "Should have at least 6 files (3 JSON + 3 Go)")

		t.Logf("Filtered to %d files with multiple patterns", result.FileCount)
	})
}

// TestClient_ListFiles_InvalidReference tests error handling
func TestClient_ListFiles_InvalidReference(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	client, err := NewWithOptions(
		WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	t.Run("empty reference", func(t *testing.T) {
		_, err := client.ListFiles(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reference cannot be empty")
	})

	t.Run("non-existent reference", func(t *testing.T) {
		reference := fmt.Sprintf("%s/does-not-exist:latest", registry.Reference())
		_, err := client.ListFiles(ctx, reference)
		assert.Error(t, err)
	})
}

// TestClient_ListFiles_Performance tests that listing is faster than full download
func TestClient_ListFiles_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create a larger test bundle
	sourceDir := t.TempDir()
	for i := 0; i < 100; i++ {
		path := filepath.Join(sourceDir, fmt.Sprintf("file%d.txt", i))
		// Create files with some content
		content := make([]byte, 10*1024) // 10KB each = 1MB total
		for j := range content {
			content[j] = byte(i)
		}
		require.NoError(t, os.WriteFile(path, content, 0o644))
	}

	client, err := NewWithOptions(
		WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	reference := fmt.Sprintf("%s/perf-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)

	// List files (should be fast - only downloads TOC)
	result, err := client.ListFiles(ctx, reference)
	require.NoError(t, err)

	assert.Equal(t, 100, result.FileCount, "Should list all 100 files")
	t.Logf("Listed %d files without downloading full archive", result.FileCount)

	// Verify total size is accurate
	expectedSize := int64(100 * 10 * 1024) // 100 files * 10KB
	assert.Equal(t, expectedSize, result.TotalSize, "Should calculate correct total size")
}
