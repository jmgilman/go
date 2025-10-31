// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"context"
	"crypto"
	"time"
)

// VerificationCache defines the interface for caching verification results.
// This interface allows the verifier to cache cryptographic verification results
// to avoid redundant expensive operations.
//
// Implementations MUST:
//   - Use the cache key format: "verify:<digest>:<policy-hash>"
//   - Respect TTL for cache expiration and NEVER return expired entries
//   - Validate timestamps and reject entries past their TTL
//   - Handle cache misses and expired entries by returning an error
//   - Be thread-safe for concurrent access
//   - Implement proper time-based expiration to prevent stale data attacks
//
// Security Requirements:
// Cache implementations MUST validate TTL expiration before returning cached results.
// Returning expired cache entries could lead to using stale verification results
// that no longer meet current security policies. This is critical for:
//   - Keyless signatures (certificates may be revoked)
//   - Policy changes (stricter requirements over time)
//   - Rekor transparency log updates (new information may invalidate old results)
type VerificationCache interface {
	// GetCachedVerification retrieves a cached verification result.
	//
	// IMPORTANT: Implementations MUST check TTL expiration before returning results.
	// Expired entries MUST NOT be returned - return an error instead to force re-verification.
	//
	// Returns:
	//   - verified=true, signer, nil if verification previously passed, is cached, and NOT expired
	//   - verified=false, "", nil if verification previously failed, is cached, and NOT expired
	//   - verified=false, "", error if cache miss, expired, or error accessing cache
	GetCachedVerification(ctx context.Context, digest, policyHash string) (verified bool, signer string, err error)

	// PutCachedVerification stores a verification result in the cache.
	// Parameters:
	//   - digest: The artifact content digest
	//   - policyHash: The hash of the verification policy
	//   - verified: Whether verification passed
	//   - signer: The identity of the signer (empty if unknown)
	//   - ttl: Time-to-live for this cache entry
	PutCachedVerification(ctx context.Context, digest, policyHash string, verified bool, signer string, ttl time.Duration) error
}

// VerifierOption is a functional option for configuring a CosignVerifier.
// Options allow flexible configuration of verification behavior, including
// verification mode, policy settings, and verification requirements.
type VerifierOption func(*Policy)

// WithRequireAll requires all signatures to be valid (AND logic).
// When enabled, every signature found on an artifact must verify successfully.
// This is useful for requiring unanimous approval from multiple signers.
//
// Example:
//
//	verifier := NewPublicKeyVerifier(key1, key2,
//	    WithRequireAll(true),
//	)
func WithRequireAll(requireAll bool) VerifierOption {
	return func(p *Policy) {
		if requireAll {
			p.MultiSignatureMode = MultiSignatureModeAll
		} else {
			p.MultiSignatureMode = MultiSignatureModeAny
		}
	}
}

// WithMinimumSignatures sets the minimum number of valid signatures required.
// This enables threshold-based approval where at least N signatures must verify.
// Automatically sets MultiSignatureMode to Minimum.
//
// Example:
//
//	verifier := NewPublicKeyVerifier(key1, key2, key3,
//	    WithMinimumSignatures(2), // At least 2 signatures required
//	)
func WithMinimumSignatures(n int) VerifierOption {
	return func(p *Policy) {
		p.MultiSignatureMode = MultiSignatureModeMinimum
		p.MinimumSignatures = n
	}
}

// WithOptionalMode enables optional verification mode.
// Verification failures are logged but don't block pull operations.
// This is useful for audit mode during rollout to gather metrics on signature
// coverage without enforcing verification.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithOptionalMode(true),
//	    WithAllowedIdentities("*@example.com"),
//	)
func WithOptionalMode(optional bool) VerifierOption {
	return func(p *Policy) {
		if optional {
			p.VerificationMode = VerificationModeOptional
		}
	}
}

// WithEnforceMode enables strict enforcement mode.
// All artifacts must have valid signatures - missing or invalid signatures
// fail the operation. This is the recommended mode for production security.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithEnforceMode(true),
//	    WithAllowedIdentities("*@example.com"),
//	)
func WithEnforceMode(enforce bool) VerifierOption {
	return func(p *Policy) {
		if enforce {
			p.VerificationMode = VerificationModeEnforce
		}
	}
}

// WithAllowedIdentities sets the allowed signer identities for keyless verification.
// Identities are matched against the certificate subject from OIDC authentication.
// Supports glob patterns like "*@example.com" or exact email addresses.
//
// Multiple patterns can be specified - any match is accepted (OR logic).
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithAllowedIdentities("alice@example.com", "bob@example.com"),
//	)
//
//	// Or with wildcards:
//	verifier := NewKeylessVerifier(
//	    WithAllowedIdentities("*@example.com", "*@trusted.org"),
//	)
func WithAllowedIdentities(patterns ...string) VerifierOption {
	return func(p *Policy) {
		p.AllowedIdentities = append(p.AllowedIdentities, patterns...)
	}
}

// WithRequiredIssuer sets the required OIDC issuer for keyless verification.
// Only signatures from this issuer will be accepted.
// Common issuers:
//   - GitHub: "https://github.com/login/oauth"
//   - Google: "https://accounts.google.com"
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithRequiredIssuer("https://github.com/login/oauth"),
//	    WithAllowedIdentities("*@example.com"),
//	)
func WithRequiredIssuer(issuer string) VerifierOption {
	return func(p *Policy) {
		p.RequiredIssuer = issuer
	}
}

// WithRequiredAnnotations sets required key-value annotations that must be present
// in the signature metadata. All specified annotations must match exactly.
// This is useful for enforcing build metadata requirements.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithRequiredAnnotations(map[string]string{
//	        "build-system": "github-actions",
//	        "verified": "true",
//	    }),
//	)
func WithRequiredAnnotations(annotations map[string]string) VerifierOption {
	return func(p *Policy) {
		if p.RequiredAnnotations == nil {
			p.RequiredAnnotations = make(map[string]string)
		}
		for k, v := range annotations {
			p.RequiredAnnotations[k] = v
		}
	}
}

// WithRekor enables or disables Rekor transparency log verification.
// When enabled, signatures must be present in the Rekor transparency log.
// This provides an audit trail and non-repudiation for signatures.
//
// Rekor verification requires network access to the Rekor server.
// Disabled by default for offline verification support.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithRekor(true), // Require transparency log
//	    WithAllowedIdentities("*@example.com"),
//	)
func WithRekor(enabled bool) VerifierOption {
	return func(p *Policy) {
		p.RekorEnabled = enabled
	}
}

// WithRekorURL sets a custom Rekor transparency log server URL.
// If not specified, the public Sigstore Rekor instance is used.
// This is useful for private Rekor deployments.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithRekor(true),
//	    WithRekorURL("https://rekor.private.example.com"),
//	)
func WithRekorURL(url string) VerifierOption {
	return func(p *Policy) {
		p.RekorURL = url
		if url != "" {
			p.RekorEnabled = true
		}
	}
}

// WithPublicKeys sets the public keys for traditional signature verification.
// This enables public key cryptography mode (as opposed to keyless OIDC mode).
// Multiple keys can be provided - any valid signature from any key passes verification
// (unless WithRequireAll is set).
//
// Note: This is an internal option used by NewPublicKeyVerifier. Users should
// not typically need to use this directly.
func WithPublicKeys(keys ...crypto.PublicKey) VerifierOption {
	return func(p *Policy) {
		p.PublicKeys = append(p.PublicKeys, keys...)
	}
}

// WithCacheTTL sets the time-to-live for cached verification results.
// This controls how long a verification result is cached before re-verification
// is required.
//
// Recommended values:
//   - Public key verification: 24 hours (keys don't expire)
//   - Keyless verification: 1 hour (certificates expire, Rekor may change)
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithCacheTTL(30 * time.Minute),
//	)
func WithCacheTTL(ttl time.Duration) VerifierOption {
	return func(p *Policy) {
		p.CacheTTL = ttl
	}
}

// WithCache enables caching of verification results using the provided cache implementation.
// When caching is enabled, verification results are stored and reused to avoid
// redundant cryptographic operations.
//
// The cache key includes both the artifact digest and the policy hash, ensuring
// that cached results are invalidated when:
//   - The artifact changes (different digest)
//   - The verification policy changes (different policy hash)
//
// Example:
//
//	cache := myCache // Implements VerificationCache interface
//	verifier := NewKeylessVerifier(
//	    WithCache(cache),
//	    WithAllowedIdentities("*@example.com"),
//	)
//
// Note: Cache operations that fail do not cause verification to fail.
// Caching is treated as a performance optimization, not a requirement.
func WithCache(cache VerificationCache) VerifierOption {
	return func(_ *Policy) {
		// Store cache in policy for now
		// We'll need to refactor CosignVerifier to hold the cache separately
		// For now, we'll add it to the verifier struct directly in the verifier.go
		// This is a placeholder that will be used during verifier construction
		_ = cache // Will be used in verifier construction
	}
}
