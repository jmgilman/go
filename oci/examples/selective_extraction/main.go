// Package main demonstrates selective file extraction from OCI bundles.
// This example shows how to extract only specific files using glob patterns,
// which can significantly reduce disk I/O and CPU usage when you only need
// a subset of files from a large bundle.
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

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create sample files representing a typical application bundle
	if err := createSampleBundle(); err != nil {
		log.Fatalf("Failed to create sample bundle: %v", err)
	}

	// Initialize the client
	client, err := ocibundle.New()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Push the complete bundle
	sourceDir := filepath.Join(cwd, "app-bundle")
	reference := "ghcr.io/your-org/app-bundle:v1.0.0" // Replace with your registry

	fmt.Println("🚀 Pushing complete application bundle...")
	if err := client.Push(ctx, sourceDir, reference); err != nil {
		log.Fatalf("Failed to push bundle: %v", err)
	}
	fmt.Println("✅ Bundle pushed successfully!\n")

	// Example 1: Extract only JSON configuration files
	fmt.Println("📥 Example 1: Extracting only JSON configuration files...")
	configDir := filepath.Join(cwd, "config-only")
	if err := client.Pull(ctx, reference, configDir,
		ocibundle.WithFilesToExtract("**/*.json"),
	); err != nil {
		log.Fatalf("Failed to pull config: %v", err)
	}
	fmt.Printf("✅ Extracted JSON files to: %s\n\n", configDir)

	// Example 2: Extract only source code files
	fmt.Println("📥 Example 2: Extracting only source code...")
	srcDir := filepath.Join(cwd, "source-only")
	if err := client.Pull(ctx, reference, srcDir,
		ocibundle.WithFilesToExtract("**/*.go", "**/*.mod", "**/*.sum"),
	); err != nil {
		log.Fatalf("Failed to pull source: %v", err)
	}
	fmt.Printf("✅ Extracted source files to: %s\n\n", srcDir)

	// Example 3: Extract specific configuration and secrets
	fmt.Println("📥 Example 3: Extracting runtime configuration...")
	runtimeDir := filepath.Join(cwd, "runtime")
	if err := client.Pull(ctx, reference, runtimeDir,
		ocibundle.WithFilesToExtract(
			"config/app.json",       // Specific file
			"config/database.yaml",  // Another specific file
			"secrets/*.env",         // All .env files in secrets/
		),
	); err != nil {
		log.Fatalf("Failed to pull runtime config: %v", err)
	}
	fmt.Printf("✅ Extracted runtime files to: %s\n\n", runtimeDir)

	// Example 4: Extract with security limits
	fmt.Println("📥 Example 4: Extracting with size limits...")
	limitedDir := filepath.Join(cwd, "limited")
	if err := client.Pull(ctx, reference, limitedDir,
		ocibundle.WithFilesToExtract("**/*.json"),
		ocibundle.WithPullMaxSize(1*1024*1024),      // 1MB total
		ocibundle.WithPullMaxFileSize(100*1024),     // 100KB per file
		ocibundle.WithPullMaxFiles(10),              // Max 10 files
	); err != nil {
		log.Fatalf("Failed to pull with limits: %v", err)
	}
	fmt.Printf("✅ Extracted with limits to: %s\n\n", limitedDir)

	fmt.Println("🎉 All selective extraction examples completed successfully!")
	fmt.Println("\nExtracted directories:")
	fmt.Printf("  - Config only:  %s\n", configDir)
	fmt.Printf("  - Source only:  %s\n", srcDir)
	fmt.Printf("  - Runtime:      %s\n", runtimeDir)
	fmt.Printf("  - Limited:      %s\n", limitedDir)
	fmt.Println("\nBenefits of selective extraction:")
	fmt.Println("  ✓ Faster extraction (skips unwanted files)")
	fmt.Println("  ✓ Saves disk space (only writes needed files)")
	fmt.Println("  ✓ Reduces I/O (non-matching files never touched)")
	fmt.Println("  ✓ Lower CPU usage (less decompression work)")
}

// createSampleBundle creates a sample application bundle with various file types
func createSampleBundle() error {
	base := "./app-bundle"

	// Create directory structure
	dirs := []string{
		"config",
		"secrets",
		"src/handlers",
		"src/models",
		"bin",
		"docs",
		"data",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Create configuration files (JSON/YAML)
	files := map[string]string{
		"config/app.json": `{
  "name": "sample-app",
  "version": "1.0.0",
  "port": 8080
}`,
		"config/database.yaml": `database:
  host: localhost
  port: 5432
  name: myapp
`,
		"config/logging.json": `{
  "level": "info",
  "format": "json"
}`,

		// Secrets
		"secrets/production.env": "DATABASE_PASSWORD=secret123\nAPI_KEY=abc123",
		"secrets/staging.env": "DATABASE_PASSWORD=staging\nAPI_KEY=staging",

		// Source code
		"src/main.go": `package main

func main() {
	println("Hello, World!")
}`,
		"src/handlers/api.go": `package handlers

func HandleAPI() {
	// API handler
}`,
		"src/models/user.go": `package models

type User struct {
	ID   int
	Name string
}`,
		"src/go.mod": `module github.com/example/app

go 1.24`,

		// Binary
		"bin/app": "#!/bin/bash\necho \"This is a binary placeholder\"",

		// Documentation
		"docs/README.md": "# Application Documentation\n\nThis is the app documentation.",
		"docs/API.md": "# API Documentation\n\nAPI endpoints and usage.",

		// Data files
		"data/sample.csv": "id,name,value\n1,test,100\n2,demo,200",
		"data/sample.json": `{"records": [{"id": 1, "name": "test"}]}`,
	}

	for path, content := range files {
		fullPath := filepath.Join(base, path)
		mode := os.FileMode(0o644)
		if filepath.Ext(path) == "" && filepath.Dir(path) == "bin" {
			mode = 0o755 // Executables
		}
		if err := os.WriteFile(fullPath, []byte(content), mode); err != nil {
			return fmt.Errorf("write file %s: %w", path, err)
		}
	}

	return nil
}
