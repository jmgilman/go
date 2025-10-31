package ocibundle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertTOCToListResult tests conversion of TOC entries to list result.
func TestConvertTOCToListResult(t *testing.T) {
	// Create a test archive with known files
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"file1.txt":      "content1",
		"dir/file2.txt":  "content2",
		"dir/file3.json": `{"test":true}`,
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create an eStargz archive
	archiveBuffer := &bytes.Buffer{}
	archiver := NewTarGzArchiver()
	err := archiver.Archive(context.Background(), sourceDir, archiveBuffer)
	require.NoError(t, err)

	// Parse the TOC from the archive
	archiveBytes := archiveBuffer.Bytes()
	tocEntries, err := parseTOCFromBytes(archiveBytes)
	require.NoError(t, err)

	// Convert to list result
	result := convertTOCToListResult(tocEntries)

	// Verify results
	assert.NotNil(t, result)
	assert.Greater(t, len(result.Files), 0, "Should have files")
	assert.Greater(t, result.FileCount, 0, "Should have counted files")
	assert.Greater(t, result.TotalSize, int64(0), "Should have total size")

	// Check that we found our files
	fileNames := make(map[string]bool)
	for _, file := range result.Files {
		fileNames[file.Name] = true

		// Verify metadata fields are populated
		if !file.IsDir {
			assert.NotEmpty(t, file.Name)
			assert.Greater(t, file.Size, int64(0), "File %s should have size > 0", file.Name)
		}
	}

	// Verify our test files are in the result
	assert.True(t, fileNames["file1.txt"], "Should find file1.txt")
	assert.True(t, fileNames["dir"], "Should find dir")
	assert.True(t, fileNames["dir/file2.txt"], "Should find dir/file2.txt")
	assert.True(t, fileNames["dir/file3.json"], "Should find dir/file3.json")
}

// TestListFilesWithFilter tests filtering files by patterns.
func TestListFilesWithFilter(t *testing.T) {
	// Create test files
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"config.json":        `{"app":"test"}`,
		"data/file1.json":    `{"data":1}`,
		"data/file2.txt":     "text content",
		"src/main.go":        "package main",
		"src/util/helper.go": "package util",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create an eStargz archive
	archiveBuffer := &bytes.Buffer{}
	archiver := NewTarGzArchiver()
	err := archiver.Archive(context.Background(), sourceDir, archiveBuffer)
	require.NoError(t, err)

	// Parse the TOC
	archiveBytes := archiveBuffer.Bytes()
	tocEntries, err := parseTOCFromBytes(archiveBytes)
	require.NoError(t, err)

	// Convert to list result (this is what ListFiles would return)
	allFiles := convertTOCToListResult(tocEntries)

	t.Run("filter JSON files", func(t *testing.T) {
		// Manually filter (simulating ListFilesWithFilter logic)
		filtered := &ListFilesResult{
			Files: make([]FileMetadata, 0),
		}

		for _, file := range allFiles.Files {
			if !file.IsDir && matchesAnyPattern(file.Name, []string{"**/*.json"}) {
				filtered.Files = append(filtered.Files, file)
				filtered.FileCount++
				filtered.TotalSize += file.Size
			}
		}

		// Verify we only got JSON files
		assert.Equal(t, 2, filtered.FileCount, "Should have 2 JSON files")
		for _, file := range filtered.Files {
			assert.Contains(t, file.Name, ".json", "File should be a .json file")
		}
	})

	t.Run("filter Go files", func(t *testing.T) {
		// Manually filter
		filtered := &ListFilesResult{
			Files: make([]FileMetadata, 0),
		}

		for _, file := range allFiles.Files {
			if !file.IsDir && matchesAnyPattern(file.Name, []string{"**/*.go"}) {
				filtered.Files = append(filtered.Files, file)
				filtered.FileCount++
				filtered.TotalSize += file.Size
			}
		}

		// Verify we only got Go files
		assert.Equal(t, 2, filtered.FileCount, "Should have 2 Go files")
		for _, file := range filtered.Files {
			assert.Contains(t, file.Name, ".go", "File should be a .go file")
		}
	})
}

// TestParseTOCFromBytes tests parsing TOC from bytes.
func TestParseTOCFromBytes(t *testing.T) {
	// Create a simple test archive
	sourceDir := t.TempDir()
	testFile := filepath.Join(sourceDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello"), 0o644))

	// Create archive
	archiveBuffer := &bytes.Buffer{}
	archiver := NewTarGzArchiver()
	err := archiver.Archive(context.Background(), sourceDir, archiveBuffer)
	require.NoError(t, err)

	// Parse TOC
	archiveBytes := archiveBuffer.Bytes()
	entries, err := parseTOCFromBytes(archiveBytes)

	// Should succeed for valid eStargz archive
	require.NoError(t, err)
	assert.Greater(t, len(entries), 0, "Should have entries")

	// Check for our test file
	foundTestFile := false
	for _, entry := range entries {
		if entry.Name == "test.txt" {
			foundTestFile = true
			assert.Equal(t, "reg", entry.Type, "Should be a regular file")
			assert.Equal(t, int64(5), entry.Size, "Should have correct size")
		}
	}
	assert.True(t, foundTestFile, "Should find test.txt in TOC")
}

// TestParseTOCFromBytes_InvalidData tests error handling for invalid data.
func TestParseTOCFromBytes_InvalidData(t *testing.T) {
	// Test with invalid data
	invalidData := []byte("not a valid estargz archive")
	_, err := parseTOCFromBytes(invalidData)
	assert.Error(t, err, "Should error on invalid data")
}

// TestCollectEntries tests collecting entries from TOC tree.
func TestCollectEntries(t *testing.T) {
	// Create a test archive with nested structure
	sourceDir := t.TempDir()
	files := []string{
		"a.txt",
		"dir1/b.txt",
		"dir1/dir2/c.txt",
	}

	for _, file := range files {
		fullPath := filepath.Join(sourceDir, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte("content"), 0o644))
	}

	// Create archive and parse TOC
	archiveBuffer := &bytes.Buffer{}
	archiver := NewTarGzArchiver()
	err := archiver.Archive(context.Background(), sourceDir, archiveBuffer)
	require.NoError(t, err)

	archiveBytes := archiveBuffer.Bytes()

	stargzReader, err := parseTOCFromBytes(archiveBytes)
	require.NoError(t, err)

	// Verify we collected all entries
	assert.Greater(t, len(stargzReader), 0, "Should have collected entries")

	// Check that we have both files and directories
	hasFiles := false
	hasDirs := false
	for _, entry := range stargzReader {
		if entry.Type == "reg" {
			hasFiles = true
		}
		if entry.Type == "dir" {
			hasDirs = true
		}
	}
	assert.True(t, hasFiles, "Should have regular files")
	assert.True(t, hasDirs, "Should have directories")
}

// TestFileMetadata tests the FileMetadata structure.
func TestFileMetadata(t *testing.T) {
	// Create a FileMetadata instance
	metadata := FileMetadata{
		Name:       "test.txt",
		Size:       100,
		Mode:       0o644,
		IsDir:      false,
		LinkTarget: "",
		Type:       "reg",
	}

	assert.Equal(t, "test.txt", metadata.Name)
	assert.Equal(t, int64(100), metadata.Size)
	assert.Equal(t, os.FileMode(0o644), metadata.Mode)
	assert.False(t, metadata.IsDir)
	assert.Equal(t, "reg", metadata.Type)
}

// TestListFilesResult tests the ListFilesResult structure.
func TestListFilesResult(t *testing.T) {
	result := &ListFilesResult{
		Files: []FileMetadata{
			{Name: "file1.txt", Size: 100, IsDir: false},
			{Name: "file2.txt", Size: 200, IsDir: false},
			{Name: "dir1", Size: 0, IsDir: true},
		},
		TotalSize: 300,
		FileCount: 2,
		DirCount:  1,
	}

	assert.Len(t, result.Files, 3)
	assert.Equal(t, int64(300), result.TotalSize)
	assert.Equal(t, 2, result.FileCount)
	assert.Equal(t, 1, result.DirCount)
}
