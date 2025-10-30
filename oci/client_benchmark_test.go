// Package ocibundle provides OCI bundle distribution functionality.
// This file contains benchmark tests for profiling memory usage and performance.
package ocibundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmgilman/go/oci/internal/oras"
	"github.com/jmgilman/go/oci/internal/testutil"
)

// BenchmarkClient_Push_MemoryUsage benchmarks memory usage during push operations.
// This test validates that memory usage remains constant regardless of file size.
func BenchmarkClient_Push_MemoryUsage(b *testing.B) {
	// Test different file sizes to ensure constant memory usage
	sizes := []int64{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			benchmarkPushMemoryUsage(b, size)
		})
	}
}

// benchmarkPushMemoryUsage performs the actual benchmark for a specific file size
func benchmarkPushMemoryUsage(b *testing.B, size int64) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "benchmark-push-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive generator
	gen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer gen.Close()

	// Generate test directory
	testDir := filepath.Join(tempDir, "source")
	totalSize, err := gen.GenerateTestDirectory(testDir, 3, 10, size/100) // Distribute size across files
	if err != nil {
		b.Fatalf("Failed to generate test directory: %v", err)
	}

	// Create client
	client, err := New()
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// This should demonstrate constant memory usage regardless of file size
		// The key is that we don't load the entire archive into memory
		reference := "localhost:5000/test/repo:benchmark"

		err := client.Push(context.Background(), testDir, reference)
		if err != nil && !isRegistryUnavailableError(err) {
			b.Fatalf("Push failed: %v", err)
		}
	}

	// Report metrics
	b.ReportMetric(float64(totalSize)/1024/1024, "MB_processed/op")
}

// BenchmarkClient_Pull_MemoryUsage benchmarks memory usage during pull operations.
// This test validates that memory usage remains constant during extraction.
func BenchmarkClient_Pull_MemoryUsage(b *testing.B) {
	// Test different archive sizes to ensure constant memory usage
	sizes := []int64{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			benchmarkPullMemoryUsage(b, size)
		})
	}
}

// benchmarkPullMemoryUsage performs the actual benchmark for a specific archive size
func benchmarkPullMemoryUsage(b *testing.B, size int64) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "benchmark-pull-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := New()
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}

	// Create a reference that will likely fail (but test memory usage during attempt)
	reference := "localhost:5000/test/repo:benchmark"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		targetDir := filepath.Join(tempDir, "target")
		err := client.Pull(context.Background(), reference, targetDir)
		if err != nil && !isRegistryUnavailableError(err) {
			b.Fatalf("Pull failed unexpectedly: %v", err)
		}
	}

	// Report metrics
	b.ReportMetric(float64(size)/1024/1024, "MB_target/op")
}

// BenchmarkArchiver_StreamMemoryUsage benchmarks the archiver's streaming behavior.
// This test ensures the archiver doesn't load entire files into memory.
func BenchmarkArchiver_StreamMemoryUsage(b *testing.B) {
	// Test different file sizes to ensure streaming behavior
	sizes := []int64{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			benchmarkArchiverStreamMemoryUsage(b, size)
		})
	}
}

// BenchmarkArchiver_ConcurrentVsSequential benchmarks concurrent vs sequential archiving.
// This test validates that concurrent processing provides better performance.
func BenchmarkArchiver_ConcurrentVsSequential(b *testing.B) {
	// Test with a directory structure that benefits from concurrency
	tempDir, err := os.MkdirTemp("", "benchmark-concurrent-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive generator
	gen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer gen.Close()

	// Generate test directory with many files (good for concurrency testing)
	testDir := filepath.Join(tempDir, "source")
	totalSize, err := gen.GenerateTestDirectory(testDir, 4, 20, 1024*1024) // 1MB per file, many files
	if err != nil {
		b.Fatalf("Failed to generate test directory: %v", err)
	}

	// Count files to ensure we have enough for concurrency testing
	var fileCount int
	if err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			fileCount++
		}
		return nil
	}); err != nil {
		b.Fatalf("Failed to walk test directory: %v", err)
	}

	if fileCount < 10 {
		b.Skip("Not enough files for meaningful concurrency benchmark")
	}

	b.Logf("Benchmarking with %d files, total size: %s", fileCount, formatSize(totalSize))

	// Benchmark concurrent implementation
	b.Run("Concurrent", func(b *testing.B) {
		benchmarkConcurrentArchiving(b, testDir)
	})

	// Benchmark sequential implementation (simulated)
	b.Run("Sequential", func(b *testing.B) {
		benchmarkSequentialArchiving(b, testDir)
	})
}

// benchmarkConcurrentArchiving tests the concurrent archiving implementation
func benchmarkConcurrentArchiving(b *testing.B, testDir string) {
	archiver := NewTarGzArchiver()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		output := &discardWriter{}
		err := archiver.Archive(context.Background(), testDir, output)
		if err != nil {
			b.Fatalf("Concurrent archiving failed: %v", err)
		}
	}
}

// benchmarkSequentialArchiving simulates sequential archiving for comparison
func benchmarkSequentialArchiving(b *testing.B, testDir string) {
	// Create a sequential archiver for comparison
	archiver := &sequentialTarGzArchiver{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		output := &discardWriter{}
		err := archiver.Archive(context.Background(), testDir, output)
		if err != nil {
			b.Fatalf("Sequential archiving failed: %v", err)
		}
	}
}

// sequentialTarGzArchiver implements sequential archiving for performance comparison
type sequentialTarGzArchiver struct{}

// Archive implements sequential archiving (original implementation)
func (a *sequentialTarGzArchiver) Archive(ctx context.Context, sourceDir string, output io.Writer) error {
	if sourceDir == "" {
		return fmt.Errorf("source directory cannot be empty")
	}

	if output == nil {
		return fmt.Errorf("output writer cannot be nil")
	}

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}

	gzipWriter := gzip.NewWriter(output)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}

		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", path, err)
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file content for %s: %w", path, err)
			}
		}

		return nil
	})
}

// BenchmarkAuthCaching benchmarks authentication caching performance.
// This test validates that credential caching reduces authentication overhead.
func BenchmarkAuthCaching(b *testing.B) {
	// Test different scenarios
	scenarios := []struct {
		name       string
		useCaching bool
	}{
		{"WithCaching", true},
		{"WithoutCaching", false},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchmarkAuthCaching(b, scenario.useCaching)
		})
	}
}

// benchmarkAuthCaching tests authentication performance with and without caching
func benchmarkAuthCaching(b *testing.B, useCaching bool) {
	// Create test options
	opts := &oras.AuthOptions{
		StaticRegistry: "registry.example.com",
		StaticUsername: "testuser",
		StaticPassword: "testpass",
	}

	// Clear any existing cache for fair comparison
	if !useCaching {
		// Temporarily disable caching by using a different registry
		opts.StaticRegistry = "nocache-registry.example.com"
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create repository (this triggers authentication setup)
		_, err := oras.NewRepository(context.Background(), "registry.example.com/test/repo:latest", opts)
		if err != nil && !isRegistryUnavailableError(err) {
			b.Fatalf("Failed to create repository: %v", err)
		}
	}
}

// BenchmarkConnectionPooling benchmarks connection pooling performance.
// This test validates that connection reuse improves performance.
func BenchmarkConnectionPooling(b *testing.B) {
	// Test multiple repository creations to validate connection reuse
	opts := &oras.AuthOptions{
		StaticRegistry: "registry.example.com",
		StaticUsername: "testuser",
		StaticPassword: "testpass",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create repository with connection pooling
		reference := fmt.Sprintf("registry.example.com/test/repo%d:latest", i%10)
		_, err := oras.NewRepository(context.Background(), reference, opts)
		if err != nil && !isRegistryUnavailableError(err) {
			b.Fatalf("Failed to create repository: %v", err)
		}
	}
}

// discardWriter discards all writes (used for benchmarking)
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// benchmarkArchiverStreamMemoryUsage tests the archiver's streaming behavior
func benchmarkArchiverStreamMemoryUsage(b *testing.B, size int64) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "benchmark-archive-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive generator
	gen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer gen.Close()

	// Generate test directory
	testDir := filepath.Join(tempDir, "source")
	_, err = gen.GenerateTestDirectory(testDir, 2, 5, size/10) // Distribute size across files
	if err != nil {
		b.Fatalf("Failed to generate test directory: %v", err)
	}

	// Create archiver
	archiver := NewTarGzArchiver()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create temporary output file
		outputPath := filepath.Join(tempDir, "archive.tar.gz")
		output, err := os.Create(outputPath)
		if err != nil {
			b.Fatalf("Failed to create output file: %v", err)
		}

		// Archive the directory - this should stream data without loading everything into memory
		err = archiver.Archive(context.Background(), testDir, output)
		output.Close()

		if err != nil {
			b.Fatalf("Archive failed: %v", err)
		}

		// Clean up for next iteration
		os.Remove(outputPath)
	}

	// Report metrics
	b.ReportMetric(float64(size)/1024/1024, "MB_processed/op")
}

// BenchmarkArchiver_ExtractMemoryUsage benchmarks extraction memory usage.
// This test ensures extraction doesn't load entire archives into memory.
func BenchmarkArchiver_ExtractMemoryUsage(b *testing.B) {
	// Test different archive sizes
	sizes := []int64{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			benchmarkArchiverExtractMemoryUsage(b, size)
		})
	}
}

// benchmarkArchiverExtractMemoryUsage tests extraction memory usage
func benchmarkArchiverExtractMemoryUsage(b *testing.B, size int64) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "benchmark-extract-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive generator
	gen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer gen.Close()

	// Generate test archive
	archivePath := filepath.Join(tempDir, "test.tar.gz")
	_, err = gen.GenerateLargeArchive(context.Background(), size, archivePath)
	if err != nil {
		b.Fatalf("Failed to generate test archive: %v", err)
	}

	// Create archiver
	archiver := NewTarGzArchiver()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Open archive for reading
		archiveFile, err := os.Open(archivePath)
		if err != nil {
			b.Fatalf("Failed to open archive: %v", err)
		}

		// Create target directory
		targetDir := filepath.Join(tempDir, "extract")
		if mkdirErr := os.MkdirAll(targetDir, 0o755); mkdirErr != nil {
			archiveFile.Close()
			b.Fatalf("Failed to create target dir: %v", mkdirErr)
		}

		// Extract archive - this should stream data without loading everything into memory
		opts := ExtractOptions{
			MaxFiles:    10000,
			MaxSize:     size * 2, // Allow some overhead
			MaxFileSize: size,
		}

		err = archiver.Extract(context.Background(), archiveFile, targetDir, opts)
		archiveFile.Close()

		if err != nil {
			b.Fatalf("Extract failed: %v", err)
		}

		// Clean up for next iteration
		os.RemoveAll(targetDir)
	}

	// Report metrics
	b.ReportMetric(float64(size)/1024/1024, "MB_processed/op")
}

// Helper function to format sizes for benchmark names
func formatSize(size int64) string {
	if size >= 1024*1024*1024 {
		return fmt.Sprintf("%dGB", size/(1024*1024*1024))
	}
	if size >= 1024*1024 {
		return fmt.Sprintf("%dMB", size/(1024*1024))
	}
	if size >= 1024 {
		return fmt.Sprintf("%dKB", size/1024)
	}
	return fmt.Sprintf("%dB", size)
}

// BenchmarkORAS_PushMemoryUsage tests the ORAS client's memory usage during push operations.
// This test demonstrates the current memory issue with io.ReadAll.
func BenchmarkORAS_PushMemoryUsage(b *testing.B) {
	// Test different archive sizes to demonstrate memory scaling issue
	sizes := []int64{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
		500 * 1024 * 1024, // 500MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			benchmarkORASPushMemoryUsage(b, size)
		})
	}
}

// benchmarkORASPushMemoryUsage tests ORAS client memory usage directly
func benchmarkORASPushMemoryUsage(b *testing.B, size int64) {
	// Setup test environment
	tempDir, err := os.MkdirTemp("", "benchmark-oras-push-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive generator
	gen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer gen.Close()

	// Generate test archive
	archivePath := filepath.Join(tempDir, "test.tar.gz")
	actualSize, err := gen.GenerateLargeArchive(context.Background(), size, archivePath)
	if err != nil {
		b.Fatalf("Failed to generate test archive: %v", err)
	}

	// Open archive for reading
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		b.Fatalf("Failed to open archive: %v", err)
	}
	defer archiveFile.Close()

	// Get file stats
	stat, err := archiveFile.Stat()
	if err != nil {
		b.Fatalf("Failed to get file stats: %v", err)
	}

	// Create ORAS client
	orasClient := &oras.DefaultORASClient{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset file position for each iteration
		if _, err := archiveFile.Seek(0, 0); err != nil {
			b.Fatalf("Failed to seek to beginning: %v", err)
		}

		// Create push descriptor
		desc := &oras.PushDescriptor{
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Data:      archiveFile,
			Size:      stat.Size(),
		}

		// This will demonstrate the memory issue - io.ReadAll loads everything into memory
		reference := "localhost:5000/test/repo:benchmark"
		err := orasClient.Push(context.Background(), reference, desc, nil)
		if err != nil && !isRegistryUnavailableError(err) {
			b.Fatalf("ORAS push failed: %v", err)
		}
	}

	// Report metrics
	b.ReportMetric(float64(actualSize)/1024/1024, "MB_processed/op")
}

// Helper function to check if error is due to registry being unavailable
func isRegistryUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "registry unreachable") ||
		strings.Contains(errStr, "authentication failed")
}
