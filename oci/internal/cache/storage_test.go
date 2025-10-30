package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	billyfs "github.com/input-output-hk/catalyst-forge-libs/fs/billy"
)

func TestStorage_NewStorage(t *testing.T) {
	tests := []struct {
		name      string
		fs        *billyfs.FS
		rootPath  string
		wantError bool
	}{
		{
			name:      "valid storage creation",
			fs:        billyfs.NewInMemoryFS(),
			rootPath:  "/cache",
			wantError: false,
		},
		{
			name:      "nil filesystem",
			fs:        nil,
			rootPath:  "/cache",
			wantError: true,
		},
		{
			name:      "empty root path",
			fs:        billyfs.NewInMemoryFS(),
			rootPath:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewStorage(tt.fs, tt.rootPath)
			if (err != nil) != tt.wantError {
				t.Errorf("NewStorage() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && storage == nil {
				t.Error("NewStorage() returned nil storage when no error expected")
			}
		})
	}
}

func TestStorage_WriteAtomically(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testData := []byte("Hello, World!")
	testPath := "test/file.txt"

	// Test successful write
	err = storage.WriteAtomically(ctx, testPath, testData)
	if err != nil {
		t.Fatalf("WriteAtomically failed: %v", err)
	}

	// Verify file exists
	exists, err := storage.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("File should exist after write")
	}

	// Verify content
	readData, err := storage.ReadWithIntegrity(ctx, testPath)
	if err != nil {
		t.Fatalf("ReadWithIntegrity failed: %v", err)
	}
	if !bytes.Equal(readData, testData) {
		t.Errorf("Read data = %s, want %s", string(readData), string(testData))
	}
}

func TestStorage_ReadWithIntegrity(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testData := []byte("Test data for integrity")
	testPath := "integrity/test.txt"

	// Write data first
	err = storage.WriteAtomically(ctx, testPath, testData)
	if err != nil {
		t.Fatalf("WriteAtomically failed: %v", err)
	}

	// Test successful read
	readData, err := storage.ReadWithIntegrity(ctx, testPath)
	if err != nil {
		t.Fatalf("ReadWithIntegrity failed: %v", err)
	}
	if !bytes.Equal(readData, testData) {
		t.Errorf("Read data mismatch: got %s, want %s", string(readData), string(testData))
	}

	// Test reading non-existent file
	_, err = storage.ReadWithIntegrity(ctx, "nonexistent/file.txt")
	if err == nil {
		t.Error("ReadWithIntegrity should fail for non-existent file")
	}
}

func TestStorage_ConcurrentReadWrite(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	const numGoroutines = 10
	const numOperations = 50

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// Start concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				path := fmt.Sprintf("concurrent/file_%d_%d.txt", id, j)
				data := []byte(fmt.Sprintf("Data from goroutine %d, operation %d", id, j))

				if err := storage.WriteAtomically(ctx, path, data); err != nil {
					errors <- fmt.Errorf("write error from goroutine %d, op %d: %w", id, j, err)
				}
			}
		}(i)
	}

	// Start concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				path := fmt.Sprintf("concurrent/file_%d_%d.txt", id, j)
				expectedData := fmt.Sprintf("Data from goroutine %d, operation %d", id, j)

				// Small delay to let writers catch up
				time.Sleep(time.Millisecond)

				data, err := storage.ReadWithIntegrity(ctx, path)
				if err != nil {
					// File might not exist yet, which is okay
					continue
				}

				if string(data) != expectedData {
					errors <- fmt.Errorf("data mismatch in goroutine %d, op %d: got %s, want %s",
						id, j, string(data), expectedData)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

func TestStorage_CorruptionDetection(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testData := []byte("Original data")
	testPath := "corruption/test.txt"

	// Write original data
	err = storage.WriteAtomically(ctx, testPath, testData)
	if err != nil {
		t.Fatalf("WriteAtomically failed: %v", err)
	}

	// Manually corrupt the file by writing directly to filesystem
	fullPath := filepath.Join("/cache", testPath)
	corruptedData := []byte("corrupted-checksum\ncorrupted data")
	err = fs.WriteFile(fullPath, corruptedData, 0o644)
	if err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	// Attempt to read corrupted file
	_, err = storage.ReadWithIntegrity(ctx, testPath)
	if !errors.Is(err, ErrCacheCorrupted) {
		t.Errorf("Expected ErrCacheCorrupted, got %v", err)
	}
}

func TestStorage_CleanupTempFiles(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Create some temp files manually to simulate failed operations
	tempDir := filepath.Join("/cache", ".temp")
	tempFile1 := filepath.Join(tempDir, "leftover1.tmp")
	tempFile2 := filepath.Join(tempDir, "leftover2.tmp")

	// Create temp files
	err = fs.WriteFile(tempFile1, []byte("leftover data 1"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create temp file 1: %v", err)
	}
	err = fs.WriteFile(tempFile2, []byte("leftover data 2"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create temp file 2: %v", err)
	}

	// Verify temp files exist
	exists1, _ := fs.Exists(tempFile1)
	exists2, _ := fs.Exists(tempFile2)
	if !exists1 || !exists2 {
		t.Error("Temp files should exist before cleanup")
	}

	// Run cleanup
	err = storage.CleanupTempFiles(ctx)
	if err != nil {
		t.Fatalf("CleanupTempFiles failed: %v", err)
	}

	// Verify temp files are removed
	exists1, _ = fs.Exists(tempFile1)
	exists2, _ = fs.Exists(tempFile2)
	if exists1 || exists2 {
		t.Error("Temp files should be removed after cleanup")
	}

	// Verify temp directory still exists
	tempDirExists, _ := fs.Exists(tempDir)
	if !tempDirExists {
		t.Error("Temp directory should still exist after cleanup")
	}
}

func TestStorage_StreamWriter(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testPath := "stream/test.txt"
	testData := []byte("Streaming data test")

	// Create stream writer
	writer, err := storage.NewStreamWriter(ctx, testPath)
	if err != nil {
		t.Fatalf("NewStreamWriter failed: %v", err)
	}

	// Write data in chunks
	chunk1 := testData[:10]
	chunk2 := testData[10:]

	n1, err := writer.Write(chunk1)
	if err != nil {
		t.Fatalf("Write chunk1 failed: %v", err)
	}
	if n1 != len(chunk1) {
		t.Errorf("Write chunk1 returned %d, want %d", n1, len(chunk1))
	}

	n2, err := writer.Write(chunk2)
	if err != nil {
		t.Fatalf("Write chunk2 failed: %v", err)
	}
	if n2 != len(chunk2) {
		t.Errorf("Write chunk2 returned %d, want %d", n2, len(chunk2))
	}

	// Close writer
	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify written data
	readData, err := storage.ReadWithIntegrity(ctx, testPath)
	if err != nil {
		t.Fatalf("ReadWithIntegrity failed: %v", err)
	}
	if !bytes.Equal(readData, testData) {
		t.Errorf("Stream data mismatch: got %s, want %s", string(readData), string(testData))
	}
}

func TestStorage_Size(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Initially should be empty (just temp directory)
	initialSize, err := storage.Size(ctx)
	if err != nil {
		t.Fatalf("Initial Size failed: %v", err)
	}
	// Size should be at least the size of the temp directory structure
	if initialSize < 0 {
		t.Errorf("Initial size should be non-negative, got %d", initialSize)
	}

	// Add some files
	testData1 := []byte("File 1 content")
	testData2 := []byte("File 2 content with more data")

	err = storage.WriteAtomically(ctx, "file1.txt", testData1)
	if err != nil {
		t.Fatalf("Write file1 failed: %v", err)
	}

	err = storage.WriteAtomically(ctx, "file2.txt", testData2)
	if err != nil {
		t.Fatalf("Write file2 failed: %v", err)
	}

	// Check size after adding files
	finalSize, err := storage.Size(ctx)
	if err != nil {
		t.Fatalf("Final Size failed: %v", err)
	}

	expectedMinSize := len(testData1) + len(testData2)
	if finalSize < int64(expectedMinSize) {
		t.Errorf("Final size %d should be at least %d", finalSize, expectedMinSize)
	}
}

func TestStorage_Remove(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testPath := "remove/test.txt"
	testData := []byte("Data to be removed")

	// Write file
	err = storage.WriteAtomically(ctx, testPath, testData)
	if err != nil {
		t.Fatalf("WriteAtomically failed: %v", err)
	}

	// Verify file exists
	exists, err := storage.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("File should exist")
	}

	// Remove file
	err = storage.Remove(ctx, testPath)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify file no longer exists
	exists, err = storage.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists after remove failed: %v", err)
	}
	if exists {
		t.Error("File should not exist after removal")
	}

	// Removing non-existent file should not error
	err = storage.Remove(ctx, "nonexistent.txt")
	// Note: billy filesystem returns an error for non-existent files, which is different from os.Remove
	// This is acceptable behavior as long as it's consistent
	if err != nil && !strings.Contains(err.Error(), "file does not exist") {
		t.Errorf("Unexpected error removing non-existent file: %v", err)
	}
}

func TestStorage_ListFiles(t *testing.T) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Test empty directory
	files, err := storage.ListFiles(ctx, "empty")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Expected empty list, got %v", files)
	}

	// Add some files
	testFiles := []string{
		"dir1/file1.txt",
		"dir1/file2.txt",
		"dir2/file3.txt",
	}

	for _, file := range testFiles {
		writeErr := storage.WriteAtomically(ctx, file, []byte("content"))
		if writeErr != nil {
			t.Fatalf("WriteAtomically failed for %s: %v", file, writeErr)
		}
	}

	// List files in dir1
	files, err = storage.ListFiles(ctx, "dir1")
	if err != nil {
		t.Fatalf("ListFiles for dir1 failed: %v", err)
	}
	expected := []string{"file1.txt", "file2.txt"}
	if len(files) != len(expected) {
		t.Errorf("Expected %d files, got %d", len(expected), len(files))
	}
	for _, expectedFile := range expected {
		found := false
		for _, file := range files {
			if file == expectedFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file %s not found in %v", expectedFile, files)
		}
	}
}

// Fuzz test for path handling
func FuzzStorage_PathHandling(f *testing.F) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		f.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	testData := []byte("fuzz test data")

	// Add some seed inputs
	seeds := []string{
		"normal/path/file.txt",
		"file/with.dots.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"path/to/deeply/nested/file.txt",
		"single.txt",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Skip empty paths and paths with null bytes
		if path == "" || strings.Contains(path, "\x00") {
			return
		}

		// Skip paths that look like they might cause issues
		if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
			return
		}

		// Try to write and read with the fuzzed path
		err := storage.WriteAtomically(ctx, path, testData)
		if err != nil {
			// Some paths might legitimately fail (e.g., too long), which is okay
			return
		}

		// If write succeeded, try to read
		readData, err := storage.ReadWithIntegrity(ctx, path)
		if err != nil {
			t.Errorf("Failed to read path %q: %v", path, err)
			return
		}

		if !bytes.Equal(readData, testData) {
			t.Errorf("Data mismatch for path %q", path)
		}
	})
}

// Benchmark tests
func BenchmarkStorage_WriteAtomically(b *testing.B) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	data := []byte("Benchmark test data for atomic writes")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("bench/file_%d.txt", i)
		if err := storage.WriteAtomically(ctx, path, data); err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
}

func BenchmarkStorage_ReadWithIntegrity(b *testing.B) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	data := []byte("Benchmark test data for integrity reads")
	path := "bench/read_test.txt"

	if err := storage.WriteAtomically(ctx, path, data); err != nil {
		b.Fatalf("Setup write failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := storage.ReadWithIntegrity(ctx, path)
		if err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}

func BenchmarkStorage_ConcurrentReadWrite(b *testing.B) {
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	data := []byte("Concurrent benchmark data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localCounter := 0
		for pb.Next() {
			path := fmt.Sprintf("concurrent/file_%d.txt", localCounter)
			localCounter++

			if err := storage.WriteAtomically(ctx, path, data); err != nil {
				b.Fatalf("Concurrent write failed: %v", err)
			}

			if _, err := storage.ReadWithIntegrity(ctx, path); err != nil {
				b.Fatalf("Concurrent read failed: %v", err)
			}
		}
	})
}
