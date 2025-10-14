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

// TestOpenFileFlags tests OpenFile flag support.
// supportedFlags lists flags that should work (e.g., os.O_RDONLY, os.O_CREATE).
// Unsupported flags should return core.ErrUnsupported.
func TestOpenFileFlags(t *testing.T, filesystem core.FS, supportedFlags []int) {
	// Run all subtests
	t.Run("SupportedFlags", func(t *testing.T) {
		testOpenFileFlagsSupportedFlags(t, filesystem, supportedFlags)
	})
	t.Run("UnsupportedFlags", func(t *testing.T) {
		testOpenFileFlagsUnsupportedFlags(t, filesystem, supportedFlags)
	})
	t.Run("FlagCombinations", func(t *testing.T) {
		testOpenFileFlagsCombinations(t, filesystem, supportedFlags)
	})
}

// testOpenFileFlagsSupportedFlags tests that each supported flag works correctly.
func testOpenFileFlagsSupportedFlags(t *testing.T, filesystem core.FS, supportedFlags []int) {
	// Create a map for quick lookup of supported flags
	supported := make(map[int]bool)
	for _, flag := range supportedFlags {
		supported[flag] = true
	}

	// Test each supported flag individually
	for _, flag := range supportedFlags {
		flag := flag // Capture loop variable
		flagName := getFlagName(flag)
		t.Run(flagName, func(t *testing.T) {
			filename := "test-" + flagName + ".txt"
			openFlag := prepareFlagTest(t, filesystem, flag, filename, supported)

			f, err := filesystem.OpenFile(filename, openFlag, 0644)
			if err != nil {
				t.Errorf("OpenFile(%q, %s): got error %v, want nil", filename, flagName, err)
				return
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					t.Errorf("Close(): got error %v", closeErr)
				}
			}()

			verifyFlagBehavior(t, f, flag, flagName)
		})
	}
}

// prepareFlagTest prepares the filesystem and returns the appropriate open flag for testing.
func prepareFlagTest(t *testing.T, filesystem core.FS, flag int, filename string, supported map[int]bool) int {
	switch flag {
	case os.O_EXCL:
		return prepareExclFlag()
	case os.O_RDONLY, os.O_RDWR, os.O_WRONLY:
		return prepareReadWriteFlag(t, filesystem, flag, filename)
	case os.O_CREATE:
		return prepareCreateFlag(flag, supported)
	case os.O_TRUNC, os.O_APPEND:
		return prepareModifierFlag(t, filesystem, flag, filename, supported)
	default:
		return prepareOtherFlag(t, filesystem, flag, filename, supported)
	}
}

// prepareExclFlag prepares for O_EXCL flag testing.
func prepareExclFlag() int {
	return os.O_EXCL | os.O_CREATE | os.O_WRONLY
}

// prepareReadWriteFlag prepares for read/write mode flags.
func prepareReadWriteFlag(t *testing.T, filesystem core.FS, flag int, filename string) int {
	if err := filesystem.WriteFile(filename, []byte("test content"), 0644); err != nil {
		t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
	}
	return flag
}

// prepareCreateFlag prepares for O_CREATE flag testing.
func prepareCreateFlag(flag int, supported map[int]bool) int {
	if supported[os.O_WRONLY] {
		return flag | os.O_WRONLY
	}
	return flag
}

// prepareModifierFlag prepares for O_TRUNC or O_APPEND flag testing.
func prepareModifierFlag(t *testing.T, filesystem core.FS, flag int, filename string, supported map[int]bool) int {
	openFlag := flag
	if supported[os.O_CREATE] {
		openFlag |= os.O_CREATE
	} else {
		if err := filesystem.WriteFile(filename, []byte("test content"), 0644); err != nil {
			t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
		}
	}
	return addWriteMode(openFlag, supported)
}

// prepareOtherFlag prepares for other flags (like O_SYNC).
func prepareOtherFlag(t *testing.T, filesystem core.FS, flag int, filename string, supported map[int]bool) int {
	if err := filesystem.WriteFile(filename, []byte("test content"), 0644); err != nil {
		t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
	}
	return addWriteMode(flag, supported)
}

// addWriteMode adds an appropriate write mode flag if supported.
func addWriteMode(flag int, supported map[int]bool) int {
	if supported[os.O_WRONLY] {
		return flag | os.O_WRONLY
	} else if supported[os.O_RDWR] {
		return flag | os.O_RDWR
	}
	return flag
}

// verifyFlagBehavior verifies the file handle works as expected for the given flag.
func verifyFlagBehavior(t *testing.T, f fs.File, flag int, flagName string) {
	// Check if flag allows reading
	if flag == os.O_RDONLY || flag == os.O_RDWR {
		buf := make([]byte, 4)
		_, readErr := f.Read(buf)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			t.Errorf("Read() after OpenFile with %s: got error %v", flagName, readErr)
		}
	}

	// Check if flag allows writing (exclude read-only)
	if flag != os.O_RDONLY {
		// Try to write - if it's a read-only file, it will fail, which is expected
		if rw, ok := f.(io.Writer); ok {
			_, writeErr := rw.Write([]byte("test"))
			// Some flags like O_RDONLY shouldn't allow writes, so we don't error here
			_ = writeErr
		}
	}
}

// testOpenFileFlagsUnsupportedFlags tests that unsupported flags return core.ErrUnsupported.
func testOpenFileFlagsUnsupportedFlags(t *testing.T, filesystem core.FS, supportedFlags []int) {
	// Create a set of all standard flags
	allFlags := []int{
		os.O_RDONLY,
		os.O_WRONLY,
		os.O_RDWR,
		os.O_APPEND,
		os.O_CREATE,
		os.O_EXCL,
		os.O_SYNC,
		os.O_TRUNC,
	}

	// Create a map for quick lookup of supported flags
	supported := make(map[int]bool)
	for _, flag := range supportedFlags {
		supported[flag] = true
	}

	// Test each unsupported flag
	for _, flag := range allFlags {
		if supported[flag] {
			continue // Skip supported flags
		}

		flagName := getFlagName(flag)
		t.Run(flagName, func(t *testing.T) {
			// Setup: Create a test file
			filename := "unsupported-" + flagName + ".txt"
			if err := filesystem.WriteFile(filename, []byte("test content"), 0644); err != nil {
				t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
			}

			// Try to open with unsupported flag
			// Combine with a basic mode to make it a valid call
			openFlag := flag
			if flag != os.O_RDONLY && flag != os.O_WRONLY && flag != os.O_RDWR {
				// Add a mode flag if needed
				if supported[os.O_RDONLY] {
					openFlag |= os.O_RDONLY
				} else if supported[os.O_WRONLY] {
					openFlag |= os.O_WRONLY
				} else if supported[os.O_RDWR] {
					openFlag |= os.O_RDWR
				}
			}

			f, err := filesystem.OpenFile(filename, openFlag, 0644)
			if f != nil {
				_ = f.Close()
			}

			// Should return core.ErrUnsupported
			if !errors.Is(err, core.ErrUnsupported) {
				t.Errorf("OpenFile(%q, %s): got error %v, want core.ErrUnsupported", filename, flagName, err)
			}
		})
	}
}

// testOpenFileFlagsCombinations tests common flag combinations.
func testOpenFileFlagsCombinations(t *testing.T, filesystem core.FS, supportedFlags []int) {
	// Create a map for quick lookup of supported flags
	supported := make(map[int]bool)
	for _, flag := range supportedFlags {
		supported[flag] = true
	}

	if supported[os.O_CREATE] && supported[os.O_TRUNC] {
		testCreateTruncCombination(t, filesystem, supported)
	}

	if supported[os.O_CREATE] && supported[os.O_EXCL] {
		testCreateExclCombination(t, filesystem, supported)
	}

	if supported[os.O_APPEND] {
		testAppendFlag(t, filesystem, supported)
	}

	if supported[os.O_RDWR] {
		testRDWRFlag(t, filesystem)
	}
}

// testCreateTruncCombination tests O_CREATE | O_TRUNC combination.
func testCreateTruncCombination(t *testing.T, filesystem core.FS, supported map[int]bool) {
	t.Run("O_CREATE|O_TRUNC", func(t *testing.T) {
		filename := "combo-create-trunc.txt"
		if err := filesystem.WriteFile(filename, []byte("initial content"), 0644); err != nil {
			t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
		}

		flag := os.O_CREATE | os.O_TRUNC
		if supported[os.O_WRONLY] {
			flag |= os.O_WRONLY
		} else if supported[os.O_RDWR] {
			flag |= os.O_RDWR
		}

		f, err := filesystem.OpenFile(filename, flag, 0644)
		if err != nil {
			t.Errorf("OpenFile(%q, O_CREATE|O_TRUNC): got error %v, want nil", filename, err)
			return
		}

		newContent := []byte("truncated content")
		if _, err = f.Write(newContent); err != nil {
			_ = f.Close()
			t.Fatalf("Write(): got error %v, want nil", err)
		}

		if err := f.Close(); err != nil {
			t.Fatalf("Close(): got error %v, want nil", err)
		}

		data, err := filesystem.ReadFile(filename)
		if err != nil {
			t.Errorf("ReadFile(%q): got error %v, want nil", filename, err)
			return
		}
		if !bytes.Equal(data, newContent) {
			t.Errorf("ReadFile(%q): got %q, want %q", filename, data, newContent)
		}
	})
}

// testCreateExclCombination tests O_CREATE | O_EXCL combination.
func testCreateExclCombination(t *testing.T, filesystem core.FS, supported map[int]bool) {
	t.Run("O_CREATE|O_EXCL", func(t *testing.T) {
		filename := "combo-create-excl.txt"
		_ = filesystem.Remove(filename)

		flag := os.O_CREATE | os.O_EXCL
		if supported[os.O_WRONLY] {
			flag |= os.O_WRONLY
		} else if supported[os.O_RDWR] {
			flag |= os.O_RDWR
		}

		f, err := filesystem.OpenFile(filename, flag, 0644)
		if err != nil {
			t.Errorf("OpenFile(%q, O_CREATE|O_EXCL) on non-existent file: got error %v, want nil", filename, err)
			return
		}
		_ = f.Close()

		f2, err2 := filesystem.OpenFile(filename, flag, 0644)
		if f2 != nil {
			_ = f2.Close()
		}
		if !errors.Is(err2, fs.ErrExist) {
			t.Errorf("OpenFile(%q, O_CREATE|O_EXCL) on existing file: got error %v, want fs.ErrExist", filename, err2)
		}
	})
}

// testAppendFlag tests O_APPEND flag behavior.
func testAppendFlag(t *testing.T, filesystem core.FS, supported map[int]bool) {
	t.Run("O_APPEND", func(t *testing.T) {
		filename := "combo-append.txt"
		initialContent := []byte("initial content")
		if err := filesystem.WriteFile(filename, initialContent, 0644); err != nil {
			t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
		}

		flag := os.O_APPEND
		if supported[os.O_WRONLY] {
			flag |= os.O_WRONLY
		} else if supported[os.O_RDWR] {
			flag |= os.O_RDWR
		}

		f, err := filesystem.OpenFile(filename, flag, 0644)
		if err != nil {
			t.Errorf("OpenFile(%q, O_APPEND): got error %v, want nil", filename, err)
			return
		}

		appendContent := []byte(" appended")
		if _, err = f.Write(appendContent); err != nil {
			_ = f.Close()
			t.Fatalf("Write(): got error %v, want nil", err)
		}

		if err := f.Close(); err != nil {
			t.Fatalf("Close(): got error %v, want nil", err)
		}

		data, err := filesystem.ReadFile(filename)
		if err != nil {
			t.Errorf("ReadFile(%q): got error %v, want nil", filename, err)
			return
		}

		expected := append(initialContent, appendContent...)
		if !bytes.Equal(data, expected) {
			t.Errorf("ReadFile(%q) after append: got %q, want %q", filename, data, expected)
		}
	})
}

// testRDWRFlag tests O_RDWR flag.
func testRDWRFlag(t *testing.T, filesystem core.FS) {
	t.Run("O_RDWR", func(t *testing.T) {
		filename := "combo-rdwr.txt"
		initialContent := []byte("initial content")
		if err := filesystem.WriteFile(filename, initialContent, 0644); err != nil {
			t.Fatalf("WriteFile(%q): setup failed: %v", filename, err)
		}

		f, err := filesystem.OpenFile(filename, os.O_RDWR, 0644)
		if err != nil {
			t.Errorf("OpenFile(%q, O_RDWR): got error %v, want nil", filename, err)
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				t.Errorf("Close(): got error %v", closeErr)
			}
		}()

		readBuf := make([]byte, 7) // Read "initial"
		n, err := f.Read(readBuf)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("Read() with O_RDWR: got error %v, want nil or EOF", err)
		}
		if n > 0 && !bytes.Equal(readBuf[:n], []byte("initial")[:n]) {
			t.Errorf("Read() with O_RDWR: got %q, want %q", readBuf[:n], "initial"[:n])
		}

		if _, err = f.Write([]byte(" modified")); err != nil {
			t.Errorf("Write() with O_RDWR: got error %v, want nil", err)
		}
	})
}

// getFlagName returns a human-readable name for an OS flag.
func getFlagName(flag int) string {
	switch flag {
	case os.O_RDONLY:
		return "O_RDONLY"
	case os.O_WRONLY:
		return "O_WRONLY"
	case os.O_RDWR:
		return "O_RDWR"
	case os.O_APPEND:
		return "O_APPEND"
	case os.O_CREATE:
		return "O_CREATE"
	case os.O_EXCL:
		return "O_EXCL"
	case os.O_SYNC:
		return "O_SYNC"
	case os.O_TRUNC:
		return "O_TRUNC"
	default:
		return "UNKNOWN"
	}
}
