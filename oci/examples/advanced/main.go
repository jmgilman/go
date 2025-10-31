// Package main demonstrates advanced usage of the OCI Bundle Distribution Module.
// This example shows custom configuration, error handling, and advanced features.
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

	// Get current working directory for absolute paths
	// Note: billy.NewLocal() creates a filesystem rooted at "/", so we need absolute paths
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create sample files with various types
	if err := createAdvancedSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	// Create client with custom configuration
	client, err := ocibundle.NewWithOptions(
		// Configure HTTP settings for local development
		ocibundle.WithHTTP(true, false, []string{"localhost:5000"}),
		// Set static auth for a specific registry
		ocibundle.WithStaticAuth("registry.example.com", "robot-user", "token123"),
		// Custom retry configuration
		// Note: These would be applied to push/pull operations, not client creation
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	sourceDir := filepath.Join(cwd, "advanced-files")
	reference := "registry.example.com/my-app/bundle:v2.1.0"

	// Advanced push with annotations, platform, and progress
	fmt.Println("🚀 Pushing bundle with advanced options...")

	annotations := map[string]string{
		"org.opencontainers.image.title":       "My Application Bundle",
		"org.opencontainers.image.description": "Bundle containing application files and configs",
		"org.opencontainers.image.version":     "2.1.0",
		"org.opencontainers.image.vendor":      "My Company",
		"com.example.build-id":                 "build-12345",
		"com.example.git-commit":               "abc123def456",
	}

	pushProgress := func(current, total int64) {
		if total > 0 {
			percentage := float64(current) / float64(total) * 100
			fmt.Printf("\r📊 Push progress: %.1f%% (%d/%d bytes)", percentage, current, total)
		}
	}

	pushErr := client.Push(ctx, sourceDir, reference,
		ocibundle.WithAnnotations(annotations),
		ocibundle.WithPlatform("linux/amd64"),
		ocibundle.WithProgressCallback(pushProgress),
		ocibundle.WithMaxRetries(5),
		ocibundle.WithRetryDelay(3*time.Second),
	)
	if pushErr != nil {
		log.Fatalf("Failed to push bundle: %v", pushErr)
	}
	fmt.Println("\n✅ Bundle pushed with annotations and platform info!")

	// Advanced pull with security constraints
	targetDir := filepath.Join(cwd, "extracted-advanced")
	fmt.Println("📥 Pulling bundle with security constraints...")

	pullErr := client.Pull(ctx, reference, targetDir,
		// Security limits
		ocibundle.WithMaxFiles(1000),
		ocibundle.WithMaxSize(500*1024*1024),        // 500MB
		ocibundle.WithPullMaxFileSize(50*1024*1024), // 50MB per file
		// Extraction options
		ocibundle.WithPullPreservePermissions(false),  // Sanitize permissions
		ocibundle.WithPullStripPrefix("bundle-root/"), // Remove prefix if present
		// Retry configuration
		ocibundle.WithPullMaxRetries(3),
		ocibundle.WithPullRetryDelay(2*time.Second),
	)
	if pullErr != nil {
		log.Fatalf("Failed to pull bundle: %v", pullErr)
	}
	fmt.Println("✅ Bundle pulled with security validation!")

	// Demonstrate error handling
	fmt.Println("\n🛡️  Demonstrating error handling...")

	// Try to pull non-existent reference
	err = client.Pull(ctx, "registry.example.com/nonexistent:missing", filepath.Join(cwd, "error-test"))
	if err != nil {
		fmt.Printf("Expected error for non-existent reference: %v\n", err)
	}

	// Try to push to non-existent directory
	err = client.Push(ctx, filepath.Join(cwd, "nonexistent-directory"), reference)
	if err != nil {
		fmt.Printf("Expected error for non-existent source directory: %v\n", err)
	}

	fmt.Println("\n🎉 Advanced example completed successfully!")
	fmt.Printf("Original files: %s\n", sourceDir)
	fmt.Printf("Extracted files: %s\n", targetDir)
	fmt.Println("Features demonstrated:")
	fmt.Println("  ✓ Custom client configuration")
	fmt.Println("  ✓ Annotations and platform metadata")
	fmt.Println("  ✓ Progress reporting")
	fmt.Println("  ✓ Security constraints")
	fmt.Println("  ✓ Error handling")
}

// createAdvancedSampleFiles creates a more complex directory structure for demonstration
func createAdvancedSampleFiles() error {
	baseDir := "./advanced-files"
	subDirs := []string{
		"bundle-root/config",
		"bundle-root/bin",
		"bundle-root/docs",
		"bundle-root/data",
	}

	// Create directory structure
	for _, dir := range subDirs {
		fullPath := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", fullPath, err)
		}
	}

	files := []struct {
		path    string
		content string
		mode    os.FileMode
	}{
		{"bundle-root/README.md", "# Advanced Bundle Example\n\nThis bundle demonstrates advanced features.", 0o644},
		{
			"bundle-root/config/app.yaml",
			"app:\n  name: advanced-bundle\n  version: 2.1.0\n  environment: production",
			0o644,
		},
		{"bundle-root/config/database.json", `{"host": "localhost", "port": 5432, "database": "myapp"}`, 0o600},
		{
			"bundle-root/bin/start.sh",
			"#!/bin/bash\necho 'Starting advanced application...'\n# Application startup logic here",
			0o755,
		},
		{"bundle-root/docs/API.md", "# API Documentation\n\n## Endpoints\n\n- GET /health\n- POST /data", 0o644},
		{
			"bundle-root/data/sample.txt",
			"This is sample data for the advanced bundle example.\nIt contains multiple lines of text.",
			0o644,
		},
	}

	for _, file := range files {
		fullPath := filepath.Join(baseDir, file.path)
		if err := os.WriteFile(fullPath, []byte(file.content), file.mode); err != nil {
			return fmt.Errorf("write file %s: %w", fullPath, err)
		}
	}

	return nil
}
