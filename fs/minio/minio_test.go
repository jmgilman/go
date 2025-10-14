package minio

import (
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/minio/internal/errs"
	"github.com/jmgilman/go/fs/minio/internal/pathutil"
	"github.com/jmgilman/go/fs/minio/internal/types"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigValidation tests Config.validate() with various scenarios.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with credentials",
			config: Config{
				Endpoint:  "localhost:9000",
				Bucket:    "test-bucket",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				UseSSL:    false,
			},
			wantErr: false,
		},
		{
			name: "valid config with client",
			config: Config{
				Client: &minio.Client{}, // Mock client
				Bucket: "test-bucket",
			},
			wantErr: false,
		},
		{
			name: "missing bucket",
			config: Config{
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
			},
			wantErr: true,
			errMsg:  "bucket is required",
		},
		{
			name: "missing endpoint without client",
			config: Config{
				Bucket:    "test-bucket",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
			},
			wantErr: true,
			errMsg:  "endpoint is required when client is not provided",
		},
		{
			name: "missing access key without client",
			config: Config{
				Endpoint:  "localhost:9000",
				Bucket:    "test-bucket",
				SecretKey: "minioadmin",
			},
			wantErr: true,
			errMsg:  "access key is required when client is not provided",
		},
		{
			name: "missing secret key without client",
			config: Config{
				Endpoint:  "localhost:9000",
				Bucket:    "test-bucket",
				AccessKey: "minioadmin",
			},
			wantErr: true,
			errMsg:  "secret key is required when client is not provided",
		},
		{
			name: "client provided ignores missing credentials",
			config: Config{
				Client: &minio.Client{}, // Mock client
				Bucket: "test-bucket",
				// No Endpoint, AccessKey, SecretKey - should still be valid
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestNewMinIO tests the NewMinIO constructor.
func TestNewMinIO(t *testing.T) {
	t.Run("invalid config returns error", func(t *testing.T) {
		cfg := Config{
			// Missing required fields
			Endpoint: "localhost:9000",
		}
		fs, err := NewMinIO(cfg)
		require.Error(t, err)
		assert.Nil(t, fs)
		assert.Contains(t, err.Error(), "invalid config")
	})

	t.Run("valid config with client", func(t *testing.T) {
		// Note: We use a real client here but don't test connection
		// since we don't have a MinIO server running in unit tests
		cfg := Config{
			Client: &minio.Client{},
			Bucket: "test-bucket",
		}
		fs, err := NewMinIO(cfg)
		require.NoError(t, err)
		require.NotNil(t, fs)
		assert.Equal(t, "test-bucket", fs.bucket)
		assert.Equal(t, "", fs.prefix)
		assert.Equal(t, int64(5*1024*1024), fs.multipartThreshold)
	})

	t.Run("prefix normalization", func(t *testing.T) {
		tests := []struct {
			name           string
			prefix         string
			expectedPrefix string
		}{
			{
				name:           "empty prefix",
				prefix:         "",
				expectedPrefix: "",
			},
			{
				name:           "dot prefix",
				prefix:         ".",
				expectedPrefix: "",
			},
			{
				name:           "simple prefix",
				prefix:         "myapp",
				expectedPrefix: "myapp",
			},
			{
				name:           "prefix with leading slash",
				prefix:         "/myapp/data",
				expectedPrefix: "myapp/data",
			},
			{
				name:           "prefix with trailing slash",
				prefix:         "myapp/data/",
				expectedPrefix: "myapp/data",
			},
			{
				name:           "prefix with both slashes",
				prefix:         "/myapp/data/",
				expectedPrefix: "myapp/data",
			},
			{
				name:           "prefix with backslashes (Windows-style)",
				prefix:         "myapp\\data",
				expectedPrefix: "myapp/data",
			},
			{
				name:           "prefix with dots",
				prefix:         "myapp/../data/./files",
				expectedPrefix: "data/files",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Config{
					Client: &minio.Client{},
					Bucket: "test-bucket",
					Prefix: tt.prefix,
				}
				fs, err := NewMinIO(cfg)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPrefix, fs.prefix)
			})
		}
	})

	t.Run("custom multipart threshold", func(t *testing.T) {
		cfg := Config{
			Client:             &minio.Client{},
			Bucket:             "test-bucket",
			MultipartThreshold: 10 * 1024 * 1024, // 10MB
		}
		fs, err := NewMinIO(cfg)
		require.NoError(t, err)
		assert.Equal(t, int64(10*1024*1024), fs.multipartThreshold)
	})

	t.Run("zero multipart threshold uses default", func(t *testing.T) {
		cfg := Config{
			Client:             &minio.Client{},
			Bucket:             "test-bucket",
			MultipartThreshold: 0,
		}
		fs, err := NewMinIO(cfg)
		require.NoError(t, err)
		assert.Equal(t, int64(5*1024*1024), fs.multipartThreshold)
	})
}

// TestNormalizePrefix tests the normalizePrefix function.
func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "dot",
			input:    ".",
			expected: "",
		},
		{
			name:     "simple path",
			input:    "myapp",
			expected: "myapp",
		},
		{
			name:     "path with forward slashes",
			input:    "myapp/data",
			expected: "myapp/data",
		},
		{
			name:     "path with leading slash",
			input:    "/myapp/data",
			expected: "myapp/data",
		},
		{
			name:     "path with trailing slash",
			input:    "myapp/data/",
			expected: "myapp/data",
		},
		{
			name:     "path with both slashes",
			input:    "/myapp/data/",
			expected: "myapp/data",
		},
		{
			name:     "path with backslashes",
			input:    "myapp\\data\\files",
			expected: "myapp/data/files",
		},
		{
			name:     "path with mixed slashes",
			input:    "myapp/data\\files",
			expected: "myapp/data/files",
		},
		{
			name:     "path with dots",
			input:    "myapp/./data/../files",
			expected: "myapp/files",
		},
		{
			name:     "complex path",
			input:    "/myapp\\data/./files/../config/",
			expected: "myapp/data/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathutil.NormalizePrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestInterfaceCompliance verifies that MinioFS implements core.FS at compile time.
func TestInterfaceCompliance(t *testing.T) {
	// This test doesn't need to run anything - it's a compile-time check
	// If this compiles, the interface is implemented correctly
	t.Run("MinioFS implements core.FS", func(t *testing.T) {
		// The compile-time check is in minio.go:
		// var _ core.FS = (*MinioFS)(nil)
		// If that line compiles, we're good
		assert.True(t, true, "Interface compliance checked at compile time")
	})
}

// TestNormalize tests the normalize function for path normalization.
func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string returns dot",
			input:    "",
			expected: ".",
		},
		{
			name:     "dot remains dot",
			input:    ".",
			expected: ".",
		},
		{
			name:     "simple path",
			input:    "file.txt",
			expected: "file.txt",
		},
		{
			name:     "path with forward slashes",
			input:    "dir/file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "path with leading slash",
			input:    "/dir/file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "path with trailing slash",
			input:    "dir/file.txt/",
			expected: "dir/file.txt",
		},
		{
			name:     "path with both leading and trailing slashes",
			input:    "/dir/file.txt/",
			expected: "dir/file.txt",
		},
		{
			name:     "path with backslashes (Windows-style)",
			input:    "dir\\file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "path with mixed slashes",
			input:    "dir/subdir\\file.txt",
			expected: "dir/subdir/file.txt",
		},
		{
			name:     "path with current directory reference",
			input:    "dir/./file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "path with parent directory reference",
			input:    "dir/subdir/../file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "path with multiple parent directory references",
			input:    "dir/subdir/sub2/../../file.txt",
			expected: "dir/file.txt",
		},
		{
			name:     "complex path with all normalization cases",
			input:    "/dir/./subdir/../another\\file.txt/",
			expected: "dir/another/file.txt",
		},
		{
			name:     "nested path",
			input:    "a/b/c/d/e",
			expected: "a/b/c/d/e",
		},
		{
			name:     "single slash becomes dot",
			input:    "/",
			expected: ".",
		},
		{
			name:     "multiple slashes become dot",
			input:    "///",
			expected: ".",
		},
		{
			name:     "only dots",
			input:    "./././",
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathutil.Normalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJoinPath tests the MinioFS.joinPath method.
func TestJoinPath(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		path     string
		expected string
	}{
		{
			name:     "empty prefix and simple path",
			prefix:   "",
			path:     "file.txt",
			expected: "file.txt",
		},
		{
			name:     "empty prefix and empty path",
			prefix:   "",
			path:     "",
			expected: "",
		},
		{
			name:     "empty prefix and dot path",
			prefix:   "",
			path:     ".",
			expected: "",
		},
		{
			name:     "prefix with simple path",
			prefix:   "myapp",
			path:     "file.txt",
			expected: "myapp/file.txt",
		},
		{
			name:     "prefix with nested path",
			prefix:   "myapp/data",
			path:     "dir/file.txt",
			expected: "myapp/data/dir/file.txt",
		},
		{
			name:     "prefix with empty path",
			prefix:   "myapp",
			path:     "",
			expected: "myapp",
		},
		{
			name:     "prefix with dot path",
			prefix:   "myapp",
			path:     ".",
			expected: "myapp",
		},
		{
			name:     "prefix with leading slash in path",
			prefix:   "myapp",
			path:     "/file.txt",
			expected: "myapp/file.txt",
		},
		{
			name:     "prefix with trailing slash in path",
			prefix:   "myapp",
			path:     "file.txt/",
			expected: "myapp/file.txt",
		},
		{
			name:     "prefix with path containing backslashes",
			prefix:   "myapp",
			path:     "dir\\file.txt",
			expected: "myapp/dir/file.txt",
		},
		{
			name:     "prefix with path containing dots",
			prefix:   "myapp/data",
			path:     "./dir/../file.txt",
			expected: "myapp/data/file.txt",
		},
		{
			name:     "nested prefix with nested path",
			prefix:   "myapp/data/files",
			path:     "subdir/file.txt",
			expected: "myapp/data/files/subdir/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &MinioFS{
				prefix: tt.prefix,
			}
			result := fs.joinPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTranslateError tests the errs.Translate function for error translation.
func TestTranslateError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		err := errs.Translate(nil)
		assert.Nil(t, err)
	})

	t.Run("NoSuchKey maps to ErrNotExist", func(t *testing.T) {
		// Create a MinIO error response with NoSuchKey code
		minioErr := minio.ErrorResponse{
			Code: "NoSuchKey",
		}
		err := errs.Translate(minioErr)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("NoSuchBucket maps to ErrNotExist", func(t *testing.T) {
		minioErr := minio.ErrorResponse{
			Code: "NoSuchBucket",
		}
		err := errs.Translate(minioErr)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("AccessDenied maps to ErrPermission", func(t *testing.T) {
		minioErr := minio.ErrorResponse{
			Code: "AccessDenied",
		}
		err := errs.Translate(minioErr)
		assert.ErrorIs(t, err, fs.ErrPermission)
	})

	t.Run("other MinIO errors are wrapped", func(t *testing.T) {
		minioErr := minio.ErrorResponse{
			Code:    "InternalError",
			Message: "Something went wrong",
		}
		err := errs.Translate(minioErr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "minio:")
		// Should wrap the original error (contains the message)
		assert.Contains(t, err.Error(), "Something went wrong")
	})

	t.Run("non-MinIO errors are wrapped", func(t *testing.T) {
		originalErr := assert.AnError
		err := errs.Translate(originalErr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "minio:")
	})
}

// TestOpenFileFlagValidation tests OpenFile flag validation (unit test).
func TestOpenFileFlagValidation(t *testing.T) {
	tests := []struct {
		name      string
		flag      int
		wantErr   bool
		errString string
	}{
		// Unsupported flags - these should fail early with validation errors
		{
			name:      "O_RDWR is not supported",
			flag:      os.O_RDWR,
			wantErr:   true,
			errString: "O_RDWR not supported",
		},
		{
			name:      "O_APPEND is not supported",
			flag:      os.O_APPEND,
			wantErr:   true,
			errString: "O_APPEND not supported",
		},
		{
			name:      "O_EXCL is not supported",
			flag:      os.O_EXCL,
			wantErr:   true,
			errString: "O_EXCL not supported",
		},
		{
			name:      "O_WRONLY|O_APPEND is not supported",
			flag:      os.O_WRONLY | os.O_APPEND,
			wantErr:   true,
			errString: "O_APPEND not supported",
		},
		{
			name:      "O_RDWR|O_CREATE is not supported",
			flag:      os.O_RDWR | os.O_CREATE,
			wantErr:   true,
			errString: "O_RDWR not supported",
		},
	}

	// Create a minimal MinioFS for testing
	// We only need it to test flag validation, not actual file operations
	mfs := &MinioFS{
		client: &minio.Client{}, // Minimal client, won't be used for unsupported flags
		bucket: "test-bucket",
		prefix: "",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := mfs.OpenFile("test.txt", tt.flag, 0644)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				assert.Nil(t, file)
			} else {
				// This shouldn't happen in this test since we only test unsupported flags
				assert.Fail(t, "unexpected success for unsupported flag")
			}
		})
	}

	// Test supported write flags separately - just verify they pass validation
	// (don't actually try to open files since that requires MinIO server)
	t.Run("supported write flags pass validation", func(t *testing.T) {
		supportedWriteFlags := []struct {
			name string
			flag int
		}{
			{"O_WRONLY", os.O_WRONLY},
			{"O_WRONLY|O_CREATE", os.O_WRONLY | os.O_CREATE},
			{"O_WRONLY|O_TRUNC", os.O_WRONLY | os.O_TRUNC},
			{"O_WRONLY|O_CREATE|O_TRUNC", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
			{"O_CREATE", os.O_CREATE},
			{"O_CREATE|O_TRUNC", os.O_CREATE | os.O_TRUNC},
		}

		for _, sf := range supportedWriteFlags {
			t.Run(sf.name, func(t *testing.T) {
				// For write flags, we can create a file object without MinIO
				file, err := mfs.OpenFile("test.txt", sf.flag, 0644)

				// Should not fail on flag validation
				// (may be nil/err due to missing MinIO, but error should not mention "not supported")
				if err != nil {
					assert.NotContains(t, err.Error(), "not supported in S3",
						"supported flag should not trigger unsupported error")
				} else {
					// If no error, file should be created (write mode returns immediately)
					assert.NotNil(t, file)
					// Don't close the file - that would try to upload to MinIO
					// The test is just about flag validation
				}
			})
		}
	})

	// Note: O_RDONLY and O_TRUNC alone would require a real MinIO server to test
	// since they attempt to read from storage. These are covered in integration tests.
}

// TestS3DirEntry tests the types.S3DirEntry implementation.
func TestS3DirEntry(t *testing.T) {
	t.Run("file entry", func(t *testing.T) {
		modTime := time.Now()
		entry := types.NewS3DirEntry("test.txt", false, 1024, modTime)

		assert.Equal(t, "test.txt", entry.Name())
		assert.False(t, entry.IsDir())
		assert.Equal(t, fs.FileMode(0), entry.Type())

		info, err := entry.Info()
		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
		assert.Equal(t, int64(1024), info.Size())
		assert.Equal(t, fs.FileMode(0644), info.Mode())
		assert.Equal(t, modTime, info.ModTime())
		assert.False(t, info.IsDir())
	})

	t.Run("directory entry", func(t *testing.T) {
		modTime := time.Now()
		entry := types.NewS3DirEntry("subdir", true, 0, modTime)

		assert.Equal(t, "subdir", entry.Name())
		assert.True(t, entry.IsDir())
		assert.Equal(t, fs.ModeDir, entry.Type())

		info, err := entry.Info()
		require.NoError(t, err)
		assert.Equal(t, "subdir", info.Name())
		assert.Equal(t, int64(0), info.Size())
		assert.Equal(t, fs.ModeDir|0755, info.Mode())
		assert.True(t, info.IsDir())
	})
}

// TestMkdir tests the Mkdir method.
func TestMkdir(t *testing.T) {
	mfs := &MinioFS{
		bucket: "test-bucket",
		prefix: "",
	}

	t.Run("mkdir is no-op for S3", func(t *testing.T) {
		err := mfs.Mkdir("newdir", 0755)
		require.NoError(t, err, "Mkdir should always succeed in S3")
	})

	t.Run("mkdir with prefix", func(t *testing.T) {
		mfs.prefix = "myapp"
		err := mfs.Mkdir("subdir", 0755)
		require.NoError(t, err)
	})
}

// TestMkdirAll tests the MkdirAll method.
func TestMkdirAll(t *testing.T) {
	mfs := &MinioFS{
		bucket: "test-bucket",
		prefix: "",
	}

	t.Run("mkdirall is no-op for S3", func(t *testing.T) {
		err := mfs.MkdirAll("a/b/c", 0755)
		require.NoError(t, err, "MkdirAll should always succeed in S3")
	})

	t.Run("mkdirall with prefix", func(t *testing.T) {
		mfs.prefix = "myapp"
		err := mfs.MkdirAll("a/b/c", 0755)
		require.NoError(t, err)
	})
}

// TestChroot tests the Chroot method.
func TestChroot(t *testing.T) {
	t.Run("chroot with no prefix", func(t *testing.T) {
		mfs := &MinioFS{
			client:             &minio.Client{},
			bucket:             "test-bucket",
			prefix:             "",
			multipartThreshold: 5 * 1024 * 1024,
		}

		chrootFS, err := mfs.Chroot("subdir")
		require.NoError(t, err)
		require.NotNil(t, chrootFS)

		// Type assert to MinioFS
		chrootMinioFS, ok := chrootFS.(*MinioFS)
		require.True(t, ok)

		assert.Equal(t, mfs.client, chrootMinioFS.client)
		assert.Equal(t, mfs.bucket, chrootMinioFS.bucket)
		assert.Equal(t, "subdir", chrootMinioFS.prefix)
		assert.Equal(t, mfs.multipartThreshold, chrootMinioFS.multipartThreshold)
	})

	t.Run("chroot with existing prefix", func(t *testing.T) {
		mfs := &MinioFS{
			client:             &minio.Client{},
			bucket:             "test-bucket",
			prefix:             "myapp",
			multipartThreshold: 5 * 1024 * 1024,
		}

		chrootFS, err := mfs.Chroot("data")
		require.NoError(t, err)

		chrootMinioFS, ok := chrootFS.(*MinioFS)
		require.True(t, ok)

		assert.Equal(t, "myapp/data", chrootMinioFS.prefix)
	})

	t.Run("chroot with nested path", func(t *testing.T) {
		mfs := &MinioFS{
			client:             &minio.Client{},
			bucket:             "test-bucket",
			prefix:             "myapp",
			multipartThreshold: 5 * 1024 * 1024,
		}

		chrootFS, err := mfs.Chroot("data/files")
		require.NoError(t, err)

		chrootMinioFS, ok := chrootFS.(*MinioFS)
		require.True(t, ok)

		assert.Equal(t, "myapp/data/files", chrootMinioFS.prefix)
	})

	t.Run("chroot with dot path", func(t *testing.T) {
		mfs := &MinioFS{
			client:             &minio.Client{},
			bucket:             "test-bucket",
			prefix:             "myapp",
			multipartThreshold: 5 * 1024 * 1024,
		}

		chrootFS, err := mfs.Chroot(".")
		require.NoError(t, err)

		chrootMinioFS, ok := chrootFS.(*MinioFS)
		require.True(t, ok)

		// Chroot(".") should keep the same prefix
		assert.Equal(t, "myapp", chrootMinioFS.prefix)
	})
}
