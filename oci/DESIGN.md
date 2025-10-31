# Signature Verification Design

## Overview

This document describes the design for adding OCI artifact signature verification to the `oci` package. The implementation uses a **separate submodule architecture** with **interface-based injection** to keep the main package dependency-free while providing opt-in signature verification capabilities.

## Goals

1. **Supply Chain Security**: Verify artifact authenticity before extraction
2. **Zero Impact**: No additional dependencies for users who don't need verification
3. **Flexibility**: Support multiple verification modes (public key, keyless/OIDC)
4. **Performance**: Cache verification results to avoid redundant checks
5. **Standards Compliance**: Use Sigstore/Cosign format (industry standard)

## Non-Goals

- Multiple signature format support in initial implementation (Notary v2 can be added later)
- Policy engines beyond basic identity/annotation checking (OPA integration is future work)
- Signature creation/signing (this is a consumer library, not a producer)

## Architecture

### Submodule Structure

The implementation uses Go modules to create complete dependency isolation:

```
oci/                          (main module: github.com/jmgilman/go/oci)
├── go.mod                    (no cosign dependency)
├── signature_interface.go    (interface definition only)
├── client.go                 (calls verifier if provided)
└── signature/                (submodule: github.com/jmgilman/go/oci/signature)
    ├── go.mod                (depends on cosign ~23MB)
    ├── verifier.go           (implements interface)
    └── ...
```

### Interface-Based Injection

The main `oci` package defines a minimal interface that the signature submodule implements:

```go
// In oci/signature_interface.go (main package)
package ocibundle

import (
    "context"
    orasint "github.com/jmgilman/go/oci/internal/oras"
)

// SignatureVerifier validates OCI artifact signatures.
// Implementations are provided by the oci/signature submodule.
type SignatureVerifier interface {
    // Verify validates the signature for the given artifact.
    // Returns an error if verification fails or signature is missing.
    Verify(ctx context.Context, reference string, descriptor *orasint.PullDescriptor) error
}
```

### Integration Flow

```
User Code
    │
    ├─ Import: github.com/jmgilman/go/oci
    └─ Import: github.com/jmgilman/go/oci/signature (optional)
    │
    ↓
Create Verifier (signature.NewPublicKeyVerifier(...))
    │
    ↓
Inject into Client (oci.WithSignatureVerifier(verifier))
    │
    ↓
Call Pull(ctx, reference, targetDir)
    │
    ↓
Client.Pull() Flow:
    1. Validate inputs
    2. Create ORAS repository
    3. Pull manifest → get descriptor {Digest, MediaType, Size, Data}
    4. ❯ IF verifier configured: verifier.Verify(ctx, ref, descriptor)
       - Fetch signature artifact from registry
       - Verify cryptographic signature
       - Check policy (identities, annotations)
       - Check transparency log (optional)
       - Cache result by digest
    5. Extract blob to targetDir (only if verification passed)
```

## Signature Verification Details

### Cosign Signature Storage

Cosign stores signatures as separate OCI artifacts with predictable references:

```
Image:     ghcr.io/org/repo:tag              → sha256:abc123...
Signature: ghcr.io/org/repo:sha256-abc123.sig
```

The signature artifact contains:
- **Media Type**: `application/vnd.dev.cosign.simplesigning.v1+json`
- **Payload**: JSON with signature and certificates
- **Annotations**: Metadata about signer, build info, etc.

### Verification Modes

#### 1. Public Key Verification

Traditional cryptographic signing with a public/private key pair.

```go
import "github.com/jmgilman/go/oci/signature"

pubKey, _ := signature.LoadPublicKey("cosign.pub")
verifier := signature.NewPublicKeyVerifier(pubKey)

client, _ := oci.NewWithOptions(
    oci.WithSignatureVerifier(verifier),
)
```

**Pros**: Offline, simple, fast
**Cons**: Key management burden, key rotation complexity

#### 2. Keyless Verification (OIDC)

Sigstore's keyless signing using OIDC identities and short-lived certificates.

```go
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("alice@example.com", "bob@example.com"),
    signature.WithRekor(true), // Require transparency log
)

client, _ := oci.NewWithOptions(
    oci.WithSignatureVerifier(verifier),
)
```

**Flow**:
1. Signer authenticates via OIDC (GitHub, Google, etc.)
2. Fulcio issues short-lived certificate (~15 min)
3. Artifact signed with ephemeral key
4. Signature + certificate uploaded to Rekor (transparency log)
5. Verifier checks certificate against Fulcio root + Rekor entry

**Pros**: No key management, transparency via Rekor, identity-based
**Cons**: Requires internet access, dependency on Sigstore infrastructure

### Policy Enforcement

The verifier supports policy-based validation:

```go
verifier := signature.NewKeylessVerifier(
    // Identity policies
    signature.WithAllowedIdentities("*@trusted-org.com"),
    signature.WithRequiredIssuer("https://github.com/login/oauth"),

    // Annotation policies
    signature.WithRequiredAnnotations(map[string]string{
        "build-system": "github-actions",
        "verified": "true",
    }),

    // Transparency requirements
    signature.WithRekor(true),
)
```

**Policy Checks**:
- ✅ Certificate identity (email, subject)
- ✅ OIDC issuer (GitHub, Google, etc.)
- ✅ Annotation key/value pairs
- ✅ Rekor transparency log inclusion
- ❌ Expiration (certificates are short-lived, verification time matters)

### Multiple Signatures

Artifacts may have multiple signatures from different signers (e.g., CI system + release manager).

**Verification Logic** (configurable):

1. **ANY signature valid** (default, OR logic)
   - Useful when multiple people can sign
   - First valid signature passes verification

2. **ALL signatures valid** (AND logic)
   - Requires unanimous approval
   - All found signatures must verify

3. **Minimum N signatures**
   - Threshold-based approval
   - At least N signatures must verify

```go
verifier := signature.NewPublicKeyVerifier(
    key1, key2, key3,
    signature.WithRequireAll(true), // All signatures must verify
)
```

## Caching Strategy

### Verification Result Caching

**Cache Key**: `verify:sha256:<digest>:<policy-hash>`

Where:
- `<digest>`: Artifact content digest (immutable)
- `<policy-hash>`: SHA256 of policy configuration (ensures cache invalidation on policy changes)

**Cache Entry**:
```go
type VerificationResult struct {
    Digest       string                 // Artifact digest
    Verified     bool                   // Verification passed
    Signer       string                 // Identity of signer
    Timestamp    time.Time              // When verified
    PolicyHash   string                 // Hash of policy used
    RekorEntry   *RekorLogEntry         // Transparency log entry (if used)
    TTL          time.Duration          // Cache lifetime
}
```

**Cache Behavior**:
- ✅ Cache HIT: Skip verification, return cached result (fast path)
- ❌ Cache MISS: Perform verification, cache result (slow path)
- 🔄 Cache EXPIRED: Re-verify if TTL exceeded
- 🚫 Policy CHANGED: Cache miss (different policy hash)

**TTL Recommendations**:
- Public key verification: 24 hours (keys don't expire)
- Keyless verification: 1 hour (certificates expire, Rekor may change)
- Production: Configurable based on risk tolerance

**Security Considerations**:
- Cache poisoning: Validate cache integrity (checksums)
- Cache invalidation: Flush on policy changes
- Trust boundary: Cache storage must be trusted

### Signature Artifact Caching

The signature artifact itself (separate from verification result) can be cached:

**Cache Key**: `signature:sha256:<digest>`

This avoids re-fetching the signature blob from the registry on repeated verifications.

## Error Handling

### Error Types

```go
// Defined in oci/errors.go
var (
    ErrSignatureNotFound          = errors.New("signature not found")
    ErrSignatureInvalid           = errors.New("signature verification failed")
    ErrUntrustedSigner            = errors.New("untrusted signer")
    ErrRekorVerificationFailed    = errors.New("rekor verification failed")
    ErrCertificateExpired         = errors.New("certificate expired")
    ErrInvalidAnnotations         = errors.New("required annotations missing")
)
```

### Error Context

Signature errors include detailed context via `BundleError`:

```go
type BundleError struct {
    Op            string              // Operation: "verify"
    Reference     string              // OCI reference
    Err           error               // Root cause
    SignatureInfo *SignatureErrorInfo // Verification details
}

type SignatureErrorInfo struct {
    Digest         string              // Artifact digest
    Signer         string              // Identity (if available)
    Reason         string              // Human-readable failure reason
    FailureStage   string              // Where it failed: "fetch", "crypto", "policy", "rekor"
    RekorEntry     *RekorLogEntry      // Transparency log entry (if checked)
    Certificate    *x509.Certificate   // Signing certificate (if available)
}
```

### Error Handling Example

```go
err := client.Pull(ctx, reference, targetDir)
if err != nil {
    var bundleErr *oci.BundleError
    if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
        // Signature verification failed
        log.Printf("Digest: %s", bundleErr.SignatureInfo.Digest)
        log.Printf("Reason: %s", bundleErr.SignatureInfo.Reason)
        log.Printf("Stage: %s", bundleErr.SignatureInfo.FailureStage)

        // Check specific error type
        switch {
        case errors.Is(bundleErr.Err, oci.ErrSignatureNotFound):
            // Handle missing signature
        case errors.Is(bundleErr.Err, oci.ErrUntrustedSigner):
            // Handle untrusted identity
        }
    }
}
```

### Retry Behavior

Signature verification errors are **NOT retryable**:
- Cryptographic verification is deterministic
- If signature is invalid once, it will always be invalid
- Network errors during signature fetch ARE retryable

```go
func isRetryableError(err error) bool {
    // Signature errors are not retryable
    if errors.Is(err, ErrSignatureNotFound) ||
       errors.Is(err, ErrSignatureInvalid) ||
       errors.Is(err, ErrUntrustedSigner) ||
       errors.Is(err, ErrRekorVerificationFailed) {
        return false
    }
    // ... existing network error retry logic ...
}
```

## Security Considerations

### Threat Model

**Threats Mitigated**:
1. ✅ **Malicious artifacts**: Unsigned or tampered artifacts rejected before extraction
2. ✅ **Supply chain attacks**: Only artifacts from trusted signers allowed
3. ✅ **Man-in-the-middle**: Signatures verified cryptographically (registry can't forge)
4. ✅ **Compromised registry**: Registry cannot serve malicious content without valid signature

**Threats NOT Mitigated**:
1. ❌ **Compromised signing keys**: If attacker steals private key, can sign malware
2. ❌ **Compromised Sigstore infrastructure**: If Fulcio/Rekor compromised, keyless signing vulnerable
3. ❌ **Social engineering**: Users can disable verification or trust wrong keys
4. ❌ **Time-of-check-to-time-of-use (TOCTOU)**: Gap between verification and extraction (mitigated by digest-based verification)

### Critical Security Requirements

1. **Verify BEFORE caching**: Never cache unverified artifacts
2. **Verify BEFORE extraction**: Reject early, don't write malicious files to disk
3. **Digest-based verification**: Use content digest, not tag (tags are mutable)
4. **Transparency logging**: Enable Rekor for audit trail in production
5. **Short-lived certificates**: Use keyless signing for automatic expiration
6. **Key rotation**: Document and support key rotation procedures

### Best Practices for Users

1. **Enforce mode in production**: Use `SignatureModeEnforce` to require signatures
2. **Use keyless when possible**: Reduces key management burden
3. **Restrict identities**: Use specific email patterns, not wildcards
4. **Enable Rekor**: Provides transparency and non-repudiation
5. **Monitor verification failures**: Log and alert on signature errors
6. **Rotate keys regularly**: For public key mode, rotate keys every 90 days

## Performance Impact

### Verification Overhead

**Public Key Verification**:
- Signature fetch: 50-200ms (network)
- Crypto verification: 5-15ms (CPU)
- **Total**: ~55-215ms per pull

**Keyless Verification**:
- Signature fetch: 50-200ms (network)
- Certificate validation: 10-30ms (CPU)
- Rekor verification: 100-500ms (network + validation)
- **Total**: ~160-730ms per pull

**With Caching** (cache hit):
- Verification lookup: <1ms
- **Total**: ~1ms per pull (99.8% reduction)

### Caching Effectiveness

Assuming:
- 1000 pulls/day
- 100 unique artifacts
- 24h cache TTL

**Without caching**: 1000 verifications/day × 200ms = 200 seconds
**With caching**: 100 verifications/day × 200ms = 20 seconds (90% reduction)

### Optimization Strategies

1. **Parallel signature fetch**: Fetch signature while streaming blob (if registry supports)
2. **Signature caching**: Cache signature artifacts separately from verification results
3. **Batch verification**: Verify multiple artifacts concurrently
4. **Lazy Rekor verification**: Check Rekor asynchronously, allow pull to continue
5. **Local Rekor cache**: Cache Rekor responses locally

## Testing Strategy

### Unit Tests

1. **Interface mocking**: Test `Client.Pull()` with mock `SignatureVerifier`
2. **Verifier logic**: Test signature verification with test keys
3. **Policy validation**: Test identity, annotation, Rekor policies
4. **Error paths**: Test all error types and error context
5. **Caching**: Test cache hits, misses, expiration, invalidation

### Integration Tests

1. **Real signature verification**: Sign test artifacts with real keys, verify
2. **Keyless flow**: Use Sigstore staging environment for keyless tests
3. **Registry compatibility**: Test against multiple registries (Docker Hub, GHCR, GCR)
4. **Multiple signatures**: Test OR/AND logic with multiple signers
5. **Cache integration**: Test end-to-end with Redis/filesystem cache

### Test Artifacts

Create test fixtures:
- Valid signed artifact (public key)
- Valid signed artifact (keyless)
- Unsigned artifact
- Tampered artifact (modified after signing)
- Expired certificate
- Wrong signer identity
- Missing annotations

### Security Tests

1. **Malicious signatures**: Attempt to bypass verification
2. **Cache poisoning**: Attempt to inject false verification results
3. **TOCTOU attacks**: Verify race conditions handled
4. **Policy bypass**: Attempt to circumvent policy checks

## Deployment Considerations

### Gradual Rollout

**Phase 1: Audit Mode** (weeks 1-2)
```go
// Log verification failures but continue
verifier := signature.NewKeylessVerifier(
    signature.WithOptionalMode(true), // Don't fail on errors
    signature.WithAllowedIdentities("*@example.com"),
)
```
- Collect metrics on signature coverage
- Identify unsigned artifacts
- Fix signing gaps

**Phase 2: Soft Enforcement** (weeks 3-4)
```go
// Fail on invalid signatures but allow missing
verifier := signature.NewKeylessVerifier(
    signature.WithAllowedIdentities("*@example.com"),
)
```
- Verify all signed artifacts
- Continue allowing unsigned (for now)

**Phase 3: Full Enforcement** (week 5+)
```go
// Require signatures on all artifacts
verifier := signature.NewKeylessVerifier(
    signature.WithEnforceMode(true), // Fail if signature missing
    signature.WithAllowedIdentities("*@example.com"),
)
```
- All artifacts must be signed
- Production security posture achieved

### Monitoring & Observability

**Metrics to Track**:
- Verification success rate
- Verification latency (p50, p95, p99)
- Cache hit rate
- Signature fetch failures
- Policy violation counts
- Signer identity distribution

**Logging**:
```go
// Successful verification
log.Info("signature verified",
    "digest", descriptor.Digest,
    "signer", verificationResult.Signer,
    "rekor_index", verificationResult.RekorEntry.Index,
)

// Verification failure
log.Error("signature verification failed",
    "digest", descriptor.Digest,
    "reason", err.SignatureInfo.Reason,
    "stage", err.SignatureInfo.FailureStage,
)
```

**Alerting**:
- Alert on verification failure rate > 5%
- Alert on sudden drop in signature coverage
- Alert on unauthorized signer identities

## Future Enhancements

### Short Term (3-6 months)

1. **Notary v2 support**: Add alternative verifier implementation
2. **Policy engine**: Integrate OPA for complex policies
3. **Signature caching**: Cache signature blobs separately
4. **Batch verification**: Verify multiple artifacts in parallel

### Long Term (6-12 months)

1. **SBOM verification**: Verify software bill of materials
2. **Attestation support**: Verify SLSA provenance attestations
3. **Custom Rekor instances**: Support private transparency logs
4. **Hardware security**: Support HSM/TPM for key storage
5. **Signature creation**: Add signing capabilities (producer side)

## References

- [Sigstore Documentation](https://docs.sigstore.dev/)
- [Cosign GitHub](https://github.com/sigstore/cosign)
- [OCI Distribution Spec](https://github.com/opencontainers/distribution-spec)
- [SLSA Framework](https://slsa.dev/)
- [Rekor Transparency Log](https://docs.sigstore.dev/rekor/overview/)

## Appendix: API Reference

### Main Package (oci)

```go
// Interface for signature verification
type SignatureVerifier interface {
    Verify(ctx context.Context, reference string, descriptor *orasint.PullDescriptor) error
}

// Client option to inject verifier
func WithSignatureVerifier(verifier SignatureVerifier) ClientOption

// Error checking
func (e *BundleError) IsSignatureError() bool
```

### Signature Submodule (oci/signature)

```go
// Verifier constructors
func NewPublicKeyVerifier(keys ...*crypto.PublicKey) *CosignVerifier
func NewKeylessVerifier(opts ...VerifierOption) *CosignVerifier

// Verifier options
func WithAllowedIdentities(patterns ...string) VerifierOption
func WithRequiredIssuer(issuer string) VerifierOption
func WithRequiredAnnotations(annotations map[string]string) VerifierOption
func WithRekor(enabled bool) VerifierOption
func WithRekorURL(url string) VerifierOption
func WithRequireAll(requireAll bool) VerifierOption
func WithMinimumSignatures(n int) VerifierOption
func WithOptionalMode(optional bool) VerifierOption
func WithEnforceMode(enforce bool) VerifierOption

// Utility functions
func LoadPublicKey(path string) (*crypto.PublicKey, error)
func LoadPublicKeyFromBytes(data []byte) (*crypto.PublicKey, error)
```
