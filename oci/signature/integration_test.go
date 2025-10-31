//go:build integration

package signature_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/signature"
)

// TestIntegration_PublicKeyVerification tests public key verification against a real local registry.
// This test requires:
// - Docker running
// - cosign CLI installed
func TestIntegration_PublicKeyVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Check prerequisites
	if err := checkDockerRunning(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if err := checkCosignInstalled(); err != nil {
		t.Skipf("Cosign not installed: %v", err)
	}

	// Start local registry
	registryPort := "5555"
	registryName := "test-registry-" + time.Now().Format("20060102150405")

	cleanup, err := startLocalRegistry(t, registryName, registryPort)
	if err != nil {
		t.Fatalf("Failed to start local registry: %v", err)
	}
	defer cleanup()

	// Wait for registry to be ready
	time.Sleep(2 * time.Second)

	// Create test directory
	testDir := t.TempDir()

	// Generate key pair
	privateKeyPath := filepath.Join(testDir, "cosign.key")
	publicKeyPath := filepath.Join(testDir, "cosign.pub")

	if err := generateKeyPair(privateKeyPath, publicKeyPath); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Build and push test image
	imageRef := fmt.Sprintf("localhost:%s/test-image:v1.0", registryPort)
	if err := buildAndPushTestImage(ctx, imageRef, testDir); err != nil {
		t.Fatalf("Failed to build and push test image: %v", err)
	}

	// Sign the image with cosign
	if err := signImageWithCosign(imageRef, privateKeyPath); err != nil {
		t.Fatalf("Failed to sign image: %v", err)
	}

	// Load public key
	pubKey, err := signature.LoadPublicKey(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	// Create verifier
	verifier := signature.NewPublicKeyVerifier(pubKey)

	// Create client with verification
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Pull and verify
	outputDir := filepath.Join(testDir, "output")
	err = client.Pull(ctx, imageRef, outputDir)
	if err != nil {
		t.Errorf("Pull with valid signature failed: %v", err)
	}

	// Verify output exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Error("Output directory was not created")
	}

	t.Logf("Successfully verified signed artifact from local registry")
}

// TestIntegration_InvalidSignature tests that verification fails with an invalid signature.
func TestIntegration_InvalidSignature(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	if err := checkDockerRunning(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if err := checkCosignInstalled(); err != nil {
		t.Skipf("Cosign not installed: %v", err)
	}

	registryPort := "5556"
	registryName := "test-registry-invalid-" + time.Now().Format("20060102150405")

	cleanup, err := startLocalRegistry(t, registryName, registryPort)
	if err != nil {
		t.Fatalf("Failed to start local registry: %v", err)
	}
	defer cleanup()

	time.Sleep(2 * time.Second)

	testDir := t.TempDir()

	// Generate TWO different key pairs
	privateKeyPath1 := filepath.Join(testDir, "cosign1.key")
	publicKeyPath1 := filepath.Join(testDir, "cosign1.pub")
	privateKeyPath2 := filepath.Join(testDir, "cosign2.key")
	publicKeyPath2 := filepath.Join(testDir, "cosign2.pub")

	if err := generateKeyPair(privateKeyPath1, publicKeyPath1); err != nil {
		t.Fatalf("Failed to generate key pair 1: %v", err)
	}
	if err := generateKeyPair(privateKeyPath2, publicKeyPath2); err != nil {
		t.Fatalf("Failed to generate key pair 2: %v", err)
	}

	// Build and push test image
	imageRef := fmt.Sprintf("localhost:%s/test-image:v1.0", registryPort)
	if err := buildAndPushTestImage(ctx, imageRef, testDir); err != nil {
		t.Fatalf("Failed to build and push test image: %v", err)
	}

	// Sign with key 1
	if err := signImageWithCosign(imageRef, privateKeyPath1); err != nil {
		t.Fatalf("Failed to sign image: %v", err)
	}

	// Try to verify with key 2 (wrong key)
	pubKey2, err := signature.LoadPublicKey(publicKeyPath2)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	verifier := signature.NewPublicKeyVerifier(pubKey2)
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	outputDir := filepath.Join(testDir, "output")
	err = client.Pull(ctx, imageRef, outputDir)

	// Should fail with signature verification error
	if err == nil {
		t.Error("Expected verification to fail with wrong key, but it succeeded")
	}

	t.Logf("Correctly rejected signature from wrong key: %v", err)
}

// TestIntegration_UnsignedArtifact tests behavior with unsigned artifacts.
func TestIntegration_UnsignedArtifact(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	if err := checkDockerRunning(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	registryPort := "5557"
	registryName := "test-registry-unsigned-" + time.Now().Format("20060102150405")

	cleanup, err := startLocalRegistry(t, registryName, registryPort)
	if err != nil {
		t.Fatalf("Failed to start local registry: %v", err)
	}
	defer cleanup()

	time.Sleep(2 * time.Second)

	testDir := t.TempDir()

	// Build and push test image WITHOUT signing
	imageRef := fmt.Sprintf("localhost:%s/test-image:v1.0", registryPort)
	if err := buildAndPushTestImage(ctx, imageRef, testDir); err != nil {
		t.Fatalf("Failed to build and push test image: %v", err)
	}

	// Generate a key for verification
	privateKeyPath := filepath.Join(testDir, "cosign.key")
	publicKeyPath := filepath.Join(testDir, "cosign.pub")
	if err := generateKeyPair(privateKeyPath, publicKeyPath); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	pubKey, err := signature.LoadPublicKey(publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to load public key: %v", err)
	}

	// Test with Optional mode (should succeed)
	t.Run("OptionalMode", func(t *testing.T) {
		verifier := signature.NewPublicKeyVerifierWithOptions(
			[]crypto.PublicKey{pubKey},
			signature.WithOptionalMode(true),
		)

		client, err := ocibundle.NewWithOptions(
			ocibundle.WithSignatureVerifier(verifier),
		)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		outputDir := filepath.Join(testDir, "output-optional")
		err = client.Pull(ctx, imageRef, outputDir)
		if err != nil {
			t.Errorf("Pull in optional mode should succeed with unsigned artifact: %v", err)
		}
	})

	// Test with Enforce mode (should fail)
	t.Run("EnforceMode", func(t *testing.T) {
		verifier := signature.NewPublicKeyVerifierWithOptions(
			[]crypto.PublicKey{pubKey},
			signature.WithEnforceMode(true),
		)

		client, err := ocibundle.NewWithOptions(
			ocibundle.WithSignatureVerifier(verifier),
		)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		outputDir := filepath.Join(testDir, "output-enforce")
		err = client.Pull(ctx, imageRef, outputDir)
		if err == nil {
			t.Error("Pull in enforce mode should fail with unsigned artifact")
		} else {
			t.Logf("Correctly failed in enforce mode: %v", err)
		}
	})
}

// Helper functions

func checkDockerRunning() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not running: %w", err)
	}
	return nil
}

func checkCosignInstalled() error {
	cmd := exec.Command("cosign", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign not installed: %w", err)
	}
	return nil
}

func startLocalRegistry(t *testing.T, name, port string) (func(), error) {
	// Start registry container
	cmd := exec.Command("docker", "run", "-d",
		"--name", name,
		"-p", port+":5000",
		"registry:2")

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to start registry: %w", err)
	}

	cleanup := func() {
		// Stop and remove container
		exec.Command("docker", "stop", name).Run()
		exec.Command("docker", "rm", name).Run()
	}

	return cleanup, nil
}

func generateKeyPair(privateKeyPath, publicKeyPath string) error {
	// Generate ECDSA key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Marshal private key to PEM
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Marshal public key to PEM
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}

func buildAndPushTestImage(ctx context.Context, imageRef, testDir string) error {
	// Create a simple Dockerfile
	dockerfile := `FROM alpine:latest
RUN echo "Test artifact" > /test.txt
CMD ["cat", "/test.txt"]
`
	dockerfilePath := filepath.Join(testDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	// Build image
	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", imageRef, testDir)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	// Push image
	pushCmd := exec.CommandContext(ctx, "docker", "push", imageRef)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}

func signImageWithCosign(imageRef, privateKeyPath string) error {
	// Sign with cosign (using --key flag and COSIGN_PASSWORD env var)
	cmd := exec.Command("cosign", "sign", "--key", privateKeyPath, "--yes", imageRef)
	cmd.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to sign image: %w", err)
	}

	return nil
}
