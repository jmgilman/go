// Package main demonstrates basic usage of the OCI Bundle Distribution Module.
// This example shows how to push and pull OCI artifacts with default settings.
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

	// Initialize the client with default settings
	// This uses ORAS's default Docker credential chain
	client, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Define the source directory and target reference
	sourceDir := filepath.Join(cwd, "sample-files")
	reference := "ghcr.io/your-org/sample-bundle:v1.0.0" // Replace with your registry

	fmt.Println("🚀 Pushing bundle to registry...")
	if err := client.Push(ctx, sourceDir, reference); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}
	fmt.Println("✅ Bundle pushed successfully!")

	// Pull the bundle to a new directory
	targetDir := filepath.Join(cwd, "pulled-files")
	fmt.Println("📥 Pulling bundle from registry...")
	if err := client.Pull(ctx, reference, targetDir); err != nil {
		log.Fatalf("Failed to pull bundle: %v", err)
	}
	fmt.Println("✅ Bundle pulled and extracted successfully!")

	fmt.Println("\n🎉 Example completed successfully!")
	fmt.Printf("Original files: %s\n", sourceDir)
	fmt.Printf("Extracted files: %s\n", targetDir)
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
	content := `This is a sample bundle created by the OCI Bundle Distribution Module.

This bundle contains:
- This README file
- A simple script
- A configuration file

The OCI Bundle Distribution Module provides secure, streaming operations
for distributing file bundles as OCI artifacts using ORAS.
`
	if err := os.WriteFile(textFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	// Create a simple script
	scriptFile := filepath.Join(dir, "hello.sh")
	script := `#!/bin/bash
echo "Hello from OCI Bundle!"
echo "This script was distributed as an OCI artifact."
`
	if err := os.WriteFile(scriptFile, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Create a config file
	configFile := filepath.Join(dir, "config.json")
	config := `{
  "name": "sample-bundle",
  "version": "1.0.0",
  "description": "Sample bundle for OCI distribution demo",
  "created": "2025-01-11"
}`
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
