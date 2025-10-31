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

	// Example 1: Basic push and pull operations
	// TODO: Advanced caching will be demonstrated here once caching is fully implemented
	fmt.Println("\n📦 Example 1: Basic Push and Pull")
	fmt.Println("----------------------------------")

	client1, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	reference1 := "ghcr.io/your-org/advanced-cache-bundle:v1.0.0"
	sourceDir := filepath.Join(cwd, "sample-files")

	fmt.Printf("📤 Pushing bundle...\n")
	if err := client1.Push(ctx, sourceDir, reference1); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}

	// Pull bundle
	targetDir1 := filepath.Join(cwd, "pulled-bundle")
	fmt.Println("📥 Pulling bundle...")
	start := time.Now()
	if err := client1.Pull(ctx, reference1, targetDir1); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Printf("✅ Pull completed in %v\n", time.Since(start))

	// Example 2: Multiple operations
	// TODO: Demonstrate pull-only caching policy once caching is implemented
	fmt.Println("\n📦 Example 2: Multiple Operations")
	fmt.Println("----------------------------------")

	client2, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	reference2 := "ghcr.io/your-org/multiple-ops-bundle:v1.0.0"

	fmt.Println("📤 Pushing bundle...")
	if err := client2.Push(ctx, sourceDir, reference2); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}

	targetDir2 := filepath.Join(cwd, "pulled-multiple")
	fmt.Println("📥 Pulling bundle...")
	start = time.Now()
	if err := client2.Pull(ctx, reference2, targetDir2); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Printf("✅ Pull completed in %v\n", time.Since(start))

	// Example 3: Push with annotations
	// TODO: Demonstrate cache bypass once caching is implemented
	fmt.Println("\n📝 Example 3: Push with Annotations")
	fmt.Println("------------------------------------")

	client3, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	reference3 := "ghcr.io/your-org/annotated-bundle:v1.0.0"

	// Push with annotations
	annotations := map[string]string{
		"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
		"org.opencontainers.image.title":   "Annotated Bundle Example",
		"org.opencontainers.image.version": "1.0.0",
	}

	fmt.Println("📤 Pushing bundle with annotations...")
	if err := client3.Push(ctx, sourceDir, reference3,
		ocibundle.WithAnnotations(annotations),
	); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}

	targetDir3 := filepath.Join(cwd, "pulled-annotated")
	fmt.Println("📥 Pulling annotated bundle...")
	if err := client3.Pull(ctx, reference3, targetDir3); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Println("✅ Annotated bundle operations completed")

	fmt.Println("\n🎉 Example completed!")
	fmt.Println("\n💡 Note: Advanced caching features are planned for future implementation")
	fmt.Println("  - Cache size limits and TTL configuration")
	fmt.Println("  - Pull-only and push-only caching policies")
	fmt.Println("  - Cache bypass options")
	fmt.Println("  - Cache statistics and monitoring")
	fmt.Println("\nFor now, this example demonstrates basic push/pull operations.")
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
