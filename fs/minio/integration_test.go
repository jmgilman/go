package minio

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/fstest"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupMinIOContainer starts a MinIO container and returns endpoint and cleanup function.
func setupMinIOContainer(t *testing.T) (string, func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MinIO container
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
	}

	minioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start MinIO container")

	// Get the endpoint
	endpoint, err := minioC.Endpoint(ctx, "")
	require.NoError(t, err, "failed to get container endpoint")

	// Return cleanup function
	cleanup := func() {
		_ = minioC.Terminate(ctx)
	}

	return endpoint, cleanup
}

// setupMinIOFS creates a fresh MinioFS instance for testing.
func setupMinIOFS(t *testing.T, endpoint string) *MinioFS {
	t.Helper()

	ctx := context.Background()

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	require.NoError(t, err, "failed to create MinIO client")

	// Create unique bucket for this test
	// Use a simple bucket name - MinIO will handle multiple tests
	bucketName := "test-bucket"

	// Try to create the bucket, but don't fail if it already exists
	err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
	if err != nil {
		// Check if bucket already exists (which is fine)
		exists, errBucketExists := client.BucketExists(ctx, bucketName)
		if !exists || errBucketExists != nil {
			require.NoError(t, err, "failed to create test bucket")
		}
	}

	// Create filesystem
	fs, err := NewMinIO(Config{
		Client: client,
		Bucket: bucketName,
	})
	require.NoError(t, err, "failed to create MinioFS")

	return fs
}

// TestMinioConformance runs fstest.TestSuite conformance tests with S3 configuration.
func TestMinioConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	// Run the conformance test suite with S3-specific configuration
	// S3TestConfig handles virtual directories, idempotent delete, and implicit parent directories
	fstest.TestSuiteWithConfig(t, func() core.FS {
		return setupMinIOFS(t, endpoint)
	}, fstest.S3TestConfig())
}

// TestOpenFileFlags tests OpenFile flag support.
func TestOpenFileFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	// Define supported flags for MinIO
	supportedFlags := []int{
		os.O_RDONLY,
		os.O_WRONLY,
		os.O_CREATE,
		os.O_TRUNC,
	}

	fstest.TestOpenFileFlags(t, fs, supportedFlags)
}

// TestLargeFileUpload tests multipart upload for large files.
func TestLargeFileUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	// Create a file larger than multipart threshold (5MB)
	// Use 10MB to ensure multipart upload is triggered
	largeDataSize := 10 * 1024 * 1024 // 10MB
	largeData := make([]byte, largeDataSize)

	// Fill with a pattern so we can verify it
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	t.Run("write large file", func(t *testing.T) {
		err := fs.WriteFile("large-file.bin", largeData, 0644)
		require.NoError(t, err, "should write large file successfully")
	})

	t.Run("read large file back", func(t *testing.T) {
		data, err := fs.ReadFile("large-file.bin")
		require.NoError(t, err, "should read large file successfully")
		assert.Equal(t, len(largeData), len(data), "file size should match")
		assert.Equal(t, largeData, data, "file content should match")
	})

	t.Run("stat large file", func(t *testing.T) {
		info, err := fs.Stat("large-file.bin")
		require.NoError(t, err, "should stat large file successfully")
		assert.Equal(t, int64(largeDataSize), info.Size(), "file size should match")
	})
}

// TestManyObjectsListing tests directory listing with many objects.
func TestManyObjectsListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	// Create many files to test pagination
	numFiles := 150 // More than typical page size (100)

	t.Run("create many files", func(t *testing.T) {
		for i := 0; i < numFiles; i++ {
			filename := fmt.Sprintf("file-%03d.txt", i)
			content := []byte(fmt.Sprintf("content of file %d", i))
			err := fs.WriteFile(filename, content, 0644)
			require.NoError(t, err, "should create file %s", filename)
		}
	})

	t.Run("list all files", func(t *testing.T) {
		entries, err := fs.ReadDir(".")
		require.NoError(t, err, "should list directory successfully")
		assert.GreaterOrEqual(t, len(entries), numFiles, "should list all files")

		// Verify entries are sorted
		for i := 1; i < len(entries); i++ {
			assert.LessOrEqual(t, entries[i-1].Name(), entries[i].Name(),
				"entries should be sorted by name")
		}
	})

	t.Run("verify file contents", func(t *testing.T) {
		// Spot check a few files
		for _, i := range []int{0, 50, 100, 149} {
			filename := fmt.Sprintf("file-%03d.txt", i)
			expectedContent := []byte(fmt.Sprintf("content of file %d", i))

			data, err := fs.ReadFile(filename)
			require.NoError(t, err, "should read file %s", filename)
			assert.Equal(t, expectedContent, data, "file content should match")
		}
	})
}

// TestRenameOperation tests the rename operation (copy+delete).
func TestRenameOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	// Create a test file
	originalName := "original.txt"
	newName := "renamed.txt"
	content := []byte("test content for rename")

	t.Run("create original file", func(t *testing.T) {
		err := fs.WriteFile(originalName, content, 0644)
		require.NoError(t, err, "should create original file")
	})

	t.Run("rename file", func(t *testing.T) {
		err := fs.Rename(originalName, newName)
		require.NoError(t, err, "should rename file successfully")
	})

	t.Run("verify original file removed", func(t *testing.T) {
		exists, err := fs.Exists(originalName)
		require.NoError(t, err, "should check existence")
		assert.False(t, exists, "original file should not exist")
	})

	t.Run("verify new file exists with correct content", func(t *testing.T) {
		data, err := fs.ReadFile(newName)
		require.NoError(t, err, "should read renamed file")
		assert.Equal(t, content, data, "content should match")
	})

	t.Run("rename large file", func(t *testing.T) {
		// Test with a larger file to ensure copy works for large objects
		largeContent := make([]byte, 1024*1024) // 1MB
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		err := fs.WriteFile("large-original.bin", largeContent, 0644)
		require.NoError(t, err, "should create large file")

		err = fs.Rename("large-original.bin", "large-renamed.bin")
		require.NoError(t, err, "should rename large file")

		data, err := fs.ReadFile("large-renamed.bin")
		require.NoError(t, err, "should read renamed large file")
		assert.Equal(t, largeContent, data, "large file content should match")
	})
}

// TestConcurrentAccess tests concurrent file operations.
func TestConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	t.Run("concurrent writes", func(t *testing.T) {
		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()

				filename := fmt.Sprintf("concurrent-%d.txt", id)
				content := []byte(fmt.Sprintf("content from goroutine %d", id))

				err := fs.WriteFile(filename, content, 0644)
				assert.NoError(t, err, "goroutine %d should write successfully", id)
			}(i)
		}

		wg.Wait()

		// Verify all files were created
		for i := 0; i < numGoroutines; i++ {
			filename := fmt.Sprintf("concurrent-%d.txt", i)
			expectedContent := []byte(fmt.Sprintf("content from goroutine %d", i))

			data, err := fs.ReadFile(filename)
			require.NoError(t, err, "should read file %s", filename)
			assert.Equal(t, expectedContent, data, "content should match for file %s", filename)
		}
	})

	t.Run("concurrent reads", func(t *testing.T) {
		// Create a test file
		testContent := []byte("shared content for concurrent reads")
		err := fs.WriteFile("shared.txt", testContent, 0644)
		require.NoError(t, err, "should create shared file")

		const numGoroutines = 20
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()

				data, err := fs.ReadFile("shared.txt")
				assert.NoError(t, err, "goroutine %d should read successfully", id)
				assert.Equal(t, testContent, data, "content should match for goroutine %d", id)
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent mixed operations", func(t *testing.T) {
		const numGoroutines = 15
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()

				filename := fmt.Sprintf("mixed-%d.txt", id)
				content := []byte(fmt.Sprintf("mixed content %d", id))

				// Write
				err := fs.WriteFile(filename, content, 0644)
				assert.NoError(t, err, "goroutine %d should write", id)

				// Stat
				info, err := fs.Stat(filename)
				assert.NoError(t, err, "goroutine %d should stat", id)
				assert.Equal(t, int64(len(content)), info.Size(), "size should match for goroutine %d", id)

				// Read
				data, err := fs.ReadFile(filename)
				assert.NoError(t, err, "goroutine %d should read", id)
				assert.Equal(t, content, data, "content should match for goroutine %d", id)
			}(i)
		}

		wg.Wait()
	})
}

// TestErrorScenarios tests various error conditions.
func TestErrorScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	t.Run("read non-existent file", func(t *testing.T) {
		_, err := fs.ReadFile("non-existent.txt")
		require.Error(t, err, "should error for non-existent file")
		assert.ErrorIs(t, err, os.ErrNotExist, "should return ErrNotExist")
	})

	t.Run("stat non-existent file", func(t *testing.T) {
		_, err := fs.Stat("non-existent.txt")
		require.Error(t, err, "should error for non-existent file")
		assert.ErrorIs(t, err, os.ErrNotExist, "should return ErrNotExist")
	})

	t.Run("open non-existent file for reading", func(t *testing.T) {
		_, err := fs.Open("non-existent.txt")
		require.Error(t, err, "should error for non-existent file")
		assert.ErrorIs(t, err, os.ErrNotExist, "should return ErrNotExist")
	})

	t.Run("remove non-existent file", func(t *testing.T) {
		err := fs.Remove("non-existent.txt")
		// MinIO doesn't error on removing non-existent objects
		// This is standard S3 behavior (idempotent delete)
		assert.NoError(t, err, "removing non-existent file should succeed (S3 behavior)")
	})

	t.Run("rename non-existent file", func(t *testing.T) {
		err := fs.Rename("non-existent.txt", "new-name.txt")
		require.Error(t, err, "should error when renaming non-existent file")
		assert.ErrorIs(t, err, os.ErrNotExist, "should return ErrNotExist")
	})

	t.Run("invalid bucket access", func(t *testing.T) {
		// Create a filesystem with a non-existent bucket
		client, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
			Secure: false,
		})
		require.NoError(t, err)

		invalidFS, err := NewMinIO(Config{
			Client: client,
			Bucket: "non-existent-bucket",
		})
		require.NoError(t, err, "creating FS with invalid bucket should succeed")

		// Operations should fail with bucket not found
		_, err = invalidFS.ReadFile("test.txt")
		require.Error(t, err, "should error for invalid bucket")
		// MinIO returns NoSuchBucket which we translate to ErrNotExist
		assert.ErrorIs(t, err, os.ErrNotExist, "should return ErrNotExist for invalid bucket")
	})

	t.Run("empty file operations", func(t *testing.T) {
		// Create an empty file
		err := fs.WriteFile("empty.txt", []byte{}, 0644)
		require.NoError(t, err, "should create empty file")

		// Read empty file
		data, err := fs.ReadFile("empty.txt")
		require.NoError(t, err, "should read empty file")
		assert.Empty(t, data, "content should be empty")

		// Stat empty file
		info, err := fs.Stat("empty.txt")
		require.NoError(t, err, "should stat empty file")
		assert.Equal(t, int64(0), info.Size(), "size should be 0")
	})

	t.Run("path traversal prevention", func(t *testing.T) {
		// Paths with .. should be normalized
		err := fs.WriteFile("subdir/../file.txt", []byte("content"), 0644)
		require.NoError(t, err, "should normalize path with ..")

		// Verify it was created at the normalized location
		data, err := fs.ReadFile("file.txt")
		require.NoError(t, err, "should read file at normalized path")
		assert.Equal(t, []byte("content"), data, "content should match")
	})
}

// TestNestedDirectories tests operations with nested directory structures.
func TestNestedDirectories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	endpoint, cleanup := setupMinIOContainer(t)
	defer cleanup()

	fs := setupMinIOFS(t, endpoint)

	t.Run("create nested files", func(t *testing.T) {
		// S3 doesn't require explicit directory creation
		err := fs.WriteFile("a/b/c/deep.txt", []byte("deeply nested"), 0644)
		require.NoError(t, err, "should create deeply nested file")

		data, err := fs.ReadFile("a/b/c/deep.txt")
		require.NoError(t, err, "should read deeply nested file")
		assert.Equal(t, []byte("deeply nested"), data, "content should match")
	})

	t.Run("list nested directories", func(t *testing.T) {
		// Create files in nested structure
		files := []string{
			"dir1/file1.txt",
			"dir1/file2.txt",
			"dir1/subdir/file3.txt",
			"dir2/file4.txt",
		}

		for _, f := range files {
			err := fs.WriteFile(f, []byte(f), 0644)
			require.NoError(t, err, "should create file %s", f)
		}

		// List root
		entries, err := fs.ReadDir(".")
		require.NoError(t, err, "should list root directory")
		assert.Greater(t, len(entries), 0, "should have entries in root")

		// List dir1
		entries, err = fs.ReadDir("dir1")
		require.NoError(t, err, "should list dir1")
		// Should have file1.txt, file2.txt, and subdir
		assert.GreaterOrEqual(t, len(entries), 3, "should have at least 3 entries in dir1")
	})

	t.Run("remove nested directories", func(t *testing.T) {
		// Create nested structure
		err := fs.WriteFile("remove/nested/file.txt", []byte("remove me"), 0644)
		require.NoError(t, err, "should create nested file")

		// RemoveAll should remove everything under the path
		err = fs.RemoveAll("remove")
		require.NoError(t, err, "should remove all under remove/")

		// Verify it's gone
		exists, err := fs.Exists("remove/nested/file.txt")
		require.NoError(t, err, "should check existence")
		assert.False(t, exists, "file should not exist after RemoveAll")
	})
}
