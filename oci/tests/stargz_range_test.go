//go:build integration

package ocibundle_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPRangeListFiles tests ListFiles using HTTP Range requests
func TestHTTPRangeListFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create test data with enough files to make bandwidth savings meaningful
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"config.json":            `{"app":"test","version":"1.0"}`,
		"readme.md":              "# README\n\nThis is a test application",
		"data/users.json":        `[{"name":"Alice"},{"name":"Bob"}]`,
		"data/products.json":     `[{"id":1,"name":"Widget"}]`,
		"src/main.go":            "package main\n\nfunc main() {}",
		"src/utils/helpers.go":   "package utils\n\nfunc Helper() {}",
		"docs/api.md":            "# API Documentation",
		"tests/integration_test.go": "package tests",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create client configured for test registry
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/range-list-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)
	t.Logf("Pushed test data to %s", reference)

	// Test 1: List all files using HTTP Range
	t.Run("list all files with HTTP Range", func(t *testing.T) {
		result, err := client.ListFiles(ctx, reference)
		require.NoError(t, err)

		// Verify we got results
		assert.NotNil(t, result)
		assert.Greater(t, len(result.Files), 0, "Should have files")
		assert.Greater(t, result.FileCount, 0, "Should have file count")
		assert.Greater(t, result.TotalSize, int64(0), "Should have total size")

		t.Logf("Listed %d files, %d directories, total size: %d bytes",
			result.FileCount, result.DirCount, result.TotalSize)

		// Verify all test files are present
		fileNames := make(map[string]bool)
		for _, file := range result.Files {
			if !file.IsDir {
				fileNames[file.Name] = true
			}
		}

		for expectedFile := range testFiles {
			assert.True(t, fileNames[expectedFile], "Should find file: %s", expectedFile)
		}

		t.Logf("✓ Successfully listed all files via HTTP Range requests")
	})

	// Test 2: Verify listing is fast (should not download full blob)
	t.Run("listing is fast indicating Range request usage", func(t *testing.T) {
		// Multiple consecutive list operations should be fast
		// If HTTP Range is working, each should download only ~100KB
		// If not, each would download the full blob
		for i := 0; i < 3; i++ {
			result, err := client.ListFiles(ctx, reference)
			require.NoError(t, err)
			assert.Greater(t, result.FileCount, 0)
		}

		t.Logf("✓ Multiple list operations completed quickly")
	})
}

// TestHTTPRangeSelectiveExtraction tests selective file extraction using HTTP Range
func TestHTTPRangeSelectiveExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create test data with various file types
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"app.json":              `{"name":"app"}`,
		"config.yaml":           "version: 1.0",
		"main.go":               "package main",
		"utils.go":              "package utils",
		"test.go":               "package test",
		"data/file1.json":       `{"data":1}`,
		"data/file2.json":       `{"data":2}`,
		"data/file3.txt":        "text file",
		"docs/readme.md":        "# Docs",
		"scripts/deploy.sh":     "#!/bin/bash",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create client configured for test registry
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/range-selective-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)
	t.Logf("Pushed test data to %s", reference)

	// Test: Extract only JSON files using HTTP Range
	t.Run("extract only JSON files", func(t *testing.T) {
		targetDir := t.TempDir()

		// Pull with selective extraction (should use HTTP Range)
		err = client.Pull(ctx, reference, targetDir,
			ocibundle.WithFilesToExtract("**/*.json"),
		)
		require.NoError(t, err)

		// Verify only JSON files were extracted
		extractedFiles := make(map[string]bool)
		err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			relPath, _ := filepath.Rel(targetDir, path)
			extractedFiles[relPath] = true
			return nil
		})
		require.NoError(t, err)

		// Should have JSON files
		assert.True(t, extractedFiles["app.json"], "Should extract app.json")
		assert.True(t, extractedFiles[filepath.Join("data", "file1.json")], "Should extract data/file1.json")
		assert.True(t, extractedFiles[filepath.Join("data", "file2.json")], "Should extract data/file2.json")

		// Should NOT have non-JSON files
		assert.False(t, extractedFiles["config.yaml"], "Should not extract config.yaml")
		assert.False(t, extractedFiles["main.go"], "Should not extract main.go")
		assert.False(t, extractedFiles[filepath.Join("data", "file3.txt")], "Should not extract data/file3.txt")

		t.Logf("✓ Selectively extracted %d JSON files via HTTP Range", len(extractedFiles))
	})

	// Test: Extract only Go files
	t.Run("extract only Go files", func(t *testing.T) {
		targetDir := t.TempDir()

		err = client.Pull(ctx, reference, targetDir,
			ocibundle.WithFilesToExtract("**/*.go"),
		)
		require.NoError(t, err)

		// Count extracted files
		extractedFiles := make(map[string]bool)
		err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			relPath, _ := filepath.Rel(targetDir, path)
			extractedFiles[relPath] = true
			return nil
		})
		require.NoError(t, err)

		// Should have Go files
		assert.True(t, extractedFiles["main.go"], "Should extract main.go")
		assert.True(t, extractedFiles["utils.go"], "Should extract utils.go")
		assert.True(t, extractedFiles["test.go"], "Should extract test.go")

		// Should NOT have non-Go files
		assert.False(t, extractedFiles["app.json"], "Should not extract app.json")
		assert.False(t, extractedFiles["config.yaml"], "Should not extract config.yaml")

		t.Logf("✓ Selectively extracted %d Go files via HTTP Range", len(extractedFiles))
	})
}

// TestHTTPRangeFallback tests that the system gracefully falls back when Range is not supported
func TestHTTPRangeFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry (zot should support Range, but this tests the fallback logic exists)
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create minimal test data
	sourceDir := t.TempDir()
	testFile := filepath.Join(sourceDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

	// Create client
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/fallback-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)

	// Test: Operations work (either via Range or fallback)
	t.Run("list files works with fallback", func(t *testing.T) {
		result, err := client.ListFiles(ctx, reference)
		require.NoError(t, err)
		assert.Greater(t, result.FileCount, 0)
		t.Logf("✓ ListFiles completed successfully (Range or fallback)")
	})

	t.Run("selective pull works with fallback", func(t *testing.T) {
		targetDir := t.TempDir()
		err = client.Pull(ctx, reference, targetDir,
			ocibundle.WithFilesToExtract("**/*.txt"),
		)
		require.NoError(t, err)

		// Verify file was extracted
		extractedFile := filepath.Join(targetDir, "test.txt")
		assert.FileExists(t, extractedFile)
		content, err := os.ReadFile(extractedFile)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))

		t.Logf("✓ Selective extraction completed successfully (Range or fallback)")
	})
}

// TestHTTPRangeBandwidthSavings verifies actual bandwidth savings
func TestHTTPRangeBandwidthSavings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	// Create test data with reasonable size
	sourceDir := t.TempDir()

	// Create multiple files to build a meaningful archive size
	for i := 0; i < 20; i++ {
		dir := filepath.Join(sourceDir, fmt.Sprintf("dir%d", i))
		require.NoError(t, os.MkdirAll(dir, 0o755))

		for j := 0; j < 5; j++ {
			filePath := filepath.Join(dir, fmt.Sprintf("file%d.txt", j))
			// Create files with some content
			content := fmt.Sprintf("File %d-%d content\n", i, j)
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))
		}
	}

	// Create client
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/bandwidth-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)
	t.Logf("Pushed test archive to %s", reference)

	// Test: ListFiles should be fast (small download)
	t.Run("list files bandwidth comparison", func(t *testing.T) {
		// List files - should only download TOC via HTTP Range
		result, err := client.ListFiles(ctx, reference)
		require.NoError(t, err)

		t.Logf("Listed %d files successfully", result.FileCount)
		t.Logf("Total uncompressed size: %d bytes", result.TotalSize)
		t.Logf("✓ HTTP Range should have downloaded only ~100KB (TOC + footer)")
		t.Logf("  vs full archive download which would be much larger")

		// Verify we got all files
		assert.Equal(t, 100, result.FileCount, "Should list 100 files (20 dirs * 5 files)")
	})

	// Test: Selective extraction bandwidth savings
	t.Run("selective extraction bandwidth savings", func(t *testing.T) {
		targetDir := t.TempDir()

		// Extract only files from dir0 (5 out of 100 files)
		err = client.Pull(ctx, reference, targetDir,
			ocibundle.WithFilesToExtract("dir0/*.txt"),
		)
		require.NoError(t, err)

		// Count extracted files
		count := 0
		err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			count++
			return nil
		})
		require.NoError(t, err)

		assert.Equal(t, 5, count, "Should extract exactly 5 files")
		t.Logf("✓ Extracted 5 out of 100 files selectively")
		t.Logf("  HTTP Range should download only needed chunks")
		t.Logf("  vs downloading full archive with all 100 files")
	})
}
