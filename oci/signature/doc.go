// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
//
// This package implements cryptographic signature verification for OCI artifacts,
// enabling supply chain security by ensuring artifacts are verified before extraction.
// It supports both traditional public key cryptography and modern keyless signing
// with OIDC identities.
//
// # Overview
//
// The signature verification module is implemented as a separate submodule to keep
// Cosign dependencies (23MB+) optional. Users who need signature verification can
// opt-in without affecting those who don't.
//
// Key features:
//   - Multiple verification modes (public key and keyless/OIDC)
//   - Policy-based control (identity, annotation, issuer restrictions)
//   - Performance caching to avoid redundant cryptographic operations
//   - Comprehensive error context for debugging
//   - Standards compliance (Sigstore/Cosign format)
//
// # Quick Start
//
// Public Key Verification:
//
//	import (
//	    "github.com/jmgilman/go/oci"
//	    "github.com/jmgilman/go/oci/signature"
//	)
//
//	// Load public key
//	pubKey, err := signature.LoadPublicKey("cosign.pub")
//	if err != nil {
//	    return err
//	}
//
//	// Create signature verifier
//	v := signature.NewPublicKeyVerifier(pubKey)
//
//	// Create client with verification
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(v),
//	)
//
//	// Pull will verify signature before extraction
//	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
//
// Keyless Verification:
//
//	// Create keyless verifier with identity restrictions
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true), // Require transparency log
//	)
//
//	// Create client with verification
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(verifier),
//	)
//
//	// Pull will verify signature using Sigstore keyless signing
//	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
//
// # Verification Modes
//
// Public Key Verification:
//
// Uses traditional public/private key pairs for signing and verification.
// Works offline, simple and fast, but requires key management.
//
//	pubKey, _ := signature.LoadPublicKey("cosign.pub")
//	verifier := signature.NewPublicKeyVerifier(pubKey)
//
// Keyless Verification:
//
// Uses Sigstore's keyless signing infrastructure with OIDC identities and
// short-lived certificates. No key management needed, built-in transparency
// via Rekor, but requires internet access.
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@trusted-org.com"),
//	    signature.WithRequiredIssuer("https://github.com/login/oauth"),
//	    signature.WithRekor(true),
//	)
//
// # Policy Configuration
//
// Control signature verification behavior with flexible policies:
//
//	verifier := signature.NewKeylessVerifier(
//	    // Identity policies
//	    signature.WithAllowedIdentities("*@trusted-org.com"),
//	    signature.WithRequiredIssuer("https://github.com/login/oauth"),
//
//	    // Annotation policies
//	    signature.WithRequiredAnnotations(map[string]string{
//	        "build-system": "github-actions",
//	        "verified": "true",
//	    }),
//
//	    // Transparency requirements
//	    signature.WithRekor(true),
//
//	    // Enforcement mode
//	    signature.WithEnforceMode(true), // Require all artifacts to be signed
//	)
//
// # Verification Caching
//
// Cache verification results to improve performance:
//
//	import "github.com/jmgilman/go/oci/internal/cache"
//
//	// Create cache coordinator
//	cacheConfig := cache.Config{
//	    MaxSizeBytes: 100 * 1024 * 1024, // 100MB
//	    DefaultTTL:   time.Hour,
//	}
//	coordinator, _ := cache.NewCoordinator(ctx, cacheConfig, fs, "/cache", logger)
//
//	// Create verifier with caching
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithCacheTTL(time.Hour),
//	).WithCacheForVerifier(coordinator)
//
// Performance Impact:
//   - Without caching: 55-730ms per verification (depending on mode)
//   - With caching: <1ms per verification (99.8% reduction)
//   - Cache hit rate: Typically 90%+ for repeated pulls
//
// # Error Handling
//
// Signature verification errors provide detailed context:
//
//	err := client.Pull(ctx, reference, targetDir)
//	if err != nil {
//	    var bundleErr *ocibundle.BundleError
//	    if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
//	        // Signature verification failed
//	        fmt.Printf("Digest: %s\n", bundleErr.SignatureInfo.Digest)
//	        fmt.Printf("Reason: %s\n", bundleErr.SignatureInfo.Reason)
//	        fmt.Printf("Stage: %s\n", bundleErr.SignatureInfo.FailureStage)
//
//	        // Check specific error type
//	        switch {
//	        case errors.Is(bundleErr.Err, ocibundle.ErrSignatureNotFound):
//	            // Handle missing signature
//	        case errors.Is(bundleErr.Err, ocibundle.ErrUntrustedSigner):
//	            // Handle untrusted identity
//	        case errors.Is(bundleErr.Err, ocibundle.ErrSignatureInvalid):
//	            // Handle invalid signature
//	        }
//	    }
//	}
//
// # Multiple Signatures
//
// Handle artifacts with multiple signatures:
//
//	// ANY signature valid (default, OR logic)
//	verifier := signature.NewPublicKeyVerifier(key1, key2, key3)
//
//	// ALL signatures must be valid (AND logic)
//	verifier := signature.NewPublicKeyVerifierWithOptions(
//	    []crypto.PublicKey{key1, key2, key3},
//	    signature.WithRequireAll(true),
//	)
//
//	// Minimum N signatures required (threshold)
//	verifier := signature.NewPublicKeyVerifierWithOptions(
//	    []crypto.PublicKey{key1, key2, key3},
//	    signature.WithMinimumSignatures(2), // At least 2 of 3
//	)
//
// # Security Best Practices
//
// 1. Enforce mode in production:
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithEnforceMode(true),
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true),
//	)
//
// 2. Restrict identity patterns:
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities(
//	        "ci-bot@example.com",
//	        "release-manager@example.com",
//	    ),
//	)
//
// 3. Enable Rekor for keyless signing:
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true), // Provides transparency and audit trail
//	)
//
// 4. Monitor verification failures for security monitoring.
//
// 5. Regularly rotate keys (for public key mode, every 90 days).
//
// # Gradual Rollout
//
// Phase 1: Audit mode (weeks 1-2)
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithOptionalMode(true),
//	    signature.WithAllowedIdentities("*@example.com"),
//	)
//
// Phase 2: Required mode (weeks 3-4)
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	)
//
// Phase 3: Enforce mode (week 5+)
//
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithEnforceMode(true),
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true),
//	)
//
// For complete documentation and examples, see the package README.md
// and the main OCI package documentation.
package signature
