// Package main demonstrates advanced caching configuration of the OCI Bundle Distribution Module.
// This example shows how to configure cache size limits, TTL settings, and cache policies.
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Println("🚀 Advanced OCI Bundle Caching Configuration Example")
	fmt.Println("=====================================================")

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

	// Example 1: Custom cache configuration with size limits
	fmt.Println("\n📊 Example 1: Custom Cache Configuration")
	fmt.Println("----------------------------------------")

	cacheDir := filepath.Join(cwd, "cache-storage")
	cacheSize := int64(50 * 1024 * 1024) // 50MB cache limit
	defaultTTL := 2 * time.Hour          // 2 hour TTL

	client1, err := ocibundle.NewWithOptions(
		ocibundle.WithCache(cacheDir, cacheSize, defaultTTL),
		ocibundle.WithCachePolicy(ocibundle.CachePolicyEnabled),
	)
	if err != nil {
		log.Fatalf("Failed to create client with custom cache: %v", err)
	}

	reference1 := "ghcr.io/your-org/advanced-cache-bundle:v1.0.0"
	sourceDir := filepath.Join(cwd, "sample-files")

	fmt.Printf("📤 Pushing bundle with custom cache config (50MB limit, 2h TTL)\n")
	if err := client1.Push(ctx, sourceDir, reference1); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}

	// Pull with custom cache
	targetDir1 := filepath.Join(cwd, "pulled-custom-cache")
	fmt.Println("📥 Pulling with custom cache configuration...")
	start := time.Now()
	if err := client1.PullWithCache(ctx, reference1, targetDir1); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Printf("✅ Pull completed in %v\n", time.Since(start))

	// Example 2: Pull-only caching policy
	fmt.Println("\n📥 Example 2: Pull-Only Caching Policy")
	fmt.Println("-------------------------------------")

	client2, err := ocibundle.NewWithOptions(
		ocibundle.WithCachePolicy(ocibundle.CachePolicyPull),
	)
	if err != nil {
		log.Fatalf("Failed to create client with pull-only cache: %v", err)
	}

	reference2 := "ghcr.io/your-org/pull-only-cache-bundle:v1.0.0"

	fmt.Println("📤 Pushing bundle (cache not used for push with pull-only policy)")
	if err := client2.Push(ctx, sourceDir, reference2); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}

	targetDir2 := filepath.Join(cwd, "pulled-pull-only")
	fmt.Println("📥 Pulling with pull-only cache policy...")
	start = time.Now()
	if err := client2.PullWithCache(ctx, reference2, targetDir2); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Printf("✅ Pull completed in %v\n", time.Since(start))

	// Example 3: Cache bypass demonstration
	fmt.Println("\n�� Example 3: Cache Bypass")
	fmt.Println("-------------------------")

	client3, err := ocibundle.NewWithOptions(
		ocibundle.WithCachePolicy(ocibundle.CachePolicyEnabled),
	)
	if err != nil {
		log.Fatalf("Failed to create client with cache bypass: %v", err)
	}

	reference3 := "ghcr.io/your-org/bypass-cache-bundle:v1.0.0"

	// First pull - will cache
	targetDir3a := filepath.Join(cwd, "pulled-bypass-a")
	fmt.Println("📥 First pull (will populate cache)...")
	if err := client3.PullWithCache(ctx, reference3, targetDir3a); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}

	// Second pull with bypass - will skip cache
	targetDir3b := filepath.Join(cwd, "pulled-bypass-b")
	fmt.Println("📥 Second pull with cache bypass...")
	start = time.Now()
	if err := client3.PullWithCache(ctx, reference3, targetDir3b,
		ocibundle.WithCacheBypass(true),
	); err != nil {
		log.Fatalf("Failed to pull bundle with bypass: %v", err)
	}
	fmt.Printf("✅ Bypassed pull completed in %v\n", time.Since(start))

	// Example 4: Cache statistics and management
	fmt.Println("\n📈 Example 4: Cache Statistics")
	fmt.Println("------------------------------")

	// Note: In a real implementation, you would access cache statistics
	// through the coordinator interface. For this example, we'll show
	// how to configure and manage cache directories.

	fmt.Printf("Cache directory: %s\n", cacheDir)
	fmt.Printf("Cache size limit: %d MB\n", cacheSize/(1024*1024))
	fmt.Printf("Default TTL: %v\n", defaultTTL)

	// Check if cache directory exists and show its size
	if info, err := os.Stat(cacheDir); err == nil {
		fmt.Printf("Cache directory exists: %s\n", info.Name())
	} else {
		fmt.Printf("Cache directory not yet created\n")
	}

	fmt.Println("\n🎉 Advanced caching configuration example completed!")
	fmt.Println("\n💡 Key takeaways:")
	fmt.Println("  - Configure cache size limits to control disk usage")
	fmt.Println("  - Set appropriate TTL values for your use case")
	fmt.Println("  - Use pull-only policy to avoid cache overhead on push operations")
	fmt.Println("  - Cache bypass allows forcing fresh pulls when needed")
	fmt.Println("  - Monitor cache directory usage and performance")
}

// createSampleFiles creates a directory with sample files for demonstration
func createSampleFiles() error {
	// Create the sample directory
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create a configuration file with cache settings
	configFile := filepath.Join(dir, "cache-config.json")
	config := `{
  "cache": {
    "enabled": true,
    "sizeLimit": "50MB",
    "defaultTTL": "2h",
    "policy": "enabled",
    "directory": "./cache-storage"
  },
  "registry": {
    "reference": "ghcr.io/your-org/advanced-cache-bundle:v1.0.0"
  },
  "bundle": {
    "name": "advanced-cache-demo",
    "version": "1.0.0",
    "description": "Demonstration of advanced OCI bundle caching"
  }
}`

	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Create a documentation file
	docsFile := filepath.Join(dir, "CACHE_ADVANCED.md")
	docs := `# Advanced OCI Bundle Caching

This bundle demonstrates advanced caching features:

## Cache Configuration Options

- **Size Limits**: Control maximum cache size to manage disk usage
- **TTL Settings**: Configure time-to-live for cache entries
- **Policies**: Choose between enabled, pull-only, or push-only caching
- **Bypass Options**: Force fresh pulls when needed

## Cache Policies

- **Enabled**: Cache for both push and pull operations
- **Pull**: Cache only for pull operations (default)
- **Push**: Cache only for push operations

## Performance Benefits

- First pull: Downloads from registry
- Subsequent pulls: Served from local cache
- Significant speed improvements for repeated operations
- Reduced network usage and registry load

## Cache Management

- Automatic cleanup of expired entries
- Size-based eviction when limits exceeded
- Corruption detection and recovery
- Concurrent access support
`

	if err := os.WriteFile(docsFile, []byte(docs), 0o644); err != nil {
		return fmt.Errorf("write docs: %w", err)
	}

	// Create some test data files
	for i := 1; i <= 5; i++ {
		dataFile := filepath.Join(dir, fmt.Sprintf("data-%d.txt", i))
		data := fmt.Sprintf("Advanced cache test data file %d\n", i)
		data += "This file demonstrates caching with different configurations.\n"
		data += fmt.Sprintf("Created at: %s\n", time.Now().Format(time.RFC3339))

		if err := os.WriteFile(dataFile, []byte(data), 0o644); err != nil {
			return fmt.Errorf("write data file %d: %w", i, err)
		}
	}

	return nil
}
