// Package testutil provides testing utilities for the OCI bundle library.
// This file contains tests for malicious archive generators.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaliciousArchive_PathTraversal tests path traversal protection.
func TestMaliciousArchive_PathTraversal(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "path-traversal.tar.gz")

	err = generator.GeneratePathTraversalArchive(outputPath)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_ZipBomb tests zip bomb protection.
func TestMaliciousArchive_ZipBomb(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "zip-bomb.zip")

	err = generator.GenerateZipBomb(outputPath)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_FileCountBomb tests file count limits.
func TestMaliciousArchive_FileCountBomb(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "file-count-bomb.tar.gz")

	err = generator.GenerateFileCountBomb(outputPath, 1000)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_SymlinkBomb tests symlink protection.
func TestMaliciousArchive_SymlinkBomb(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "symlink-bomb.tar.gz")

	err = generator.GenerateSymlinkBomb(outputPath)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_Malformed tests malformed archive handling.
func TestMaliciousArchive_Malformed(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "malformed.tar.gz")

	err = generator.GenerateMalformedArchive(outputPath)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_Nested tests deeply nested structures.
func TestMaliciousArchive_Nested(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "nested.tar.gz")

	err = generator.GenerateNestedArchive(outputPath, 10)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestMaliciousArchive_LargeFile tests large file handling.
func TestMaliciousArchive_LargeFile(t *testing.T) {
	generator, err := NewMaliciousArchiveGenerator()
	require.NoError(t, err)
	defer generator.Close()

	outputPath := filepath.Join(generator.tempDir, "large-file.tar.gz")

	// Claim 1GB file
	err = generator.GenerateLargeFileArchive(outputPath, 1024*1024*1024)
	require.NoError(t, err)

	// Verify file was created
	info, err := os.Stat(outputPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}
