package billy

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/jmgilman/go/fs/core"
)

// testCloser is a helper to handle defer close in tests.
func testCloser(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Logf("Close error (non-fatal): %v", err)
	}
}

// TestFile_Interfaces verifies File implements all required interfaces.
func TestFile_Interfaces(t *testing.T) {
	// Compile-time checks are in file.go, but we verify at runtime too
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	file := &File{file: bf, fs: bfs, name: "test.txt"}

	// Verify core.File interface
	var _ core.File = file

	// Verify fs.File interface
	var _ fs.File = file

	// Verify io.Seeker interface
	var _ io.Seeker = file

	// Verify core.Truncater interface
	var _ core.Truncater = file

	// Verify core.Syncer interface
	var _ core.Syncer = file
}

// TestFile_Name verifies Name() returns the stored filename.
func TestFile_Name(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test-file.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	tests := []struct {
		name     string
		filename string
	}{
		{"simple", "test.txt"},
		{"with path", "dir/subdir/file.txt"},
		{"with dots", "../parent/file.txt"},
		{"root", "/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &File{file: bf, fs: bfs, name: tt.filename}
			if got := file.Name(); got != tt.filename {
				t.Errorf("Name() = %q, want %q", got, tt.filename)
			}
		})
	}
}

// TestFile_Read verifies Read delegates to billy.File.
func TestFile_Read(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	// Write test data
	testData := []byte("Hello, World!")
	if _, err := bf.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to start: %v", err)
	}

	// Read through File wrapper
	file := &File{file: bf, fs: bfs, name: "test.txt"}
	buf := make([]byte, len(testData))
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() n = %d, want %d", n, len(testData))
	}
	if string(buf) != string(testData) {
		t.Errorf("Read() data = %q, want %q", buf, testData)
	}
}

// TestFile_Write verifies Write delegates to billy.File.
func TestFile_Write(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	file := &File{file: bf, fs: bfs, name: "test.txt"}
	testData := []byte("Test write data")
	n, err := file.Write(testData)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() n = %d, want %d", n, len(testData))
	}

	// Verify data was written
	if _, err := bf.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to start: %v", err)
	}
	buf := make([]byte, len(testData))
	if _, err := bf.Read(buf); err != nil {
		t.Fatalf("Failed to read back data: %v", err)
	}
	if string(buf) != string(testData) {
		t.Errorf("Written data = %q, want %q", buf, testData)
	}
}

// TestFile_Close verifies Close delegates to billy.File.
func TestFile_Close(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}

	file := &File{file: bf, fs: bfs, name: "test.txt"}
	if err := file.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify file is closed by attempting an operation
	_, err = file.Read(make([]byte, 1))
	if !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Expected ErrClosed after Close(), got %v", err)
	}
}

// TestFile_Stat verifies Stat delegates to billy.File.
func TestFile_Stat(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	file := &File{file: bf, fs: bfs, name: "test.txt"}
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info == nil {
		t.Fatal("Stat() returned nil FileInfo")
	}
	if info.IsDir() {
		t.Error("Stat() IsDir() = true, want false for file")
	}
}

// TestFile_Seek verifies Seek delegates to billy.File.
func TestFile_Seek(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	// Write test data
	testData := []byte("0123456789")
	if _, err := bf.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	file := &File{file: bf, fs: bfs, name: "test.txt"}

	tests := []struct {
		name   string
		offset int64
		whence int
		want   int64
	}{
		{"seek start", 0, io.SeekStart, 0},
		{"seek middle", 5, io.SeekStart, 5},
		{"seek end", 0, io.SeekEnd, int64(len(testData))},
		{"seek relative", 2, io.SeekCurrent, int64(len(testData)) + 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := file.Seek(tt.offset, tt.whence)
			if err != nil {
				t.Fatalf("Seek(%d, %d) error = %v", tt.offset, tt.whence, err)
			}
			if got != tt.want {
				t.Errorf("Seek(%d, %d) = %d, want %d", tt.offset, tt.whence, got, tt.want)
			}
		})
	}
}

// TestFile_Truncate verifies Truncate delegates to billy.File.
func TestFile_Truncate(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	// Write test data
	testData := []byte("0123456789")
	if _, err := bf.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	file := &File{file: bf, fs: bfs, name: "test.txt"}

	// Truncate to smaller size
	newSize := int64(5)
	if err := file.Truncate(newSize); err != nil {
		t.Fatalf("Truncate(%d) error = %v", newSize, err)
	}

	// Verify new size
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() != newSize {
		t.Errorf("After Truncate(%d), Size() = %d, want %d", newSize, info.Size(), newSize)
	}
}

// TestFile_Sync verifies Sync handles both supporting and non-supporting backends.
func TestFile_Sync(t *testing.T) {
	// Test with memfs (doesn't support Sync)
	t.Run("memfs no-op", func(t *testing.T) {
		bfs := memfs.New()
		bf, err := bfs.Create("test.txt")
		if err != nil {
			t.Fatalf("Failed to create billy file: %v", err)
		}
		defer testCloser(t, bf)

		file := &File{file: bf, fs: bfs, name: "test.txt"}
		// Should not error even though memfs doesn't support Sync
		if err := file.Sync(); err != nil {
			t.Errorf("Sync() error = %v, want nil (no-op for memfs)", err)
		}
	})

	// Note: Testing with osfs would require actual file I/O and cleanup.
	// The memfs test verifies the no-op path, which is the critical behavior.
}

// TestFile_ReadWrite verifies Read and Write work together correctly.
func TestFile_ReadWrite(t *testing.T) {
	bfs := memfs.New()
	bf, err := bfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create billy file: %v", err)
	}
	defer testCloser(t, bf)

	file := &File{file: bf, fs: bfs, name: "test.txt"}

	// Write data
	writeData := []byte("Hello, Billy!")
	n, err := file.Write(writeData)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Write() n = %d, want %d", n, len(writeData))
	}

	// Seek to start
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}

	// Read data back
	readData := make([]byte, len(writeData))
	n, err = file.Read(readData)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Read() n = %d, want %d", n, len(writeData))
	}
	if string(readData) != string(writeData) {
		t.Errorf("Read data = %q, want %q", readData, writeData)
	}
}
