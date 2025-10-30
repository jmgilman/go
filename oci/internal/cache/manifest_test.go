package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	billyfs "github.com/input-output-hk/catalyst-forge-libs/fs/billy"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConfigProvider implements ConfigProvider for testing
type testConfigProvider struct {
	config Config
}

func (t *testConfigProvider) Config() Config {
	return t.config
}

// TestManifestCacheTTLExpiration tests that manifests expire correctly after TTL
func TestManifestCacheTTLExpiration(t *testing.T) {
	// Create a temporary filesystem for testing
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	// Create cache with short TTL for testing
	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024,      // 100MB
		DefaultTTL:   100 * time.Millisecond, // Very short TTL for testing
	}

	// Create manifest cache
	cache := NewManifestCache(storage, &testConfigProvider{config: config})

	ctx := context.Background()
	digestStr := "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	manifest := &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    digest.FromString("sha256:abcdef1234567890"),
			Size:      int64(1234),
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    digest.FromString("sha256:fedcba0987654321"),
				Size:      int64(5678),
			},
		},
	}

	// Put manifest in cache
	err = cache.PutManifest(ctx, digestStr, manifest)
	require.NoError(t, err)

	// Verify manifest exists immediately
	retrieved, err := cache.GetManifest(ctx, digestStr)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, manifest.MediaType, retrieved.MediaType)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Manifest should no longer be retrievable
	retrieved, err = cache.GetManifest(ctx, digestStr)
	assert.Error(t, err)
	assert.Nil(t, retrieved)
}

// TestManifestCacheConcurrentAccess tests concurrent access to manifest cache
func TestManifestCacheConcurrentAccess(t *testing.T) {
	// Create a temporary filesystem for testing
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	// Create cache with reasonable TTL
	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   5 * time.Minute,
	}
	cache := NewManifestCache(storage, &testConfigProvider{config: config})

	ctx := context.Background()
	const numGoroutines = 10
	const numOperations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Generate a valid SHA256 digest for testing
				digestStr := fmt.Sprintf("sha256:%064x", id*numOperations+j)
				manifest := &ocispec.Manifest{
					Versioned: specs.Versioned{SchemaVersion: 2},
					MediaType: ocispec.MediaTypeImageManifest,
					Config: ocispec.Descriptor{
						MediaType: "application/vnd.oci.image.config.v1+json",
						Digest: digest.FromString(
							fmt.Sprintf("sha256:%064x", (id+1)*numOperations+j),
						),
						Size: int64(1234),
					},
				}

				// Put manifest
				err := cache.PutManifest(ctx, digestStr, manifest)
				assert.NoError(t, err)

				// Get manifest
				retrieved, err := cache.GetManifest(ctx, digestStr)
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, manifest.MediaType, retrieved.MediaType)

				// Check existence
				exists, err := cache.HasManifest(ctx, digestStr)
				assert.NoError(t, err)
				assert.True(t, exists)
			}
		}(i)
	}

	wg.Wait()
}

// TestManifestCacheValidation tests manifest validation functionality
func TestManifestCacheValidation(t *testing.T) {
	// Create a temporary filesystem for testing
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   5 * time.Minute,
	}
	cache := NewManifestCache(storage, &testConfigProvider{config: config})

	ctx := context.Background()

	tests := []struct {
		name        string
		reference   string
		digest      string
		expectValid bool
		expectError bool
	}{
		{
			name:        "valid manifest reference",
			reference:   "example.com/repo:tag",
			digest:      "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expectValid: false, // Should be false since manifest doesn't exist in cache
			expectError: false,
		},
		{
			name:        "invalid digest format",
			reference:   "example.com/repo:tag",
			digest:      "invalid-digest",
			expectValid: false,
			expectError: true,
		},
		{
			name:        "empty reference",
			reference:   "",
			digest:      "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expectValid: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := cache.ValidateManifest(ctx, tt.reference, tt.digest)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectValid, valid)
			}
		})
	}
}

// TestManifestCacheMalformedManifests tests handling of malformed manifests
func TestManifestCacheMalformedManifests(t *testing.T) {
	// Create a temporary filesystem for testing
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   5 * time.Minute,
	}
	cache := NewManifestCache(storage, &testConfigProvider{config: config})

	ctx := context.Background()
	digestStr := "sha256:malformed"

	tests := []struct {
		name     string
		manifest *ocispec.Manifest
	}{
		{
			name:     "nil manifest",
			manifest: nil,
		},
		{
			name: "manifest with negative size",
			manifest: &ocispec.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: ocispec.MediaTypeImageManifest,
				Config: ocispec.Descriptor{
					MediaType: "application/vnd.oci.image.config.v1+json",
					Digest:    digest.FromString("sha256:abcdef1234567890"),
					Size:      -1, // Invalid negative size
				},
			},
		},
		{
			name: "manifest with invalid media type",
			manifest: &ocispec.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: "invalid/media-type",
				Config: ocispec.Descriptor{
					MediaType: "application/vnd.oci.image.config.v1+json",
					Digest:    digest.FromString("sha256:abcdef1234567890"),
					Size:      1234,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// PutManifest should reject malformed manifests
			err := cache.PutManifest(ctx, digestStr, tt.manifest)
			assert.Error(t, err)
		})
	}
}

// TestManifestCacheBasicOperations tests basic cache operations
func TestManifestCacheBasicOperations(t *testing.T) {
	// Create a temporary filesystem for testing
	fs := billyfs.NewInMemoryFS()
	storage, err := NewStorage(fs, "/cache")
	require.NoError(t, err)

	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   5 * time.Minute,
	}
	cache := NewManifestCache(storage, &testConfigProvider{config: config})

	ctx := context.Background()

	t.Run("put and get manifest", func(t *testing.T) {
		digestStr := "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		manifest := &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config: ocispec.Descriptor{
				MediaType: "application/vnd.oci.image.config.v1+json",
				Digest: digest.FromString(
					"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				),
				Size: int64(1234),
			},
			Layers: []ocispec.Descriptor{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest: digest.FromString(
						"sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
					),
					Size: int64(5678),
				},
			},
		}

		// Put manifest
		err := cache.PutManifest(ctx, digestStr, manifest)
		require.NoError(t, err)

		// Get manifest
		retrieved, err := cache.GetManifest(ctx, digestStr)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		// Verify content
		assert.Equal(t, manifest.MediaType, retrieved.MediaType)
		assert.Equal(t, manifest.Config.Digest, retrieved.Config.Digest)
		assert.Len(t, retrieved.Layers, 1)
		assert.Equal(t, manifest.Layers[0].Digest, retrieved.Layers[0].Digest)
	})

	t.Run("has manifest", func(t *testing.T) {
		digestStr := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
		manifest := &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config: ocispec.Descriptor{
				MediaType: "application/vnd.oci.image.config.v1+json",
				Digest: digest.FromString(
					"sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				),
				Size: 1234,
			},
		}

		// Should not exist initially
		exists, err := cache.HasManifest(ctx, digestStr)
		require.NoError(t, err)
		assert.False(t, exists)

		// Put manifest
		err = cache.PutManifest(ctx, digestStr, manifest)
		require.NoError(t, err)

		// Should exist now
		exists, err = cache.HasManifest(ctx, digestStr)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("get non-existent manifest", func(t *testing.T) {
		digestStr := "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

		retrieved, err := cache.GetManifest(ctx, digestStr)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}
