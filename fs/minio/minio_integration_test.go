package minio

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestMinIO creates a MinIO container and returns a configured MinioFS instance.
func setupTestMinIO(t *testing.T) (*MinioFS, func()) {
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

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	require.NoError(t, err, "failed to create MinIO client")

	// Create test bucket
	bucketName := "test-bucket"
	err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
	require.NoError(t, err, "failed to create test bucket")

	// Create filesystem
	fs, err := NewMinIO(Config{
		Client: client,
		Bucket: bucketName,
	})
	require.NoError(t, err, "failed to create MinioFS")

	// Return cleanup function
	cleanup := func() {
		_ = minioC.Terminate(ctx)
	}

	return fs, cleanup
}

// TestIntegration_StreamingFile tests the newStreamingFile function with a real MinIO instance.
func TestIntegration_StreamingFile(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("stream existing object", func(t *testing.T) {
		// Upload test data first
		testData := []byte("hello from minio")
		_, err := fs.client.PutObject(
			ctx,
			fs.bucket,
			"test-file.txt",
			bytes.NewReader(testData),
			int64(len(testData)),
			minio.PutObjectOptions{},
		)
		require.NoError(t, err, "failed to upload test object")

		// Now test newStreamingFile
		file, err := newStreamingFile(ctx, fs, "test-file.txt", "test-file.txt")
		require.NoError(t, err, "newStreamingFile should succeed")
		require.NotNil(t, file, "file should not be nil")
		defer func() {
			_ = file.Close()
		}()

		// Verify file properties
		assert.Equal(t, "test-file.txt", file.name)
		assert.Equal(t, "test-file.txt", file.key)
		assert.False(t, file.closed)
		assert.Equal(t, int64(len(testData)), file.info.Size)
		assert.False(t, file.info.LastModified.IsZero())

		// Read the data and verify
		buf, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, testData, buf)
	})

	t.Run("stream non-existent object returns error", func(t *testing.T) {
		file, err := newStreamingFile(ctx, fs, "non-existent.txt", "non-existent.txt")
		require.Error(t, err, "should return error for non-existent object")
		assert.Nil(t, file)
	})
}

// TestIntegration_FileSync tests the sync method with a real MinIO instance.
func TestIntegration_FileSync(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("upload buffer contents", func(t *testing.T) {
		// Create a file in write mode
		file := newFileWrite(fs, "upload-test.txt", "upload-test.txt", os.O_WRONLY)

		// Write data to buffer
		testData := []byte("data to upload")
		n, err := file.Write(testData)
		require.NoError(t, err)
		assert.Equal(t, len(testData), n)

		// Call sync to upload
		err = file.sync(ctx)
		require.NoError(t, err, "sync should upload successfully")

		// Verify the object exists in MinIO
		obj, err := fs.client.GetObject(ctx, fs.bucket, "upload-test.txt", minio.GetObjectOptions{})
		require.NoError(t, err, "object should exist after sync")
		defer func() { _ = obj.Close() }()

		// Verify the uploaded content
		buf, err := io.ReadAll(obj)
		require.NoError(t, err)
		assert.Equal(t, testData, buf)
	})

	t.Run("sync is idempotent", func(t *testing.T) {
		file := newFileWrite(fs, "idempotent-test.txt", "idempotent-test.txt", os.O_WRONLY)

		// Write data
		testData := []byte("idempotent test")
		_, err := file.Write(testData)
		require.NoError(t, err)

		// Call sync multiple times
		err = file.sync(ctx)
		require.NoError(t, err, "first sync should succeed")

		err = file.sync(ctx)
		require.NoError(t, err, "second sync should succeed")

		err = file.sync(ctx)
		require.NoError(t, err, "third sync should succeed")

		// Verify the object exists and has correct content
		obj, err := fs.client.GetObject(ctx, fs.bucket, "idempotent-test.txt", minio.GetObjectOptions{})
		require.NoError(t, err)
		defer func() { _ = obj.Close() }()

		buf, err := io.ReadAll(obj)
		require.NoError(t, err)
		assert.Equal(t, testData, buf)
	})

	t.Run("sync empty buffer", func(t *testing.T) {
		file := newFileWrite(fs, "empty-test.txt", "empty-test.txt", os.O_WRONLY)

		// Sync without writing anything
		err := file.sync(ctx)
		require.NoError(t, err, "sync should succeed for empty buffer")

		// Verify empty object was created
		stat, err := fs.client.StatObject(ctx, fs.bucket, "empty-test.txt", minio.StatObjectOptions{})
		require.NoError(t, err, "empty object should exist")
		assert.Equal(t, int64(0), stat.Size, "object should be empty")
	})
}

// TestIntegration_FileClose tests the Close method with upload.
func TestIntegration_FileClose(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("close uploads buffer in write mode", func(t *testing.T) {
		file := newFileWrite(fs, "close-test.txt", "close-test.txt", os.O_WRONLY)

		// Write data
		testData := []byte("close should upload this")
		_, err := file.Write(testData)
		require.NoError(t, err)

		// Close should trigger upload
		err = file.Close() //nolint:contextcheck // Close() implements io.Closer and cannot accept context
		require.NoError(t, err, "close should succeed")

		// Verify the object exists
		obj, err := fs.client.GetObject(ctx, fs.bucket, "close-test.txt", minio.GetObjectOptions{})
		require.NoError(t, err)
		defer func() { _ = obj.Close() }()

		buf, err := io.ReadAll(obj)
		require.NoError(t, err)
		assert.Equal(t, testData, buf)
	})

	t.Run("close in read mode doesn't upload", func(t *testing.T) {
		// First create an object
		testData := []byte("read mode test")
		_, err := fs.client.PutObject(
			ctx,
			fs.bucket,
			"read-close-test.txt",
			bytes.NewReader(testData),
			int64(len(testData)),
			minio.PutObjectOptions{},
		)
		require.NoError(t, err)

		// Open in read mode
		file, err := newStreamingFile(ctx, fs, "read-close-test.txt", "read-close-test.txt")
		require.NoError(t, err)

		// Close should release resources
		err = file.Close() //nolint:contextcheck // Close() implements io.Closer and cannot accept context
		require.NoError(t, err, "close should succeed in read mode")

		// Verify object is unchanged
		stat, err := fs.client.StatObject(ctx, fs.bucket, "read-close-test.txt", minio.StatObjectOptions{})
		require.NoError(t, err)
		assert.Equal(t, int64(len(testData)), stat.Size)
	})
}

// TestIntegration_RoundTrip tests the full workflow: Create → Write → Close → Open → Read.
func TestIntegration_RoundTrip(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	testData := []byte("round trip test data")
	filename := "roundtrip.txt"

	// Step 1: Create file
	file, err := fs.Create(filename)
	require.NoError(t, err, "Create should succeed")

	// Step 2: Write data
	n, err := file.Write(testData)
	require.NoError(t, err, "Write should succeed")
	assert.Equal(t, len(testData), n)

	// Step 3: Stat before close (should show buffer size)
	info, err := file.Stat()
	require.NoError(t, err)
	assert.Equal(t, int64(len(testData)), info.Size())

	// Step 4: Close (uploads to MinIO)
	err = file.Close()
	require.NoError(t, err, "Close should succeed")

	// Step 5: Open for reading
	file2, err := fs.Open(filename)
	require.NoError(t, err, "Open should succeed")
	defer func() { _ = file2.Close() }()

	// Step 6: Read data
	buf := make([]byte, len(testData))
	n, err = file2.Read(buf)
	require.NoError(t, err, "Read should succeed")
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buf, "Read data should match written data")

	// Step 7: Stat after reading (should show actual size)
	info2, err := file2.Stat()
	require.NoError(t, err)
	assert.Equal(t, int64(len(testData)), info2.Size())
	assert.False(t, info2.ModTime().IsZero())
}

// TestIntegration_SyncBeforeClose tests calling Sync before Close.
func TestIntegration_SyncBeforeClose(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	testData := []byte("sync before close")
	filename := "sync-test.txt"

	// Create file
	file, err := fs.Create(filename)
	require.NoError(t, err)

	// Write data
	_, err = file.Write(testData)
	require.NoError(t, err)

	// Sync before close
	syncer, ok := file.(interface{ Sync() error })
	require.True(t, ok, "File should implement Sync()")

	err = syncer.Sync()
	require.NoError(t, err, "Sync should succeed")

	// Verify object exists immediately (without close)
	stat, err := fs.client.StatObject(ctx, fs.bucket, filename, minio.StatObjectOptions{})
	require.NoError(t, err, "Object should exist after Sync")
	assert.Equal(t, int64(len(testData)), stat.Size)

	// Close should still succeed (idempotent)
	err = file.Close()
	require.NoError(t, err, "Close after Sync should succeed")
}

// TestIntegration_SeekOperations tests seek operations on read mode files.
func TestIntegration_SeekOperations(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	// Upload test file
	testData := []byte("0123456789abcdefghij")
	_, err := fs.client.PutObject(
		ctx,
		fs.bucket,
		"seek-test.txt",
		bytes.NewReader(testData),
		int64(len(testData)),
		minio.PutObjectOptions{},
	)
	require.NoError(t, err)

	// Open file
	file, err := fs.Open("seek-test.txt")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	// Type assert to get Seek capability
	seeker, ok := file.(interface {
		Seek(offset int64, whence int) (int64, error)
	})
	require.True(t, ok, "File should implement Seek in read mode")

	t.Run("seek to middle and read", func(t *testing.T) {
		pos, err := seeker.Seek(10, 0) // Seek to position 10
		require.NoError(t, err)
		assert.Equal(t, int64(10), pos)

		buf := make([]byte, 5)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("abcde"), buf)
	})

	t.Run("seek relative to current position", func(t *testing.T) {
		// After previous read, position should be at 15
		pos, err := seeker.Seek(-5, 1) // Seek back 5 bytes
		require.NoError(t, err)
		assert.Equal(t, int64(10), pos)
	})

	t.Run("seek from end", func(t *testing.T) {
		pos, err := seeker.Seek(-5, 2) // 5 bytes from end
		require.NoError(t, err)
		assert.Equal(t, int64(15), pos)

		buf := make([]byte, 5)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("fghij"), buf)
	})
}

// TestIntegration_ReadAtOperation tests ReadAt on read mode files.
func TestIntegration_ReadAtOperation(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	ctx := context.Background()

	// Upload test file
	testData := []byte("0123456789abcdefghij")
	_, err := fs.client.PutObject(
		ctx,
		fs.bucket,
		"readat-test.txt",
		bytes.NewReader(testData),
		int64(len(testData)),
		minio.PutObjectOptions{},
	)
	require.NoError(t, err)

	// Open file
	file, err := fs.Open("readat-test.txt")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	// Type assert to get ReadAt capability
	readerAt, ok := file.(interface {
		ReadAt(p []byte, off int64) (n int, err error)
	})
	require.True(t, ok, "File should implement ReadAt in read mode")

	t.Run("read at specific offset", func(t *testing.T) {
		buf := make([]byte, 5)
		n, err := readerAt.ReadAt(buf, 10)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("abcde"), buf)
	})

	t.Run("read at doesn't affect read position", func(t *testing.T) {
		// ReadAt at offset 15
		buf1 := make([]byte, 3)
		n, err := readerAt.ReadAt(buf1, 15)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, []byte("fgh"), buf1)

		// Regular read should still work from beginning
		// (Note: we need to seek to start first since we may have read before)
		if seeker, ok := file.(interface {
			Seek(int64, int) (int64, error)
		}); ok {
			_, _ = seeker.Seek(0, 0)
		}

		buf2 := make([]byte, 5)
		n2, err2 := file.Read(buf2)
		require.NoError(t, err2)
		assert.Equal(t, 5, n2)
		assert.Equal(t, []byte("01234"), buf2)
	})
}

// TestIntegration_WriteModeTiming tests that writes are buffered until close/sync.
func TestIntegration_WriteModeTiming(t *testing.T) {
	fs, cleanup := setupTestMinIO(t)
	defer cleanup()

	testData := []byte("buffered write test")
	filename := "timing-test.txt"

	// Create and write
	file, err := fs.Create(filename)
	require.NoError(t, err)

	_, err = file.Write(testData)
	require.NoError(t, err)

	// Object should NOT exist yet (buffered)
	ctx := context.Background()
	_, err = fs.client.StatObject(ctx, fs.bucket, filename, minio.StatObjectOptions{})
	assert.Error(t, err, "Object should not exist before Close/Sync")

	// Now close to trigger upload
	err = file.Close()
	require.NoError(t, err)

	// Now object should exist
	stat, err := fs.client.StatObject(ctx, fs.bucket, filename, minio.StatObjectOptions{})
	require.NoError(t, err, "Object should exist after Close")
	assert.Equal(t, int64(len(testData)), stat.Size)
}

// TestIntegration_WithPrefix tests file operations with a prefix.
func TestIntegration_WithPrefix(t *testing.T) {
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
	require.NoError(t, err)
	defer func() { _ = minioC.Terminate(ctx) }()

	endpoint, err := minioC.Endpoint(ctx, "")
	require.NoError(t, err)

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	require.NoError(t, err)

	// Create bucket
	bucketName := "test-bucket"
	err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
	require.NoError(t, err)

	// Create filesystem with prefix
	fs, err := NewMinIO(Config{
		Client: client,
		Bucket: bucketName,
		Prefix: "myapp/data",
	})
	require.NoError(t, err)

	// Create file through filesystem
	testData := []byte("prefixed data")
	file, err := fs.Create("test.txt")
	require.NoError(t, err)

	_, err = file.Write(testData)
	require.NoError(t, err)

	err = file.Close()
	require.NoError(t, err)

	// Verify object is stored with prefix
	stat, err := client.StatObject(ctx, bucketName, "myapp/data/test.txt", minio.StatObjectOptions{})
	require.NoError(t, err, "Object should exist with prefix")
	assert.Equal(t, int64(len(testData)), stat.Size)

	// Verify we can read it back
	file2, err := fs.Open("test.txt")
	require.NoError(t, err)
	defer func() { _ = file2.Close() }()

	buf := make([]byte, len(testData))
	n, err := file2.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buf)
}

// Benchmark_FileWrite benchmarks write operations.
func Benchmark_FileWrite(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark in short mode")
	}

	fs, cleanup := setupTestMinIO(&testing.T{})
	defer cleanup()

	testData := []byte("benchmark data for write operations")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file := newFileWrite(fs, "bench-write.txt", "bench-write.txt", os.O_WRONLY)
		_, _ = file.Write(testData)
		_ = file.Close()
	}
}

// Benchmark_FileRead benchmarks read operations with streaming.
func Benchmark_FileRead(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark in short mode")
	}

	fs, cleanup := setupTestMinIO(&testing.T{})
	defer cleanup()

	ctx := context.Background()

	// Setup: create test file
	testData := []byte("benchmark data for read operations")
	_, _ = fs.client.PutObject(
		ctx,
		fs.bucket,
		"bench-read.txt",
		bytes.NewReader(testData),
		int64(len(testData)),
		minio.PutObjectOptions{},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file, _ := newStreamingFile(ctx, fs, "bench-read.txt", "bench-read.txt")
		buf := make([]byte, len(testData))
		_, _ = file.Read(buf)
		_ = file.Close()
	}
}
