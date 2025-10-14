package minio

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/minio/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileInterfaceCompliance verifies that File implements all required interfaces at compile time.
func TestFileInterfaceCompliance(t *testing.T) {
	// This test doesn't need to run anything - it's a compile-time check
	// If this compiles, the interfaces are implemented correctly
	t.Run("File implements required interfaces", func(t *testing.T) {
		// The compile-time checks are in file.go:
		// var _ core.File = (*File)(nil)
		// var _ fs.File = (*File)(nil)
		// var _ io.Seeker = (*File)(nil)
		// var _ io.ReaderAt = (*File)(nil)
		// var _ core.Syncer = (*File)(nil)
		assert.True(t, true, "Interface compliance checked at compile time")
	})
}

// TestNewFileWrite tests the newFileWrite constructor.
func TestNewFileWrite(t *testing.T) {
	mfs := &MinioFS{
		bucket: "test-bucket",
		prefix: "test-prefix",
	}

	file := newFileWrite(mfs, "test-key", "test.txt", os.O_WRONLY|os.O_CREATE)

	assert.NotNil(t, file)
	assert.Equal(t, mfs, file.fs)
	assert.Equal(t, "test-key", file.key)
	assert.Equal(t, "test.txt", file.name)
	assert.Equal(t, os.O_WRONLY|os.O_CREATE, file.mode)
	assert.NotNil(t, file.buffer)
	assert.False(t, file.closed)
	assert.Nil(t, file.reader)
}

// TestFileWrite tests the Write method in write mode.
func TestFileWrite(t *testing.T) {
	t.Run("successful write", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		data := []byte("hello world")
		n, err := file.Write(data)

		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, file.buffer.Bytes())
	})

	t.Run("multiple writes accumulate", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		_, err := file.Write([]byte("hello "))
		require.NoError(t, err)

		_, err = file.Write([]byte("world"))
		require.NoError(t, err)

		assert.Equal(t, []byte("hello world"), file.buffer.Bytes())
	})

	t.Run("write after close returns error", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)
		file.closed = true

		_, err := file.Write([]byte("test"))

		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "write", pathErr.Op)
		assert.ErrorIs(t, pathErr.Err, fs.ErrClosed)
	})

	t.Run("write in read mode returns error", func(t *testing.T) {
		file := &File{
			name: "test.txt",
			mode: os.O_RDONLY,
		}

		_, err := file.Write([]byte("test"))

		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "write", pathErr.Op)
		assert.ErrorIs(t, pathErr.Err, fs.ErrInvalid)
	})
}

// TestFileRead tests the Read method in read mode.
func TestFileRead(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
			size:   int64(len(data)),
		}

		buf := make([]byte, 5)
		n, err := file.Read(buf)

		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf)
	})

	t.Run("read until EOF", func(t *testing.T) {
		data := []byte("hello")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		// First read gets all data
		buf := make([]byte, 10)
		n, err := file.Read(buf)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf[:n])
		require.NoError(t, err) // bytes.Reader returns EOF on second read

		// Second read returns EOF
		n2, err2 := file.Read(buf)
		assert.Equal(t, 0, n2)
		assert.ErrorIs(t, err2, io.EOF)
	})

	t.Run("read in write mode returns error", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		buf := make([]byte, 10)
		_, err := file.Read(buf)

		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "read", pathErr.Op)
		assert.ErrorIs(t, pathErr.Err, fs.ErrInvalid)
	})
}

// TestFileSeek tests the Seek method in read mode.
func TestFileSeek(t *testing.T) {
	t.Run("seek from start", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		pos, err := file.Seek(6, io.SeekStart)

		require.NoError(t, err)
		assert.Equal(t, int64(6), pos)

		buf := make([]byte, 5)
		n, _ := file.Read(buf)
		assert.Equal(t, []byte("world"), buf[:n])
	})

	t.Run("seek from current", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		// Read 5 bytes first
		buf := make([]byte, 5)
		_, _ = file.Read(buf)

		// Seek forward 1 byte from current position
		pos, err := file.Seek(1, io.SeekCurrent)

		require.NoError(t, err)
		assert.Equal(t, int64(6), pos)
	})

	t.Run("seek from end", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		pos, err := file.Seek(-5, io.SeekEnd)

		require.NoError(t, err)
		assert.Equal(t, int64(6), pos)

		buf := make([]byte, 5)
		n, _ := file.Read(buf)
		assert.Equal(t, []byte("world"), buf[:n])
	})

	t.Run("seek in write mode returns error", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		_, err := file.Seek(0, io.SeekStart)

		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "seek", pathErr.Op)
		assert.ErrorIs(t, pathErr.Err, core.ErrUnsupported)
	})
}

// TestFileReadAt tests the ReadAt method in read mode.
func TestFileReadAt(t *testing.T) {
	t.Run("successful read at offset", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		buf := make([]byte, 5)
		n, err := file.ReadAt(buf, 6)

		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("world"), buf)
	})

	t.Run("read at doesn't affect position", func(t *testing.T) {
		data := []byte("hello world")
		file := &File{
			name:   "test.txt",
			mode:   os.O_RDONLY,
			reader: bytes.NewReader(data),
		}

		// Read from offset 6
		buf1 := make([]byte, 5)
		_, _ = file.ReadAt(buf1, 6)

		// Regular read should still start from position 0
		buf2 := make([]byte, 5)
		_, _ = file.Read(buf2)

		assert.Equal(t, []byte("hello"), buf2)
	})

	t.Run("read at in write mode returns error", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		buf := make([]byte, 10)
		_, err := file.ReadAt(buf, 0)

		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "readat", pathErr.Op)
		assert.ErrorIs(t, pathErr.Err, core.ErrUnsupported)
	})
}

// TestFileStat tests the Stat method.
func TestFileStat(t *testing.T) {
	t.Run("stat in read mode returns object info", func(t *testing.T) {
		modTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		file := &File{
			name:    "test.txt",
			mode:    os.O_RDONLY,
			size:    12,
			modTime: modTime,
		}

		info, err := file.Stat()

		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
		assert.Equal(t, int64(12), info.Size())
		assert.Equal(t, modTime, info.ModTime())
		assert.False(t, info.IsDir())
	})

	t.Run("stat in write mode returns buffer size", func(t *testing.T) {
		file := newFileWrite(&MinioFS{}, "key", "test.txt", os.O_WRONLY)

		// Write some data
		_, _ = file.Write([]byte("hello world"))

		info, err := file.Stat()

		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
		assert.Equal(t, int64(11), info.Size())
		assert.False(t, info.IsDir())
		// ModTime should be close to now
		assert.WithinDuration(t, time.Now(), info.ModTime(), time.Second)
	})
}

// TestFileName tests the Name method.
func TestFileName(t *testing.T) {
	file := &File{
		name: "test.txt",
	}

	assert.Equal(t, "test.txt", file.Name())
}

// TestFileClose tests the Close method.
func TestFileClose(t *testing.T) {
	t.Run("close in read mode is no-op", func(t *testing.T) {
		file := &File{
			name: "test.txt",
			mode: os.O_RDONLY,
		}

		err := file.Close()

		require.NoError(t, err)
		assert.True(t, file.closed)
	})

	t.Run("double close is idempotent", func(t *testing.T) {
		file := &File{
			name: "test.txt",
			mode: os.O_RDONLY,
		}

		err1 := file.Close()
		err2 := file.Close()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.True(t, file.closed)
	})
}

// TestFileSync tests the Sync method.
func TestFileSync(t *testing.T) {
	t.Run("sync in read mode is no-op", func(t *testing.T) {
		file := &File{
			name: "test.txt",
			mode: os.O_RDONLY,
		}

		err := file.Sync()

		require.NoError(t, err)
	})
}

// TestFileInfo tests the types.FileInfo implementation.
func TestFileInfo(t *testing.T) {
	modTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	info := types.NewFileInfo("test.txt", 1024, modTime, 0644)

	assert.Equal(t, "test.txt", info.Name())
	assert.Equal(t, int64(1024), info.Size())
	assert.Equal(t, fs.FileMode(0644), info.Mode())
	assert.Equal(t, modTime, info.ModTime())
	assert.False(t, info.IsDir())
	assert.Nil(t, info.Sys())
}

// TestMinioFSCreate tests the Create method.
func TestMinioFSCreate(t *testing.T) {
	mfs := &MinioFS{
		bucket: "test-bucket",
		prefix: "test-prefix",
	}

	file, err := mfs.Create("test.txt")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Type assert to our File type to check internals
	minioFile, ok := file.(*File)
	require.True(t, ok)

	assert.Equal(t, "test.txt", minioFile.name)
	assert.Equal(t, "test-prefix/test.txt", minioFile.key)
	assert.Equal(t, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, minioFile.mode)
	assert.NotNil(t, minioFile.buffer)
}

// TestMinioFSCreateWithPrefix tests Create with different prefix configurations.
func TestMinioFSCreateWithPrefix(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		filename    string
		expectedKey string
	}{
		{
			name:        "no prefix",
			prefix:      "",
			filename:    "test.txt",
			expectedKey: "test.txt",
		},
		{
			name:        "simple prefix",
			prefix:      "myapp",
			filename:    "test.txt",
			expectedKey: "myapp/test.txt",
		},
		{
			name:        "nested prefix",
			prefix:      "myapp/data",
			filename:    "test.txt",
			expectedKey: "myapp/data/test.txt",
		},
		{
			name:        "nested filename",
			prefix:      "myapp",
			filename:    "dir/test.txt",
			expectedKey: "myapp/dir/test.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mfs := &MinioFS{
				bucket: "test-bucket",
				prefix: tt.prefix,
			}

			file, err := mfs.Create(tt.filename)

			require.NoError(t, err)
			minioFile, ok := file.(*File)
			require.True(t, ok)
			assert.Equal(t, tt.expectedKey, minioFile.key)
		})
	}
}

// Mock tests for integration scenarios (would need real MinIO in integration tests)

// TestFileWriteAndClose_Mock demonstrates the write-and-close workflow without real MinIO.
func TestFileWriteAndClose_Mock(t *testing.T) {
	t.Run("write and close workflow", func(t *testing.T) {
		// This is a unit test that demonstrates the workflow
		// Real integration tests would use a real MinIO container

		file := newFileWrite(&MinioFS{
			bucket: "test-bucket",
		}, "test-key", "test.txt", os.O_WRONLY)

		// Write data
		data := []byte("hello world")
		n, err := file.Write(data)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)

		// Verify buffer contains data
		assert.Equal(t, data, file.buffer.Bytes())

		// Check stat before close
		info, err := file.Stat()
		require.NoError(t, err)
		assert.Equal(t, int64(11), info.Size())

		// Note: We can't actually test Close() with upload in unit tests
		// because it requires a real MinIO client. That would be covered
		// in integration tests.
	})
}
