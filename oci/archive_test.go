package ocibundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmgilman/go/fs/billy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTarGzArchiver_BasicArchive tests basic archive creation functionality
func TestTarGzArchiver_BasicArchive(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create test files
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("Hello World"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "nested.txt"), []byte("Nested content"), 0o644))

	archiver := NewTarGzArchiver()
	var buf bytes.Buffer

	err := archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)
	assert.Greater(t, buf.Len(), 0, "Archive should contain data")
}

// TestTarGzArchiver_BasicExtract tests basic extraction functionality
func TestTarGzArchiver_BasicExtract(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	targetDir := filepath.Join(tempDir, "target")

	// Create test files to archive
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("Hello World"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "nested.txt"), []byte("Nested content"), 0o644))

	// Archive the files
	archiver := NewTarGzArchiver()
	var archiveBuf bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &archiveBuf)
	require.NoError(t, err)

	// Extract the archive
	err = archiver.Extract(context.Background(), &archiveBuf, targetDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Verify extracted files
	content, err := os.ReadFile(filepath.Join(targetDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello World", string(content))

	nestedContent, err := os.ReadFile(filepath.Join(targetDir, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Nested content", string(nestedContent))
}

// TestTarGzArchiver_DebugRoundTrip is a debug test to understand round-trip issues
func TestTarGzArchiver_DebugRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	intermediateDir := filepath.Join(tempDir, "intermediate")

	// Create simple test file
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("Hello World"), 0o644))

	archiver := NewTarGzArchiver()

	// First archive
	var archive1 bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &archive1)
	require.NoError(t, err)
	t.Logf("First archive size: %d bytes", archive1.Len())
	assert.Greater(t, archive1.Len(), 0, "First archive should contain data")

	// Extract to intermediate directory
	err = archiver.Extract(context.Background(), &archive1, intermediateDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Check what was extracted
	entries, err := os.ReadDir(intermediateDir)
	require.NoError(t, err)
	t.Logf("Extracted entries: %d", len(entries))
	for _, entry := range entries {
		t.Logf("  - %s (dir: %v)", entry.Name(), entry.IsDir())
		if !entry.IsDir() {
			content, readErr := os.ReadFile(filepath.Join(intermediateDir, entry.Name()))
			if readErr == nil {
				t.Logf("    content: %q", string(content))
			}
		}
	}

	// Second archive from extracted files
	var archive2 bytes.Buffer
	err = archiver.Archive(context.Background(), intermediateDir, &archive2)
	require.NoError(t, err)
	t.Logf("Second archive size: %d bytes", archive2.Len())
	assert.Greater(t, archive2.Len(), 0, "Second archive should contain data")
}

// TestTarGzArchiver_RoundTrip tests archive -> extract -> archive consistency
func TestTarGzArchiver_RoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	intermediateDir := filepath.Join(tempDir, "intermediate")

	// Create complex test structure
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "dir1", "subdir"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "dir2"), 0o755))

	testFiles := map[string]string{
		"root.txt":              "Root file content",
		"dir1/file1.txt":        "File 1 content",
		"dir1/subdir/file2.txt": "File 2 content",
		"dir2/file3.txt":        "File 3 content",
		"dir2/empty.txt":        "",
		filepath.Join("dir2", "special-chars_!@#$%^&()"): "Special chars content",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	archiver := NewTarGzArchiver()

	// First archive
	var archive1 bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &archive1)
	require.NoError(t, err)
	originalSize := archive1.Len()

	// Extract to intermediate directory
	err = archiver.Extract(context.Background(), &archive1, intermediateDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Second archive from extracted files
	var archive2 bytes.Buffer
	err = archiver.Archive(context.Background(), intermediateDir, &archive2)
	require.NoError(t, err)

	// For round-trip testing, the key is that we can extract and get the same content,
	// not necessarily the same archive size (due to metadata differences)
	assert.Greater(t, archive2.Len(), 0, "Second archive should contain data")

	// Extract second archive and compare content
	finalDir := filepath.Join(tempDir, "final")
	err = archiver.Extract(context.Background(), &archive2, finalDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Collect all files from final directory
	finalFiles := make(map[string]string)
	err = filepath.Walk(finalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(finalDir, path)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			finalFiles[relPath] = string(content)
		}
		return nil
	})
	require.NoError(t, err)

	// Verify all original files are present with correct content
	for path, expectedContent := range testFiles {
		actualContent, exists := finalFiles[path]
		assert.True(t, exists, "File %s should exist in final archive", path)
		if exists {
			assert.Equal(t, expectedContent, actualContent, "Content of %s should match", path)
		}
	}

	// Verify we have the same number of files
	assert.Equal(t, len(testFiles), len(finalFiles), "Should have same number of files after round-trip")

	t.Logf("Round-trip successful: original=%d bytes, roundtrip=%d bytes", originalSize, archive2.Len())
}

// TestTarGzArchiver_MediaType tests the media type method
func TestTarGzArchiver_MediaType(t *testing.T) {
	archiver := NewTarGzArchiver()
	expected := "application/vnd.oci.image.layer.v1.tar+gzip"
	assert.Equal(t, expected, archiver.MediaType())
}

// TestTarGzArchiver_WithSecurityValidators tests integration with security validators
func TestTarGzArchiver_WithSecurityValidators(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create a file with path traversal in the name (this will be caught during extraction)
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))

	// Create a file with .. in the name within the source directory
	// This simulates a malicious archive that contains path traversal
	evilPath := filepath.Join(sourceDir, "evil.txt")
	require.NoError(t, os.WriteFile(evilPath, []byte("evil"), 0o644))

	archiver := NewTarGzArchiver()
	var buf bytes.Buffer

	// Archive the normal file
	err := archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)

	// Now manually create a tar.gz with path traversal to test validation
	// This simulates what a malicious archive might contain
	var maliciousBuf bytes.Buffer
	createMaliciousArchive(t, &maliciousBuf)

	// Extract should fail due to path traversal in the malicious archive
	targetDir := filepath.Join(tempDir, "target")
	opts := DefaultExtractOptions

	err = archiver.Extract(context.Background(), &maliciousBuf, targetDir, opts)
	assert.Error(t, err, "Extraction should fail due to path traversal")
	assert.Contains(t, err.Error(), "security", "Error should be security-related")
}

// createMaliciousArchive creates a tar.gz archive with path traversal for testing
func createMaliciousArchive(t *testing.T, output io.Writer) {
	gzipWriter := gzip.NewWriter(output)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Create a header with path traversal
	header := &tar.Header{
		Name: "../evil.txt",
		Mode: 0o644,
		Size: int64(len("evil content")),
	}

	err := tarWriter.WriteHeader(header)
	require.NoError(t, err)

	_, err = tarWriter.Write([]byte("evil content"))
	require.NoError(t, err)
}

// TestTarGzArchiver_LargeFile tests handling of large files
func TestTarGzArchiver_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create a 10MB test file with deterministic content
	largeSize := 10 * 1024 * 1024 // 10MB
	largeData := make([]byte, largeSize)

	// Fill with deterministic pattern instead of random data
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "large.dat"), largeData, 0o644))

	archiver := NewTarGzArchiver()

	// Archive the large file
	var archiveBuf bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &archiveBuf)
	require.NoError(t, err)

	// Extract the archive
	targetDir := filepath.Join(tempDir, "target")
	err = archiver.Extract(context.Background(), &archiveBuf, targetDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Verify the large file was extracted correctly
	extractedData, err := os.ReadFile(filepath.Join(targetDir, "large.dat"))
	require.NoError(t, err)
	assert.Equal(t, largeData, extractedData, "Large file content should match")
}

// TestTarGzArchiver_EmptyDirectory tests archiving empty directories
func TestTarGzArchiver_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create empty directory structure
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "empty1", "nested"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "empty2"), 0o755))

	archiver := NewTarGzArchiver()
	var buf bytes.Buffer

	err := archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)
	assert.Greater(t, buf.Len(), 0, "Archive should contain directory entries")

	// Extract and verify directory structure
	targetDir := filepath.Join(tempDir, "target")
	err = archiver.Extract(context.Background(), &buf, targetDir, DefaultExtractOptions)
	require.NoError(t, err)

	// Check that directories exist
	assert.DirExists(t, filepath.Join(targetDir, "empty1"))
	assert.DirExists(t, filepath.Join(targetDir, "empty1", "nested"))
	assert.DirExists(t, filepath.Join(targetDir, "empty2"))
}

// TestTarGzArchiver_ErrorHandling tests various error conditions
func TestTarGzArchiver_ErrorHandling(t *testing.T) {
	archiver := NewTarGzArchiver()

	// Test non-existent source directory
	var buf bytes.Buffer
	err := archiver.Archive(context.Background(), "/non/existent/path", &buf)
	assert.Error(t, err, "Should error on non-existent source directory")

	// Test nil writer
	err = archiver.Archive(context.Background(), ".", nil)
	assert.Error(t, err, "Should error on nil writer")

	// Test nil reader for extraction
	targetDir := t.TempDir()
	err = archiver.Extract(context.Background(), nil, targetDir, DefaultExtractOptions)
	assert.Error(t, err, "Should error on nil reader")

	// Test non-existent target directory (should create it)
	tempDir := t.TempDir()
	nonExistentTarget := filepath.Join(tempDir, "deep", "nested", "path")

	var archiveBuf bytes.Buffer
	// Create a minimal archive
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0o644))
	require.NoError(t, archiver.Archive(context.Background(), tempDir, &archiveBuf))

	err = archiver.Extract(context.Background(), &archiveBuf, nonExistentTarget, DefaultExtractOptions)
	assert.NoError(t, err, "Should create target directory if it doesn't exist")

	_, err = os.Stat(nonExistentTarget)
	assert.NoError(t, err, "Target directory should be created")
}

// TestTarGzArchiver_Compatibility tests that created archives are valid tar.gz format
func TestTarGzArchiver_Compatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping compatibility test in short mode")
	}

	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create test files with various content
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "hello.txt"), []byte("Hello World"), 0o644))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(sourceDir, "subdir", "nested.txt"), []byte("Nested file content"), 0o644),
	)

	archiver := NewTarGzArchiver()
	var archiveBuf bytes.Buffer

	err := archiver.Archive(context.Background(), sourceDir, &archiveBuf)
	require.NoError(t, err)

	// Verify the archive is a valid tar.gz file by reading it back with Go's standard library
	// This ensures the format is compatible without requiring external tools
	gzipReader, err := gzip.NewReader(&archiveBuf)
	require.NoError(t, err)
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	// Extract files and verify content
	extractedFiles := make(map[string]string)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Read file content
		content, err := io.ReadAll(tarReader)
		require.NoError(t, err)

		extractedFiles[header.Name] = string(content)
	}

	// Verify extracted files match expected content
	expectedFiles := map[string]string{
		"hello.txt":         "Hello World",
		"subdir/nested.txt": "Nested file content",
	}

	for filename, expectedContent := range expectedFiles {
		actualContent, exists := extractedFiles[filename]
		assert.True(t, exists, "File %s should exist in archive", filename)
		if exists {
			assert.Equal(t, expectedContent, actualContent, "Content of %s should match", filename)
		}
	}

	t.Log("Archive format validated as proper tar.gz")
}

// TestTarGzArchiver_SecurityLimits tests security limit enforcement
func TestTarGzArchiver_SecurityLimits(t *testing.T) {
	tempDir := t.TempDir()

	// Create a large file that exceeds limits
	largeSize := int64(200 * 1024 * 1024) // 200MB (exceeds 100MB default limit)
	largeData := make([]byte, largeSize)
	_, err := rand.Read(largeData)
	require.NoError(t, err)

	sourceDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "large.dat"), largeData, 0o644))

	archiver := NewTarGzArchiver()
	var buf bytes.Buffer

	// Archive should succeed
	err = archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)

	// Extract with strict limits should fail
	targetDir := filepath.Join(tempDir, "target")
	opts := ExtractOptions{
		MaxFiles:    1000,
		MaxSize:     50 * 1024 * 1024, // 50MB limit
		MaxFileSize: 10 * 1024 * 1024, // 10MB per file limit
	}

	err = archiver.Extract(context.Background(), &buf, targetDir, opts)
	assert.Error(t, err, "Should fail due to size limits")
	assert.Contains(t, err.Error(), "security", "Error should be security-related")
}

// TestTarGzArchiver_MemFS_RoundTrip tests archive/extract using an in-memory filesystem
func TestTarGzArchiver_MemFS_RoundTrip(t *testing.T) {
	tempFS := billy.NewMemory()

	// Build source tree in memfs
	require.NoError(t, tempFS.MkdirAll("/src/sub", 0o755))
	require.NoError(t, tempFS.WriteFile("/src/a.txt", []byte("A"), 0o644))
	require.NoError(t, tempFS.WriteFile("/src/sub/b.txt", []byte("B"), 0o644))

	archiver := NewTarGzArchiverWithFS(tempFS)
	var buf bytes.Buffer

	require.NoError(t, archiver.Archive(context.Background(), "/src", &buf))
	assert.Greater(t, buf.Len(), 0)

	// Extract into target directory within the same memfs
	require.NoError(t, archiver.Extract(context.Background(), &buf, "/dst", DefaultExtractOptions))

	ab, err := tempFS.ReadFile("/dst/a.txt")
	require.NoError(t, err)
	assert.Equal(t, "A", string(ab))

	bb, err := tempFS.ReadFile("/dst/sub/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "B", string(bb))
}

// mockArchiver implements Archiver interface for testing
type mockArchiver struct{}

func (m *mockArchiver) Archive(ctx context.Context, sourceDir string, output io.Writer) error {
	return nil
}

func (m *mockArchiver) ArchiveWithProgress(
	ctx context.Context,
	sourceDir string,
	output io.Writer,
	progress func(current, total int64),
) error {
	return nil
}

func (m *mockArchiver) Extract(ctx context.Context, input io.Reader, targetDir string, opts ExtractOptions) error {
	return nil
}

func (m *mockArchiver) MediaType() string {
	return "application/octet-stream"
}

// TestTarGzArchiver_EstargzFormat verifies that archives are created in eStargz format
// with the expected metadata files (TOC and landmark)
func TestTarGzArchiver_EstargzFormat(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")

	// Create test files
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("Hello World"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "file2.txt"), []byte("Nested"), 0o644))

	// Create archive
	archiver := NewTarGzArchiver()
	var buf bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)

	t.Logf("Archive size: %d bytes", buf.Len())

	// Verify archive contains eStargz metadata files
	gzReader, err := gzip.NewReader(&buf)
	require.NoError(t, err)
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	foundLandmark := false
	foundTOC := false
	foundFile1 := false
	foundFile2 := false

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		t.Logf("Found entry: %s (size: %d)", header.Name, header.Size)

		switch header.Name {
		case ".no.prefetch.landmark":
			foundLandmark = true
			t.Log("  ✓ Found eStargz landmark file")
		case "stargz.index.json":
			foundTOC = true
			t.Log("  ✓ Found eStargz TOC (Table of Contents)")
			// Verify TOC has content
			assert.Greater(t, header.Size, int64(0), "TOC should have content")
		case "file1.txt":
			foundFile1 = true
		case "subdir/file2.txt":
			foundFile2 = true
		}
	}

	// Verify eStargz-specific files are present
	assert.True(t, foundLandmark, "Archive should contain .no.prefetch.landmark")
	assert.True(t, foundTOC, "Archive should contain stargz.index.json (TOC)")

	// Verify actual content files are present
	assert.True(t, foundFile1, "Archive should contain file1.txt")
	assert.True(t, foundFile2, "Archive should contain subdir/file2.txt")

	t.Log("✓ Archive is in eStargz format with TOC")
}

// TestMatchesPattern tests the glob pattern matching functionality
func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		// Simple wildcard patterns
		{"exact match", "config.json", "config.json", true},
		{"wildcard extension", "app.json", "*.json", true},
		{"wildcard extension no match", "app.txt", "*.json", false},
		{"wildcard prefix", "test-file.txt", "test-*.txt", true},
		{"wildcard middle", "app-v1-final.zip", "app-*-final.zip", true},

		// Directory patterns
		{"directory exact", "config/app.json", "config/app.json", true},
		{"directory wildcard", "config/app.json", "config/*.json", true},
		{"directory wildcard no match", "data/app.json", "config/*.json", false},
		{"nested directory", "a/b/c.txt", "a/b/c.txt", true},

		// Recursive patterns (**)
		{"recursive simple", "data/file.txt", "data/**/*.txt", true},
		{"recursive nested", "data/sub/file.txt", "data/**/*.txt", true},
		{"recursive deep", "data/a/b/c/file.txt", "data/**/*.txt", true},
		{"recursive root", "data/file.txt", "**/*.txt", true},
		{"recursive no prefix", "any/path/file.txt", "**/*.txt", true},
		{"recursive with prefix", "src/main/java/App.java", "src/**/*.java", true},
		{"recursive wrong extension", "src/main/java/App.kt", "src/**/*.java", false},

		// Edge cases
		{"empty pattern", "any/file.txt", "", false},
		{"root file", "file.txt", "*.txt", true},
		{"dot file", ".gitignore", ".gitignore", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.path, tt.pattern)
			assert.Equal(t, tt.expected, result,
				"matchesPattern(%q, %q) = %v, want %v",
				tt.path, tt.pattern, result, tt.expected)
		})
	}
}

// TestMatchesAnyPattern tests matching against multiple patterns
func TestMatchesAnyPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		expected bool
	}{
		{
			name:     "empty patterns matches all",
			path:     "any/file.txt",
			patterns: []string{},
			expected: true,
		},
		{
			name:     "nil patterns matches all",
			path:     "any/file.txt",
			patterns: nil,
			expected: true,
		},
		{
			name:     "matches first pattern",
			path:     "config.json",
			patterns: []string{"*.json", "*.txt"},
			expected: true,
		},
		{
			name:     "matches second pattern",
			path:     "readme.txt",
			patterns: []string{"*.json", "*.txt"},
			expected: true,
		},
		{
			name:     "no match",
			path:     "app.py",
			patterns: []string{"*.json", "*.txt"},
			expected: false,
		},
		{
			name:     "multiple directories",
			path:     "src/main.go",
			patterns: []string{"test/*", "src/*"},
			expected: true,
		},
		{
			name:     "recursive pattern",
			path:     "src/cmd/app/main.go",
			patterns: []string{"**/*.go"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesAnyPattern(tt.path, tt.patterns)
			assert.Equal(t, tt.expected, result,
				"matchesAnyPattern(%q, %v) = %v, want %v",
				tt.path, tt.patterns, result, tt.expected)
		})
	}
}

// TestSelectiveExtraction tests extracting only files matching patterns
func TestSelectiveExtraction(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	targetDir := filepath.Join(tempDir, "target")

	// Create test files with various paths
	testFiles := map[string]string{
		"config.json":         `{"app":"test"}`,
		"readme.txt":          "README content",
		"data/file1.json":     `{"data":1}`,
		"data/file2.txt":      "Data 2",
		"data/sub/file3.json": `{"data":3}`,
		"src/main.go":         "package main",
		"src/util/helper.go":  "package util",
		"test/main_test.go":   "package main",
	}

	// Create source files
	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create archive
	archiver := NewTarGzArchiver()
	var buf bytes.Buffer
	err := archiver.Archive(context.Background(), sourceDir, &buf)
	require.NoError(t, err)

	t.Logf("Created archive with %d files, size: %d bytes", len(testFiles), buf.Len())

	// Test selective extraction with patterns
	extractOpts := ExtractOptions{
		MaxFiles:       100,
		MaxSize:        10 * 1024 * 1024,
		MaxFileSize:    1 * 1024 * 1024,
		FilesToExtract: []string{"**/*.json", "**/*.go"},
	}

	err = archiver.Extract(context.Background(), &buf, targetDir, extractOpts)
	require.NoError(t, err)

	// Verify only matching files were extracted
	expectedFiles := []string{
		"config.json",
		"data/file1.json",
		"data/sub/file3.json",
		"src/main.go",
		"src/util/helper.go",
		"test/main_test.go",
	}

	unexpectedFiles := []string{
		"readme.txt",
		"data/file2.txt",
	}

	// Check expected files exist
	for _, file := range expectedFiles {
		fullPath := filepath.Join(targetDir, file)
		assert.FileExists(t, fullPath, "Expected file %s should be extracted", file)
	}

	// Check unexpected files don't exist
	for _, file := range unexpectedFiles {
		fullPath := filepath.Join(targetDir, file)
		assert.NoFileExists(t, fullPath, "Unexpected file %s should not be extracted", file)
	}

	t.Logf("✓ Selective extraction worked correctly")
}

// TestTarGzArchiver_BackwardCompatibility tests that plain tar.gz archives
// (created without eStargz) can still be extracted
func TestTarGzArchiver_BackwardCompatibility(t *testing.T) {
	// Create a plain tar.gz archive (not eStargz) manually
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"file1.txt":      "content1",
		"dir/file2.txt":  "content2",
		"dir/file3.json": `{"test":true}`,
	}

	// Create test files
	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create a plain tar.gz (not eStargz) using standard library
	archivePath := filepath.Join(t.TempDir(), "plain.tar.gz")
	archiveFile, err := os.Create(archivePath)
	require.NoError(t, err)
	defer archiveFile.Close()

	gzWriter := gzip.NewWriter(archiveFile)
	tarWriter := tar.NewWriter(gzWriter)

	// Walk source directory and add files
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // Skip directories in plain tar.gz
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Create tar header
		header := &tar.Header{
			Name: relPath,
			Mode: 0o644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(content); err != nil {
			return err
		}

		return nil
	})
	require.NoError(t, err)

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzWriter.Close())
	archiveFile.Close()

	// Test 1: Extract plain tar.gz with full extraction
	t.Run("full extraction from plain tar.gz", func(t *testing.T) {
		targetDir := t.TempDir()
		archiver := NewTarGzArchiver()

		archiveReader, err := os.Open(archivePath)
		require.NoError(t, err)
		defer archiveReader.Close()

		err = archiver.Extract(context.Background(), archiveReader, targetDir, DefaultExtractOptions)
		assert.NoError(t, err, "Should extract plain tar.gz successfully")

		// Verify all files were extracted
		for path, expectedContent := range testFiles {
			fullPath := filepath.Join(targetDir, path)
			assert.FileExists(t, fullPath)
			content, err := os.ReadFile(fullPath)
			assert.NoError(t, err)
			assert.Equal(t, expectedContent, string(content))
		}
	})

	// Test 2: Extract plain tar.gz with selective extraction
	t.Run("selective extraction from plain tar.gz", func(t *testing.T) {
		targetDir := t.TempDir()
		archiver := NewTarGzArchiver()

		archiveReader, err := os.Open(archivePath)
		require.NoError(t, err)
		defer archiveReader.Close()

		opts := DefaultExtractOptions
		opts.FilesToExtract = []string{"**/*.json"}

		err = archiver.Extract(context.Background(), archiveReader, targetDir, opts)
		assert.NoError(t, err, "Should extract plain tar.gz with patterns successfully")

		// Verify only JSON files were extracted
		assert.FileExists(t, filepath.Join(targetDir, "dir/file3.json"))
		assert.NoFileExists(t, filepath.Join(targetDir, "file1.txt"))
		assert.NoFileExists(t, filepath.Join(targetDir, "dir/file2.txt"))
	})

	t.Logf("✓ Backward compatibility verified")
}

// TestSelectiveExtraction_SecurityValidation tests that security validators
// are still enforced during selective extraction
func TestSelectiveExtraction_SecurityValidation(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string // files to create
		patterns      []string          // selective extraction patterns
		opts          ExtractOptions    // extraction options with limits
		shouldFail    bool              // whether extraction should fail
		errorContains string            // expected error substring
	}{
		{
			name: "size limit enforced with selective extraction",
			files: map[string]string{
				"file1.json": strings.Repeat("x", 500), // 500 bytes
				"file2.json": strings.Repeat("y", 600), // 600 bytes
			},
			patterns: []string{"**/*.json"},
			opts: ExtractOptions{
				MaxSize:     1000, // Total limit: 1000 bytes
				MaxFileSize: 1000,
				MaxFiles:    100,
			},
			shouldFail:    true,
			errorContains: "validation failed",
		},
		{
			name: "file count limit enforced with selective extraction",
			files: map[string]string{
				"file1.json": "data1",
				"file2.json": "data2",
				"file3.json": "data3",
			},
			patterns: []string{"**/*.json"},
			opts: ExtractOptions{
				MaxSize:     10000,
				MaxFileSize: 1000,
				MaxFiles:    2, // Limit to 2 files (but we have 3)
			},
			shouldFail:    true,
			errorContains: "security constraint",
		},
		{
			name: "selective extraction respects security with safe files",
			files: map[string]string{
				"config.json": `{"config":true}`,
				"data.json":   `{"data":true}`,
				"readme.txt":  "not extracted",
			},
			patterns: []string{"**/*.json"},
			opts: ExtractOptions{
				MaxSize:     10000,
				MaxFileSize: 1000,
				MaxFiles:    10,
			},
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create source directory with test files
			sourceDir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(sourceDir, path)
				require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
				require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
			}

			// Create the archive
			archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
			archiver := NewTarGzArchiver()

			archiveFile, err := os.Create(archivePath)
			require.NoError(t, err)
			defer archiveFile.Close()

			err = archiver.Archive(context.Background(), sourceDir, archiveFile)
			require.NoError(t, err)

			// Extract with selective patterns
			targetDir := t.TempDir()
			tt.opts.FilesToExtract = tt.patterns

			archiveReader, err := os.Open(archivePath)
			require.NoError(t, err)
			defer archiveReader.Close()

			err = archiver.Extract(context.Background(), archiveReader, targetDir, tt.opts)

			if tt.shouldFail {
				assert.Error(t, err, "Should fail due to security constraint")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err, "Should succeed with safe files")

				// Verify only matching files were extracted
				for path := range tt.files {
					fullPath := filepath.Join(targetDir, path)
					matched := false
					for _, pattern := range tt.patterns {
						if match, _ := filepath.Match(pattern, path); match {
							matched = true
							break
						}
						// Check recursive patterns
						if strings.Contains(pattern, "**") {
							if matchesPattern(path, pattern) {
								matched = true
								break
							}
						}
					}

					if matched {
						assert.FileExists(t, fullPath, "Matched file should exist")
					} else {
						assert.NoFileExists(t, fullPath, "Unmatched file should not exist")
					}
				}
			}
		})
	}
}
