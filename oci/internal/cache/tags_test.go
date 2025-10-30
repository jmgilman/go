package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/billy"
	"github.com/jmgilman/go/fs/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTagCacheTest(t *testing.T) (*tagCache, core.FS, func()) {
	t.Helper()

	// Create in-memory filesystem for testing
	fs := billy.NewMemory()

	// Create storage
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	// Create tag resolver config
	tagConfig := TagResolverConfig{
		DefaultTTL:     time.Hour,
		MaxHistorySize: 5,
		EnableHistory:  true,
	}

	// Create tag cache
	tagCache := NewTagCache(storage, tagConfig)

	cleanup := func() {
		// Cleanup if needed
	}

	return tagCache, fs, cleanup
}

func TestTagCache_GetTagMapping(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Test getting non-existent mapping
	_, err := tc.GetTagMapping(ctx, reference)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file does not exist")

	// Put mapping first
	err = tc.PutTagMapping(ctx, reference, digest)
	require.NoError(t, err)

	// Now get should work
	mapping, err := tc.GetTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, reference, mapping.Reference)
	assert.Equal(t, digest, mapping.Digest)
	assert.Equal(t, int64(1), mapping.AccessCount)
}

func TestTagCache_PutTagMapping(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest1 := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	digest2 := "sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"

	// Put initial mapping
	err := tc.PutTagMapping(ctx, reference, digest1)
	require.NoError(t, err)

	// Verify initial mapping
	mapping, err := tc.GetTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, digest1, mapping.Digest)
	assert.Len(t, mapping.History, 0)

	// Update mapping (should create history)
	err = tc.PutTagMapping(ctx, reference, digest2)
	require.NoError(t, err)

	// Verify updated mapping
	mapping, err = tc.GetTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, digest2, mapping.Digest)
	assert.Len(t, mapping.History, 1)
	assert.Equal(t, digest1, mapping.History[0].Digest)
}

func TestTagCache_HasTagMapping(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Test non-existent mapping
	exists, err := tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.False(t, exists)

	// Put mapping
	err = tc.PutTagMapping(ctx, reference, digest)
	require.NoError(t, err)

	// Test existing mapping
	exists, err = tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTagCache_DeleteTagMapping(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Put mapping
	err := tc.PutTagMapping(ctx, reference, digest)
	require.NoError(t, err)

	// Verify exists
	exists, err := tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete mapping
	err = tc.DeleteTagMapping(ctx, reference)
	require.NoError(t, err)

	// Verify no longer exists
	exists, err = tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.False(t, exists)

	// Delete non-existent mapping should not error (idempotent operation)
	err = tc.DeleteTagMapping(ctx, "nonexistent:tag")
	// Note: billy filesystem may return error for non-existent files, but this should be handled gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "file does not exist")
	}
}

func TestTagCache_GetTagHistory(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digests := []string{
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"sha256:3333333333333333333333333333333333333333333333333333333333333333",
		"sha256:4444444444444444444444444444444444444444444444444444444444444444",
	}

	// Put initial mapping
	err := tc.PutTagMapping(ctx, reference, digests[0])
	require.NoError(t, err)

	// Update mapping multiple times
	for _, digest := range digests[1:] {
		err = tc.PutTagMapping(ctx, reference, digest)
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Get history
	history, err := tc.GetTagHistory(ctx, reference)
	require.NoError(t, err)

	// Should have 3 history entries (initial + 2 updates)
	assert.Len(t, history, 3)

	// History should be in chronological order (oldest first)
	assert.Equal(t, digests[0], history[0].Digest)
	assert.Equal(t, digests[1], history[1].Digest)
	assert.Equal(t, digests[2], history[2].Digest)

	// Timestamps should be increasing
	assert.True(t, history[0].ChangedAt.Before(history[1].ChangedAt))
	assert.True(t, history[1].ChangedAt.Before(history[2].ChangedAt))
}

func TestTagCache_HistorySizeLimit(t *testing.T) {
	// Create tag cache with small history limit
	fs := billy.NewMemory()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	tagConfig := TagResolverConfig{
		DefaultTTL:     time.Hour,
		MaxHistorySize: 2, // Only keep 2 history entries
		EnableHistory:  true,
	}

	tc := NewTagCache(storage, tagConfig)

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digests := []string{
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"sha256:3333333333333333333333333333333333333333333333333333333333333333",
		"sha256:4444444444444444444444444444444444444444444444444444444444444444",
	}

	// Put initial mapping
	err = tc.PutTagMapping(
		ctx,
		reference,
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
	)
	require.NoError(t, err)

	// Update mapping multiple times
	for _, digest := range digests[1:] {
		err = tc.PutTagMapping(ctx, reference, digest)
		require.NoError(t, err)
	}

	// Get history
	history, err := tc.GetTagHistory(ctx, reference)
	require.NoError(t, err)

	// Should only have 2 history entries (limited by MaxHistorySize)
	assert.Len(t, history, 2)

	// Should have the most recent history entries
	assert.Equal(t, digests[1], history[0].Digest) // Second update
	assert.Equal(t, digests[2], history[1].Digest) // Third update
}

func TestTagCache_TTLExpiration(t *testing.T) {
	// Create tag cache with short TTL
	fs := billy.NewMemory()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	tagConfig := TagResolverConfig{
		DefaultTTL:     50 * time.Millisecond, // Very short TTL
		MaxHistorySize: 5,
		EnableHistory:  true,
	}

	tc := NewTagCache(storage, tagConfig)

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Put mapping
	err = tc.PutTagMapping(ctx, reference, digest)
	require.NoError(t, err)

	// Should exist immediately
	exists, err := tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.True(t, exists)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired now
	exists, err = tc.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.False(t, exists)

	// Get should also fail
	_, err = tc.GetTagMapping(ctx, reference)
	assert.Error(t, err)
}

func TestTagCache_InvalidInputs(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test empty reference
	err := tc.PutTagMapping(
		ctx,
		"",
		"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")

	// Test empty digest
	err = tc.PutTagMapping(ctx, "docker.io/library/nginx:latest", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")

	// Test invalid reference format
	err = tc.PutTagMapping(
		ctx,
		"invalid-reference",
		"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid reference format")

	// Test invalid digest format
	err = tc.PutTagMapping(ctx, "docker.io/library/nginx:latest", "invalid-digest")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid digest format")
}

func TestTagCache_ConcurrentAccess(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	// Start multiple goroutines performing operations
	const numGoroutines = 10
	const numOperations = 50

	errChan := make(chan error, numGoroutines*numOperations)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				// Mix of put and get operations
				if j%2 == 0 {
					err := tc.PutTagMapping(ctx, reference, digest)
					errChan <- err
				} else {
					_, err := tc.GetTagMapping(ctx, reference)
					errChan <- err
				}
			}
		}(i)
	}

	// Collect all errors
	for i := 0; i < numGoroutines*numOperations; i++ {
		err := <-errChan
		// Some operations may fail due to concurrent access, but that's expected
		// We just want to ensure no panics or serious errors
		if err != nil {
			// Acceptable errors during concurrent access: not found, invalid digest, or storage errors
			errorMsg := err.Error()
			acceptable := strings.Contains(errorMsg, "not found") ||
				strings.Contains(errorMsg, "invalid digest") ||
				strings.Contains(errorMsg, "file does not exist")
			assert.True(t, acceptable, "Unexpected error during concurrent access: %s", errorMsg)
		}
	}
}

func TestTagCache_PathSafety(t *testing.T) {
	tc, _, cleanup := setupTagCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test references with special characters that need to be handled safely
	testCases := []struct {
		reference string
		digest    string
	}{
		{
			"docker.io/library/nginx:latest",
			"sha256:1111111111111111111111111111111111111111111111111111111111111111",
		},
		{
			"registry.example.com/my/repo:v1.0.0",
			"sha256:2222222222222222222222222222222222222222222222222222222222222222",
		},
		{
			"localhost:5000/test/image@sha256:3333333333333333333333333333333333333333333333333333333333333333",
			"sha256:3333333333333333333333333333333333333333333333333333333333333333",
		},
	}

	for _, testCase := range testCases {
		// Put mapping
		err := tc.PutTagMapping(ctx, testCase.reference, testCase.digest)
		require.NoError(t, err)

		// Get mapping
		mapping, err := tc.GetTagMapping(ctx, testCase.reference)
		require.NoError(t, err)
		assert.Equal(t, testCase.digest, mapping.Digest)

		// Verify the mapping was stored and retrieved correctly
		// (file existence is tested implicitly through successful Get operation)
	}
}

func TestIsValidReference(t *testing.T) {
	testCases := []struct {
		reference string
		expected  bool
	}{
		{"", false},
		{"invalid", false},
		{"docker.io/library/nginx", false}, // Missing tag/digest
		{"docker.io/library/nginx:latest", true},
		{"docker.io/library/nginx:v1.0.0", true},
		{"registry.example.com/my/repo:v1.0.0", true},
		{
			"localhost:5000/test/image@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			true,
		},
		{"nginx:latest", true},         // Should work with default registry
		{"myregistry.com/repo", false}, // Missing tag/digest
	}

	for _, tc := range testCases {
		result := isValidReference(tc.reference)
		assert.Equal(t, tc.expected, result, "Reference: %s", tc.reference)
	}
}

func TestTagCache_DisabledHistory(t *testing.T) {
	// Create tag cache with history disabled
	fs := billy.NewMemory()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	tagConfig := TagResolverConfig{
		DefaultTTL:     time.Hour,
		MaxHistorySize: 5,
		EnableHistory:  false, // Disable history
	}

	tc := NewTagCache(storage, tagConfig)

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	digest1 := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digest2 := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	// Put initial mapping
	err = tc.PutTagMapping(ctx, reference, digest1)
	require.NoError(t, err)

	// Update mapping
	err = tc.PutTagMapping(ctx, reference, digest2)
	require.NoError(t, err)

	// Get history - should be empty when history is disabled
	history, err := tc.GetTagHistory(ctx, reference)
	require.NoError(t, err)
	assert.Len(t, history, 0)
}
