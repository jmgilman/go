// Package testutil provides testing utilities for the OCI bundle library.
// This file contains tests for coverage reporting utilities.
package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoverageReporter_BasicFunctionality tests basic coverage reporter functionality.
func TestCoverageReporter_BasicFunctionality(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "coverage-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	reporter, err := NewCoverageReporter(tempDir)
	require.NoError(t, err)
	defer reporter.Close()

	// Test reporter creation
	assert.NotNil(t, reporter)
	assert.Equal(t, tempDir, reporter.outputDir)
	assert.NotEmpty(t, reporter.tempDir)
}

// TestCoverageReporter_FileOperations tests file operations.
func TestCoverageReporter_FileOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "coverage-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	reporter, err := NewCoverageReporter(tempDir)
	require.NoError(t, err)
	defer reporter.Close()

	// Test GetCoverageFiles
	files := reporter.GetCoverageFiles()
	assert.Contains(t, files, "html")
	assert.Contains(t, files, "functions")
	assert.Contains(t, files, "summary")

	// Verify paths contain the output directory
	for _, path := range files {
		assert.Contains(t, path, tempDir)
	}
}

// TestCoverageThresholds tests coverage threshold validation.
func TestCoverageThresholds(t *testing.T) {
	thresholds := DefaultCoverageThresholds()
	assert.Equal(t, 80.0, thresholds.Excellent)
	assert.Equal(t, 60.0, thresholds.Good)
	assert.Equal(t, 40.0, thresholds.Minimum)

	// Test coverage validation
	results := map[string]float64{
		"package1": 85.0, // Excellent
		"package2": 65.0, // Good
		"package3": 45.0, // Warning
		"package4": 25.0, // Critical
	}

	issues := ValidateCoverage(results, thresholds)

	// Should have 3 issues (package2, package3, and package4)
	assert.Len(t, issues, 3)

	// Check specific issues
	foundCritical := false
	foundWarning := false
	foundInfo := false

	for _, issue := range issues {
		switch issue.Package {
		case "package4":
			assert.Equal(t, "critical", issue.Severity)
			foundCritical = true
		case "package3":
			assert.Equal(t, "warning", issue.Severity)
			foundWarning = true
		case "package2":
			assert.Equal(t, "info", issue.Severity)
			foundInfo = true
		}
	}

	assert.True(t, foundCritical, "Should have critical issue for package4")
	assert.True(t, foundWarning, "Should have warning issue for package3")
	assert.True(t, foundInfo, "Should have info issue for package2")
}

// TestFormatBytes tests the byte formatting utility.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{2147483648, "2.00 GB"},
	}

	for _, test := range tests {
		result := formatBytes(test.input)
		assert.Equal(t, test.expected, result, "Failed for input %d", test.input)
	}
}
