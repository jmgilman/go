# OCI Signature Verification

Cryptographic signature verification for OCI artifacts using Sigstore/Cosign. This submodule provides supply chain security by ensuring artifacts are verified before extraction.

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Verification Modes](#verification-modes)
- [Policy Configuration](#policy-configuration)
- [Caching](#caching)
- [Error Handling](#error-handling)
- [Security Best Practices](#security-best-practices)
- [Advanced Usage](#advanced-usage)
- [FAQ](#faq)
- [API Reference](#api-reference)

## Overview

The signature verification module enables validation of OCI artifacts using Sigstore/Cosign signatures. It supports both traditional public key cryptography and modern keyless signing with OIDC identities.

**Key Features:**
- **Zero Dependencies on Main Package**: Implemented as a separate submodule
- **Multiple Verification Modes**: Public key and keyless (OIDC)
- **Policy-Based Control**: Flexible identity, annotation, and issuer policies
- **Performance Caching**: Cache verification results to avoid redundant operations
- **Comprehensive Error Context**: Detailed failure information for debugging
- **Standards Compliance**: Uses Sigstore/Cosign format (industry standard)

## Installation

```bash
# Install main package
go get github.com/jmgilman/go/oci

# Install signature submodule
go get github.com/jmgilman/go/oci/signature
```

The signature submodule is separate to keep Cosign dependencies (23MB+) optional.

## Quick Start

### Public Key Verification

Traditional cryptographic signing with a public/private key pair:

```go
package main

import (
    "context"
    "log"

    "github.com/jmgilman/go/oci"
    "github.com/jmgilman/go/oci/signature"
)

func main() {
    ctx := context.Background()

    // Load public key
    pubKey, err := signature.LoadPublicKey("cosign.pub")
    if err != nil {
        log.Fatal(err)
    }

    // Create verifier
    verifier := signature.NewPublicKeyVerifier(pubKey)

    // Create client with verification
    client, err := ocibundle.NewWithOptions(
        ocibundle.WithSignatureVerifier(verifier),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Pull will verify signature before extraction
    err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Artifact verified and extracted successfully")
}
```

### Keyless Verification

Modern signing using OIDC identities and short-lived certificates:

```go
package main

import (
    "context"
    "log"

    "github.com/jmgilman/go/oci"
    "github.com/jmgilman/go/oci/signature"
)

func main() {
    ctx := context.Background()

    // Create keyless verifier with identity restrictions
    verifier := signature.NewKeylessVerifier(
        signature.WithAllowedIdentities("*@example.com"),
        signature.WithRekor(true), // Require transparency log
    )

    // Create client with verification
    client, err := ocibundle.NewWithOptions(
        ocibundle.WithSignatureVerifier(verifier),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Pull will verify signature using Sigstore keyless signing
    err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Artifact verified and extracted successfully")
}
```

## Verification Modes

### Public Key Verification

Uses traditional public/private key pairs for signing and verification.

**How it works:**
1. Artifact is signed with a private key (using Cosign CLI)
2. Signature is stored as a separate OCI artifact
3. Verifier checks signature using the public key
4. Artifact is only extracted if signature is valid

**Pros:**
- ✅ Works offline (no internet required)
- ✅ Simple and fast
- ✅ No external dependencies

**Cons:**
- ❌ Key management burden
- ❌ Key rotation complexity
- ❌ No built-in transparency

**Example:**

```go
// Load single public key
pubKey, err := signature.LoadPublicKey("cosign.pub")
verifier := signature.NewPublicKeyVerifier(pubKey)

// Load multiple public keys (any valid signature passes)
key1, _ := signature.LoadPublicKey("key1.pub")
key2, _ := signature.LoadPublicKey("key2.pub")
verifier := signature.NewPublicKeyVerifier(key1, key2)

// Require all keys to have valid signatures
verifier := signature.NewPublicKeyVerifierWithOptions(
    []crypto.PublicKey{key1, key2},
    signature.WithRequireAll(true),
)
```

### Keyless Verification

Uses Sigstore's keyless signing infrastructure with OIDC identities.

**How it works:**
1. Signer authenticates via OIDC (GitHub, Google, etc.)
2. Fulcio issues short-lived certificate (~15 minutes)
3. Artifact is signed with ephemeral key
4. Signature + certificate uploaded to Rekor (transparency log)
5. Verifier checks certificate against Fulcio root + Rekor entry

**Pros:**
- ✅ No key management
- ✅ Built-in transparency via Rekor
- ✅ Identity-based policies
- ✅ Automatic certificate expiration

**Cons:**
- ❌ Requires internet access
- ❌ Dependency on Sigstore infrastructure
- ❌ Slightly slower (network requests)

**Example:**

```go
// Basic keyless verification
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true),
)

// Restrict to specific issuer
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRequiredIssuer("https://github.com/login/oauth"),
    signature.WithRekor(true),
)

// Multiple identity patterns
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities(
        "alice@example.com",
        "bob@example.com",
        "*@trusted-org.com",
    ),
    signature.WithRekor(true),
)
```

## Policy Configuration

Control signature verification behavior with flexible policies.

### Verification Modes

```go
// Optional: Log failures but don't block (audit mode)
verifier := signature.NewKeylessVerifier(
    signature.WithOptionalMode(true),
    signature.WithAllowedIdentities("*@example.com"),
)

// Required: Fail on invalid signatures but allow unsigned artifacts
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
)

// Enforce: Require all artifacts to be signed (production mode)
verifier := signature.NewKeylessVerifier(
    signature.WithEnforceMode(true),
    signature.WithAllowedIdentities("*@example.com"),
)
```

### Identity Policies

Restrict which identities can sign artifacts:

```go
verifier := signature.NewKeylessVerifier(
    // Exact email
    signature.WithAllowedIdentities("alice@example.com"),
)

verifier := signature.NewKeylessVerifier(
    // Domain wildcard
    signature.WithAllowedIdentities("*@example.com"),
)

verifier := signature.NewKeylessVerifier(
    // Multiple patterns (OR logic)
    signature.WithAllowedIdentities(
        "*@example.com",
        "*@trusted-org.com",
        "service-account@project.iam.gserviceaccount.com",
    ),
)

// Require specific OIDC issuer
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRequiredIssuer("https://github.com/login/oauth"),
)
```

### Annotation Policies

Enforce required metadata in signatures:

```go
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRequiredAnnotations(map[string]string{
        "build-system":     "github-actions",
        "verified":         "true",
        "security-scan":    "passed",
        "build-id":         "12345",
    }),
)
```

All specified annotations must match exactly for verification to succeed.

### Multiple Signatures

Handle artifacts with multiple signatures:

```go
// ANY signature valid (default, OR logic)
verifier := signature.NewPublicKeyVerifier(key1, key2, key3)

// ALL signatures must be valid (AND logic)
verifier := signature.NewPublicKeyVerifierWithOptions(
    []crypto.PublicKey{key1, key2, key3},
    signature.WithRequireAll(true),
)

// Minimum N signatures required (threshold)
verifier := signature.NewPublicKeyVerifierWithOptions(
    []crypto.PublicKey{key1, key2, key3},
    signature.WithMinimumSignatures(2), // At least 2 of 3
)
```

### Rekor Transparency Log

Enable transparency and non-repudiation:

```go
// Use public Rekor instance
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true),
)

// Use custom Rekor instance
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekorURL("https://rekor.private.example.com"),
)
```

## Caching

Cache verification results to improve performance:

```go
import (
    "github.com/jmgilman/go/oci"
    "github.com/jmgilman/go/oci/signature"
    "github.com/jmgilman/go/oci/internal/cache"
    "github.com/jmgilman/go/fs/core"
)

func main() {
    ctx := context.Background()
    fs := core.NewOSFS()

    // Create cache coordinator
    cacheConfig := cache.Config{
        MaxSizeBytes: 100 * 1024 * 1024, // 100MB
        DefaultTTL:   time.Hour,
    }
    coordinator, err := cache.NewCoordinator(ctx, cacheConfig, fs, "/var/cache/oci", nil)
    if err != nil {
        log.Fatal(err)
    }
    defer coordinator.Close()

    // Create verifier with caching
    verifier := signature.NewKeylessVerifier(
        signature.WithAllowedIdentities("*@example.com"),
        signature.WithCacheTTL(time.Hour), // Cache for 1 hour
    ).WithCacheForVerifier(coordinator)

    // Create client
    client, err := ocibundle.NewWithOptions(
        ocibundle.WithSignatureVerifier(verifier),
    )

    // Subsequent pulls of the same artifact will use cached verification
    err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
}
```

### Cache Performance

**Without caching:**
- Public key verification: 55-215ms per pull
- Keyless verification: 160-730ms per pull

**With caching (cache hit):**
- Verification: <1ms per pull
- **Performance improvement: 99.8% reduction**

### Cache Invalidation

Cache is automatically invalidated when:
- The artifact digest changes (different content)
- The verification policy changes (different policy hash)
- The TTL expires

### TTL Recommendations

```go
// Public key mode: 24 hours (keys don't expire)
verifier := signature.NewPublicKeyVerifier(pubKey,
    signature.WithCacheTTL(24 * time.Hour),
)

// Keyless mode: 1 hour (certificates expire, Rekor may change)
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithCacheTTL(time.Hour),
)

// Production: Balance between performance and freshness
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithCacheTTL(30 * time.Minute),
)
```

## Error Handling

Signature verification errors provide detailed context for debugging:

```go
err := client.Pull(ctx, reference, targetDir)
if err != nil {
    var bundleErr *ocibundle.BundleError
    if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
        // Signature verification failed
        fmt.Printf("Operation: %s\n", bundleErr.Op)
        fmt.Printf("Reference: %s\n", bundleErr.Reference)
        fmt.Printf("Digest: %s\n", bundleErr.SignatureInfo.Digest)
        fmt.Printf("Reason: %s\n", bundleErr.SignatureInfo.Reason)
        fmt.Printf("Failure Stage: %s\n", bundleErr.SignatureInfo.FailureStage)

        // Check specific error type
        switch {
        case errors.Is(bundleErr.Err, ocibundle.ErrSignatureNotFound):
            fmt.Println("No signature found for artifact")
            fmt.Println("Ensure artifact was signed with: cosign sign <image>")

        case errors.Is(bundleErr.Err, ocibundle.ErrSignatureInvalid):
            fmt.Println("Signature verification failed")
            fmt.Println("Artifact may have been tampered with")

        case errors.Is(bundleErr.Err, ocibundle.ErrUntrustedSigner):
            fmt.Println("Signature valid but signer not trusted")
            fmt.Printf("Signer: %s\n", bundleErr.SignatureInfo.Signer)
            fmt.Println("Update allowed identities to trust this signer")

        case errors.Is(bundleErr.Err, ocibundle.ErrRekorVerificationFailed):
            fmt.Println("Transparency log verification failed")
            fmt.Println("Signature may not be logged in Rekor")

        case errors.Is(bundleErr.Err, ocibundle.ErrCertificateExpired):
            fmt.Println("Certificate expired")
            fmt.Println("For keyless signing, this may indicate an old signature")

        case errors.Is(bundleErr.Err, ocibundle.ErrInvalidAnnotations):
            fmt.Println("Required annotations not found")
            fmt.Println("Check signature annotations with: cosign verify <image>")
        }

        return
    }

    // Other error types
    log.Fatal(err)
}
```

### Failure Stages

The `FailureStage` field indicates where verification failed:

- `"policy"`: Policy configuration invalid
- `"fetch"`: Failed to fetch signature artifact from registry
- `"crypto"`: Cryptographic signature verification failed
- `"policy"`: Signature valid but policy check failed (identity, annotations)
- `"rekor"`: Transparency log verification failed

## Security Best Practices

### 1. Enforce Mode in Production

```go
// Development/Staging: Audit mode
verifier := signature.NewKeylessVerifier(
    signature.WithOptionalMode(true),
    signature.WithAllowedIdentities("*@example.com"),
)

// Production: Enforce mode
verifier := signature.NewKeylessVerifier(
    signature.WithEnforceMode(true),
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true),
)
```

### 2. Restrict Identity Patterns

```go
// BAD: Too permissive
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*"),
)

// GOOD: Specific domains
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities(
        "*@example.com",
        "*@trusted-partner.com",
    ),
)

// BETTER: Specific accounts
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities(
        "ci-bot@example.com",
        "release-manager@example.com",
    ),
)
```

### 3. Enable Rekor for Keyless

```go
// Always enable Rekor for keyless signing
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true), // Provides transparency and audit trail
)
```

### 4. Monitor Verification Failures

```go
// Log all verification failures for security monitoring
err := client.Pull(ctx, reference, targetDir)
if err != nil {
    var bundleErr *ocibundle.BundleError
    if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
        // Log to security monitoring system
        securityLog.Error("signature_verification_failed",
            "reference", bundleErr.Reference,
            "digest", bundleErr.SignatureInfo.Digest,
            "reason", bundleErr.SignatureInfo.Reason,
            "stage", bundleErr.SignatureInfo.FailureStage,
        )
    }
}
```

### 5. Regular Key Rotation

For public key mode:
- Rotate keys every 90 days
- Maintain key lifecycle documentation
- Test key rotation procedures regularly

### 6. Gradual Rollout

```go
// Phase 1: Audit mode (weeks 1-2)
verifier := signature.NewKeylessVerifier(
    signature.WithOptionalMode(true),
    signature.WithAllowedIdentities("*@example.com"),
)

// Phase 2: Required mode (weeks 3-4)
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
)

// Phase 3: Enforce mode (week 5+)
verifier := signature.NewKeylessVerifier(
    signature.WithEnforceMode(true),
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true),
)
```

## Advanced Usage

### Custom Cache Implementation

Implement the `VerificationCache` interface for custom caching:

```go
type CustomCache struct {
    // Your custom cache implementation
}

func (c *CustomCache) GetCachedVerification(
    ctx context.Context,
    digest, policyHash string,
) (verified bool, signer string, err error) {
    // Implement cache lookup
    return false, "", errors.New("not found")
}

func (c *CustomCache) PutCachedVerification(
    ctx context.Context,
    digest, policyHash string,
    verified bool,
    signer string,
    ttl time.Duration,
) error {
    // Implement cache storage
    return nil
}

// Use custom cache
customCache := &CustomCache{}
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
).WithCacheForVerifier(customCache)
```

### Policy Inspection

```go
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
    signature.WithRekor(true),
)

// Get policy for inspection
policy := verifier.Policy()

fmt.Printf("Verification Mode: %s\n", policy.VerificationMode)
fmt.Printf("Allowed Identities: %v\n", policy.AllowedIdentities)
fmt.Printf("Rekor Enabled: %t\n", policy.RekorEnabled)
fmt.Printf("Cache TTL: %s\n", policy.CacheTTL)
```

### Loading Keys from Multiple Sources

```go
// From file
pubKey1, _ := signature.LoadPublicKey("/path/to/key1.pub")

// From bytes
keyBytes := []byte("-----BEGIN PUBLIC KEY-----\n...")
pubKey2, _ := signature.LoadPublicKeyFromBytes(keyBytes)

// From environment variable
keyData := os.Getenv("COSIGN_PUBLIC_KEY")
pubKey3, _ := signature.LoadPublicKeyFromBytes([]byte(keyData))

// Create verifier with all keys
verifier := signature.NewPublicKeyVerifier(pubKey1, pubKey2, pubKey3)
```

## FAQ

### Q: Do I need to use signature verification?

A: No, signature verification is completely optional. It's implemented in a separate submodule so you only pay for it if you use it.

### Q: What's the difference between public key and keyless verification?

A: Public key uses traditional key pairs (you manage the keys), while keyless uses OIDC identities and Sigstore infrastructure (no key management needed).

### Q: Can I verify signatures created by the Cosign CLI?

A: Yes! This module is fully compatible with signatures created by `cosign sign`.

### Q: Does caching affect security?

A: No. The cache key includes the policy hash, so any policy changes invalidate the cache. Plus, signatures are verified against immutable content digests.

### Q: Can I use both public key and keyless in the same application?

A: Not simultaneously on the same verifier, but you can create different clients with different verifiers for different artifacts.

### Q: What happens if Rekor is unavailable?

A: If Rekor verification is required and Rekor is unavailable, verification will fail. For production, consider monitoring Rekor availability.

### Q: How do I sign artifacts?

A: Use the Cosign CLI:
```bash
# Public key signing
cosign sign --key cosign.key ghcr.io/org/app:v1.0

# Keyless signing
cosign sign ghcr.io/org/app:v1.0
```

### Q: Can I verify signatures for specific digests?

A: Yes, signature verification works with both tag and digest references. Digest references are recommended for immutability.

### Q: How do I troubleshoot verification failures?

A: Check the `SignatureErrorInfo` for detailed context including the failure stage and reason. Enable debug logging to see the full verification flow.

## API Reference

### Types

- [`CosignVerifier`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#CosignVerifier) - Main verifier implementation
- [`Policy`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#Policy) - Verification policy configuration
- [`VerificationMode`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#VerificationMode) - Enforcement mode enum
- [`MultiSignatureMode`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#MultiSignatureMode) - Multi-signature validation mode

### Functions

- [`NewPublicKeyVerifier`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#NewPublicKeyVerifier) - Create public key verifier
- [`NewKeylessVerifier`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#NewKeylessVerifier) - Create keyless verifier
- [`LoadPublicKey`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#LoadPublicKey) - Load public key from file
- [`LoadPublicKeyFromBytes`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#LoadPublicKeyFromBytes) - Load public key from bytes
- [`ComputePolicyHash`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#ComputePolicyHash) - Compute policy hash for caching

### Options

- [`WithAllowedIdentities`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithAllowedIdentities) - Set allowed signer identities
- [`WithRequiredIssuer`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithRequiredIssuer) - Set required OIDC issuer
- [`WithRequiredAnnotations`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithRequiredAnnotations) - Set required annotations
- [`WithRekor`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithRekor) - Enable Rekor transparency log
- [`WithRekorURL`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithRekorURL) - Set custom Rekor URL
- [`WithEnforceMode`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithEnforceMode) - Require all artifacts to be signed
- [`WithOptionalMode`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithOptionalMode) - Log failures but don't block
- [`WithRequireAll`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithRequireAll) - Require all signatures to be valid
- [`WithMinimumSignatures`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithMinimumSignatures) - Set minimum signature threshold
- [`WithCacheTTL`](https://pkg.go.dev/github.com/jmgilman/go/oci/signature#WithCacheTTL) - Set cache TTL

## Related Links

- [Main OCI Package README](../README.md)
- [Sigstore Documentation](https://docs.sigstore.dev/)
- [Cosign GitHub](https://github.com/sigstore/cosign)
- [SLSA Framework](https://slsa.dev/)
- [Rekor Transparency Log](https://docs.sigstore.dev/rekor/overview/)

## License

See [LICENSE](../../../LICENSE) for license information.
