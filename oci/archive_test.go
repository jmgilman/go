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
	"testing"

	"github.com/jmgilman/go/fs/billy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchiverInterface verifies the Archiver interface is properly defined
func TestArchiverInterface(t *testing.T) {
	// Test that Archiver interface exists and has the expected methods
	var _ Archiver = (*mockArchiver)(nil)
}

// TestExtractOptionsStruct verifies the ExtractOptions struct exists
func TestExtractOptionsStruct(t *testing.T) {
	// Test that ExtractOptions struct exists
	opts := ExtractOptions{}
	_ = opts
}

// TestDefaultExtractOptions verifies the default security options are properly configured
func TestDefaultExtractOptions(t *testing.T) {
	// Test that DefaultExtractOptions has the expected security defaults
	expectedMaxFiles := 10000
	expectedMaxSize := int64(1 * 1024 * 1024 * 1024) // 1GB
	expectedMaxFileSize := int64(100 * 1024 * 1024)  // 100MB
	expectedPreservePerms := false

	if DefaultExtractOptions.MaxFiles != expectedMaxFiles {
		t.Errorf("DefaultExtractOptions.MaxFiles = %d, want %d", DefaultExtractOptions.MaxFiles, expectedMaxFiles)
	}

	if DefaultExtractOptions.MaxSize != expectedMaxSize {
		t.Errorf("DefaultExtractOptions.MaxSize = %d, want %d", DefaultExtractOptions.MaxSize, expectedMaxSize)
	}

	if DefaultExtractOptions.MaxFileSize != expectedMaxFileSize {
		t.Errorf(
			"DefaultExtractOptions.MaxFileSize = %d, want %d",
			DefaultExtractOptions.MaxFileSize,
			expectedMaxFileSize,
		)
	}

	if DefaultExtractOptions.PreservePerms != expectedPreservePerms {
		t.Errorf(
			"DefaultExtractOptions.PreservePerms = %t, want %t",
			DefaultExtractOptions.PreservePerms,
			expectedPreservePerms,
		)
	}

	if DefaultExtractOptions.StripPrefix != "" {
		t.Errorf("DefaultExtractOptions.StripPrefix = %q, want empty string", DefaultExtractOptions.StripPrefix)
	}
}

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
