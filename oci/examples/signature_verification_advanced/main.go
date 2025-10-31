package main

import (
	"context"
	"crypto"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/internal/cache"
	"github.com/jmgilman/go/oci/signature"
	"github.com/jmgilman/go/fs/billy"
)

func main() {
	// Parse command-line flags
	mode := flag.String("mode", "multi-sig", "Mode: multi-sig, annotations, rekor, or caching")
	reference := flag.String("ref", "", "OCI reference to pull (required)")
	targetDir := flag.String("target", "./output", "Target directory for extraction")

	// Multi-signature options
	keyPaths := flag.String("keys", "", "Comma-separated list of public key files")
	requireAll := flag.Bool("require-all", false, "Require all signatures to be valid (AND logic)")
	minSigs := flag.Int("min-sigs", 1, "Minimum number of valid signatures required")

	// Annotation options
	annotations := flag.String("annotations", "", "Required annotations (format: key1=value1,key2=value2)")

	// Keyless options
	identity := flag.String("identity", "*@example.com", "Allowed identity pattern")
	issuer := flag.String("issuer", "", "Required OIDC issuer")
	rekorURL := flag.String("rekor-url", "", "Custom Rekor URL (default: public Sigstore)")

	// Cache options
	cacheDir := flag.String("cache-dir", "/tmp/oci-cache", "Cache directory")
	cacheTTL := flag.Duration("cache-ttl", time.Hour, "Cache TTL duration")

	flag.Parse()

	if *reference == "" {
		fmt.Println("Error: -ref flag is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Run the appropriate mode
	switch *mode {
	case "multi-sig":
		runMultiSignatureExample(ctx, *keyPaths, *requireAll, *minSigs, *reference, *targetDir)

	case "annotations":
		runAnnotationPolicyExample(ctx, *identity, *annotations, *reference, *targetDir)

	case "rekor":
		runRekorExample(ctx, *identity, *issuer, *rekorURL, *reference, *targetDir)

	case "caching":
		runCachingExample(ctx, *identity, *cacheDir, *cacheTTL, *reference, *targetDir)

	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

// runMultiSignatureExample demonstrates multi-signature verification
func runMultiSignatureExample(ctx context.Context, keyPaths string, requireAll bool, minSigs int, reference, targetDir string) {
	fmt.Println("=== Multi-Signature Verification Example ===")

	if keyPaths == "" {
		log.Fatal("Error: -keys flag required for multi-sig mode (comma-separated key files)")
	}

	// Load all public keys
	keyFiles := strings.Split(keyPaths, ",")
	pubKeys := make([]crypto.PublicKey, 0, len(keyFiles))

	fmt.Println("Loading public keys:")
	for _, keyFile := range keyFiles {
		keyFile = strings.TrimSpace(keyFile)
		pubKey, err := signature.LoadPublicKey(keyFile)
		if err != nil {
			log.Fatalf("Failed to load key %s: %v", keyFile, err)
		}
		pubKeys = append(pubKeys, pubKey)
		fmt.Printf("  ✓ Loaded: %s\n", keyFile)
	}

	// Build verifier options
	opts := []signature.VerifierOption{}

	if requireAll {
		fmt.Println("\nPolicy: ALL signatures must be valid (AND logic)")
		opts = append(opts, signature.WithRequireAll(true))
	} else if minSigs > 1 {
		fmt.Printf("\nPolicy: At least %d signatures must be valid (threshold)\n", minSigs)
		opts = append(opts, signature.WithMinimumSignatures(minSigs))
	} else {
		fmt.Println("\nPolicy: ANY signature valid (OR logic)")
	}

	// Create verifier
	verifier := signature.NewPublicKeyVerifierWithOptions(pubKeys, opts...)

	// Create client and pull
	pullAndExtract(ctx, verifier, reference, targetDir)
}

// runAnnotationPolicyExample demonstrates annotation-based policies
func runAnnotationPolicyExample(ctx context.Context, identity, annotationsStr, reference, targetDir string) {
	fmt.Println("=== Annotation Policy Example ===")

	if annotationsStr == "" {
		log.Fatal("Error: -annotations flag required for annotations mode (format: key1=value1,key2=value2)")
	}

	// Parse annotations
	requiredAnnotations := make(map[string]string)
	pairs := strings.Split(annotationsStr, ",")

	fmt.Println("Required Annotations:")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid annotation format: %s (expected key=value)", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		requiredAnnotations[key] = value
		fmt.Printf("  %s = %s\n", key, value)
	}

	// Create keyless verifier with annotation policy
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities(identity),
		signature.WithRequiredAnnotations(requiredAnnotations),
		signature.WithRekor(true),
	)

	fmt.Printf("\nIdentity Pattern: %s\n", identity)
	fmt.Println("Rekor: Enabled")

	// Create client and pull
	pullAndExtract(ctx, verifier, reference, targetDir)
}

// runRekorExample demonstrates Rekor transparency log integration
func runRekorExample(ctx context.Context, identity, issuer, rekorURL, reference, targetDir string) {
	fmt.Println("=== Rekor Transparency Log Example ===")

	fmt.Println("Configuration:")
	fmt.Printf("  Identity: %s\n", identity)

	opts := []signature.VerifierOption{
		signature.WithAllowedIdentities(identity),
		signature.WithRekor(true),
	}

	if issuer != "" {
		fmt.Printf("  Issuer: %s\n", issuer)
		opts = append(opts, signature.WithRequiredIssuer(issuer))
	} else {
		fmt.Println("  Issuer: Any")
	}

	if rekorURL != "" {
		fmt.Printf("  Rekor URL: %s\n", rekorURL)
		opts = append(opts, signature.WithRekorURL(rekorURL))
	} else {
		fmt.Println("  Rekor URL: https://rekor.sigstore.dev (public)")
	}

	fmt.Println("\nRekor provides:")
	fmt.Println("  ✓ Transparency: All signatures are publicly logged")
	fmt.Println("  ✓ Non-repudiation: Signer cannot deny signing")
	fmt.Println("  ✓ Audit trail: Historical record of all signatures")
	fmt.Println("  ✓ Timestamp: Trusted timestamp of signature creation")

	// Create keyless verifier with Rekor
	verifier := signature.NewKeylessVerifier(opts...)

	// Create client and pull
	pullAndExtract(ctx, verifier, reference, targetDir)
}

// runCachingExample demonstrates verification result caching
func runCachingExample(ctx context.Context, identity, cacheDir string, cacheTTL time.Duration, reference, targetDir string) {
	fmt.Println("=== Verification Caching Example ===")

	fmt.Println("Cache Configuration:")
	fmt.Printf("  Directory: %s\n", cacheDir)
	fmt.Printf("  TTL: %s\n", cacheTTL)
	fmt.Println()

	// Create filesystem for cache
	fs := billy.NewLocal()

	// Create cache coordinator
	cacheConfig := cache.Config{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		DefaultTTL:   cacheTTL,
	}

	coordinator, err := cache.NewCoordinator(ctx, cacheConfig, fs, cacheDir, nil)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer coordinator.Close()

	// Create verifier with caching
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities(identity),
		signature.WithRekor(true),
		signature.WithCacheTTL(cacheTTL),
	).WithCacheForVerifier(coordinator)

	fmt.Println("Performing first pull (cache miss - full verification):")
	start := time.Now()

	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	err = client.Pull(ctx, reference, targetDir)
	firstDuration := time.Since(start)

	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ Verification completed in %s\n", firstDuration)
	fmt.Printf("  ✓ Result cached for %s\n\n", cacheTTL)

	// Second pull to demonstrate caching
	fmt.Println("Performing second pull (cache hit - cached verification):")
	targetDir2 := targetDir + "-2"

	start = time.Now()
	err = client.Pull(ctx, reference, targetDir2)
	secondDuration := time.Since(start)

	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ Verification completed in %s\n", secondDuration)

	speedup := float64(firstDuration) / float64(secondDuration)
	fmt.Printf("\n  Performance Improvement: %.1fx faster with caching\n", speedup)
	fmt.Printf("  Time Saved: %s\n", firstDuration-secondDuration)

	fmt.Println("\n✓ Both artifacts extracted successfully!")
	fmt.Printf("  First extraction: %s\n", targetDir)
	fmt.Printf("  Second extraction: %s\n", targetDir2)
}

// pullAndExtract is a helper to pull and extract with a given verifier
func pullAndExtract(ctx context.Context, verifier *signature.CosignVerifier, reference, targetDir string) {
	fmt.Printf("Pulling: %s\n", reference)
	fmt.Printf("Target: %s\n\n", targetDir)

	// Create client with verification
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Pull artifact
	err = client.Pull(ctx, reference, targetDir)
	if err != nil {
		handleError(err)
		os.Exit(1)
	}

	fmt.Println("\n✓ Signature verified successfully!")
	fmt.Printf("✓ Artifact extracted to: %s\n", targetDir)
}

// handleError provides detailed error information
func handleError(err error) {
	fmt.Println("\n✗ Verification failed!")

	var bundleErr *ocibundle.BundleError
	if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
		fmt.Println("\nSignature Verification Error:")
		if bundleErr.SignatureInfo != nil {
			fmt.Printf("  Digest: %s\n", bundleErr.SignatureInfo.Digest)
			fmt.Printf("  Reason: %s\n", bundleErr.SignatureInfo.Reason)
			fmt.Printf("  Stage: %s\n", bundleErr.SignatureInfo.FailureStage)
		}
	} else {
		fmt.Printf("\nError: %v\n", err)
	}
}
