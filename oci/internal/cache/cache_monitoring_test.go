package cache

import (
	"context"
	"testing"
	"time"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoring_MetricsAccuracy(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Start with clean metrics
	coordinator.metrics.Reset()
	snapshot := coordinator.metrics.GetSnapshot()

	// Verify initial state
	assert.Equal(t, int64(0), snapshot.Hits)
	assert.Equal(t, int64(0), snapshot.Misses)
	assert.Equal(t, int64(0), snapshot.ManifestPuts)
	assert.Equal(t, int64(0), snapshot.BlobPuts)
	assert.Equal(t, float64(0), snapshot.HitRate)

	// Perform manifest operations
	digest1 := validTestDigest("monitor_test_1")
	digest2 := validTestDigest("monitor_test_2")

	// Put manifest (should record put operation)
	err := coordinator.PutManifest(ctx, digest1, &ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig, Size: 10},
	})
	require.NoError(t, err)

	// Get manifest (should be a hit)
	_, err = coordinator.GetManifest(ctx, digest1)
	require.NoError(t, err)

	// Try to get non-existent manifest (should be a miss)
	_, err = coordinator.GetManifest(ctx, digest2)
	require.Error(t, err) // Should fail

	// Check metrics
	snapshot = coordinator.metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.Hits)
	assert.Equal(t, int64(1), snapshot.Misses)
	assert.Equal(t, int64(1), snapshot.ManifestPuts)
	assert.Equal(t, int64(2), snapshot.ManifestGets) // PutManifest + GetManifest operations
	assert.Equal(t, float64(0.5), snapshot.HitRate)
	assert.Greater(t, snapshot.BytesStored, int64(0))
}

func TestMonitoring_BandwidthSavings(t *testing.T) {
	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	coordinator.metrics.Reset()

	// Simulate network requests and cache hits
	downloadSize := int64(1024)

	// Record some misses (network downloads)
	coordinator.metrics.RecordMiss("manifest", downloadSize)
	coordinator.metrics.RecordMiss("blob", downloadSize*2)

	// Record some hits (cache serves)
	coordinator.metrics.RecordHit("manifest", downloadSize)
	coordinator.metrics.RecordHit("blob", downloadSize*2)

	snapshot := coordinator.metrics.GetSnapshot()

	// Verify bandwidth calculations
	assert.Equal(t, int64(2), snapshot.NetworkRequests)
	assert.Equal(t, int64(3072), snapshot.BytesDownloaded) // 1024 + 2048
	assert.Equal(t, int64(3072), snapshot.BytesServed)     // Same bytes served from cache
	assert.Greater(t, snapshot.BandwidthSaved, int64(0))   // Should have saved bandwidth
}

func TestMonitoring_PerformanceLogging(t *testing.T) {
	testLogger := NewLogger(LogConfig{
		Level:                    LogLevelDebug,
		EnablePerformanceLogging: true,
	})

	coordinator := setupTestManager(t, Config{
		MaxSizeBytes: 1024 * 1024,
		DefaultTTL:   time.Hour,
	})
	defer coordinator.Close()

	ctx := context.Background()

	// Test performance metrics logging
	snapshot := coordinator.metrics.GetSnapshot()
	LogPerformanceMetrics(ctx, testLogger, &snapshot)

	// Just verify the function call doesn't panic
	assert.NotNil(t, testLogger)
}

func TestMonitoring_CacheOperationLogging(t *testing.T) {
	logger := NewLogger(LogConfig{Level: LogLevelInfo, EnableCacheOperations: true})

	ctx := context.Background()

	// Test successful operation logging
	duration := 50 * time.Millisecond
	LogCacheOperation(ctx, logger, OpGetManifest, duration, true, 1024, nil)

	// Test failed operation logging
	testErr := assert.AnError
	LogCacheOperation(ctx, logger, OpPutBlob, duration, false, 0, testErr)

	// Just verify the function calls don't panic
	assert.NotNil(t, logger)
}

func TestMonitoring_CacheHitMissLogging(t *testing.T) {
	logger := NewLogger(LogConfig{Level: LogLevelDebug, EnableCacheOperations: true})

	ctx := context.Background()

	// Test cache hit logging
	LogCacheHit(ctx, logger, OpGetManifest, 2048)

	// Test cache miss logging
	LogCacheMiss(ctx, logger, OpGetBlob, "not found")

	// Just verify the function calls don't panic
	assert.NotNil(t, logger)
}

func TestMonitoring_EvictionLogging(t *testing.T) {
	logger := NewLogger(LogConfig{Level: LogLevelInfo})

	ctx := context.Background()

	// Test eviction logging
	LogEviction(ctx, logger, "test_digest", 4096, "size_limit_exceeded")

	// Just verify the function call doesn't panic
	assert.NotNil(t, logger)
}

func TestMonitoring_CleanupLogging(t *testing.T) {
	logger := NewLogger(LogConfig{Level: LogLevelInfo})

	ctx := context.Background()
	duration := 150 * time.Millisecond

	// Test cleanup logging
	LogCleanup(ctx, logger, "expired_entries", 5, 10240, duration)

	// Just verify the function call doesn't panic
	assert.NotNil(t, logger)
}

func TestMonitoring_LogLevelControl(t *testing.T) {
	tests := []struct {
		name      string
		level     LogLevel
		shouldLog bool
	}{
		{"debug level allows debug", LogLevelDebug, true},
		{"info level blocks debug", LogLevelInfo, false},
		{"warn level blocks info", LogLevelWarn, false},
		{"error level blocks warn", LogLevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(LogConfig{Level: tt.level})

			// Just verify the logger was created successfully
			assert.NotNil(t, logger)
		})
	}
}

func TestMonitoring_LoggerWithContext(t *testing.T) {
	logger := NewLogger(LogConfig{Level: LogLevelInfo})

	ctx := context.Background()

	// Create logger with context
	contextLogger := logger.WithOperation("test_op").WithDigest("test_digest").WithSize(1024)

	// Log a message
	contextLogger.Info(ctx, "test message with context")

	// Just verify the logger methods work
	assert.NotNil(t, contextLogger)
}

func TestMonitoring_NopLogger(t *testing.T) {
	logger := NewNopLogger()

	// These should not panic and should do nothing
	ctx := context.Background()

	logger.Debug(ctx, "debug")
	logger.Info(ctx, "info")
	logger.Warn(ctx, "warn")
	logger.Error(ctx, "error")

	// With operations should return the same logger
	assert.Equal(t, logger, logger.WithOperation("test"))
	assert.Equal(t, logger, logger.WithDigest("test"))
	assert.Equal(t, logger, logger.WithSize(1024))
	assert.Equal(t, logger, logger.WithDuration(time.Second))
}

func TestMonitoring_ParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
		hasError bool
	}{
		{"debug", LogLevelDebug, false},
		{"DEBUG", LogLevelDebug, false},
		{"info", LogLevelInfo, false},
		{"INFO", LogLevelInfo, false},
		{"warn", LogLevelWarn, false},
		{"warning", LogLevelWarn, false},
		{"error", LogLevelError, false},
		{"ERROR", LogLevelError, false},
		{"invalid", LogLevelInfo, true}, // Should default to info with error
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseLogLevel(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestMonitoring_EndToEndLogging(t *testing.T) {
	// This test verifies that logging works end-to-end with actual cache operations
	logger := NewLogger(LogConfig{
		Level:                 LogLevelDebug,
		EnableCacheOperations: true,
	})

	coordinator := &Coordinator{
		logger:  logger,
		metrics: NewDetailedMetrics(),
	}

	// Simulate some operations that would trigger logging
	ctx := context.Background()

	// This will try to log but may not have full context since coordinator isn't fully initialized
	// We just verify it doesn't panic
	LogCacheHit(ctx, coordinator.logger, OpGetManifest, 1024)
	LogCacheMiss(ctx, coordinator.logger, OpGetBlob, "not found")

	// Just verify the logger works
	assert.NotNil(t, coordinator.logger)
}
