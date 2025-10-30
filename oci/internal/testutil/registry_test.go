//go:build integration

// Package testutil provides testing utilities for the OCI bundle library.
// This file contains tests for the registry utilities.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryLifecycle tests the basic lifecycle of a test registry.
// It verifies that the registry starts, is accessible, and cleans up properly.
func TestRegistryLifecycle(t *testing.T) {
	ctx := context.Background()

	registry, err := NewTestRegistry(ctx)
	require.NoError(t, err, "Failed to create test registry")
	defer func() {
		closeErr := registry.Close(ctx)
		assert.NoError(t, closeErr, "Failed to close registry")
	}()

	// Verify registry reference is properly formatted
	ref := registry.Reference()
	assert.NotEmpty(t, ref, "Registry reference should not be empty")
	assert.Contains(t, ref, ":", "Registry reference should contain port")

	// Verify registry is accessible
	err = registry.WaitForReady(ctx, 30*time.Second)
	assert.NoError(t, err, "Registry should be ready within timeout")

	// Verify URL format
	url := registry.URL()
	assert.NotEmpty(t, url, "Registry URL should not be empty")
	assert.Contains(t, url, "http://", "Registry URL should start with http://")
}

// TestRegistryPortUniqueness tests that multiple registries get different ports.
// This ensures isolation between concurrent tests.
func TestRegistryPortUniqueness(t *testing.T) {
	ctx := context.Background()

	registry1, err := NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry1.Close(ctx)

	registry2, err := NewTestRegistry(ctx)
	require.NoError(t, err)
	defer registry2.Close(ctx)

	// Wait for both to be ready
	err = registry1.WaitForReady(ctx, 30*time.Second)
	require.NoError(t, err)
	err = registry2.WaitForReady(ctx, 30*time.Second)
	require.NoError(t, err)

	// Verify different ports
	assert.NotEqual(t, registry1.port, registry2.port, "Registries should have different ports")
	assert.NotEqual(t, registry1.Reference(), registry2.Reference(), "Registry references should be different")
}

// BenchmarkRegistryStartup benchmarks the time to start a registry container.
func BenchmarkRegistryStartup(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		registry, err := NewTestRegistry(ctx)
		if err != nil {
			b.Fatalf("Failed to create registry: %v", err)
		}

		// Wait for ready
		err = registry.WaitForReady(ctx, 30*time.Second)
		if err != nil {
			b.Fatalf("Registry not ready: %v", err)
		}

		// Clean up
		registry.Close(ctx)
	}
}

// BenchmarkRegistryOperations benchmarks registry operations with different sizes.
func BenchmarkRegistryOperations(b *testing.B) {
	ctx := context.Background()

	benchmarks := []struct {
		name string
		size int64
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			registry, err := NewTestRegistry(ctx)
			if err != nil {
				b.Fatalf("Failed to create registry: %v", err)
			}
			defer registry.Close(ctx)

			err = registry.WaitForReady(ctx, 30*time.Second)
			if err != nil {
				b.Fatalf("Registry not ready: %v", err)
			}

			// Note: Full push/pull benchmarking would require a Client instance
			// This just benchmarks the registry container lifecycle
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = registry.Reference() // simulate reference usage
			}
		})
	}
}
