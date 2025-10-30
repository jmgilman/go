// Package validate provides path and content validation functionality.
// This package contains security validators for archive extraction to prevent
// path traversal attacks and other security vulnerabilities.
package validate

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathTraversalValidator validates file paths to prevent security vulnerabilities.
// It detects and rejects various forms of path traversal attacks and other
// problematic path patterns that could compromise archive extraction security.
type PathTraversalValidator struct {
	// AllowHiddenFiles determines whether hidden files (starting with .) are allowed
	AllowHiddenFiles bool

	// RootPath is the extraction root directory used for symlink validation
	RootPath string
}

// NewPathTraversalValidator creates a new PathTraversalValidator with default settings.
func NewPathTraversalValidator() *PathTraversalValidator {
	return &PathTraversalValidator{
		AllowHiddenFiles: false, // Default to rejecting hidden files
		RootPath:         "",    // Root path can be set later if needed
	}
}

// ValidatePath validates a file path for security issues.
// It checks for path traversal attempts, absolute paths, and other security concerns.
// Returns nil if the path is safe, or an error describing the security violation.
func (v *PathTraversalValidator) ValidatePath(path string) error {
	// Check for nil or empty path
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// Check for whitespace-only paths
	if v.isWhitespaceOnly(path) {
		return fmt.Errorf("empty path")
	}

	// Check for absolute paths (including Windows and UNC paths)
	if v.isAbsolutePath(path) {
		return fmt.Errorf("absolute path not allowed: %s", path)
	}

	// Check for path traversal attempts
	if err := v.detectPathTraversal(path); err != nil {
		return err
	}

	// Check for problematic characters
	if err := v.detectProblematicCharacters(path); err != nil {
		return err
	}

	// Check for hidden files if not allowed
	if !v.AllowHiddenFiles && v.isHiddenFile(path) {
		return fmt.Errorf("hidden files not allowed: %s", path)
	}

	return nil
}

// detectPathTraversal detects various forms of path traversal attacks.
// This includes:
// - Direct .. in paths
// - Encoded variants (..%2f, ..%5c, etc.)
// - Unicode variants
// - Multiple .. sequences
func (v *PathTraversalValidator) detectPathTraversal(path string) error {
	// Check for encoded traversal attempts first (before cleaning)
	if v.hasEncodedTraversal(path) {
		return fmt.Errorf("encoded path traversal detected: %s", path)
	}

	// Clean the path to resolve any . or .. components
	cleanPath := filepath.Clean(path)

	// Check if the cleaned path tries to go outside the root
	if strings.HasPrefix(cleanPath, "..") {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	// Check for .. sequences (including in backslash paths)
	if v.containsPathTraversal(path) {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	return nil
}

// hasEncodedTraversal checks for URL-encoded path traversal attempts.
func (v *PathTraversalValidator) hasEncodedTraversal(path string) bool {
	lowerPath := strings.ToLower(path)

	// Common URL encoding variants of ..
	encodedVariants := []string{
		"..%2f", "..%5c", // / and \
		"%2e%2e%2f", "%2e%2e%5c", // ..//
		"%2e%2e/", "%2e%2e\\", // .. with encoded dots
		"..%c0%af", "..%c1%9c", // UTF-8 encoded / and \
	}

	for _, variant := range encodedVariants {
		if strings.Contains(lowerPath, variant) {
			return true
		}
	}

	return false
}

// containsPathTraversal checks for .. sequences in paths, including backslash paths.
func (v *PathTraversalValidator) containsPathTraversal(path string) bool {
	// Check for .. in forward slash paths
	if strings.Contains(path, "..") {
		parts := strings.Split(path, "/")
		for _, part := range parts {
			if part == ".." {
				return true
			}
		}
	}

	// Check for .. in backslash paths
	if strings.Contains(path, "..") {
		parts := strings.Split(path, "\\")
		for _, part := range parts {
			if part == ".." {
				return true
			}
		}
	}

	return false
}

// detectProblematicCharacters checks for characters that could cause issues.
func (v *PathTraversalValidator) detectProblematicCharacters(path string) error {
	// Check for NUL bytes
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("NUL byte detected in path: %s", path)
	}

	// Check for control characters (except newline in filenames)
	for _, r := range path {
		if r < 32 && r != 9 && r != 10 && r != 13 { // Allow tab, LF, CR
			return fmt.Errorf("control character detected in path: %s (U+%04X)", path, r)
		}
		// Check for DEL character and high bytes that could cause issues
		if r == 127 || r > 127 { // DEL (127) and any character above ASCII
			return fmt.Errorf("problematic character detected in path: %s (U+%04X)", path, r)
		}
	}

	// Check for other problematic characters
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("null byte in path: %s", path)
	}

	return nil
}

// isHiddenFile checks if a path represents a hidden file or directory.
func (v *PathTraversalValidator) isHiddenFile(path string) bool {
	// Check if any component starts with .
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

// ValidateSymlink validates a symlink target to ensure it doesn't escape the root.
// This is crucial for preventing symlink-based directory traversal attacks.
func (v *PathTraversalValidator) ValidateSymlink(linkPath, targetPath string) error {
	if v.RootPath == "" {
		// If no root path is set, we can't validate symlinks properly
		return fmt.Errorf("root path not set for symlink validation")
	}

	// First, check if the target path itself is absolute or contains traversal
	if v.isAbsolutePath(targetPath) {
		return fmt.Errorf("symlink target is absolute path: %s -> %s", linkPath, targetPath)
	}

	if err := v.detectPathTraversal(targetPath); err != nil {
		return fmt.Errorf("symlink target contains path traversal: %s -> %s: %w", linkPath, targetPath, err)
	}

	// Resolve the symlink target relative to the link's directory
	linkDir := filepath.Dir(linkPath)
	if linkDir == "." && !strings.Contains(linkPath, "/") {
		// If link is in root directory, use root as linkDir
		linkDir = ""
	}

	var resolvedTarget string
	if linkDir == "" {
		resolvedTarget = targetPath
	} else {
		resolvedTarget = filepath.Join(linkDir, targetPath)
	}
	resolvedTarget = filepath.Clean(resolvedTarget)

	// For validation, we need to simulate the extraction directory structure
	// The key is to ensure that even after resolving the symlink,
	// the final target doesn't escape our intended extraction root

	// Get absolute paths for comparison
	rootAbs, err := filepath.Abs(v.RootPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute root path: %w", err)
	}

	// Simulate the full extraction path
	fullExtractPath := filepath.Join(v.RootPath, resolvedTarget)
	targetAbs, err := filepath.Abs(fullExtractPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute target path: %w", err)
	}

	// Check if target is within root
	if !strings.HasPrefix(targetAbs, rootAbs) {
		return fmt.Errorf(
			"symlink target escapes root directory: %s -> %s (resolved: %s)",
			linkPath,
			targetPath,
			resolvedTarget,
		)
	}

	return nil
}

// isAbsolutePath checks for absolute paths on all platforms including Windows and UNC paths.
func (v *PathTraversalValidator) isAbsolutePath(path string) bool {
	// Check standard Go absolute path detection first
	if filepath.IsAbs(path) {
		return true
	}

	// Check for Windows drive letters (C:, D:, etc.)
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		drive := path[0]
		if (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z') {
			return true
		}
	}

	// Check for UNC paths (\\server\share)
	if strings.HasPrefix(path, "\\\\") {
		return true
	}

	return false
}

// isWhitespaceOnly checks if a path contains only whitespace characters.
func (v *PathTraversalValidator) isWhitespaceOnly(path string) bool {
	for _, r := range path {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// IsPathSafe is a convenience method that returns true if the path is safe.
func (v *PathTraversalValidator) IsPathSafe(path string) bool {
	return v.ValidatePath(path) == nil
}
