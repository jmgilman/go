//go:build integration

// Package ocibundle provides integration tests for the OCI Bundle Distribution Module.
// This file contains tests that verify the client works correctly against real registries.
//
// These tests require Docker to be available and may be skipped if Docker is not running.
// Use the build tag "integration" to run these tests: go test -tags=integration
package ocibundle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/jmgilman/go/oci/internal/testutil"
)

// IntegrationTestSuite contains integration tests for OCI bundle operations
type IntegrationTestSuite struct {
	suite.Suite
	testRegistry *testutil.TestRegistry
	archiveGen   *testutil.ArchiveGenerator
	tempDir      string
}

// SetupSuite is called once before all tests in the suite
func (suite *IntegrationTestSuite) SetupSuite() {
	ctx := context.Background()

	// Skip integration tests if not explicitly requested
	if testing.Short() {
		suite.T().Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		suite.T().Skip("Docker not available, skipping integration tests")
	}

	// Create test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(suite.T(), err, "Failed to create test registry")
	suite.testRegistry = registry

	// Wait for registry to be ready
	err = registry.WaitForReady(ctx, 30*time.Second)
	require.NoError(suite.T(), err, "Registry failed to start")

	// Create archive generator
	archiveGen, err := testutil.NewArchiveGenerator()
	require.NoError(suite.T(), err, "Failed to create archive generator")
	suite.archiveGen = archiveGen

	// Create temp directory for test files
	tempDir, err := os.MkdirTemp("", "oci-integration-test-")
	require.NoError(suite.T(), err, "Failed to create temp directory")
	suite.tempDir = tempDir
}

// createTestClient creates an OCI client configured for the test registry
func (suite *IntegrationTestSuite) createTestClient() (*Client, error) {
	registryRef := suite.testRegistry.Reference()
	registryHost := strings.Split(registryRef, "/")[0] // Extract hostname:port

	return NewWithOptions(WithHTTP(true, true, []string{registryHost}))
}

// TearDownSuite is called once after all tests in the suite
func (suite *IntegrationTestSuite) TearDownSuite() {
	ctx := context.Background()

	if suite.archiveGen != nil {
		suite.archiveGen.Close()
	}

	if suite.testRegistry != nil {
		err := suite.testRegistry.Close(ctx)
		if err != nil {
			suite.T().Logf("Failed to close test registry: %v", err)
		}
	}

	if suite.tempDir != "" {
		os.RemoveAll(suite.tempDir)
	}
}

// TestLocalRegistryPushPull tests basic push and pull operations against the local test registry
func (suite *IntegrationTestSuite) TestLocalRegistryPushPull() {
	ctx := context.Background()
	require := suite.Require()
	assert := suite.Assert()

	// Create a test directory with some files
	sourceDir := filepath.Join(suite.tempDir, "source")
	err := os.MkdirAll(sourceDir, 0o755)
	require.NoError(err)

	// Create some test files
	testFiles := map[string]string{
		"hello.txt":   "Hello, World!",
		"data.json":   `{"key": "value", "number": 42}`,
		"subdir/file": "Content in subdirectory",
	}

	for filePath, content := range testFiles {
		fullPath := filepath.Join(sourceDir, filePath)

		// Create subdirectory if needed
		dir := filepath.Dir(fullPath)
		if dir != sourceDir {
			mkdirErr := os.MkdirAll(dir, 0o755)
			require.NoError(mkdirErr)
		}

		writeErr := os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(writeErr)
	}

	// Create OCI client configured for test registry
	client, err := suite.createTestClient()
	require.NoError(err)

	// Generate a unique reference for this test
	timestamp := time.Now().Unix()
	reference := fmt.Sprintf("%s/test-bundle-%d:latest", suite.testRegistry.Reference(), timestamp)

	// Test push operation
	err = client.Push(ctx, sourceDir, reference)
	assert.NoError(err, "Push operation should succeed")

	// Create target directory for pull
	targetDir := filepath.Join(suite.tempDir, "target")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(err)

	// Test pull operation
	err = client.Pull(ctx, reference, targetDir)
	assert.NoError(err, "Pull operation should succeed")

	// Verify pulled files match original
	for filePath, expectedContent := range testFiles {
		pulledPath := filepath.Join(targetDir, filePath)
		actualContent, err := os.ReadFile(pulledPath)
		assert.NoError(err, "Should be able to read pulled file: %s", filePath)
		assert.Equal(expectedContent, string(actualContent), "File content should match: %s", filePath)
	}
}

// TestLocalRegistryPushWithOptions tests push with various options
func (suite *IntegrationTestSuite) TestLocalRegistryPushWithOptions() {
	ctx := context.Background()
	require := suite.Require()
	assert := suite.Assert()

	// Create a simple test file
	sourceDir := filepath.Join(suite.tempDir, "source-options")
	err := os.MkdirAll(sourceDir, 0o755)
	require.NoError(err)

	err = os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("test content"), 0o644)
	require.NoError(err)

	// Create client configured for test registry
	client, err := suite.createTestClient()
	require.NoError(err)

	// Generate unique reference
	timestamp := time.Now().Unix()
	reference := fmt.Sprintf("%s/test-options-%d:v1.0.0", suite.testRegistry.Reference(), timestamp)

	// Test push with options (annotations)
	err = client.Push(ctx, sourceDir, reference,
		WithAnnotations(map[string]string{
			"version":     "1.0.0",
			"author":      "test-suite",
			"description": "Integration test bundle",
		}),
		WithPlatform("linux/amd64"),
	)
	assert.NoError(err, "Push with options should succeed")

	// Verify we can pull it back
	targetDir := filepath.Join(suite.tempDir, "target-options")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(err)

	err = client.Pull(ctx, reference, targetDir)
	assert.NoError(err, "Pull should succeed after push with options")

	// Verify the file was pulled correctly
	content, err := os.ReadFile(filepath.Join(targetDir, "test.txt"))
	assert.NoError(err)
	assert.Equal("test content", string(content))
}

// TestLocalRegistrySecurityValidation tests that security validators work during pull
func (suite *IntegrationTestSuite) TestLocalRegistrySecurityValidation() {
	ctx := context.Background()
	require := suite.Require()
	assert := suite.Assert()

	// Create a test directory with many files (potential zip bomb scenario)
	sourceDir := filepath.Join(suite.tempDir, "source-security")
	err := os.MkdirAll(sourceDir, 0o755)
	require.NoError(err)

	// Create exactly 100 files (under the default limit of 10000)
	for i := 0; i < 100; i++ {
		fileName := fmt.Sprintf("file-%03d.txt", i)
		content := fmt.Sprintf("Content of file %d", i)
		writeErr := os.WriteFile(filepath.Join(sourceDir, fileName), []byte(content), 0o644)
		require.NoError(writeErr)
	}

	// Push the bundle
	client, err := suite.createTestClient()
	require.NoError(err)

	timestamp := time.Now().Unix()
	reference := fmt.Sprintf("%s/test-security-%d:latest", suite.testRegistry.Reference(), timestamp)

	err = client.Push(ctx, sourceDir, reference)
	assert.NoError(err, "Push should succeed with valid file count")

	// Pull with restrictive limits
	targetDir := filepath.Join(suite.tempDir, "target-security")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(err)

	err = client.Pull(ctx, reference, targetDir,
		WithMaxFiles(200),      // Allow up to 200 files
		WithMaxSize(1024*1024), // 1MB limit
	)
	assert.NoError(err, "Pull should succeed with security limits")

	// Verify all files were pulled
	for i := 0; i < 100; i++ {
		fileName := fmt.Sprintf("file-%03d.txt", i)
		expectedContent := fmt.Sprintf("Content of file %d", i)

		content, err := os.ReadFile(filepath.Join(targetDir, fileName))
		assert.NoError(err, "Should be able to read file %s", fileName)
		assert.Equal(expectedContent, string(content), "File content should match for %s", fileName)
	}
}

// TestLocalRegistryErrorHandling tests error handling scenarios
func (suite *IntegrationTestSuite) TestLocalRegistryErrorHandling() {
	ctx := context.Background()
	assert := suite.Assert()

	// Test pulling non-existent reference
	client, err := suite.createTestClient()
	assert.NoError(err)

	nonExistentRef := fmt.Sprintf("%s/non-existent:latest", suite.testRegistry.Reference())
	targetDir := filepath.Join(suite.tempDir, "non-existent-target")
	err = os.MkdirAll(targetDir, 0o755)
	assert.NoError(err)

	err = client.Pull(ctx, nonExistentRef, targetDir)
	assert.Error(err, "Pulling non-existent reference should fail")

	// Test pushing to non-existent source directory (create a new client for this)
	client2, err := suite.createTestClient()
	assert.NoError(err)
	err = client2.Push(
		ctx,
		"/non/existent/directory",
		fmt.Sprintf("%s/test-error:latest", suite.testRegistry.Reference()),
	)
	assert.Error(err, "Pushing from non-existent directory should fail")
}

// TestLocalRegistryConcurrentOperations tests concurrent push/pull operations
func (suite *IntegrationTestSuite) TestLocalRegistryConcurrentOperations() {
	ctx := context.Background()
	require := suite.Require()
	assert := suite.Assert()

	client, err := suite.createTestClient()
	require.NoError(err)

	// Number of concurrent operations
	numOperations := 5

	// Channel to collect results
	results := make(chan error, numOperations*2) // *2 for push and pull

	// Run concurrent operations
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			// Create unique source directory
			sourceDir := filepath.Join(suite.tempDir, fmt.Sprintf("concurrent-source-%d", id))
			err := os.MkdirAll(sourceDir, 0o755)
			if err != nil {
				e := fmt.Errorf("failed to create source dir: %w", err)
				results <- e
				results <- e // ensure two results are sent for this operation
				return
			}

			// Create test file
			testFile := filepath.Join(sourceDir, "test.txt")
			content := fmt.Sprintf("Concurrent test content %d", id)
			err = os.WriteFile(testFile, []byte(content), 0o644)
			if err != nil {
				e := fmt.Errorf("failed to write test file: %w", err)
				results <- e
				results <- e // ensure two results are sent for this operation
				return
			}

			// Generate unique reference
			reference := fmt.Sprintf("%s/concurrent-test-%d:latest", suite.testRegistry.Reference(), id)

			// Push operation
			err = client.Push(ctx, sourceDir, reference)
			results <- err
			if err != nil {
				// send second result to satisfy expected count (pull is skipped)
				results <- err
				return
			}

			// Create target directory
			targetDir := filepath.Join(suite.tempDir, fmt.Sprintf("concurrent-target-%d", id))
			err = os.MkdirAll(targetDir, 0o755)
			if err != nil {
				results <- fmt.Errorf("failed to create target dir: %w", err)
				return
			}

			// Pull operation
			err = client.Pull(ctx, reference, targetDir)
			results <- err
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numOperations*2; i++ { // *2 for push and pull per operation
		select {
		case err := <-results:
			assert.NoError(err, "Concurrent operation should not fail")
		case <-time.After(60 * time.Second):
			assert.Fail("Concurrent operation timed out")
		}
	}
}

// TestLocalRegistryLargeFiles tests handling of larger files
func (suite *IntegrationTestSuite) TestLocalRegistryLargeFiles() {
	ctx := context.Background()
	require := suite.Require()
	assert := suite.Assert()

	// Create a moderately large file (1MB) for testing
	sourceDir := filepath.Join(suite.tempDir, "source-large")
	err := os.MkdirAll(sourceDir, 0o755)
	require.NoError(err)

	// Generate 1MB of test data
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	err = os.WriteFile(filepath.Join(sourceDir, "large-file.bin"), largeContent, 0o644)
	require.NoError(err)

	// Push and pull the large file
	client, err := suite.createTestClient()
	require.NoError(err)

	timestamp := time.Now().Unix()
	reference := fmt.Sprintf("%s/test-large-%d:latest", suite.testRegistry.Reference(), timestamp)

	// Push
	err = client.Push(ctx, sourceDir, reference)
	assert.NoError(err, "Push of large file should succeed")

	// Pull
	targetDir := filepath.Join(suite.tempDir, "target-large")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(err)

	err = client.Pull(ctx, reference, targetDir)
	assert.NoError(err, "Pull of large file should succeed")

	// Verify the large file was transferred correctly
	pulledContent, err := os.ReadFile(filepath.Join(targetDir, "large-file.bin"))
	assert.NoError(err)
	assert.Equal(largeContent, pulledContent, "Large file content should match")
}

// TestArchiveRoundTrip tests archive creation and extraction without registry operations
func (suite *IntegrationTestSuite) TestArchiveRoundTrip() {
	require := suite.Require()
	assert := suite.Assert()

	// Create a test directory with some files
	sourceDir := filepath.Join(suite.tempDir, "source-roundtrip")
	err := os.MkdirAll(sourceDir, 0o755)
	require.NoError(err)

	// Create some test files
	testFiles := map[string]string{
		"hello.txt":   "Hello, World!",
		"data.json":   `{"key": "value", "number": 42}`,
		"subdir/file": "Content in subdirectory",
	}

	for filePath, content := range testFiles {
		fullPath := filepath.Join(sourceDir, filePath)

		// Create subdirectory if needed
		dir := filepath.Dir(fullPath)
		if dir != sourceDir {
			mkdirErr := os.MkdirAll(dir, 0o755)
			require.NoError(mkdirErr)
		}

		writeErr := os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(writeErr)
	}

	// Create temporary file for archive
	archiveFile := filepath.Join(suite.tempDir, "test-archive.tar.gz")

	// Test archive creation
	archiver := NewTarGzArchiver()

	// Create the archive file
	archiveFileHandle, err := os.Create(archiveFile)
	require.NoError(err)
	defer archiveFileHandle.Close()

	err = archiver.Archive(context.Background(), sourceDir, archiveFileHandle)
	assert.NoError(err, "Archive creation should succeed")

	// Close the file to ensure it's written
	archiveFileHandle.Close()

	// Verify archive file was created
	info, err := os.Stat(archiveFile)
	assert.NoError(err, "Archive file should exist")
	assert.Greater(info.Size(), int64(0), "Archive file should not be empty")

	// Test archive extraction
	targetDir := filepath.Join(suite.tempDir, "target-roundtrip")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(err)

	// Open archive file for reading
	archiveReader, err := os.Open(archiveFile)
	require.NoError(err)
	defer archiveReader.Close()

	err = archiver.Extract(context.Background(), archiveReader, targetDir, ExtractOptions{})
	assert.NoError(err, "Archive extraction should succeed")

	// Verify extracted files match original
	for filePath, expectedContent := range testFiles {
		extractedPath := filepath.Join(targetDir, filePath)
		actualContent, err := os.ReadFile(extractedPath)
		assert.NoError(err, "Should be able to read extracted file: %s", filePath)
		assert.Equal(expectedContent, string(actualContent), "File content should match: %s", filePath)
	}
}

// TestRegistryConnectivity tests that the test registry infrastructure works
func (suite *IntegrationTestSuite) TestRegistryConnectivity() {
	require := suite.Require()

	// Test that registry reference is properly formatted
	ref := suite.testRegistry.Reference()
	require.NotEmpty(ref, "Registry reference should not be empty")
	require.Contains(ref, ":", "Registry reference should contain port")

	// Test that registry URL is accessible
	url := suite.testRegistry.URL()
	require.NotEmpty(url, "Registry URL should not be empty")
	require.Contains(url, "http://", "Registry URL should start with http://")

	// Test registry readiness
	ctx := context.Background()
	err := suite.testRegistry.WaitForReady(ctx, 10*time.Second)
	require.NoError(err, "Registry should be ready within timeout")
}

// TestSuite runs the integration test suite
func TestIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	suite.Run(t, new(IntegrationTestSuite))
}

// BenchmarkLocalRegistryOperations benchmarks push/pull operations
func BenchmarkLocalRegistryOperations(b *testing.B) {
	// This benchmark requires Docker to be available
	if _, err := exec.LookPath("docker"); err != nil {
		b.Skip("Docker not available for benchmark")
	}

	ctx := context.Background()

	// Setup test registry
	registry, err := testutil.NewTestRegistry(ctx)
	if err != nil {
		b.Fatalf("Failed to create test registry: %v", err)
	}
	defer registry.Close(ctx)

	err = registry.WaitForReady(ctx, 30*time.Second)
	if err != nil {
		b.Fatalf("Registry failed to start: %v", err)
	}

	// Setup archive generator
	archiveGen, err := testutil.NewArchiveGenerator()
	if err != nil {
		b.Fatalf("Failed to create archive generator: %v", err)
	}
	defer archiveGen.Close()

	// Create test client
	client, err := New()
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}

	// Create test data directory
	tempDir, err := os.MkdirTemp("", "benchmark-oci-")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourceDir := filepath.Join(tempDir, "source")
	err = os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		b.Fatalf("Failed to create source dir: %v", err)
	}

	// Create test files
	for i := 0; i < 10; i++ {
		fileName := fmt.Sprintf("file-%d.txt", i)
		content := fmt.Sprintf("Benchmark content for file %d", i)
		writeErr := os.WriteFile(filepath.Join(sourceDir, fileName), []byte(content), 0o644)
		if writeErr != nil {
			b.Fatalf("Failed to create test file: %v", writeErr)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reference := fmt.Sprintf("%s/benchmark-%d:latest", registry.Reference(), i)

		// Push operation
		err = client.Push(ctx, sourceDir, reference)
		if err != nil {
			b.Fatalf("Push failed: %v", err)
		}

		// Pull operation
		targetDir := filepath.Join(tempDir, fmt.Sprintf("target-%d", i))
		err = os.MkdirAll(targetDir, 0o755)
		if err != nil {
			b.Fatalf("Failed to create target dir: %v", err)
		}

		err = client.Pull(ctx, reference, targetDir)
		if err != nil {
			b.Fatalf("Pull failed: %v", err)
		}

		// Clean up target directory
		os.RemoveAll(targetDir)
	}
}

// TestClient_SelectiveExtraction tests selective file extraction with glob patterns
func TestClient_SelectiveExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start test registry
	registry, err := testutil.NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry.Close(ctx)

	err = registry.WaitForReady(ctx, 30*time.Second)
	require.NoError(t, err)

	// Create test data with various files
	sourceDir := t.TempDir()
	testFiles := map[string]string{
		"config.json":           `{"app":"test"}`,
		"readme.txt":            "README content",
		"data/file1.json":       `{"data":1}`,
		"data/file2.txt":        "Data 2",
		"data/sub/file3.json":   `{"data":3}`,
		"src/main.go":           "package main",
		"src/util/helper.go":    "package util",
		"test/main_test.go":     "package main",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	// Create client
	client, err := NewWithOptions(
		WithHTTP(true, true, []string{registry.Reference()}),
	)
	require.NoError(t, err)

	// Push the test data
	reference := fmt.Sprintf("%s/selective-test:latest", registry.Reference())
	err = client.Push(ctx, sourceDir, reference)
	require.NoError(t, err)
	t.Logf("Pushed test data to %s", reference)

	// Test 1: Pull only JSON files
	t.Run("extract only JSON files", func(t *testing.T) {
		targetDir := t.TempDir()
		err := client.Pull(ctx, reference, targetDir,
			WithFilesToExtract("**/*.json"),
		)
		require.NoError(t, err)

		// Verify only JSON files were extracted
		assert.FileExists(t, filepath.Join(targetDir, "config.json"))
		assert.FileExists(t, filepath.Join(targetDir, "data/file1.json"))
		assert.FileExists(t, filepath.Join(targetDir, "data/sub/file3.json"))

		// Verify non-JSON files were NOT extracted
		assert.NoFileExists(t, filepath.Join(targetDir, "readme.txt"))
		assert.NoFileExists(t, filepath.Join(targetDir, "data/file2.txt"))
		assert.NoFileExists(t, filepath.Join(targetDir, "src/main.go"))
	})

	// Test 2: Pull only Go files
	t.Run("extract only Go files", func(t *testing.T) {
		targetDir := t.TempDir()
		err := client.Pull(ctx, reference, targetDir,
			WithFilesToExtract("**/*.go"),
		)
		require.NoError(t, err)

		// Verify only Go files were extracted
		assert.FileExists(t, filepath.Join(targetDir, "src/main.go"))
		assert.FileExists(t, filepath.Join(targetDir, "src/util/helper.go"))
		assert.FileExists(t, filepath.Join(targetDir, "test/main_test.go"))

		// Verify non-Go files were NOT extracted
		assert.NoFileExists(t, filepath.Join(targetDir, "config.json"))
		assert.NoFileExists(t, filepath.Join(targetDir, "readme.txt"))
	})

	// Test 3: Pull multiple patterns
	t.Run("extract with multiple patterns", func(t *testing.T) {
		targetDir := t.TempDir()
		err := client.Pull(ctx, reference, targetDir,
			WithFilesToExtract("config.json", "data/*.json"),
		)
		require.NoError(t, err)

		// Verify matched files were extracted
		assert.FileExists(t, filepath.Join(targetDir, "config.json"))
		assert.FileExists(t, filepath.Join(targetDir, "data/file1.json"))

		// Note: data/sub/file3.json should NOT match "data/*.json" (not recursive)
		assert.NoFileExists(t, filepath.Join(targetDir, "data/sub/file3.json"))
		assert.NoFileExists(t, filepath.Join(targetDir, "src/main.go"))
	})

	t.Log("✓ Selective extraction tests passed")
}
