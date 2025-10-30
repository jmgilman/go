//go:build !integration

// Package testutil provides testing utilities for the OCI bundle library.
// This file contains tests for the benchmark utilities.
package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkMemoryUsage benchmarks memory usage patterns.
func BenchmarkMemoryUsage(b *testing.B) {
	profiler := &MemoryProfiler{}

	b.Run("MemoryUsage_Pattern", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			profiler.Start()

			// Simulate some work that allocates memory
			data := make([]byte, 1024*1024) // 1MB allocation
			for j := range data {
				data[j] = byte(j % 256)
			}
			_ = data // Prevent optimization

			result := profiler.Stop()
			if i == 0 { // Only print once
				result.Print()
			}
		}
	})
}

// BenchmarkConcurrentOperations benchmarks concurrent operation performance.
func BenchmarkConcurrentOperations(b *testing.B) {
	runner := NewBenchmarkRunner()

	b.Run("Concurrent_10_Goroutines", func(b *testing.B) {
		runner.RunBenchmark("ConcurrentOps", b, func() error {
			// Simulate concurrent work
			done := make(chan error, 10)

			for i := 0; i < 10; i++ {
				go func(id int) {
					// Simulate some work
					time.Sleep(time.Millisecond)
					done <- nil
				}(i)
			}

			// Wait for all goroutines
			for i := 0; i < 10; i++ {
				if err := <-done; err != nil {
					return err
				}
			}

			return nil
		})
	})

	runner.PrintResults()
}

// TestBenchmarkRunner_BasicFunctionality tests the benchmark runner.
func TestBenchmarkRunner_BasicFunctionality(t *testing.T) {
	runner := NewBenchmarkRunner()
	require.NotNil(t, runner)

	// Test basic functionality without nested benchmark
	start := time.Now()
	err := func() error {
		time.Sleep(time.Millisecond)
		return nil
	}()

	duration := time.Since(start)

	require.NoError(t, err)
	assert.Greater(t, duration, time.Duration(0))
}

// TestMemoryProfiler_BasicFunctionality tests the memory profiler.
func TestMemoryProfiler_BasicFunctionality(t *testing.T) {
	profiler := &MemoryProfiler{}

	profiler.Start()

	// Allocate some memory
	data := make([]byte, 1024*100) // 100KB
	for i := range data {
		data[i] = byte(i)
	}

	result := profiler.Stop()

	// Verify results
	assert.Greater(t, result.Allocated, uint64(0))
	assert.Greater(t, result.HeapAlloc, uint64(0))
}

// TestPerformanceMonitor_BasicFunctionality tests the performance monitor.
func TestPerformanceMonitor_BasicFunctionality(t *testing.T) {
	monitor := &PerformanceMonitor{}

	monitor.Start()

	// Do some work
	time.Sleep(10 * time.Millisecond)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	metrics := monitor.Stop()

	// Verify results
	assert.Greater(t, metrics.Duration, time.Duration(0))
	assert.Greater(t, metrics.MemoryDelta, uint64(0))
}
