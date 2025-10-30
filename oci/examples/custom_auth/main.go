// Package main demonstrates various authentication methods for OCI registries.
// This example shows how to configure different authentication approaches.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"oras.land/oras-go/v2/registry/remote/auth"

	ocibundle "github.com/jmgilman/go/oci"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create sample files to bundle
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	sourceDir := "./sample-files"

	fmt.Println("🔐 Demonstrating different authentication methods...")

	// Example 1: Default Docker credential chain (most common)
	fmt.Println("\n1️⃣  Using default Docker credential chain...")
	client1, err := ocibundle.New() // Uses ~/.docker/config.json and credential helpers
	if err != nil {
		log.Fatalf("Failed to create default client: %v", err)
	}

	reference1 := "ghcr.io/your-org/bundle:v1.0.0"
	if err := demonstratePush(client1, ctx, sourceDir, reference1); err != nil {
		fmt.Printf("   Default auth example failed (expected if no credentials): %v\n", err)
	}

	// Example 2: Static authentication for specific registry
	fmt.Println("\n2️⃣  Using static authentication...")
	client2, err := ocibundle.NewWithOptions(
		ocibundle.WithStaticAuth("registry.example.com", "robot-user", "token123"),
		// Other registries will still use default Docker chain
	)
	if err != nil {
		log.Fatalf("Failed to create static auth client: %v", err)
	}

	reference2 := "registry.example.com/my-app/bundle:v1.0.0"
	if err := demonstratePush(client2, ctx, sourceDir, reference2); err != nil {
		fmt.Printf("   Static auth example failed (expected for demo registry): %v\n", err)
	}

	// Example 3: Custom credential function (advanced)
	fmt.Println("\n3️⃣  Using custom credential function...")
	customCreds := func(ctx context.Context, registry string) (auth.Credential, error) {
		switch registry {
		case "ghcr.io":
			return auth.Credential{
				Username: "your-username",
				Password: "your-personal-access-token",
			}, nil
		case "registry.company.com":
			// Could implement custom logic here
			return auth.Credential{
				Username: "robot-user",
				Password: "company-token",
			}, nil
		default:
			// Return empty credentials for anonymous access
			return auth.Credential{}, nil
		}
	}

	client3, err := ocibundle.NewWithOptions(
		ocibundle.WithCredentialFunc(customCreds),
	)
	if err != nil {
		log.Fatalf("Failed to create custom auth client: %v", err)
	}

	reference3 := "ghcr.io/your-org/bundle:v1.0.0"
	if err := demonstratePush(client3, ctx, sourceDir, reference3); err != nil {
		fmt.Printf("   Custom auth example failed (expected with placeholder credentials): %v\n", err)
	}

	// Example 4: HTTP-only registry (local development)
	fmt.Println("\n4️⃣  Using HTTP-only registry (local development)...")
	client4, err := ocibundle.NewWithOptions(
		ocibundle.WithAllowHTTP(), // Enables HTTP for all registries
		ocibundle.WithStaticAuth("localhost:5000", "test", "test"),
	)
	if err != nil {
		log.Fatalf("Failed to create HTTP client: %v", err)
	}

	reference4 := "localhost:5000/my-local/bundle:v1.0.0"
	if err := demonstratePush(client4, ctx, sourceDir, reference4); err != nil {
		fmt.Printf("   HTTP registry example failed (expected if no local registry): %v\n", err)
	}

	// Example 5: Insecure registry (testing only)
	fmt.Println("\n5️⃣  Using insecure registry (testing only)...")
	client5, err := ocibundle.NewWithOptions(
		ocibundle.WithInsecureHTTP(), // HTTP + self-signed certificates
		ocibundle.WithStaticAuth("test.registry.com:443", "user", "pass"),
	)
	if err != nil {
		log.Fatalf("Failed to create insecure client: %v", err)
	}

	reference5 := "test.registry.com/my-test/bundle:v1.0.0"
	if err := demonstratePush(client5, ctx, sourceDir, reference5); err != nil {
		fmt.Printf("   Insecure registry example failed (expected for test registry): %v\n", err)
	}

	fmt.Println("\n✅ Authentication examples completed!")
	fmt.Println("\n📝 Authentication Methods Summary:")
	fmt.Println("   • Default: Uses Docker config and credential helpers automatically")
	fmt.Println("   • Static: Override credentials for specific registries")
	fmt.Println("   • Custom: Full control over credential resolution")
	fmt.Println("   • HTTP: Enable HTTP for local registries")
	fmt.Println("   • Insecure: Allow self-signed certificates (testing only)")
}

// demonstratePush attempts to push a bundle and handles errors gracefully
func demonstratePush(client *ocibundle.Client, ctx context.Context, sourceDir, reference string) error {
	return client.Push(ctx, sourceDir, reference)
}

// createSampleFiles creates sample files for the authentication demonstration
func createSampleFiles() error {
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	files := []struct {
		name    string
		content string
	}{
		{"README.md", "# Authentication Example Bundle\n\nDemonstrates various OCI registry authentication methods."},
		{"config.yaml", "app:\n  name: auth-example\n  version: 1.0.0"},
	}

	for _, file := range files {
		path := fmt.Sprintf("%s/%s", dir, file.name)
		if err := os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", file.name, err)
		}
	}

	return nil
}
