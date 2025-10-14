// Package fstest provides a conformance test suite for validating filesystem
// provider implementations against the core.FS interface contracts.
//
// This package contains test functions that can be imported and executed by
// filesystem provider packages to verify they correctly implement the core.FS
// interface and its optional extensions (MetadataFS, SymlinkFS, TempFS).
//
// The test suite is designed to validate interface contracts, not backend-specific
// behavior. Different providers have different capabilities, and the tests verify
// that all providers honor the interface contract while gracefully handling
// documented differences.
//
// Example usage:
//
//	func TestMyProvider(t *testing.T) {
//	    fstest.TestSuite(t, func() core.FS {
//	        return myprovider.New()
//	    })
//	}
package fstest

import (
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// FSTestConfig configures the test suite to match filesystem behavior characteristics.
type FSTestConfig struct {
	// VirtualDirectories indicates directories are virtual (e.g., S3 prefixes).
	// When true, directories don't need to be explicitly created and cannot be stat'd directly.
	VirtualDirectories bool

	// IdempotentDelete indicates delete operations succeed on non-existent files.
	// When true, Remove() on non-existent files returns nil instead of fs.ErrNotExist.
	IdempotentDelete bool

	// ImplicitParentDirs indicates files can be created without parent directories.
	// When true, Create("a/b/c.txt") succeeds even if "a" and "a/b" don't exist.
	ImplicitParentDirs bool

	// SkipTests lists specific test names to skip (for edge cases).
	// Format: "TestGroup/SubTest" (e.g., "WriteFS/CreateInNonExistentDir").
	SkipTests []string
}

// POSIXTestConfig returns configuration for POSIX-like filesystems (local, memory).
func POSIXTestConfig() FSTestConfig {
	return FSTestConfig{
		VirtualDirectories: false,
		IdempotentDelete:   false,
		ImplicitParentDirs: false,
	}
}

// S3TestConfig returns configuration for S3-like filesystems (MinIO, S3).
func S3TestConfig() FSTestConfig {
	return FSTestConfig{
		VirtualDirectories: true,
		IdempotentDelete:   true,
		ImplicitParentDirs: true,
	}
}

// TestSuite runs all applicable conformance tests against a filesystem.
// The newFS function should return a fresh, empty filesystem for each test.
// Tests will create/modify files, so each invocation should start clean.
// Uses POSIXTestConfig() by default.
func TestSuite(t *testing.T, newFS func() core.FS) {
	TestSuiteWithConfig(t, newFS, POSIXTestConfig())
}

// TestSuiteWithSkip runs conformance tests with optional test skipping.
// The skipTests parameter is a slice of test names to skip (e.g., "WriteFS/CreateInNonExistentDir").
// This is useful for providers with known behavioral differences from the standard contract.
// Deprecated: Use TestSuiteWithConfig instead.
func TestSuiteWithSkip(t *testing.T, newFS func() core.FS, skipTests []string) {
	config := POSIXTestConfig()
	config.SkipTests = skipTests
	TestSuiteWithConfig(t, newFS, config)
}

// TestSuiteWithConfig runs conformance tests with behavior configuration.
// The config parameter specifies filesystem behavior characteristics to adapt tests accordingly.
func TestSuiteWithConfig(t *testing.T, newFS func() core.FS, config FSTestConfig) {
	// Helper to check if a test should be skipped
	shouldSkip := func(testName string) bool {
		for _, skip := range config.SkipTests {
			if skip == testName {
				return true
			}
		}
		return false
	}

	// Run all core FS interface tests with fresh filesystem instances
	t.Run("ReadFS", func(t *testing.T) {
		if shouldSkip("ReadFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestReadFSWithConfig(t, newFS(), config)
	})

	t.Run("WriteFS", func(t *testing.T) {
		if shouldSkip("WriteFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestWriteFSWithConfig(t, newFS(), config)
	})

	t.Run("ManageFS", func(t *testing.T) {
		if shouldSkip("ManageFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestManageFSWithConfig(t, newFS(), config)
	})

	t.Run("WalkFS", func(t *testing.T) {
		if shouldSkip("WalkFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestWalkFSWithConfig(t, newFS(), config)
	})

	t.Run("ChrootFS", func(t *testing.T) {
		if shouldSkip("ChrootFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestChrootFSWithConfig(t, newFS(), config)
	})

	// OpenFileFlags test is intentionally not included in TestSuite
	// because it requires provider-specific supportedFlags parameter.
	// Providers should call TestOpenFileFlags directly with their supported flags.

	// Run optional FS-level interface tests
	t.Run("MetadataFS", func(t *testing.T) {
		if shouldSkip("MetadataFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestMetadataFSWithConfig(t, newFS(), config)
	})

	t.Run("SymlinkFS", func(t *testing.T) {
		if shouldSkip("SymlinkFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestSymlinkFSWithConfig(t, newFS(), config)
	})

	t.Run("TempFS", func(t *testing.T) {
		if shouldSkip("TempFS") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestTempFSWithConfig(t, newFS(), config)
	})

	// Run optional File-level capability tests
	t.Run("FileCapabilities", func(t *testing.T) {
		if shouldSkip("FileCapabilities") {
			t.Skip("Skipped by provider configuration")
			return
		}
		TestFileCapabilitiesWithConfig(t, newFS(), config)
	})
}
