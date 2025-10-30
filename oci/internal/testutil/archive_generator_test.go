// Package testutil provides testing utilities for the OCI bundle library.
// This file contains tests for the archive generator.
package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchiveGenerator_BasicGeneration tests basic archive generation functionality.
func TestArchiveGenerator_BasicGeneration(t *testing.T) {
	generator, err := NewArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	ctx := context.Background()
	outputPath := filepath.Join(generator.tempDir, "test-archive.tar.gz")

	// Generate a small test archive
	size, err := generator.GenerateTestArchive(ctx, 1024, 5, "text", outputPath)
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestArchiveGenerator_ContentPatterns tests different content generation patterns.
func TestArchiveGenerator_ContentPatterns(t *testing.T) {
	generator, err := NewArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	ctx := context.Background()
	patterns := []string{"zeros", "random", "text", "mixed"}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			outputPath := filepath.Join(generator.tempDir, fmt.Sprintf("test-%s.tar.gz", pattern))

			size, err := generator.GenerateTestArchive(ctx, 512, 3, pattern, outputPath)
			require.NoError(t, err)
			assert.Greater(t, size, int64(0))
		})
	}
}

// TestArchiveGenerator_DirectoryGeneration tests directory structure generation.
func TestArchiveGenerator_DirectoryGeneration(t *testing.T) {
	generator, err := NewArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	testDir := filepath.Join(generator.tempDir, "test-dir")

	size, err := generator.GenerateTestDirectory(testDir, 2, 3, 100)
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))

	// Count files created
	var fileCount int
	err = filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	require.NoError(t, err)

	// Should have 3 files in root + 3 subdirs * 3 files each = 12 files
	expectedFiles := 3 + (3 * 3)
	assert.Equal(t, expectedFiles, fileCount)
}

// TestArchiveGenerator_LargeArchive tests large archive generation.
func TestArchiveGenerator_LargeArchive(t *testing.T) {
	generator, err := NewArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	ctx := context.Background()
	outputPath := filepath.Join(generator.tempDir, "large-archive.tar.gz")

	// Generate 1MB archive
	targetSize := int64(1024 * 1024)
	size, err := generator.GenerateLargeArchive(ctx, targetSize, outputPath)
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))

	// Verify file size is reasonable (compressed size should be close to original for random data)
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
	// For random data, compressed size might be larger than original due to entropy
	assert.Less(t, info.Size(), targetSize*2)
}

// TestArchiveGenerator_EmptyArchive tests empty archive generation.
func TestArchiveGenerator_EmptyArchive(t *testing.T) {
	generator, err := NewArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "empty-archive.tar.gz")

	err = generator.GenerateEmptyArchive(outputPath)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// BenchmarkArchiveGeneration benchmarks archive generation performance.
func BenchmarkArchiveGeneration(b *testing.B) {
	generator, err := NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create generator: %v", err)
	}
	defer generator.Close()

	ctx := context.Background()

	sizes := []int64{1024, 10 * 1024, 100 * 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				outputPath := filepath.Join(generator.tempDir, fmt.Sprintf("bench-%d.tar.gz", i))
				_, err := generator.GenerateTestArchive(ctx, size, 5, "random", outputPath)
				if err != nil {
					b.Fatalf("Failed to generate archive: %v", err)
				}
				os.Remove(outputPath) // Clean up
			}
		})
	}
}

// BenchmarkDirectoryGeneration benchmarks directory structure generation.
func BenchmarkDirectoryGeneration(b *testing.B) {
	generator, err := NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create generator: %v", err)
	}
	defer generator.Close()

	for i := 0; i < b.N; i++ {
		testDir := filepath.Join(generator.tempDir, fmt.Sprintf("bench-dir-%d", i))
		_, err := generator.GenerateTestDirectory(testDir, 2, 5, 1024)
		if err != nil {
			b.Fatalf("Failed to generate directory: %v", err)
		}
		os.RemoveAll(testDir) // Clean up
	}
}
