// Package testutil provides testing utilities for the OCI bundle library.
// This file contains benchmark utilities for performance measurement.
package testutil

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

// BenchmarkResult holds the results of a benchmark measurement.
type BenchmarkResult struct {
	Name            string
	Duration        time.Duration
	MemoryAllocated int64
	MemoryPeak      int64
	GCCycles        int32
	Operations      int64
	OpsPerSecond    float64
	MBPerSecond     float64
}

// BenchmarkRunner provides utilities for running performance benchmarks.
type BenchmarkRunner struct {
	results []BenchmarkResult
}

// NewBenchmarkRunner creates a new benchmark runner.
func NewBenchmarkRunner() *BenchmarkRunner {
	return &BenchmarkRunner{
		results: make([]BenchmarkResult, 0),
	}
}

// RunBenchmark executes a benchmark function and measures its performance.
func (r *BenchmarkRunner) RunBenchmark(name string, b *testing.B, benchmarkFunc func() error) {
	b.Run(name, func(b *testing.B) {
		// Reset benchmark timer and memory stats
		b.ResetTimer()
		b.ReportAllocs()

		// Run garbage collection before starting
		runtime.GC()

		// Get initial memory stats
		var initialStats runtime.MemStats
		runtime.ReadMemStats(&initialStats)

		startTime := time.Now()
		operations := int64(0)

		// Run the benchmark
		for i := 0; i < b.N; i++ {
			if err := benchmarkFunc(); err != nil {
				b.Fatalf("Benchmark function failed: %v", err)
			}
			operations++
		}

		endTime := time.Now()

		// Get final memory stats
		var finalStats runtime.MemStats
		runtime.ReadMemStats(&finalStats)

		// Calculate results
		duration := endTime.Sub(startTime)
		memoryAllocated := finalStats.TotalAlloc - initialStats.TotalAlloc
		memoryPeak := finalStats.Sys - initialStats.Sys
		gcCycles := finalStats.NumGC - initialStats.NumGC

		result := BenchmarkResult{
			Name:            name,
			Duration:        duration,
			MemoryAllocated: int64(memoryAllocated),
			MemoryPeak:      int64(memoryPeak),
			GCCycles:        int32(gcCycles),
			Operations:      operations,
			OpsPerSecond:    float64(operations) / duration.Seconds(),
		}

		r.results = append(r.results, result)

		// Report results
		b.ReportMetric(float64(memoryAllocated)/float64(operations), "B/op")
		b.ReportMetric(float64(result.OpsPerSecond), "ops/sec")
	})
}

// GetResults returns all benchmark results.
func (r *BenchmarkRunner) GetResults() []BenchmarkResult {
	return r.results
}

// PrintResults prints benchmark results in a formatted way.
func (r *BenchmarkRunner) PrintResults() {
	if len(r.results) == 0 {
		fmt.Println("No benchmark results available")
		return
	}

	fmt.Println("Benchmark Results:")
	fmt.Println("==================")

	for _, result := range r.results {
		fmt.Printf("\n%s:\n", result.Name)
		fmt.Printf("  Duration: %v\n", result.Duration)
		fmt.Printf("  Operations: %d\n", result.Operations)
		fmt.Printf("  Ops/sec: %.2f\n", result.OpsPerSecond)
		fmt.Printf("  Memory allocated: %s\n", formatBytes(result.MemoryAllocated))
		fmt.Printf("  Memory peak: %s\n", formatBytes(result.MemoryPeak))
		fmt.Printf("  GC cycles: %d\n", result.GCCycles)
	}
}

// MemoryProfiler provides detailed memory profiling utilities.
type MemoryProfiler struct {
	startStats runtime.MemStats
	endStats   runtime.MemStats
}

// Start begins memory profiling.
func (p *MemoryProfiler) Start() {
	runtime.GC() // Clean up before measuring
	runtime.ReadMemStats(&p.startStats)
}

// Stop ends memory profiling and returns the results.
func (p *MemoryProfiler) Stop() MemoryProfileResult {
	runtime.ReadMemStats(&p.endStats)

	return MemoryProfileResult{
		Allocated:     p.endStats.TotalAlloc - p.startStats.TotalAlloc,
		Freed:         p.endStats.TotalAlloc - p.startStats.TotalAlloc, // Simplified
		HeapAlloc:     p.endStats.HeapAlloc,
		HeapSys:       p.endStats.HeapSys,
		StackSys:      p.endStats.StackSys,
		GCCycles:      p.endStats.NumGC - p.startStats.NumGC,
		PauseTotalNs:  p.endStats.PauseTotalNs - p.startStats.PauseTotalNs,
		NumGoroutines: runtime.NumGoroutine(),
	}
}

// MemoryProfileResult contains detailed memory profiling results.
type MemoryProfileResult struct {
	Allocated     uint64
	Freed         uint64
	HeapAlloc     uint64
	HeapSys       uint64
	StackSys      uint64
	GCCycles      uint32
	PauseTotalNs  uint64
	NumGoroutines int
}

// Print prints memory profile results.
func (r MemoryProfileResult) Print() {
	fmt.Printf("Memory Profile:\n")
	fmt.Printf("  Allocated: %s\n", formatBytes(int64(r.Allocated)))
	fmt.Printf("  Heap Alloc: %s\n", formatBytes(int64(r.HeapAlloc)))
	fmt.Printf("  Heap Sys: %s\n", formatBytes(int64(r.HeapSys)))
	fmt.Printf("  Stack Sys: %s\n", formatBytes(int64(r.StackSys)))
	fmt.Printf("  GC Cycles: %d\n", r.GCCycles)
	fmt.Printf("  GC Pause: %v\n", time.Duration(r.PauseTotalNs))
	fmt.Printf("  Goroutines: %d\n", r.NumGoroutines)
}

// PerformanceMonitor monitors system performance during operations.
type PerformanceMonitor struct {
	startTime time.Time
	startMem  runtime.MemStats
}

// Start begins performance monitoring.
func (p *PerformanceMonitor) Start() {
	runtime.GC()
	runtime.ReadMemStats(&p.startMem)
	p.startTime = time.Now()
}

// Stop ends performance monitoring and returns metrics.
func (p *PerformanceMonitor) Stop() PerformanceMetrics {
	endTime := time.Now()
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)

	return PerformanceMetrics{
		Duration:    endTime.Sub(p.startTime),
		MemoryDelta: endMem.TotalAlloc - p.startMem.TotalAlloc,
		GCCycles:    endMem.NumGC - p.startMem.NumGC,
		HeapObjects: endMem.HeapObjects - p.startMem.HeapObjects,
	}
}

// PerformanceMetrics contains performance measurement results.
type PerformanceMetrics struct {
	Duration    time.Duration
	MemoryDelta uint64
	GCCycles    uint32
	HeapObjects uint64
}

// Print prints performance metrics.
func (m PerformanceMetrics) Print() {
	fmt.Printf("Performance Metrics:\n")
	fmt.Printf("  Duration: %v\n", m.Duration)
	fmt.Printf("  Memory Delta: %s\n", formatBytes(int64(m.MemoryDelta)))
	fmt.Printf("  GC Cycles: %d\n", m.GCCycles)
	fmt.Printf("  Heap Objects Delta: %d\n", m.HeapObjects)
}

// formatBytes formats byte counts into human-readable strings.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// BenchmarkOCIClient provides benchmark utilities specifically for OCI client operations.
type BenchmarkOCIClient struct {
	*BenchmarkRunner
	archiveGen *ArchiveGenerator
	registry   *TestRegistry
}

// NewBenchmarkOCIClient creates a new OCI client benchmark runner.
func NewBenchmarkOCIClient() (*BenchmarkOCIClient, error) {
	runner := NewBenchmarkRunner()
	archiveGen, err := NewArchiveGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to create archive generator: %w", err)
	}

	return &BenchmarkOCIClient{
		BenchmarkRunner: runner,
		archiveGen:      archiveGen,
	}, nil
}

// Close cleans up benchmark resources.
func (b *BenchmarkOCIClient) Close() error {
	if b.archiveGen != nil {
		b.archiveGen.Close()
	}
	if b.registry != nil {
		return b.registry.Close(context.Background())
	}
	return nil
}

// SetupRegistry sets up a test registry for benchmarks.
func (b *BenchmarkOCIClient) SetupRegistry(ctx context.Context) error {
	registry, err := NewTestRegistry(ctx)
	if err != nil {
		return err
	}

	err = registry.WaitForReady(ctx, 30*time.Second)
	if err != nil {
		registry.Close(ctx)
		return err
	}

	b.registry = registry
	return nil
}

// BenchmarkArchiveCreation benchmarks archive creation performance.
func (b *BenchmarkOCIClient) BenchmarkArchiveCreation(bench *testing.B) {
	ctx := context.Background()

	b.RunBenchmark("ArchiveCreation_1MB", bench, func() error {
		outputPath := fmt.Sprintf("/tmp/benchmark-archive-%d.tar.gz", time.Now().UnixNano())
		defer os.Remove(outputPath) // Clean up

		_, err := b.archiveGen.GenerateTestArchive(ctx, 1024*1024, 10, "random", outputPath)
		return err
	})
}
