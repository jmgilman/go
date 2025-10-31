// Package main demonstrates cache management operations of the OCI Bundle Distribution Module.
// This example shows how to monitor cache statistics, perform cleanup, and manage cache lifecycle.
//
// NOTE: Caching functionality is currently in development. This example demonstrates
// basic operations that will benefit from caching once it's fully implemented.
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
	fmt.Println("\nNOTE: Advanced caching features are planned for future implementation.")
	fmt.Println("This example demonstrates basic operations that will benefit from caching.\n")

	// Get current working directory for absolute paths
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create some sample files to bundle
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	// Initialize the client
	client, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fmt.Printf("📊 Client initialized\n")

	// Example 1: Push multiple bundles
	fmt.Println("\n📥 Example 1: Push Multiple Bundles")
	fmt.Println("-----------------------------------")

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

		// Pull bundle
		targetDir := filepath.Join(cwd, fmt.Sprintf("pulled-%s", bundle.name))
		fmt.Printf("📥 Pulling %s...\n", bundle.name)
		if err := client.Pull(ctx, reference, targetDir); err != nil {
			log.Fatalf("Failed to pull %s: %v", bundle.name, err)
		}
	}

	fmt.Println("✅ Bundle operations completed")

	// Example 2: Performance Comparison
	fmt.Println("\n⚡ Example 2: Performance Comparison")
	fmt.Println("------------------------------------")

	reference := "ghcr.io/your-org/cache-mgmt-small-bundle:v1.0.0"

	// First pull
	targetDir1 := filepath.Join(cwd, "performance-test-1")
	fmt.Println("🏃 First pull...")
	start := time.Now()
	if err := client.Pull(ctx, reference, targetDir1); err != nil {
		log.Fatalf("Failed first pull: %v", err)
	}
	firstPullTime := time.Since(start)
	fmt.Printf("   Completed in: %v\n", firstPullTime)

	// Second pull to different directory
	targetDir2 := filepath.Join(cwd, "performance-test-2")
	fmt.Println("🚀 Second pull...")
	start = time.Now()
	if err := client.Pull(ctx, reference, targetDir2); err != nil {
		log.Fatalf("Failed second pull: %v", err)
	}
	secondPullTime := time.Since(start)
	fmt.Printf("   Completed in: %v\n", secondPullTime)

	fmt.Println("\n✅ Performance comparison completed")
	fmt.Println("   Note: With caching enabled, the second pull would be significantly faster")

	fmt.Println("\n🎉 Example completed!")
	fmt.Println("\n💡 Planned cache management features:")
	fmt.Println("  - Cache statistics and hit/miss monitoring")
	fmt.Println("  - Cache size limits and automatic cleanup")
	fmt.Println("  - TTL-based expiration")
	fmt.Println("  - Manual cache maintenance operations")
	fmt.Println("  - Cache bypass for critical operations")
	fmt.Println("\nFor now, this example demonstrates basic push/pull operations.")
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

This bundle demonstrates cache management features (planned for future implementation):

## Planned Cache Operations

- **Statistics**: Monitor hit rates, size, and performance
- **Cleanup**: Automatic and manual cache maintenance
- **Size Limits**: Control disk usage with configurable limits
- **TTL Management**: Time-based expiration of cache entries

## Planned Cache Monitoring

Monitor these key metrics:
- Cache hit/miss ratios
- Total cache size vs limits
- Entry count and distribution
- Performance improvements

## Planned Maintenance Tasks

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
    "description": "Demonstration of OCI bundle cache management (planned)"
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
