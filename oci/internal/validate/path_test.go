package validate

import (
	"strings"
	"testing"
)

// TestNewPathTraversalValidator tests the constructor
func TestNewPathTraversalValidator(t *testing.T) {
	validator := NewPathTraversalValidator()

	if validator.AllowHiddenFiles != false {
		t.Errorf("Expected AllowHiddenFiles to be false by default, got %t", validator.AllowHiddenFiles)
	}

	if validator.RootPath != "" {
		t.Errorf("Expected RootPath to be empty by default, got %q", validator.RootPath)
	}
}

// TestValidatePathSafePaths tests that legitimate paths pass validation
func TestValidatePathSafePaths(t *testing.T) {
	validator := NewPathTraversalValidator()

	safePaths := []string{
		"file.txt",
		"dir/file.txt",
		"dir/subdir/file.txt",
		"normal-file-name.txt",
		"file.with.dots.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"123numeric.txt",
		"file (with parentheses).txt",
		"path/to/file",
		"a/b/c/d/e/f/g/file.txt",
		"file.tar.gz",
		"archive.zip",
		"document.pdf",
		"script.py",
		"config.json",
		"data.yaml",
		"readme.md",
	}

	for _, path := range safePaths {
		t.Run("safe_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err != nil {
				t.Errorf("Expected path %q to be safe, but got error: %v", path, err)
			}
		})
	}
}

// TestValidatePathAbsolutePaths tests rejection of absolute paths
func TestValidatePathAbsolutePaths(t *testing.T) {
	validator := NewPathTraversalValidator()

	absolutePaths := []string{
		"/absolute/path",
		"/usr/bin/file",
		"/home/user/file.txt",
		"C:\\Windows\\file.txt",   // Windows absolute path
		"D:\\folder\\file.txt",    // Windows absolute path
		"\\\\server\\share\\file", // UNC path
	}

	for _, path := range absolutePaths {
		t.Run("absolute_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected absolute path %q to be rejected, but it was accepted", path)
			}
			if err != nil && !contains(err.Error(), "absolute path") {
				t.Errorf("Expected error message to mention 'absolute path', got: %v", err)
			}
		})
	}
}

// TestValidatePathTraversal tests rejection of path traversal attempts
func TestValidatePathTraversal(t *testing.T) {
	validator := NewPathTraversalValidator()

	traversalPaths := []string{
		"../file.txt",
		"../../file.txt",
		"../../../file.txt",
		"dir/../../../file.txt",
		"../../../etc/passwd",
		"..\\file.txt",          // Windows backslash
		"..\\..\\file.txt",      // Windows backslash
		"dir\\..\\..\\file.txt", // Mixed slashes
		"normal/../escape.txt",
		"deep/nested/../../../escape.txt",
	}

	for _, path := range traversalPaths {
		t.Run("traversal_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected path traversal %q to be rejected, but it was accepted", path)
			}
			if err != nil && !contains(err.Error(), "traversal") {
				t.Errorf("Expected error message to mention 'traversal', got: %v", err)
			}
		})
	}
}

// TestValidatePathEncodedTraversal tests rejection of encoded path traversal
func TestValidatePathEncodedTraversal(t *testing.T) {
	validator := NewPathTraversalValidator()

	encodedPaths := []string{
		"..%2ffile.txt",          // %2f = /
		"..%5cfile.txt",          // %5c = \
		"%2e%2e%2ffile.txt",      // %2e = .
		"%2e%2e%5cfile.txt",      // %2e = .
		"%2e%2e/file.txt",        // encoded dots
		"%2e%2e\\file.txt",       // encoded dots
		"..%c0%affile.txt",       // UTF-8 encoded /
		"..%c1%9cfile.txt",       // UTF-8 encoded \
		"normal%2f..%2ffile.txt", // encoded in middle
	}

	for _, path := range encodedPaths {
		t.Run("encoded_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected encoded traversal %q to be rejected, but it was accepted", path)
			}
			if err != nil && !contains(err.Error(), "encoded") {
				t.Errorf("Expected error message to mention 'encoded', got: %v", err)
			}
		})
	}
}

// TestValidatePathHiddenFiles tests rejection of hidden files
func TestValidatePathHiddenFiles(t *testing.T) {
	validator := NewPathTraversalValidator()
	validator.AllowHiddenFiles = false // Explicitly disallow hidden files

	hiddenPaths := []string{
		".hidden",
		".hidden.txt",
		"dir/.hidden",
		".config/file.txt",
		".git/config",
		"normal/.hidden/file.txt",
		".DS_Store",
		".Trashes/file",
		"dir/.hidden/subdir/file.txt",
	}

	for _, path := range hiddenPaths {
		t.Run("hidden_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected hidden file %q to be rejected, but it was accepted", path)
			}
			if err != nil && !contains(err.Error(), "hidden") {
				t.Errorf("Expected error message to mention 'hidden', got: %v", err)
			}
		})
	}
}

// TestValidatePathHiddenFilesAllowed tests that hidden files are allowed when configured
func TestValidatePathHiddenFilesAllowed(t *testing.T) {
	validator := NewPathTraversalValidator()
	validator.AllowHiddenFiles = true // Allow hidden files

	hiddenPaths := []string{
		".hidden",
		".config/file.txt",
		"dir/.hidden/file.txt",
	}

	for _, path := range hiddenPaths {
		t.Run("allowed_hidden_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err != nil {
				t.Errorf("Expected hidden file %q to be allowed, but got error: %v", path, err)
			}
		})
	}
}

// TestValidatePathProblematicCharacters tests rejection of problematic characters
func TestValidatePathProblematicCharacters(t *testing.T) {
	validator := NewPathTraversalValidator()

	problematicPaths := []string{
		"file\x00name.txt", // NUL byte
		"file\x01name.txt", // SOH
		"file\x02name.txt", // STX
		"file\x03name.txt", // ETX
		"file\x04name.txt", // EOT
		"file\x05name.txt", // ENQ
		"file\x06name.txt", // ACK
		"file\x07name.txt", // BEL
		"file\x08name.txt", // BS
		"file\x0bname.txt", // VT
		"file\x0cname.txt", // FF
		"file\x0ename.txt", // SO
		"file\x0fname.txt", // SI
		"file\x10name.txt", // DLE
		"file\x11name.txt", // DC1
		"file\x12name.txt", // DC2
		"file\x13name.txt", // DC3
		"file\x14name.txt", // DC4
		"file\x15name.txt", // NAK
		"file\x16name.txt", // SYN
		"file\x17name.txt", // ETB
		"file\x18name.txt", // CAN
		"file\x19name.txt", // EM
		"file\x1aname.txt", // SUB
		"file\x1bname.txt", // ESC
		"file\x1cname.txt", // FS
		"file\x1dname.txt", // GS
		"file\x1ename.txt", // RS
		"file\x1fname.txt", // US
		"file\x7fname.txt", // DEL
		"file\x80name.txt", // High byte
		"file\x81name.txt", // High byte
	}

	for _, path := range problematicPaths {
		t.Run("problematic_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected problematic path %q to be rejected, but it was accepted", path)
			}
		})
	}
}

// TestValidatePathAllowedCharacters tests that normal characters are allowed
func TestValidatePathAllowedCharacters(t *testing.T) {
	validator := NewPathTraversalValidator()

	allowedPaths := []string{
		"file\tname.txt", // Tab
		"file\nname.txt", // LF
		"file\rname.txt", // CR
		"file name.txt",  // Space
		"file-name.txt",  // Dash
		"file_name.txt",  // Underscore
		"file.name.txt",  // Dot
		"file@name.txt",  // @
		"file+name.txt",  // +
		"file=name.txt",  // =
	}

	for _, path := range allowedPaths {
		t.Run("allowed_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err != nil {
				t.Errorf("Expected allowed path %q to pass validation, but got error: %v", path, err)
			}
		})
	}
}

// TestValidatePathEmpty tests rejection of empty paths
func TestValidatePathEmpty(t *testing.T) {
	validator := NewPathTraversalValidator()

	emptyPaths := []string{
		"",
		"   ",  // whitespace only
		"\t\t", // tabs only
		"\n\n", // newlines only
	}

	for _, path := range emptyPaths {
		t.Run("empty_"+path, func(t *testing.T) {
			err := validator.ValidatePath(path)
			if err == nil {
				t.Errorf("Expected empty path %q to be rejected, but it was accepted", path)
			}
			if err != nil && !contains(err.Error(), "empty") {
				t.Errorf("Expected error message to mention 'empty', got: %v", err)
			}
		})
	}
}

// TestIsPathSafe tests the convenience method
func TestIsPathSafe(t *testing.T) {
	validator := NewPathTraversalValidator()

	safePaths := []string{
		"safe/file.txt",
		"normal/path/file.txt",
		"document.pdf",
	}

	for _, path := range safePaths {
		t.Run("safe_"+path, func(t *testing.T) {
			if !validator.IsPathSafe(path) {
				t.Errorf("Expected path %q to be safe", path)
			}
		})
	}

	unsafePaths := []string{
		"../escape.txt",
		"/absolute/path",
		"file\x00name.txt",
	}

	for _, path := range unsafePaths {
		t.Run("unsafe_"+path, func(t *testing.T) {
			if validator.IsPathSafe(path) {
				t.Errorf("Expected path %q to be unsafe", path)
			}
		})
	}
}

// TestValidateSymlink tests symlink validation
func TestValidateSymlink(t *testing.T) {
	validator := NewPathTraversalValidator()
	validator.RootPath = "/tmp/extract"

	// Test symlink without root path set
	validatorNoRoot := NewPathTraversalValidator()
	err := validatorNoRoot.ValidateSymlink("link.txt", "../target.txt")
	if err == nil {
		t.Error("Expected error when root path is not set")
	}

	// Test valid symlink within root
	err = validator.ValidateSymlink("subdir/link.txt", "target.txt")
	if err != nil {
		t.Errorf("Expected valid symlink to pass, but got error: %v", err)
	}

	// Test symlink that escapes root (should fail)
	err = validator.ValidateSymlink("link.txt", "../../../etc/passwd")
	if err == nil {
		t.Error("Expected symlink escaping root to be rejected")
	}

	// Test symlink pointing to absolute path (should fail)
	err = validator.ValidateSymlink("link.txt", "/etc/passwd")
	if err == nil {
		t.Error("Expected absolute symlink target to be rejected")
	}
}

// TestIsAbsolutePath tests the absolute path detection helper
func TestIsAbsolutePath(t *testing.T) {
	validator := NewPathTraversalValidator()

	absolutePaths := []string{
		"/usr/bin/file",
		"C:\\Windows\\file.txt",
		"D:\\folder\\file.txt",
		"\\\\server\\share\\file",
	}

	for _, path := range absolutePaths {
		t.Run("absolute_"+path, func(t *testing.T) {
			if !validator.isAbsolutePath(path) {
				t.Errorf("Expected path %q to be detected as absolute", path)
			}
		})
	}

	relativePaths := []string{
		"file.txt",
		"dir/file.txt",
		"./file.txt",
		"../file.txt",
		"subdir\\file.txt", // backslash but relative
	}

	for _, path := range relativePaths {
		t.Run("relative_"+path, func(t *testing.T) {
			if validator.isAbsolutePath(path) {
				t.Errorf("Expected path %q to be detected as relative", path)
			}
		})
	}
}

// TestIsWhitespaceOnly tests the whitespace detection helper
func TestIsWhitespaceOnly(t *testing.T) {
	validator := NewPathTraversalValidator()

	whitespacePaths := []string{
		"   ",
		"\t\t",
		"\n\n",
		" \t\n ",
		"\r\n\t ",
	}

	for _, path := range whitespacePaths {
		t.Run("whitespace_"+path, func(t *testing.T) {
			if !validator.isWhitespaceOnly(path) {
				t.Errorf("Expected path %q to be detected as whitespace-only", path)
			}
		})
	}

	nonWhitespacePaths := []string{
		"file.txt",
		" file.txt",
		"file.txt ",
		" \tfile.txt\n ",
		"file\x00name.txt",
	}

	for _, path := range nonWhitespacePaths {
		t.Run("non_whitespace_"+path, func(t *testing.T) {
			if validator.isWhitespaceOnly(path) {
				t.Errorf("Expected path %q to NOT be detected as whitespace-only", path)
			}
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				strings.Contains(strings.ToLower(s), strings.ToLower(substr))))
}
