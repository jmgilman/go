package cache

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/billy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBlobCache(t *testing.T) {
	tests := []struct {
		name        string
		storage     *Storage
		defaultTTL  time.Duration
		expectError bool
	}{
		{
			name:        "valid storage and TTL",
			storage:     createTestStorage(t),
			defaultTTL:  time.Hour,
			expectError: false,
		},
		{
			name:        "nil storage",
			storage:     nil,
			defaultTTL:  time.Hour,
			expectError: true,
		},
		{
			name:        "zero TTL uses default",
			storage:     createTestStorage(t),
			defaultTTL:  0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewBlobCache(tt.storage, tt.defaultTTL)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cache)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cache)
			}
		})
	}
}

func TestBlobCache_GetBlob(t *testing.T) {
	cache := createTestBlobCache(t)

	tests := []struct {
		name        string
		digest      string
		setupData   []byte
		expectError bool
		errorMsg    string
	}{
		{
			name:        "existing blob",
			digest:      "sha256:" + sha256Hash([]byte("test data")),
			setupData:   []byte("test data"),
			expectError: false,
		},
		{
			name:        "non-existing blob",
			digest:      "sha256:" + sha256Hash([]byte("non-existing")),
			expectError: true,
			errorMsg:    "blob not found",
		},
		{
			name:        "empty digest",
			digest:      "",
			expectError: true,
		},
		{
			name:        "invalid digest format",
			digest:      "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup data if needed
			if tt.setupData != nil {
				err := cache.PutBlob(ctx, tt.digest, bytes.NewReader(tt.setupData))
				require.NoError(t, err)
			}

			reader, err := cache.GetBlob(ctx, tt.digest)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, reader)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, reader)
				defer reader.Close()

				data, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, tt.setupData, data)
			}
		})
	}
}

func TestBlobCache_PutBlob(t *testing.T) {
	cache := createTestBlobCache(t)

	tests := []struct {
		name        string
		digest      string
		data        []byte
		expectError bool
	}{
		{
			name:        "new blob",
			digest:      "sha256:" + sha256Hash([]byte("test data")),
			data:        []byte("test data"),
			expectError: false,
		},
		{
			name:        "duplicate blob (deduplication)",
			digest:      "sha256:" + sha256Hash([]byte("test data")),
			data:        []byte("test data"),
			expectError: false,
		},
		{
			name:        "empty digest",
			digest:      "",
			data:        []byte("data"),
			expectError: true,
		},
		{
			name:        "nil reader",
			digest:      "sha256:" + sha256Hash([]byte("data")),
			data:        nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var reader io.Reader
			if tt.data != nil {
				reader = bytes.NewReader(tt.data)
			}

			err := cache.PutBlob(ctx, tt.digest, reader)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify blob was stored
				exists, err := cache.HasBlob(ctx, tt.digest)
				assert.NoError(t, err)
				assert.True(t, exists)
			}
		})
	}
}

func TestBlobCache_HasBlob(t *testing.T) {
	cache := createTestBlobCache(t)

	tests := []struct {
		name         string
		digest       string
		setupData    []byte
		expectExists bool
		expectError  bool
	}{
		{
			name:         "existing blob",
			digest:       "sha256:" + sha256Hash([]byte("test")),
			setupData:    []byte("test"),
			expectExists: true,
			expectError:  false,
		},
		{
			name:         "non-existing blob",
			digest:       "sha256:" + sha256Hash([]byte("non-existing")),
			expectExists: false,
			expectError:  false,
		},
		{
			name:        "empty digest",
			digest:      "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if tt.setupData != nil {
				err := cache.PutBlob(ctx, tt.digest, bytes.NewReader(tt.setupData))
				require.NoError(t, err)
			}

			exists, err := cache.HasBlob(ctx, tt.digest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectExists, exists)
			}
		})
	}
}

func TestBlobCache_DeleteBlob(t *testing.T) {
	cache := createTestBlobCache(t)

	tests := []struct {
		name        string
		digest      string
		setupData   []byte
		expectError bool
	}{
		{
			name:        "existing blob",
			digest:      "sha256:" + sha256Hash([]byte("test")),
			setupData:   []byte("test"),
			expectError: false,
		},
		{
			name:        "non-existing blob",
			digest:      "sha256:" + sha256Hash([]byte("non-existing")),
			expectError: false, // Idempotent operation
		},
		{
			name:        "empty digest",
			digest:      "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if tt.setupData != nil {
				err := cache.PutBlob(ctx, tt.digest, bytes.NewReader(tt.setupData))
				require.NoError(t, err)
			}

			err := cache.DeleteBlob(ctx, tt.digest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify blob was deleted
				exists, err := cache.HasBlob(ctx, tt.digest)
				assert.NoError(t, err)
				assert.False(t, exists)
			}
		})
	}
}

func TestBlobCache_Deduplication(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	// Test data
	data := []byte("deduplication test data")
	digest := "sha256:" + sha256Hash(data)

	// Store the same blob multiple times
	for i := 0; i < 3; i++ {
		err := cache.PutBlob(ctx, digest, bytes.NewReader(data))
		assert.NoError(t, err)
	}

	// Verify it still exists and is accessible
	exists, err := cache.HasBlob(ctx, digest)
	assert.NoError(t, err)
	assert.True(t, exists)

	reader, err := cache.GetBlob(ctx, digest)
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	defer reader.Close()

	retrievedData, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, data, retrievedData)
}

func TestBlobCache_ReferenceCounting(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	data := []byte("reference counting test")
	digest := "sha256:" + sha256Hash(data)

	// Store blob multiple times (increases reference count)
	for i := 0; i < 3; i++ {
		err := cache.PutBlob(ctx, digest, bytes.NewReader(data))
		assert.NoError(t, err)
	}

	// Delete blob multiple times (decreases reference count)
	for i := 0; i < 2; i++ {
		err := cache.DeleteBlob(ctx, digest)
		assert.NoError(t, err)

		// Blob should still exist after partial deletions
		exists, err := cache.HasBlob(ctx, digest)
		assert.NoError(t, err)
		assert.True(t, exists)
	}

	// Final deletion should remove the blob
	err := cache.DeleteBlob(ctx, digest)
	assert.NoError(t, err)

	exists, err := cache.HasBlob(ctx, digest)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestBlobCache_Streaming(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	// Use a simple, predictable test data instead of random data
	testData := []byte(
		"This is a test string for streaming operations. It should be exactly the same when read back.",
	)
	digest := "sha256:" + sha256Hash(testData)

	// Store blob
	err := cache.PutBlob(ctx, digest, bytes.NewReader(testData))
	assert.NoError(t, err)

	// Stream the blob back
	reader, err := cache.GetBlob(ctx, digest)
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	defer reader.Close()

	// Read in chunks to simulate streaming
	buffer := make([]byte, 32) // Small chunks to test chunking
	var retrievedData []byte

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			retrievedData = append(retrievedData, buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
	}

	// Debug output
	if !bytes.Equal(testData, retrievedData) {
		t.Logf("Original length: %d", len(testData))
		t.Logf("Retrieved length: %d", len(retrievedData))
		t.Logf("Original hash: %s", sha256Hash(testData))
		t.Logf("Retrieved hash: %s", sha256Hash(retrievedData))
		t.Logf("Expected digest: %s", digest)
	}

	assert.Equal(t, testData, retrievedData)
}

func TestBlobCache_LargeBlob(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large blob test in short mode")
	}

	cache := createTestBlobCache(t)
	ctx := context.Background()

	// Create a very large blob (>100MB)
	const size = 150 * 1024 * 1024 // 150MB
	largeData := make([]byte, size)
	_, err := rand.Read(largeData)
	require.NoError(t, err)

	digest := "sha256:" + sha256Hash(largeData)

	// This should not cause memory exhaustion
	err = cache.PutBlob(ctx, digest, bytes.NewReader(largeData))
	assert.NoError(t, err)

	// Verify blob exists
	exists, err := cache.HasBlob(ctx, digest)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Note: We don't read back the full blob to avoid memory issues in tests
	// In a real scenario, you'd stream it or use a different verification method
}

func TestBlobCache_ConcurrentAccess(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	const numGoroutines = 10
	const numOperations = 5

	data := []byte("concurrent test data")
	digest := "sha256:" + sha256Hash(data)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Mix of operations
				switch j % 4 {
				case 0:
					// Put blob
					err := cache.PutBlob(ctx, digest, bytes.NewReader(data))
					assert.NoError(t, err)
				case 1:
					// Get blob
					reader, err := cache.GetBlob(ctx, digest)
					if err == nil && reader != nil {
						reader.Close()
					}
				case 2:
					// Has blob
					_, err := cache.HasBlob(ctx, digest)
					assert.NoError(t, err)
				case 3:
					// Delete blob
					err := cache.DeleteBlob(ctx, digest)
					assert.NoError(t, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Final state should be consistent
	_, err := cache.HasBlob(ctx, digest)
	assert.NoError(t, err) // No panic or deadlock
}

func TestBlobCache_TTL(t *testing.T) {
	// Create cache with short TTL for testing
	storage := createTestStorage(t)
	cache, err := NewBlobCache(storage, 100*time.Millisecond)
	require.NoError(t, err)

	ctx := context.Background()

	data := []byte("TTL test data")
	digest := "sha256:" + sha256Hash(data)

	// Store blob
	err = cache.PutBlob(ctx, digest, bytes.NewReader(data))
	assert.NoError(t, err)

	// Verify blob exists immediately
	exists, err := cache.HasBlob(ctx, digest)
	assert.NoError(t, err)
	assert.True(t, exists, "Blob should exist immediately after storage")

	// Wait for TTL to expire (wait longer to ensure expiration)
	time.Sleep(500 * time.Millisecond)

	// Blob should be considered expired
	exists, err = cache.HasBlob(ctx, digest)
	assert.NoError(t, err)
	assert.False(t, exists, "Blob should be expired after TTL")
}

func TestBlobCache_IntegrityVerification(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	data := []byte("integrity test data")
	digest := "sha256:" + sha256Hash(data)

	// Store blob
	err := cache.PutBlob(ctx, digest, bytes.NewReader(data))
	assert.NoError(t, err)

	// Retrieve and verify integrity
	reader, err := cache.GetBlob(ctx, digest)
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	defer reader.Close()

	retrievedData, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, data, retrievedData)

	// Verify hash matches
	computedDigest := "sha256:" + sha256Hash(retrievedData)
	assert.Equal(t, digest, computedDigest)
}

func TestBlobCache_PathHandling(t *testing.T) {
	cache := createTestBlobCache(t)
	ctx := context.Background()

	// Test various digest formats and path handling
	testCases := []struct {
		name   string
		data   []byte
		digest string
	}{
		{
			name:   "normal digest",
			data:   []byte("test1"),
			digest: "sha256:" + sha256Hash([]byte("test1")),
		},
		{
			name:   "digest with different hash",
			data:   []byte("test2"),
			digest: "sha256:" + sha256Hash([]byte("test2")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Store blob
			err := cache.PutBlob(ctx, tc.digest, bytes.NewReader(tc.data))
			assert.NoError(t, err)

			// Verify blob exists
			exists, err := cache.HasBlob(ctx, tc.digest)
			assert.NoError(t, err)
			assert.True(t, exists)

			// Retrieve blob
			reader, err := cache.GetBlob(ctx, tc.digest)
			assert.NoError(t, err)
			assert.NotNil(t, reader)
			defer reader.Close()

			retrievedData, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, tc.data, retrievedData)
		})
	}
}

// Helper functions

//nolint:ireturn // test helper function returning interface for consistency
func createTestBlobCache(t *testing.T) BlobCache {
	storage := createTestStorage(t)
	cache, err := NewBlobCache(storage, time.Hour)
	require.NoError(t, err)
	return cache
}

func createTestStorage(t *testing.T) *Storage {
	// Create in-memory filesystem for testing
	fs := billy.NewMemory()

	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	return storage
}

func sha256Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
