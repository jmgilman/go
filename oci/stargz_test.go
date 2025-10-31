package ocibundle

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildBlobURL tests blob URL construction
func TestBuildBlobURL(t *testing.T) {
	tests := []struct {
		name        string
		registryURL string
		repository  string
		digest      string
		expected    string
	}{
		{
			name:        "basic URL",
			registryURL: "http://localhost:5000",
			repository:  "myrepo",
			digest:      "sha256:abc123",
			expected:    "http://localhost:5000/v2/myrepo/blobs/sha256:abc123",
		},
		{
			name:        "trailing slash on registry",
			registryURL: "http://localhost:5000/",
			repository:  "myrepo",
			digest:      "sha256:abc123",
			expected:    "http://localhost:5000/v2/myrepo/blobs/sha256:abc123",
		},
		{
			name:        "leading slash on repository",
			registryURL: "http://localhost:5000",
			repository:  "/myrepo",
			digest:      "sha256:abc123",
			expected:    "http://localhost:5000/v2/myrepo/blobs/sha256:abc123",
		},
		{
			name:        "nested repository",
			registryURL: "http://registry.example.com",
			repository:  "myorg/myrepo",
			digest:      "sha256:def456",
			expected:    "http://registry.example.com/v2/myorg/myrepo/blobs/sha256:def456",
		},
		{
			name:        "HTTPS registry",
			registryURL: "https://ghcr.io",
			repository:  "org/repo",
			digest:      "sha256:xyz789",
			expected:    "https://ghcr.io/v2/org/repo/blobs/sha256:xyz789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBlobURL(tt.registryURL, tt.repository, tt.digest)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTestBlobRangeSupport tests Range request detection
func TestTestBlobRangeSupport(t *testing.T) {
	t.Run("supports range requests", func(t *testing.T) {
		// Create a test server that supports Range requests
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader != "" {
				w.Header().Set("Content-Range", "bytes 0-0/100")
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte("A"))
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("full content"))
			}
		}))
		defer server.Close()

		httpClient := &http.Client{Timeout: 5 * time.Second}
		result := testBlobRangeSupport(context.Background(), httpClient, server.URL)
		assert.True(t, result, "Should detect Range support")
	})

	t.Run("does not support range requests", func(t *testing.T) {
		// Create a test server that doesn't support Range requests
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ignore Range header and return 200 OK with full content
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("full content"))
		}))
		defer server.Close()

		httpClient := &http.Client{Timeout: 5 * time.Second}
		result := testBlobRangeSupport(context.Background(), httpClient, server.URL)
		assert.False(t, result, "Should detect no Range support")
	})

	t.Run("network error", func(t *testing.T) {
		httpClient := &http.Client{Timeout: 5 * time.Second}
		result := testBlobRangeSupport(context.Background(), httpClient, "http://invalid-url-that-does-not-exist:99999")
		assert.False(t, result, "Should return false on network error")
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Create a server that delays response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond) // Just enough to trigger timeout
			w.WriteHeader(http.StatusPartialContent)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// Create HTTP client with timeout that respects context
		httpClient := &http.Client{
			Timeout: 200 * time.Millisecond, // Shorter timeout to fail fast
		}
		result := testBlobRangeSupport(ctx, httpClient, server.URL)
		assert.False(t, result, "Should return false on context cancellation")
	})
}

// TestReaderAtFromSeeker tests the ReaderAt adapter
func TestReaderAtFromSeeker(t *testing.T) {
	t.Run("basic read at", func(t *testing.T) {
		// Create a simple in-memory seeker
		data := []byte("Hello, World! This is test data.")
		seeker := bytes.NewReader(data)

		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		// Read from offset 7
		buf := make([]byte, 5)
		n, err := adapter.ReadAt(buf, 7)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "World", string(buf))
	})

	t.Run("read at beginning", func(t *testing.T) {
		data := []byte("Hello, World!")
		seeker := bytes.NewReader(data)
		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		buf := make([]byte, 5)
		n, err := adapter.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "Hello", string(buf))
	})

	t.Run("read at end", func(t *testing.T) {
		data := []byte("Hello, World!")
		seeker := bytes.NewReader(data)
		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		buf := make([]byte, 6)
		n, err := adapter.ReadAt(buf, int64(len(data)-6))
		require.NoError(t, err)
		assert.Equal(t, 6, n)
		assert.Equal(t, "World!", string(buf))
	})

	t.Run("size method", func(t *testing.T) {
		data := []byte("Hello, World!")
		seeker := bytes.NewReader(data)
		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		assert.Equal(t, int64(len(data)), adapter.Size())
	})

	t.Run("concurrent reads", func(t *testing.T) {
		data := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		seeker := bytes.NewReader(data)
		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		// Launch multiple concurrent reads
		const numReads = 10
		results := make(chan error, numReads)

		for i := 0; i < numReads; i++ {
			offset := int64(i * 3)
			go func(off int64) {
				buf := make([]byte, 3)
				_, err := adapter.ReadAt(buf, off)
				results <- err
			}(offset)
		}

		// Check all reads succeeded
		for i := 0; i < numReads; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent read %d should succeed", i)
		}
	})

	t.Run("read beyond end", func(t *testing.T) {
		data := []byte("Hello")
		seeker := bytes.NewReader(data)
		adapter := newReaderAtFromSeeker(seeker, int64(len(data)))

		buf := make([]byte, 10)
		_, err := adapter.ReadAt(buf, 0)
		assert.Error(t, err, "Reading beyond end should error")
		assert.Equal(t, io.ErrUnexpectedEOF, err)
	})
}

// TestTOCOnlyReaderAt tests the virtual ReaderAt for TOC and footer
func TestTOCOnlyReaderAt(t *testing.T) {
	// Create sample TOC and footer data
	tocData := []byte("This is the TOC data")
	footerData := []byte("Footer")
	tocOffset := int64(100)
	blobSize := int64(200)

	reader := &tocOnlyReaderAt{
		tocData:    tocData,
		tocOffset:  tocOffset,
		footerData: footerData,
		blobSize:   blobSize,
	}

	t.Run("read from TOC region", func(t *testing.T) {
		buf := make([]byte, 10)
		n, err := reader.ReadAt(buf, tocOffset)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, "This is th", string(buf))
	})

	t.Run("read from footer region", func(t *testing.T) {
		footerStart := blobSize - int64(len(footerData))
		buf := make([]byte, 6)
		n, err := reader.ReadAt(buf, footerStart)
		require.NoError(t, err)
		assert.Equal(t, 6, n)
		assert.Equal(t, "Footer", string(buf))
	})

	t.Run("read from unknown region returns zeros", func(t *testing.T) {
		// Reading from a region we don't have (before TOC, after footer start)
		buf := make([]byte, 5)
		n, err := reader.ReadAt(buf, 0) // Read from beginning
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		// Should be zeros
		assert.Equal(t, []byte{0, 0, 0, 0, 0}, buf)
	})

	t.Run("read partial TOC at end", func(t *testing.T) {
		// Read near the end of TOC
		tocEnd := tocOffset + int64(len(tocData))
		readStart := tocEnd - 5
		buf := make([]byte, 10) // Request more than available
		n, err := reader.ReadAt(buf, readStart)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 5, n) // Only 5 bytes available
		assert.Equal(t, " data", string(buf[:n]))
	})
}

// TestGetBlobURLFromRepository tests blob URL extraction from repository
// Note: This is a basic test; full integration tests are in tests/stargz_range_test.go
func TestGetBlobURLFromRepository(t *testing.T) {
	t.Run("basic functionality placeholder", func(t *testing.T) {
		// This function requires a real *remote.Repository which is complex to mock
		// The actual functionality is tested in integration tests
		// Here we just verify the function signature exists
		assert.NotNil(t, getBlobURLFromRepository, "getBlobURLFromRepository should exist")
	})
}
