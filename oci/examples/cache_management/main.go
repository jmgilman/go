// Package main demonstrates cache management operations of the OCI Bundle Distribution Module.
// This example shows how to monitor cache statistics, perform cleanup, and manage cache lifecycle.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Println("🚀 OCI Bundle Cache Management Example")
	fmt.Println("======================================")

	// Get current working directory for absolute paths
	// Note: billy.NewLocal() creates a filesystem rooted at "/", so we need absolute paths
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create some sample files to bundle
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	// Initialize the client with caching enabled
	cacheDir := filepath.Join(cwd, "cache-management")
	cacheSize := int64(100 * 1024 * 1024) // 100MB cache

	client, err := ocibundle.NewWithOptions(
		ocibundle.WithCache(cacheDir, cacheSize, 1*time.Hour),
		ocibundle.WithCachePolicy(ocibundle.CachePolicyEnabled),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fmt.Printf("📊 Cache Configuration:\n")
	fmt.Printf("  Directory: %s\n", cacheDir)
	fmt.Printf("  Size Limit: %d MB\n", cacheSize/(1024*1024))
	fmt.Printf("  Default TTL: 1 hour\n")

	// Example 1: Populate cache with multiple bundles
	fmt.Println("\n📥 Example 1: Populating Cache")
	fmt.Println("-----------------------------")

	bundles := []struct {
		name string
		size string
	}{
		{"small-bundle", "1KB"},
		{"medium-bundle", "100KB"},
		{"large-bundle", "1MB"},
	}

	sourceDir := filepath.Join(cwd, "sample-files")

	for i, bundle := range bundles {
		reference := fmt.Sprintf("ghcr.io/your-org/cache-mgmt-%s:v1.0.%d", bundle.name, i)

		fmt.Printf("📤 Pushing %s (%s)...\n", bundle.name, bundle.size)
		if err := client.Push(ctx, sourceDir, reference); err != nil {
			log.Fatalf("Failed to push %s: %v", bundle.name, err)
		}

		// Pull to populate cache
		targetDir := filepath.Join(cwd, fmt.Sprintf("pulled-%s", bundle.name))
		fmt.Printf("📥 Pulling %s to populate cache...\n", bundle.name)
		if err := client.PullWithCache(ctx, reference, targetDir); err != nil {
			log.Fatalf("Failed to pull %s: %v", bundle.name, err)
		}
	}

	fmt.Println("✅ Cache populated with test bundles")

	// Example 2: Cache Statistics
	fmt.Println("\n📈 Example 2: Cache Statistics")
	fmt.Println("------------------------------")

	displayCacheStats(cacheDir)

	// Example 3: Cache Performance Demonstration
	fmt.Println("\n⚡ Example 3: Cache Performance")
	fmt.Println("------------------------------")

	reference := "ghcr.io/your-org/cache-mgmt-small-bundle:v1.0.0"

	// First pull (from registry)
	targetDir1 := filepath.Join(cwd, "performance-test-1")
	fmt.Println("🏃 First pull (from registry)...")
	start := time.Now()
	if err := client.PullWithCache(ctx, reference, targetDir1); err != nil {
		log.Fatalf("Failed first pull: %v", err)
	}
	registryPullTime := time.Since(start)
	fmt.Printf("   Completed in: %v\n", registryPullTime)

	// Second pull (from cache)
	targetDir2 := filepath.Join(cwd, "performance-test-2")
	fmt.Println("🚀 Second pull (from cache)...")
	start = time.Now()
	if err := client.PullWithCache(ctx, reference, targetDir2); err != nil {
		log.Fatalf("Failed cached pull: %v", err)
	}
	cachedPullTime := time.Since(start)
	fmt.Printf("   Completed in: %v\n", cachedPullTime)

	// Calculate improvement
	if cachedPullTime < registryPullTime {
		improvement := float64(registryPullTime) / float64(cachedPullTime)
		fmt.Printf("�� Performance improvement: %.1fx faster!\n", improvement)
	}

	// Example 4: Manual Cache Management
	fmt.Println("\n🧹 Example 4: Manual Cache Management")
	fmt.Println("------------------------------------")

	fmt.Println("📂 Cache directory contents before cleanup:")
	listCacheContents(cacheDir)

	// Simulate cache maintenance (in a real scenario, this would be automatic)
	fmt.Println("\n🕐 Simulating cache maintenance...")
	time.Sleep(2 * time.Second)

	fmt.Println("📂 Cache directory contents after maintenance:")
	listCacheContents(cacheDir)

	// Example 5: Cache Size Monitoring
	fmt.Println("\n📏 Example 5: Cache Size Monitoring")
	fmt.Println("----------------------------------")

	fmt.Println("💾 Cache size information:")
	displayCacheSize(cacheDir)

	fmt.Println("\n🎉 Cache management example completed!")
	fmt.Println("\n💡 Cache management best practices:")
	fmt.Println("  - Monitor cache size and set appropriate limits")
	fmt.Println("  - Configure TTL based on your update frequency")
	fmt.Println("  - Use cache statistics to optimize performance")
	fmt.Println("  - Implement cleanup routines for maintenance")
	fmt.Println("  - Consider cache bypass for critical fresh pulls")
}

// createSampleFiles creates a directory with sample files for demonstration
func createSampleFiles() error {
	// Create the sample directory
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create a README for cache management
	readmeFile := filepath.Join(dir, "CACHE_MANAGEMENT.md")
	readme := `# OCI Bundle Cache Management

This bundle demonstrates cache management features:

## Cache Operations

- **Statistics**: Monitor hit rates, size, and performance
- **Cleanup**: Automatic and manual cache maintenance
- **Size Limits**: Control disk usage with configurable limits
- **TTL Management**: Time-based expiration of cache entries

## Cache Monitoring

Monitor these key metrics:
- Cache hit/miss ratios
- Total cache size vs limits
- Entry count and distribution
- Performance improvements

## Maintenance Tasks

- Regular cleanup of expired entries
- Size-based eviction when limits exceeded
- Integrity verification and corruption recovery
- Performance optimization

## Best Practices

1. Set appropriate size limits based on available disk space
2. Configure TTL values matching your update frequency
3. Monitor cache statistics regularly
4. Implement proper cleanup schedules
5. Use cache bypass for critical operations when needed
`

	if err := os.WriteFile(readmeFile, []byte(readme), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	// Create management configuration
	configFile := filepath.Join(dir, "management-config.json")
	config := `{
  "cacheManagement": {
    "monitoring": {
      "enabled": true,
      "metrics": ["size", "hits", "misses", "evictions"],
      "interval": "5m"
    },
    "cleanup": {
      "automatic": true,
      "schedule": "hourly",
      "maxAge": "24h"
    },
    "limits": {
      "maxSize": "100MB",
      "maxEntries": 1000,
      "evictionPolicy": "LRU"
    }
  },
  "bundle": {
    "name": "cache-management-demo",
    "version": "1.0.0",
    "description": "Demonstration of OCI bundle cache management"
  }
}`

	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Create test data of various sizes
	sizes := []struct {
		name string
		size int
	}{
		{"small.txt", 1024},        // 1KB
		{"medium.txt", 1024 * 100}, // 100KB
		{"large.txt", 1024 * 1024}, // 1MB
	}

	for _, s := range sizes {
		dataFile := filepath.Join(dir, s.name)
		data := make([]byte, s.size)
		for i := range data {
			data[i] = byte((i % 26) + 'a') // Fill with repeating alphabet
		}

		if err := os.WriteFile(dataFile, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", s.name, err)
		}
	}

	return nil
}

// displayCacheStats shows cache statistics
func displayCacheStats(cacheDir string) {
	info, err := os.Stat(cacheDir)
	if err != nil {
		fmt.Printf("Cache directory does not exist yet\n")
		return
	}

	fmt.Printf("Cache directory: %s\n", info.Name())
	fmt.Printf("Last modified: %s\n", info.ModTime().Format(time.RFC3339))
}

// listCacheContents lists the contents of the cache directory
func listCacheContents(cacheDir string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		fmt.Printf("Cannot read cache directory: %v\n", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("  (empty)")
		return
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			fmt.Printf("  %s (error getting info)\n", entry.Name())
			continue
		}

		if entry.IsDir() {
			fmt.Printf("  �� %s/ (%d items)\n", entry.Name(), countDirContents(filepath.Join(cacheDir, entry.Name())))
		} else {
			fmt.Printf("  📄 %s (%s)\n", entry.Name(), formatSize(info.Size()))
		}
	}
}

// countDirContents counts items in a directory
func countDirContents(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	return len(entries)
}

// displayCacheSize shows cache size information
func displayCacheSize(cacheDir string) {
	size, err := calculateDirSize(cacheDir)
	if err != nil {
		fmt.Printf("Error calculating cache size: %v\n", err)
		return
	}

	fmt.Printf("Total cache size: %s\n", formatSize(size))

	// Count files and directories
	fileCount, dirCount := countFilesAndDirs(cacheDir)
	fmt.Printf("Files: %d, Directories: %d\n", fileCount, dirCount)
}

// calculateDirSize calculates the total size of a directory
func calculateDirSize(dir string) (int64, error) {
	var size int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// countFilesAndDirs counts files and directories in a path
func countFilesAndDirs(dir string) (files, dirs int) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirs++
		} else {
			files++
		}
		return nil
	})
	return files, dirs
}

// formatSize formats a size in bytes to human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
