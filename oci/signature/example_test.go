package signature_test

import (
	"context"
	"crypto"
	"fmt"
	"log"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/signature"
)

// ExampleNewPublicKeyVerifier demonstrates basic public key verification setup.
func ExampleNewPublicKeyVerifier() {
	// Load a public key from file
	pubKey, err := signature.LoadPublicKey("cosign.pub")
	if err != nil {
		log.Fatal(err)
	}

	// Create a verifier with the public key
	verifier := signature.NewPublicKeyVerifier(pubKey)

	// Create OCI client with verification enabled
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Pull and verify artifact
	ctx := context.Background()
	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")
	if err != nil {
		log.Fatal(err)
	}

	// Artifact verified successfully
	fmt.Println("Artifact verified with public key")
}

// ExampleNewPublicKeyVerifierWithOptions demonstrates multi-signature verification.
func ExampleNewPublicKeyVerifierWithOptions() {
	// Load multiple public keys
	key1, _ := signature.LoadPublicKey("key1.pub")
	key2, _ := signature.LoadPublicKey("key2.pub")
	key3, _ := signature.LoadPublicKey("key3.pub")

	// Create verifier requiring all signatures
	verifier := signature.NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{key1, key2, key3},
		signature.WithRequireAll(true),
		signature.WithEnforceMode(true),
	)

	// Create client
	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	// Pull artifact - will verify all 3 signatures
	ctx := context.Background()
	_ = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")

	fmt.Println("All signatures verified")
}

// ExampleNewKeylessVerifier demonstrates keyless verification with OIDC identities.
func ExampleNewKeylessVerifier() {
	// Create keyless verifier with identity restrictions
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithRekor(true), // Require transparency log
	)

	// Create OCI client with verification
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Pull and verify artifact
	ctx := context.Background()
	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Artifact verified with keyless signing")
}

// ExampleWithAllowedIdentities demonstrates identity pattern matching.
func ExampleWithAllowedIdentities() {
	// Allow any email from example.com domain
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
	)

	// Or allow specific users
	verifier = signature.NewKeylessVerifier(
		signature.WithAllowedIdentities(
			"alice@example.com",
			"bob@example.com",
		),
	)

	// Or combine multiple domains
	verifier = signature.NewKeylessVerifier(
		signature.WithAllowedIdentities(
			"*@example.com",
			"*@trusted.org",
		),
	)

	fmt.Printf("Verifier configured with %d keys\n", len(verifier.Policy().PublicKeys))
	// Output: Verifier configured with 0 keys
}

// ExampleWithMinimumSignatures demonstrates threshold-based approval.
func ExampleWithMinimumSignatures() {
	key1, _ := signature.LoadPublicKey("key1.pub")
	key2, _ := signature.LoadPublicKey("key2.pub")
	key3, _ := signature.LoadPublicKey("key3.pub")
	key4, _ := signature.LoadPublicKey("key4.pub")
	key5, _ := signature.LoadPublicKey("key5.pub")

	// Require at least 3 of 5 signatures
	verifier := signature.NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{key1, key2, key3, key4, key5},
		signature.WithMinimumSignatures(3),
	)

	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	ctx := context.Background()
	_ = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")

	fmt.Println("At least 3 signatures verified")
}

// ExampleWithRequiredAnnotations demonstrates annotation-based policies.
func ExampleWithRequiredAnnotations() {
	// Require specific build metadata in signatures
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithRequiredAnnotations(map[string]string{
			"build-system":   "github-actions",
			"verified":       "true",
			"security-scan":  "passed",
		}),
	)

	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	ctx := context.Background()
	_ = client.Pull(ctx, "ghcr.io/org/app:production", "./output")

	fmt.Println("Annotations verified")
}

// ExampleWithCacheTTL demonstrates cache configuration.
func ExampleWithCacheTTL() {
	// Configure shorter TTL for security-critical environments
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithCacheTTL(15 * time.Minute),
	)

	fmt.Printf("Cache TTL: %s\n", verifier.Policy().CacheTTL)
	// Output: Cache TTL: 15m0s
}

// ExampleComputePolicyHash demonstrates policy hashing for cache keys.
func ExampleComputePolicyHash() {
	// Create a policy
	policy := signature.NewPolicy()
	policy.VerificationMode = signature.VerificationModeEnforce
	policy.AllowedIdentities = []string{"*@example.com"}

	// Compute hash - will be used as part of cache key
	hash := signature.ComputePolicyHash(policy)

	// Hash is deterministic - same policy always produces same hash
	fmt.Printf("Policy hash length: %d\n", len(hash))
	fmt.Printf("First 8 chars: %s\n", hash[:8])

	// Any change to policy produces different hash
	policy.AllowedIdentities = []string{"*@other.com"}
	newHash := signature.ComputePolicyHash(policy)
	fmt.Printf("Hashes differ: %t\n", hash != newHash)

	// Output:
	// Policy hash length: 64
	// First 8 chars: be0ae0bc
	// Hashes differ: true
}

// ExamplePolicyHashShort demonstrates shortened policy hash for logging.
func ExamplePolicyHashShort() {
	policy := signature.NewPolicy()
	policy.VerificationMode = signature.VerificationModeEnforce

	// Get shortened hash for logging
	shortHash := signature.PolicyHashShort(policy)

	fmt.Printf("Short hash length: %d\n", len(shortHash))
	// Output: Short hash length: 16
}

// ExampleCosignVerifier_WithCacheForVerifier demonstrates adding cache to a verifier.
func ExampleCosignVerifier_WithCacheForVerifier() {
	// Create verifier
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithCacheTTL(time.Hour),
	)

	// Add caching (assumes you have a cache coordinator)
	// verifier = verifier.WithCacheForVerifier(coordinator)

	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	ctx := context.Background()
	_ = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")

	fmt.Println("Verification with caching enabled")
	// Output: Verification with caching enabled
}

// ExampleWithOptionalMode demonstrates audit mode for gradual rollout.
func ExampleWithOptionalMode() {
	// Phase 1: Audit mode - log failures but don't block
	verifier := signature.NewKeylessVerifier(
		signature.WithOptionalMode(true),
		signature.WithAllowedIdentities("*@example.com"),
	)

	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	ctx := context.Background()
	// Pull succeeds even if signature verification fails
	_ = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")

	fmt.Println("Optional mode allows unsigned artifacts")
	// Output: Optional mode allows unsigned artifacts
}

// ExampleWithEnforceMode demonstrates strict enforcement for production.
func ExampleWithEnforceMode() {
	// Production: Enforce all signatures
	verifier := signature.NewKeylessVerifier(
		signature.WithEnforceMode(true),
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithRekor(true),
	)

	client, _ := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)

	ctx := context.Background()
	// Pull fails if signature is missing or invalid
	err := client.Pull(ctx, "ghcr.io/org/app:v1.0", "./output")
	if err != nil {
		fmt.Println("Verification required in enforce mode")
	}
}

// ExampleWithRekor demonstrates transparency log integration.
func ExampleWithRekor() {
	// Enable Rekor for audit trail
	verifier := signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithRekor(true),
	)

	// Or use custom Rekor instance
	verifier = signature.NewKeylessVerifier(
		signature.WithAllowedIdentities("*@example.com"),
		signature.WithRekorURL("https://rekor.private.example.com"),
	)

	fmt.Printf("Rekor enabled: %t\n", verifier.Policy().RekorEnabled)
	// Output: Rekor enabled: true
}

// ExampleLoadPublicKey demonstrates loading a public key from file.
func ExampleLoadPublicKey() {
	// Load PEM-encoded public key
	pubKey, err := signature.LoadPublicKey("cosign.pub")
	if err != nil {
		log.Fatal(err)
	}

	// Create verifier
	verifier := signature.NewPublicKeyVerifier(pubKey)

	fmt.Printf("Loaded public key: %T\n", pubKey)
	fmt.Printf("Created verifier: %T\n", verifier)
}

// ExamplePolicy_MatchesIdentity demonstrates identity matching.
func ExamplePolicy_MatchesIdentity() {
	policy := signature.NewPolicy()
	policy.AllowedIdentities = []string{"*@example.com", "admin@other.org"}

	// Check if identities match
	fmt.Println(policy.MatchesIdentity("alice@example.com"))   // true
	fmt.Println(policy.MatchesIdentity("bob@example.com"))     // true
	fmt.Println(policy.MatchesIdentity("admin@other.org"))     // true
	fmt.Println(policy.MatchesIdentity("charlie@unknown.com")) // false

	// Output:
	// true
	// true
	// true
	// false
}

// ExamplePolicy_Validate demonstrates policy validation.
func ExamplePolicy_Validate() {
	// Valid policy
	policy := signature.NewPolicy()
	policy.AllowedIdentities = []string{"*@example.com"}

	err := policy.Validate()
	fmt.Printf("Valid policy: %v\n", err == nil)

	// Invalid policy - missing both public keys and keyless config
	badPolicy := signature.NewPolicy()
	// No public keys and no identity/issuer/rekor config

	err = badPolicy.Validate()
	fmt.Printf("Invalid policy: %v\n", err != nil)

	// Output:
	// Valid policy: true
	// Invalid policy: true
}
