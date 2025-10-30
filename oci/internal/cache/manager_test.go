package cache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	billyfs "github.com/input-output-hk/catalyst-forge-libs/fs/billy"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var digestCounter int64

// validTestDigest creates a valid SHA256 digest for testing
func validTestDigest(prefix string) string {
	// Use atomic counter to ensure unique digests
	counter := atomic.AddInt64(&digestCounter, 1)

	// Create a 64-character hex string that's valid for SHA256
	// Use the counter to make each digest unique
	base := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	hash := fmt.Sprintf("%016x", counter) + base[16:64]

	return "sha256:" + hash
}

func setupTestManager(t *testing.T, config Config) *Coordinator {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cache_manager_test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	// Create filesystem
	fs := billyfs.NewInMemoryFS()

	ctx := context.Background()
	coordinator, err := NewCoordinator(ctx, config, fs, tempDir, nil)
	require.NoError(t, err)

	// Ensure coordinator is properly closed after test
	t.Cleanup(func() {
		if coordinator != nil {
			coordinator.Close()
		}
	})

	return coordinator
}

func TestNewCoordinator(t *testing.T) {
	config := Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   time.Hour,
	}

	coordinator := setupTestManager(t, config)

	assert.NotNil(t, coordinator)
	assert.Equal(t, config.MaxSizeBytes, coordinator.config.MaxSizeBytes)
	assert.Equal(t, config.DefaultTTL, coordinator.config.DefaultTTL)
	assert.NotNil(t, coordinator.storage)
	assert.NotNil(t, coordinator.index)
	assert.NotNil(t, coordinator.eviction)
	assert.NotNil(t, coordinator.metrics)
}

func TestCoordinator_GetPutManifest(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()
	digest := validTestDigest("test")
	manifest := &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
		Layers:    []ocispec.Descriptor{{MediaType: "application/octet-stream", Size: 100}},
	}

	// Put manifest
	err := coordinator.PutManifest(ctx, digest, manifest)
	require.NoError(t, err)

	// Get manifest
	retrieved, err := coordinator.GetManifest(ctx, digest)
	require.NoError(t, err)
	assert.Equal(t, manifest.SchemaVersion, retrieved.SchemaVersion)
	assert.Len(t, retrieved.Layers, 1)
}

func TestCoordinator_GetPutBlob(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()
	digest := validTestDigest("blob")
	data := []byte("test blob data")

	// Put blob
	err := coordinator.PutBlob(ctx, digest, bytes.NewReader(data))
	require.NoError(t, err)

	// Get blob
	reader, err := coordinator.GetBlob(ctx, digest)
	require.NoError(t, err)
	defer reader.Close()

	retrievedData, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, data, retrievedData)
}

func TestCoordinator_Size(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Initially empty
	size, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size)

	// Add some entries
	digest1 := validTestDigest("test1")
	digest2 := validTestDigest("test2")

	err = coordinator.PutManifest(ctx, digest1, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
	})
	require.NoError(t, err)

	err = coordinator.PutBlob(ctx, digest2, bytes.NewReader([]byte("test")))
	require.NoError(t, err)

	// Size should be greater than 0
	size, err = coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, size > 0)
}

func TestCoordinator_Clear(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Add entries
	digest := validTestDigest("clear_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
	})
	require.NoError(t, err)

	// Verify entry exists
	size, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, size > 0)

	// Clear cache
	err = coordinator.Clear(ctx)
	require.NoError(t, err)

	// Verify cache is empty
	size, err = coordinator.Size(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size)
}

func TestCoordinator_GetStats(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 100 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Add some entries
	for i := 0; i < 5; i++ {
		digest := validTestDigest(fmt.Sprintf("test%d", i))
		err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
		})
		require.NoError(t, err)
	}

	stats := coordinator.GetStats()

	assert.Equal(t, 5, stats.TotalEntries)
	assert.True(t, stats.TotalSize > 0)
	assert.Equal(t, int64(100*1024), stats.MaxSize)
	assert.True(t, stats.HitRate >= 0.0)
	assert.True(t, stats.HitRate <= 1.0)
}

func TestCoordinator_PerformCleanup(t *testing.T) {
	config := Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cache_manager_test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	// Create filesystem
	fs := billyfs.NewInMemoryFS()

	ctx := context.Background()
	coordinator, err := NewCoordinator(ctx, config, fs, tempDir, nil)
	require.NoError(t, err)

	// Close coordinator after test (disable automatic cleanup to avoid interference)
	t.Cleanup(func() {
		if coordinator != nil {
			coordinator.Close()
		}
	})

	// Add entries with short TTL
	for i := 0; i < 10; i++ {
		digest := validTestDigest(fmt.Sprintf("test%d", i))
		err = coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
		})
		require.NoError(t, err)
	}

	initialSize, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, initialSize > 0)

	// Manually expire some entries by updating their timestamps
	// Get keys to expire (this is a test-only scenario)
	coordinator.mu.RLock()
	allKeys := coordinator.index.Keys(nil)
	coordinator.mu.RUnlock()

	for i, key := range allKeys {
		if i >= 5 { // Expire first 5 entries
			break
		}
		// Get the current entry and update it with expired timestamp
		if entry, exists := coordinator.index.Get(key); exists {
			expiredEntry := &IndexEntry{
				Key:        entry.Key,
				Size:       entry.Size,
				CreatedAt:  time.Now().Add(-2 * time.Hour), // Make it expired
				AccessedAt: entry.AccessedAt,
				TTL:        entry.TTL,
			}
			err = coordinator.index.Put(key, expiredEntry)
			require.NoError(t, err)
		}
	}

	// Perform cleanup
	err = coordinator.performCleanup(ctx)
	require.NoError(t, err)

	// Size should be smaller since we expired 5 entries
	sizeAfterCleanup, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, sizeAfterCleanup < initialSize, "Size should decrease after cleanup of expired entries")
}

func TestCoordinator_PerformEviction(t *testing.T) {
	config := Config{
		MaxSizeBytes: 100, // Very small limit
		DefaultTTL:   time.Hour,
	}
	coordinator := setupTestManager(t, config)

	ctx := context.Background()

	// Add entries that exceed the size limit
	for i := 0; i < 20; i++ {
		digest := validTestDigest(fmt.Sprintf("test%d", i))
		// Create manifest with data to consume space
		manifest := &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
			Layers: []ocispec.Descriptor{
				{MediaType: "application/octet-stream", Size: 50},
			},
		}
		err := coordinator.PutManifest(ctx, digest, manifest)
		require.NoError(t, err)
	}

	// Perform eviction
	err := coordinator.performEviction(ctx)
	require.NoError(t, err)

	// Size should be reduced
	size, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, size <= config.MaxSizeBytes)
}

func TestCoordinator_Metrics(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Initially no hits or misses
	metricsSnapshot := coordinator.GetMetrics().GetSnapshot()
	assert.Equal(t, int64(0), metricsSnapshot.Hits)
	assert.Equal(t, int64(0), metricsSnapshot.Misses)

	// Perform some operations
	digest := validTestDigest("metrics_test")
	_, err := coordinator.GetManifest(ctx, digest) // Miss
	require.Error(t, err)                          // Should fail since manifest doesn't exist

	// Add manifest
	err = coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
	})
	require.NoError(t, err)

	// Get manifest (should be a hit)
	_, err = coordinator.GetManifest(ctx, digest)
	require.NoError(t, err)

	// Check metrics
	metricsSnapshot = coordinator.GetMetrics().GetSnapshot()
	assert.Equal(t, int64(1), metricsSnapshot.Hits)
	assert.Equal(t, int64(1), metricsSnapshot.Misses)
	assert.Equal(t, float64(0.5), metricsSnapshot.HitRate)
}

func TestCoordinator_Close(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Add some data
	digest := validTestDigest("close_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
	})
	require.NoError(t, err)

	// Close coordinator
	err = coordinator.Close()
	require.NoError(t, err)

	// Verify cleanup happened
	assert.Nil(t, coordinator.cleanupTicker)
}

func TestCoordinator_InvalidConfig(t *testing.T) {
	invalidConfig := Config{
		MaxSizeBytes: -1, // Invalid
		DefaultTTL:   time.Hour,
	}

	tempDir, err := os.MkdirTemp("", "cache_manager_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	fs := billyfs.NewInMemoryFS()
	require.NoError(t, err)

	ctx := context.Background()
	_, err = NewCoordinator(ctx, invalidConfig, fs, tempDir, nil)
	assert.Error(t, err)
}

func TestCoordinator_CachePath(t *testing.T) {
	config := Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	tempDir, err := os.MkdirTemp("", "cache_manager_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cachePath := filepath.Join(tempDir, "cache")

	fs := billyfs.NewInMemoryFS()
	require.NoError(t, err)

	ctx := context.Background()
	coordinator, err := NewCoordinator(ctx, config, fs, cachePath, nil)
	require.NoError(t, err)

	// Verify cache directory was created
	assert.NotNil(t, coordinator.storage)
}

func TestCoordinator_BackgroundCleanup(t *testing.T) {
	config := Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator := setupTestManager(t, config)

	// Verify cleanup scheduler is running
	assert.NotNil(t, coordinator.cleanupTicker)
	assert.NotNil(t, coordinator.cleanupDone)

	// Close to stop cleanup
	err := coordinator.Close()
	require.NoError(t, err)
}

func TestCoordinator_ConcurrentOperations(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 10 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	})

	ctx := context.Background()

	// Test concurrent read/write operations
	done := make(chan bool, 20)

	// Concurrent puts
	for i := 0; i < 10; i++ {
		go func(id int) {
			digest := validTestDigest(fmt.Sprintf("put%d", id))
			manifest := &ocispec.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				MediaType: ocispec.MediaTypeImageManifest,
				Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
			}
			err := coordinator.PutManifest(ctx, digest, manifest)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Concurrent gets
	for i := 0; i < 10; i++ {
		go func(id int) {
			digest := validTestDigest(fmt.Sprintf("get%d", id))
			_, err := coordinator.GetManifest(ctx, digest)
			// Error is expected for non-existent entries
			_ = err
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	size, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, size > 0)
}

func TestCoordinator_EvictionUnderPressure(t *testing.T) {
	config := Config{
		MaxSizeBytes: 1000, // Very small limit
		DefaultTTL:   time.Hour,
	}

	coordinator := setupTestManager(t, config)
	ctx := context.Background()

	// Add many entries to trigger eviction
	for i := 0; i < 50; i++ {
		digest := validTestDigest(fmt.Sprintf("press%d", i))
		manifest := &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
			Layers: []ocispec.Descriptor{
				{MediaType: "application/octet-stream", Size: 100},
			},
		}
		err := coordinator.PutManifest(ctx, digest, manifest)
		require.NoError(t, err)

		// Periodically check size and trigger eviction
		if i%10 == 0 {
			err := coordinator.performEviction(ctx)
			require.NoError(t, err)
		}
	}

	// Final size should be under limit
	finalSize, err := coordinator.Size(ctx)
	require.NoError(t, err)
	assert.True(t, finalSize <= config.MaxSizeBytes)
}

func TestCoordinator_IndexRecovery(t *testing.T) {
	config := Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	tempDir, err := os.MkdirTemp("", "cache_manager_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create first coordinator and add data
	fs1 := billyfs.NewInMemoryFS()
	require.NoError(t, err)

	ctx := context.Background()
	coordinator1, err := NewCoordinator(ctx, config, fs1, tempDir, nil)
	require.NoError(t, err)

	// Add some data
	for i := 0; i < 5; i++ {
		digest := validTestDigest(fmt.Sprintf("recov%d", i))
		err = coordinator1.PutManifest(ctx, digest, &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Size: 10},
		})
		require.NoError(t, err)
	}

	// Close first coordinator (should persist index)
	err = coordinator1.Close()
	require.NoError(t, err)

	// Create second coordinator (should load persisted index)
	fs2 := billyfs.NewInMemoryFS()
	require.NoError(t, err)

	coordinator2, err := NewCoordinator(ctx, config, fs2, tempDir, nil)
	require.NoError(t, err)
	defer coordinator2.Close()

	// Verify data was recovered
	size, err := coordinator2.Size(ctx)
	require.NoError(t, err)
	assert.True(t, size > 0)
}

func TestConfig_SetDefaults(t *testing.T) {
	config := Config{}
	config.SetDefaults()

	// SetDefaults doesn't actually set defaults for Config currently
	// This test ensures the method exists and runs without error
	assert.NotNil(t, config)
}

func TestTagResolverConfig_SetDefaults(t *testing.T) {
	config := TagResolverConfig{}
	config.SetDefaults()

	assert.Equal(t, 10, config.MaxHistorySize)
	assert.True(t, config.EnableHistory)
}
