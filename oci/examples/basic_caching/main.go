// Package main demonstrates basic caching functionality of the OCI Bundle Distribution Module.
// This example shows how to use caching to improve performance for repeated pulls.
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("🚀 OCI Bundle Caching Example")
	fmt.Println("================================")

	// Create some sample files to bundle
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	// Initialize the client with caching enabled
	// This uses ORAS's default Docker credential chain
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithCachePolicy(ocibundle.CachePolicyEnabled),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Define the source directory and target reference
	sourceDir := "./sample-files"
	reference := "ghcr.io/your-org/cached-bundle:v1.0.0" // Replace with your registry

	fmt.Printf("📤 Pushing bundle to registry: %s\n", reference)
	if err := client.Push(ctx, sourceDir, reference); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}
	fmt.Println("✅ Bundle pushed successfully!")

	// Pull the bundle to a new directory (first time - will cache)
	targetDir1 := "./pulled-files-first"
	fmt.Println("📥 First pull (will cache)...")
	start := time.Now()
	if err := client.PullWithCache(ctx, reference, targetDir1); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	firstPullDuration := time.Since(start)
	fmt.Printf("✅ First pull completed in %v\n", firstPullDuration)

	// Pull the bundle again (should use cache)
	targetDir2 := "./pulled-files-cached"
	fmt.Println("📥 Second pull (should use cache)...")
	start = time.Now()
	if err := client.PullWithCache(ctx, reference, targetDir2); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	cachedPullDuration := time.Since(start)
	fmt.Printf("✅ Cached pull completed in %v\n", cachedPullDuration)

	// Calculate performance improvement
	if cachedPullDuration < firstPullDuration {
		improvement := float64(firstPullDuration) / float64(cachedPullDuration)
		fmt.Printf("🚀 Performance improvement: %.1fx faster!\n", improvement)
	}

	fmt.Println("\n🎉 Caching example completed successfully!")
	fmt.Printf("Original files: %s\n", sourceDir)
	fmt.Printf("First pull:     %s\n", targetDir1)
	fmt.Printf("Cached pull:    %s\n", targetDir2)
	fmt.Println("\n💡 Tip: The cache automatically stores pulled bundles to speed up")
	fmt.Println("   subsequent pulls of the same reference.")
}

// createSampleFiles creates a directory with sample files for demonstration
func createSampleFiles() error {
	// Create the sample directory
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create a simple text file
	textFile := filepath.Join(dir, "README.txt")
	content := `This is a sample bundle demonstrating OCI caching.

This bundle contains:
- This README file
- A simple script
- A configuration file
- Test data

The OCI Bundle Distribution Module with caching provides:
- Automatic caching of pulled bundles
- Significant performance improvements for repeated pulls
- Transparent cache management
- Registry fallback on cache corruption
`

	if err := os.WriteFile(textFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	// Create a simple script
	scriptFile := filepath.Join(dir, "hello.sh")
	script := `#!/bin/bash
echo "Hello from cached OCI Bundle!"
echo "This bundle was pulled with caching enabled."
echo "Subsequent pulls of this bundle will be much faster!"
`

	if err := os.WriteFile(scriptFile, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Create a config file
	configFile := filepath.Join(dir, "config.json")
	config := `{
  "name": "cached-bundle",
  "version": "1.0.0",
  "description": "Sample bundle for OCI caching demo",
  "features": ["caching", "fast-pulls", "registry-fallback"],
  "created": "` + time.Now().Format("2006-01-02") + `"
}`

	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Create some test data
	dataFile := filepath.Join(dir, "data.txt")
	data := ""
	for i := 0; i < 1000; i++ {
		data += fmt.Sprintf("Line %d: This is test data for the cached bundle.\n", i+1)
	}

	if err := os.WriteFile(dataFile, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}
