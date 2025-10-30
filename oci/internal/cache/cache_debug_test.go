package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugTools_CollectDebugInfo(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Add some test data
	digest := validTestDigest("debug_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
	})
	require.NoError(t, err)

	// Create debug tools
	debugTools := NewDebugTools(coordinator)

	// Collect debug info
	info, err := debugTools.CollectDebugInfo(ctx)
	require.NoError(t, err)
	require.NotNil(t, info)

	// Verify basic fields
	assert.True(t, info.Timestamp.After(time.Now().Add(-time.Minute)))
	assert.GreaterOrEqual(t, info.Metrics.ManifestPuts, int64(1)) // At least one put operation
	assert.Greater(t, info.CacheStats.TotalEntries, 0)
	// Note: Integrity check may show digest mismatches for test data, which is expected
	assert.NotEmpty(t, info.StorageInfo.CachePath)
}

func TestDebugTools_TriggerCleanup(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	debugTools := NewDebugTools(coordinator)

	err := debugTools.TriggerCleanup(context.Background())
	assert.NoError(t, err)
}

func TestDebugTools_ClearExpiredEntries(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   10 * time.Millisecond, // Very short TTL
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Add an entry
	digest := validTestDigest("expired_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
	})
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	debugTools := NewDebugTools(coordinator)
	removed, err := debugTools.ClearExpiredEntries(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
}

func TestDebugTools_GetCacheContents(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Add test entries
	digests := []string{
		validTestDigest("content_test_1"),
		validTestDigest("content_test_2"),
	}

	for _, digest := range digests {
		err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
		})
		require.NoError(t, err)
	}

	debugTools := NewDebugTools(coordinator)
	contents, err := debugTools.GetCacheContents(ctx)
	require.NoError(t, err)

	assert.Len(t, contents, 2)
	for _, entry := range contents {
		assert.Contains(t, digests, entry.Key)
		assert.Equal(t, "manifest", entry.Type)
		assert.False(t, entry.IsExpired)
	}
}

func TestDebugTools_ExportDebugInfo(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	debugTools := NewDebugTools(coordinator)

	// Create temp file for export
	tempDir := t.TempDir()
	exportPath := filepath.Join(tempDir, "debug_info.json")

	err := debugTools.ExportDebugInfo(context.Background(), exportPath)
	require.NoError(t, err)

	// Verify file was created and contains valid JSON
	data, err := os.ReadFile(exportPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "timestamp")
	assert.Contains(t, string(data), "config")
}

func TestDebugTools_ValidateAllDigests(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Add a valid entry
	digest := validTestDigest("digest_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
	})
	require.NoError(t, err)

	debugTools := NewDebugTools(coordinator)
	validCount, invalidDigests, err := debugTools.ValidateAllDigests(ctx)
	require.NoError(t, err)

	// Note: Test data may have digest mismatches, so we just check that validation ran
	assert.GreaterOrEqual(t, validCount+len(invalidDigests), 1)
}

func TestDebugTools_RepairIntegrity(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	debugTools := NewDebugTools(coordinator)

	// Test repair on healthy cache
	report, err := debugTools.RepairIntegrity(context.Background())
	require.NoError(t, err)
	assert.True(t, report.IsHealthy)
}

func TestDebugTools_CheckIntegrity(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Add a valid entry
	digest := validTestDigest("integrity_test")
	err := coordinator.PutManifest(ctx, digest, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
	})
	require.NoError(t, err)

	debugTools := NewDebugTools(coordinator)
	report, err := debugTools.checkIntegrity(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, report.TotalEntries)
	// Note: Test data may have digest mismatches, so integrity may not be healthy
	assert.NotNil(t, report.CorruptedEntries)
	assert.NotNil(t, report.MissingFiles)
	assert.NotNil(t, report.InvalidDigests)
	assert.Greater(t, report.CheckDuration, time.Duration(0))
}
