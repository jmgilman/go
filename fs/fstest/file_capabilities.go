package fstest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestFileCapabilities tests optional File-level capabilities.
// Tests: io.Seeker, io.ReaderAt, io.WriterAt, core.Truncater, core.Syncer, fs.ReadDirFile.
// Uses type assertions on files returned by the FS - skips unsupported capabilities.
// Uses POSIXTestConfig() by default.
func TestFileCapabilities(t *testing.T, filesystem core.FS) {
	TestFileCapabilitiesWithConfig(t, filesystem, POSIXTestConfig())
}

// TestFileCapabilitiesWithSkip is the internal version with skip support.
// Deprecated: Use TestFileCapabilitiesWithConfig instead.
func TestFileCapabilitiesWithSkip(t *testing.T, filesystem core.FS, skipTests []string) {
	config := POSIXTestConfig()
	config.SkipTests = skipTests
	TestFileCapabilitiesWithConfig(t, filesystem, config)
}

// TestFileCapabilitiesWithConfig tests file capabilities with behavior configuration.
func TestFileCapabilitiesWithConfig(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Helper to check if a test should be skipped
	shouldSkip := func(testName string) bool {
		fullName := "FileCapabilities/" + testName
		for _, skip := range config.SkipTests {
			if skip == fullName {
				return true
			}
		}
		return false
	}

	// Run all capability tests
	t.Run("Seeker", func(t *testing.T) {
		if shouldSkip("Seeker") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilitySeeker(t, filesystem, config)
	})
	t.Run("ReaderAt", func(t *testing.T) {
		if shouldSkip("ReaderAt") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilityReaderAt(t, filesystem, config)
	})
	t.Run("WriterAt", func(t *testing.T) {
		if shouldSkip("WriterAt") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilityWriterAt(t, filesystem, config)
	})
	t.Run("Truncater", func(t *testing.T) {
		if shouldSkip("Truncater") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilityTruncater(t, filesystem, config)
	})
	t.Run("Syncer", func(t *testing.T) {
		if shouldSkip("Syncer") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilitySyncer(t, filesystem, config)
	})
	t.Run("ReadDirFile", func(t *testing.T) {
		if shouldSkip("ReadDirFile") {
			t.Skip("Skipped by provider configuration")
			return
		}
		testFileCapabilityReadDirFile(t, filesystem, config)
	})
}

// testFileCapabilitySeeker tests io.Seeker capability on file handles.
//
//nolint:gocyclo,cyclop // Test function with multiple validation checks
func testFileCapabilitySeeker(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	testContent := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := filesystem.WriteFile("seeker-test.txt", testContent, 0644); err != nil {
		t.Fatalf("WriteFile(seeker-test.txt): setup failed: %v", err)
	}

	// Open the file for reading
	f, err := filesystem.Open("seeker-test.txt")
	if err != nil {
		t.Fatalf("Open(seeker-test.txt): got error %v, want nil", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Errorf("Close(): got error %v", closeErr)
		}
	}()

	// Check if file supports io.Seeker
	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Skip("io.Seeker not supported by this file implementation")
		return
	}

	// Test 1: Seek to position 10 from start
	pos, err := seeker.Seek(10, io.SeekStart)
	if err != nil {
		t.Errorf("Seek(10, SeekStart): got error %v, want nil", err)
		return
	}
	if pos != 10 {
		t.Errorf("Seek(10, SeekStart): position = %d, want 10", pos)
	}

	// Verify we can read from the new position
	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("Read() after Seek: got error %v, want nil or EOF", err)
		return
	}
	if n != 5 {
		t.Errorf("Read() after Seek: read %d bytes, want 5", n)
	}
	expected := []byte("abcde")
	if !bytes.Equal(buf, expected) {
		t.Errorf("Read() after Seek(10): got %q, want %q", buf, expected)
	}

	// Test 2: Seek from current position
	pos, err = seeker.Seek(5, io.SeekCurrent)
	if err != nil {
		t.Errorf("Seek(5, SeekCurrent): got error %v, want nil", err)
		return
	}
	// Current position was 15 (10 + 5 read), seeking +5 should be 20
	if pos != 20 {
		t.Errorf("Seek(5, SeekCurrent): position = %d, want 20", pos)
	}

	// Test 3: Seek from end
	pos, err = seeker.Seek(-5, io.SeekEnd)
	if err != nil {
		t.Errorf("Seek(-5, SeekEnd): got error %v, want nil", err)
		return
	}
	expectedPos := int64(len(testContent) - 5)
	if pos != expectedPos {
		t.Errorf("Seek(-5, SeekEnd): position = %d, want %d", pos, expectedPos)
	}

	// Verify we can read from end position
	buf2 := make([]byte, 5)
	n, err = f.Read(buf2)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("Read() after Seek from end: got error %v, want nil or EOF", err)
		return
	}
	if n != 5 {
		t.Errorf("Read() after Seek from end: read %d bytes, want 5", n)
	}
	expected2 := []byte("vwxyz")
	if !bytes.Equal(buf2, expected2) {
		t.Errorf("Read() after Seek(-5, SeekEnd): got %q, want %q", buf2, expected2)
	}

	// Test 4: Seek back to start
	pos, err = seeker.Seek(0, io.SeekStart)
	if err != nil {
		t.Errorf("Seek(0, SeekStart): got error %v, want nil", err)
		return
	}
	if pos != 0 {
		t.Errorf("Seek(0, SeekStart): position = %d, want 0", pos)
	}
}

// testFileCapabilityReaderAt tests io.ReaderAt capability on file handles.
//
//nolint:gocyclo,cyclop,funlen // Test function with multiple validation checks
func testFileCapabilityReaderAt(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	testContent := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := filesystem.WriteFile("readerat-test.txt", testContent, 0644); err != nil {
		t.Fatalf("WriteFile(readerat-test.txt): setup failed: %v", err)
	}

	// Open the file for reading
	f, err := filesystem.Open("readerat-test.txt")
	if err != nil {
		t.Fatalf("Open(readerat-test.txt): got error %v, want nil", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Errorf("Close(): got error %v", closeErr)
		}
	}()

	// Check if file supports io.ReaderAt
	readerAt, ok := f.(io.ReaderAt)
	if !ok {
		t.Skip("io.ReaderAt not supported by this file implementation")
		return
	}

	// Test 1: ReadAt from offset 10
	buf := make([]byte, 5)
	n, err := readerAt.ReadAt(buf, 10)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt(buf, 10): got error %v, want nil or EOF", err)
		return
	}
	if n != 5 {
		t.Errorf("ReadAt(buf, 10): read %d bytes, want 5", n)
	}
	expected := []byte("abcde")
	if !bytes.Equal(buf, expected) {
		t.Errorf("ReadAt(buf, 10): got %q, want %q", buf, expected)
	}

	// Test 2: ReadAt from offset 0
	buf2 := make([]byte, 10)
	n, err = readerAt.ReadAt(buf2, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt(buf2, 0): got error %v, want nil or EOF", err)
		return
	}
	if n != 10 {
		t.Errorf("ReadAt(buf2, 0): read %d bytes, want 10", n)
	}
	expected2 := []byte("0123456789")
	if !bytes.Equal(buf2, expected2) {
		t.Errorf("ReadAt(buf2, 0): got %q, want %q", buf2, expected2)
	}

	// Test 3: ReadAt from near end of file
	buf3 := make([]byte, 5)
	offset := int64(len(testContent) - 5)
	n, err = readerAt.ReadAt(buf3, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt(buf3, %d): got error %v, want nil or EOF", offset, err)
		return
	}
	if n != 5 {
		t.Errorf("ReadAt(buf3, %d): read %d bytes, want 5", offset, n)
	}
	expected3 := []byte("vwxyz")
	if !bytes.Equal(buf3, expected3) {
		t.Errorf("ReadAt(buf3, %d): got %q, want %q", offset, buf3, expected3)
	}

	// Test 4: Verify ReadAt doesn't affect regular read position
	// Open a new file handle for this test
	f2, err := filesystem.Open("readerat-test.txt")
	if err != nil {
		t.Fatalf("Open(readerat-test.txt) second time: got error %v, want nil", err)
	}
	defer func() {
		if closeErr := f2.Close(); closeErr != nil {
			t.Errorf("Close() f2: got error %v", closeErr)
		}
	}()

	readerAt2, ok := f2.(io.ReaderAt)
	if !ok {
		return // Already tested support above
	}

	// Do a ReadAt
	bufTest := make([]byte, 3)
	_, _ = readerAt2.ReadAt(bufTest, 5)

	// Now do a regular Read - should start from beginning (position 0)
	bufRegular := make([]byte, 3)
	n, err = f2.Read(bufRegular)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("Read() after ReadAt: got error %v, want nil or EOF", err)
		return
	}
	if n != 3 {
		t.Errorf("Read() after ReadAt: read %d bytes, want 3", n)
	}
	expectedRegular := []byte("012")
	if !bytes.Equal(bufRegular, expectedRegular) {
		t.Errorf("Read() after ReadAt: got %q, want %q (ReadAt should not affect read position)", bufRegular, expectedRegular)
	}
}

// testFileCapabilityWriterAt tests io.WriterAt capability on file handles.
//
//nolint:gocyclo,cyclop,funlen // Test function with multiple validation checks
func testFileCapabilityWriterAt(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	initialContent := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := filesystem.WriteFile("writerat-test.txt", initialContent, 0644); err != nil {
		t.Fatalf("WriteFile(writerat-test.txt): setup failed: %v", err)
	}

	// Open the file for writing (need write permission)
	// Try OpenFile with O_RDWR if supported, otherwise skip test
	f, err := filesystem.OpenFile("writerat-test.txt", os.O_RDWR, 0644)
	if err != nil {
		// If O_RDWR not supported, try O_WRONLY
		f, err = filesystem.OpenFile("writerat-test.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot open file for writing, skipping WriterAt test")
			return
		}
	}

	// Check if file supports io.WriterAt
	writerAt, ok := f.(io.WriterAt)
	if !ok {
		t.Skip("io.WriterAt not supported by this file implementation")
		return
	}

	// Test 1: WriteAt to offset 10
	data := []byte("ABCDE")
	n, err := writerAt.WriteAt(data, 10)
	if err != nil {
		_ = f.Close()
		t.Errorf("WriteAt(ABCDE, 10): got error %v, want nil", err)
		return
	}
	if n != 5 {
		_ = f.Close()
		t.Errorf("WriteAt(ABCDE, 10): wrote %d bytes, want 5", n)
	}

	// Close the file to ensure write is flushed
	if err := f.Close(); err != nil {
		t.Errorf("Close() after WriteAt: got error %v, want nil", err)
		return
	}

	// Verify the write by reading the file
	content, err := filesystem.ReadFile("writerat-test.txt")
	if err != nil {
		t.Errorf("ReadFile(writerat-test.txt) after WriteAt: got error %v, want nil", err)
		return
	}
	// WriteAt replaces bytes at offset 10-14: "abcde" -> "ABCDE"
	expected := []byte("0123456789ABCDEfghijklmnopqrstuvwxyz")
	if !bytes.Equal(content, expected) {
		t.Errorf("Content after WriteAt(ABCDE, 10): got %q, want %q", content, expected)
	}

	// Test 2: WriteAt to offset 0
	f2, err := filesystem.OpenFile("writerat-test.txt", os.O_RDWR, 0644)
	if err != nil {
		f2, err = filesystem.OpenFile("writerat-test.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot reopen file for writing")
			return
		}
	}

	writerAt2, ok := f2.(io.WriterAt)
	if !ok {
		_ = f2.Close()
		return // Already tested support above
	}

	data2 := []byte("XYZ")
	n, err = writerAt2.WriteAt(data2, 0)
	if err != nil {
		_ = f2.Close()
		t.Errorf("WriteAt(XYZ, 0): got error %v, want nil", err)
		return
	}
	if n != 3 {
		_ = f2.Close()
		t.Errorf("WriteAt(XYZ, 0): wrote %d bytes, want 3", n)
	}

	if err := f2.Close(); err != nil {
		t.Errorf("Close() f2 after WriteAt: got error %v, want nil", err)
		return
	}

	// Verify the write
	content2, err := filesystem.ReadFile("writerat-test.txt")
	if err != nil {
		t.Errorf("ReadFile(writerat-test.txt) after second WriteAt: got error %v, want nil", err)
		return
	}
	// WriteAt replaces bytes at offset 0-2: "012" -> "XYZ"
	expected2 := []byte("XYZ3456789ABCDEfghijklmnopqrstuvwxyz")
	if !bytes.Equal(content2, expected2) {
		t.Errorf("Content after WriteAt(XYZ, 0): got %q, want %q", content2, expected2)
	}
}

// testFileCapabilityTruncater tests core.Truncater capability on file handles.
func testFileCapabilityTruncater(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Test 1: Truncate to smaller size
	t.Run("TruncateSmaller", func(t *testing.T) {
		testTruncateSmaller(t, filesystem, config)
	})

	// Test 2: Truncate to larger size
	t.Run("TruncateLarger", func(t *testing.T) {
		testTruncateLarger(t, filesystem, config)
	})

	// Test 3: Truncate to same size
	t.Run("TruncateSameSize", func(t *testing.T) {
		testTruncateSameSize(t, filesystem, config)
	})
}

// testTruncateSmaller tests truncating a file to a smaller size.
func testTruncateSmaller(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	initialContent := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := filesystem.WriteFile("truncate-smaller.txt", initialContent, 0644); err != nil {
		t.Fatalf("WriteFile(truncate-smaller.txt): setup failed: %v", err)
	}

	// Open the file for writing
	f, err := filesystem.OpenFile("truncate-smaller.txt", os.O_RDWR, 0644)
	if err != nil {
		f, err = filesystem.OpenFile("truncate-smaller.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot open file for writing, skipping Truncater test")
			return
		}
	}

	// Check if file supports core.Truncater
	truncater, ok := f.(core.Truncater)
	if !ok {
		_ = f.Close()
		t.Skip("core.Truncater not supported by this file implementation")
		return
	}

	// Truncate to size 10 (smaller than original 36)
	err = truncater.Truncate(10)
	if err != nil {
		_ = f.Close()
		t.Errorf("Truncate(10): got error %v, want nil", err)
		return
	}

	// Close the file to ensure truncate is applied
	if err := f.Close(); err != nil {
		t.Errorf("Close() after Truncate: got error %v, want nil", err)
		return
	}

	// Verify the file size
	info, err := filesystem.Stat("truncate-smaller.txt")
	if err != nil {
		t.Errorf("Stat(truncate-smaller.txt) after Truncate: got error %v, want nil", err)
		return
	}
	if info.Size() != 10 {
		t.Errorf("Stat(truncate-smaller.txt): Size() = %d, want 10", info.Size())
	}

	// Verify the content
	content, err := filesystem.ReadFile("truncate-smaller.txt")
	if err != nil {
		t.Errorf("ReadFile(truncate-smaller.txt) after Truncate: got error %v, want nil", err)
		return
	}
	expected := []byte("0123456789")
	if !bytes.Equal(content, expected) {
		t.Errorf("Content after Truncate(10): got %q, want %q", content, expected)
	}
}

// testTruncateLarger tests truncating a file to a larger size.
func testTruncateLarger(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	initialContent := []byte("0123456789")
	if err := filesystem.WriteFile("truncate-larger.txt", initialContent, 0644); err != nil {
		t.Fatalf("WriteFile(truncate-larger.txt): setup failed: %v", err)
	}

	// Open the file for writing
	f, err := filesystem.OpenFile("truncate-larger.txt", os.O_RDWR, 0644)
	if err != nil {
		f, err = filesystem.OpenFile("truncate-larger.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot open file for writing, skipping Truncater larger test")
			return
		}
	}

	// Check if file supports core.Truncater
	truncater, ok := f.(core.Truncater)
	if !ok {
		_ = f.Close()
		t.Skip("core.Truncater not supported by this file implementation")
		return
	}

	// Truncate to size 20 (larger than original 10)
	err = truncater.Truncate(20)
	if err != nil {
		_ = f.Close()
		t.Errorf("Truncate(20): got error %v, want nil", err)
		return
	}

	// Close the file to ensure truncate is applied
	if err := f.Close(); err != nil {
		t.Errorf("Close() after Truncate: got error %v, want nil", err)
		return
	}

	// Verify the file size
	info, err := filesystem.Stat("truncate-larger.txt")
	if err != nil {
		t.Errorf("Stat(truncate-larger.txt) after Truncate: got error %v, want nil", err)
		return
	}
	if info.Size() != 20 {
		t.Errorf("Stat(truncate-larger.txt): Size() = %d, want 20", info.Size())
	}

	// Verify the content (should have null bytes at the end)
	content, err := filesystem.ReadFile("truncate-larger.txt")
	if err != nil {
		t.Errorf("ReadFile(truncate-larger.txt) after Truncate: got error %v, want nil", err)
		return
	}
	if len(content) != 20 {
		t.Errorf("Content length after Truncate(20): got %d, want 20", len(content))
		return
	}
	// Check that first 10 bytes are original content
	expected := []byte("0123456789")
	if !bytes.Equal(content[:10], expected) {
		t.Errorf("Content first 10 bytes after Truncate(20): got %q, want %q", content[:10], expected)
	}
	// Check that bytes 10-19 are null bytes (or zeros)
	for i := 10; i < 20; i++ {
		if content[i] != 0 {
			t.Errorf("Content[%d] after Truncate(20): got %d, want 0 (null byte)", i, content[i])
		}
	}
}

// testTruncateSameSize tests truncating a file to the same size.
func testTruncateSameSize(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file with known content
	initialContent := []byte("0123456789")
	if err := filesystem.WriteFile("truncate-same.txt", initialContent, 0644); err != nil {
		t.Fatalf("WriteFile(truncate-same.txt): setup failed: %v", err)
	}

	// Open the file for writing
	f, err := filesystem.OpenFile("truncate-same.txt", os.O_RDWR, 0644)
	if err != nil {
		f, err = filesystem.OpenFile("truncate-same.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot open file for writing, skipping Truncater same size test")
			return
		}
	}

	// Check if file supports core.Truncater
	truncater, ok := f.(core.Truncater)
	if !ok {
		_ = f.Close()
		t.Skip("core.Truncater not supported by this file implementation")
		return
	}

	// Truncate to size 10 (same as original)
	err = truncater.Truncate(10)
	if err != nil {
		_ = f.Close()
		t.Errorf("Truncate(10): got error %v, want nil", err)
		return
	}

	// Close the file to ensure truncate is applied
	if err := f.Close(); err != nil {
		t.Errorf("Close() after Truncate: got error %v, want nil", err)
		return
	}

	// Verify the file size
	info, err := filesystem.Stat("truncate-same.txt")
	if err != nil {
		t.Errorf("Stat(truncate-same.txt) after Truncate: got error %v, want nil", err)
		return
	}
	if info.Size() != 10 {
		t.Errorf("Stat(truncate-same.txt): Size() = %d, want 10", info.Size())
	}

	// Verify the content is unchanged
	content, err := filesystem.ReadFile("truncate-same.txt")
	if err != nil {
		t.Errorf("ReadFile(truncate-same.txt) after Truncate: got error %v, want nil", err)
		return
	}
	if !bytes.Equal(content, initialContent) {
		t.Errorf("Content after Truncate(10) same size: got %q, want %q", content, initialContent)
	}
}

// testFileCapabilitySyncer tests core.Syncer capability on file handles.
//
//nolint:gocyclo,cyclop // Test function with multiple validation checks
func testFileCapabilitySyncer(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Setup: Create a test file
	if err := filesystem.WriteFile("syncer-test.txt", []byte("initial"), 0644); err != nil {
		t.Fatalf("WriteFile(syncer-test.txt): setup failed: %v", err)
	}

	// Open the file for writing
	f, err := filesystem.OpenFile("syncer-test.txt", os.O_RDWR, 0644)
	if err != nil {
		f, err = filesystem.OpenFile("syncer-test.txt", os.O_WRONLY, 0644)
		if err != nil {
			t.Skip("Cannot open file for writing, skipping Syncer test")
			return
		}
	}

	// Check if file supports core.Syncer
	syncer, ok := f.(core.Syncer)
	if !ok {
		_ = f.Close()
		t.Skip("core.Syncer not supported by this file implementation")
		return
	}

	// Write some data to the file
	data := []byte("synced data")
	_, err = f.Write(data)
	if err != nil {
		_ = f.Close()
		t.Errorf("Write(): got error %v, want nil", err)
		return
	}

	// Test Sync to flush writes to storage
	err = syncer.Sync()
	if err != nil {
		_ = f.Close()
		t.Errorf("Sync(): got error %v, want nil", err)
		return
	}

	// Close the file
	if err := f.Close(); err != nil {
		t.Errorf("Close() after Sync: got error %v, want nil", err)
		return
	}

	// Verify the data was written (basic validation that Sync didn't break anything)
	content, err := filesystem.ReadFile("syncer-test.txt")
	if err != nil {
		t.Errorf("ReadFile(syncer-test.txt) after Sync: got error %v, want nil", err)
		return
	}
	// Content should contain the synced data (may also have "initial" prefix depending on write mode)
	// We just verify Sync succeeded and file is readable
	if len(content) == 0 {
		t.Errorf("Content after Sync: file is empty, expected data")
	}

	// Test Sync multiple times (should be idempotent)
	f2, err := filesystem.OpenFile("syncer-test.txt", os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		f2, err = filesystem.OpenFile("syncer-test.txt", os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			t.Skip("Cannot reopen file for appending")
			return
		}
	}

	syncer2, ok := f2.(core.Syncer)
	if !ok {
		_ = f2.Close()
		return // Already tested support above
	}

	// Sync multiple times
	for i := 0; i < 3; i++ {
		if err := syncer2.Sync(); err != nil {
			_ = f2.Close()
			t.Errorf("Sync() iteration %d: got error %v, want nil", i, err)
		}
	}

	// Close the second file
	if err := f2.Close(); err != nil {
		t.Errorf("Close() f2: got error %v", err)
	}
}

// testFileCapabilityReadDirFile tests fs.ReadDirFile capability on directory handles.
//
//nolint:gocyclo,cyclop,funlen // Test function with multiple validation checks
func testFileCapabilityReadDirFile(t *testing.T, filesystem core.FS, config FSTestConfig) {
	// Skip if filesystem has virtual directories - can't Open() directories in S3
	if config.VirtualDirectories {
		t.Skip("Skipping ReadDirFile test - filesystem has virtual directories (can't Open directories)")
		return
	}

	// Setup: Create a directory with some files
	if err := filesystem.Mkdir("readdir-test", 0755); err != nil {
		t.Fatalf("Mkdir(readdir-test): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("readdir-test/file1.txt", []byte("content1"), 0644); err != nil {
		t.Fatalf("WriteFile(readdir-test/file1.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("readdir-test/file2.txt", []byte("content2"), 0644); err != nil {
		t.Fatalf("WriteFile(readdir-test/file2.txt): setup failed: %v", err)
	}
	if err := filesystem.WriteFile("readdir-test/file3.txt", []byte("content3"), 0644); err != nil {
		t.Fatalf("WriteFile(readdir-test/file3.txt): setup failed: %v", err)
	}

	// Open the directory
	f, err := filesystem.Open("readdir-test")
	if err != nil {
		t.Fatalf("Open(readdir-test): got error %v, want nil", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Errorf("Close(): got error %v", closeErr)
		}
	}()

	// Check if file supports fs.ReadDirFile
	readDirFile, ok := f.(fs.ReadDirFile)
	if !ok {
		t.Skip("fs.ReadDirFile not supported by this file implementation")
		return
	}

	// Test 1: ReadDir with n=-1 (read all entries)
	entries, err := readDirFile.ReadDir(-1)
	if err != nil {
		t.Errorf("ReadDir(-1): got error %v, want nil", err)
		return
	}
	if len(entries) != 3 {
		t.Errorf("ReadDir(-1): got %d entries, want 3", len(entries))
		return
	}

	// Verify entries are the expected files
	expectedNames := map[string]bool{
		"file1.txt": false,
		"file2.txt": false,
		"file3.txt": false,
	}
	for _, entry := range entries {
		name := entry.Name()
		if _, exists := expectedNames[name]; !exists {
			t.Errorf("ReadDir(-1): unexpected entry %q", name)
		} else {
			expectedNames[name] = true
		}
		if entry.IsDir() {
			t.Errorf("ReadDir(-1): entry %q is a directory, want file", name)
		}
	}
	for name, seen := range expectedNames {
		if !seen {
			t.Errorf("ReadDir(-1): missing expected entry %q", name)
		}
	}

	// Test 2: ReadDir with n=2 (read 2 entries at a time)
	// Open a new directory handle for this test
	f2, err := filesystem.Open("readdir-test")
	if err != nil {
		t.Fatalf("Open(readdir-test) second time: got error %v, want nil", err)
	}
	defer func() {
		if closeErr := f2.Close(); closeErr != nil {
			t.Errorf("Close() f2: got error %v", closeErr)
		}
	}()

	readDirFile2, ok := f2.(fs.ReadDirFile)
	if !ok {
		return // Already tested support above
	}

	// First call: read 2 entries
	entries1, err := readDirFile2.ReadDir(2)
	if err != nil {
		t.Errorf("ReadDir(2) first call: got error %v, want nil", err)
		return
	}
	if len(entries1) != 2 {
		t.Errorf("ReadDir(2) first call: got %d entries, want 2", len(entries1))
	}

	// Second call: read remaining entries (should be 1)
	entries2, err := readDirFile2.ReadDir(2)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("ReadDir(2) second call: got error %v, want nil or EOF", err)
		return
	}
	if len(entries2) != 1 {
		t.Errorf("ReadDir(2) second call: got %d entries, want 1", len(entries2))
	}

	// Third call: should return EOF (no more entries)
	entries3, err := readDirFile2.ReadDir(2)
	if !errors.Is(err, io.EOF) {
		t.Errorf("ReadDir(2) third call: got error %v, want io.EOF", err)
	}
	if len(entries3) != 0 {
		t.Errorf("ReadDir(2) third call: got %d entries, want 0", len(entries3))
	}

	// Test 3: ReadDir on empty directory
	// Skip if filesystem has virtual directories (S3-like) - empty dirs can't be opened
	if !config.VirtualDirectories {
		if err := filesystem.Mkdir("empty-readdir-test", 0755); err != nil {
			t.Fatalf("Mkdir(empty-readdir-test): setup failed: %v", err)
		}

		f3, err := filesystem.Open("empty-readdir-test")
		if err != nil {
			t.Fatalf("Open(empty-readdir-test): got error %v, want nil", err)
		}
		defer func() {
			if closeErr := f3.Close(); closeErr != nil {
				t.Errorf("Close() f3: got error %v", closeErr)
			}
		}()

		readDirFile3, ok := f3.(fs.ReadDirFile)
		if !ok {
			return // Already tested support above
		}

		entriesEmpty, err := readDirFile3.ReadDir(-1)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("ReadDir(-1) on empty directory: got error %v, want nil or EOF", err)
			return
		}
		if len(entriesEmpty) != 0 {
			t.Errorf("ReadDir(-1) on empty directory: got %d entries, want 0", len(entriesEmpty))
		}
	}
}
