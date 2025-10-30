// Package main demonstrates basic usage with progress reporting.
// This example shows how to monitor push/pull operations with progress callbacks.
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create sample files to bundle
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	// Initialize the client
	client, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	sourceDir := "./sample-files"
	reference := "ghcr.io/your-org/sample-bundle:v1.0.0" // Replace with your registry

	// Push with progress reporting
	fmt.Println("🚀 Pushing bundle with progress reporting...")
	pushProgress := func(current, total int64) {
		if total > 0 {
			percentage := float64(current) / float64(total) * 100
			fmt.Printf("\r📊 Push progress: %.1f%% (%d/%d bytes)", percentage, current, total)
		} else {
			fmt.Printf("\r📊 Push progress: %d bytes", current)
		}
	}

	if err := client.Push(ctx, sourceDir, reference,
		ocibundle.WithProgressCallback(pushProgress),
	); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}
	fmt.Println("\n✅ Bundle pushed successfully!")

	// Pull bundle
	targetDir := "./pulled-files"
	fmt.Println("📥 Pulling bundle...")

	if err := client.Pull(ctx, reference, targetDir); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Println("\n✅ Bundle pulled and extracted successfully!")

	fmt.Println("\n🎉 Example completed successfully!")
}

// createSampleFiles creates sample files for the demonstration
func createSampleFiles() error {
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Create multiple files to show progress
	files := []struct {
		name    string
		content string
	}{
		{"README.md", "# Sample Bundle\n\nThis bundle demonstrates progress reporting."},
		{"config.yaml", "app:\n  name: sample-bundle\n  version: 1.0.0"},
		{"script.py", "print('Hello from OCI Bundle!')\nprint('This was distributed as an OCI artifact.')"},
		{"data.json", `{"items": ["item1", "item2", "item3"], "metadata": {"created": "2025-01-11"}}`},
	}

	for _, file := range files {
		path := filepath.Join(dir, file.name)
		if err := os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", file.name, err)
		}
	}

	return nil
}
