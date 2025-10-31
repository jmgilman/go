// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ComputePolicyHash generates a deterministic SHA256 hash of the policy configuration.
// This hash is used as part of the cache key to ensure cache invalidation when
// the policy changes.
//
// The hash includes all policy fields that affect verification behavior:
//   - VerificationMode (optional, required, enforce)
//   - MultiSignatureMode (any, all, minimum)
//   - MinimumSignatures (for minimum mode)
//   - PublicKeys (fingerprints of public keys)
//   - AllowedIdentities (sorted list of identity patterns)
//   - RequiredIssuer (OIDC issuer URL)
//   - RequiredAnnotations (sorted key-value pairs)
//   - RekorEnabled (whether Rekor verification is required)
//   - RekorURL (URL of Rekor server)
//
// The hash is computed by concatenating all relevant fields in a deterministic
// order and computing SHA256. This ensures that:
//   - Identical policies produce identical hashes
//   - Different policies produce different hashes
//   - Cache is invalidated when policy changes
//
// Returns a 64-character hex string representing the SHA256 hash.
func ComputePolicyHash(policy *Policy) string {
	if policy == nil {
		// Nil policy gets a fixed hash
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}

	h := sha256.New()

	// Add verification mode
	fmt.Fprintf(h, "verification_mode:%s\n", policy.VerificationMode.String())

	// Add multi-signature mode
	fmt.Fprintf(h, "multi_signature_mode:%s\n", policy.MultiSignatureMode.String())
	fmt.Fprintf(h, "minimum_signatures:%d\n", policy.MinimumSignatures)

	// Add public keys (if present)
	if len(policy.PublicKeys) > 0 {
		// For public keys, we compute a cryptographically secure fingerprint
		// using DER encoding and SHA256 hashing
		keyFingerprints := make([]string, len(policy.PublicKeys))
		for i, key := range policy.PublicKeys {
			keyFingerprints[i] = computeKeyFingerprint(key)
		}
		// Sort for determinism
		sort.Strings(keyFingerprints)
		for _, fp := range keyFingerprints {
			fmt.Fprintf(h, "public_key:%s\n", fp)
		}
	}

	// Add allowed identities (sorted for determinism)
	if len(policy.AllowedIdentities) > 0 {
		identities := make([]string, len(policy.AllowedIdentities))
		copy(identities, policy.AllowedIdentities)
		sort.Strings(identities)
		for _, identity := range identities {
			fmt.Fprintf(h, "allowed_identity:%s\n", identity)
		}
	}

	// Add required issuer
	if policy.RequiredIssuer != "" {
		fmt.Fprintf(h, "required_issuer:%s\n", policy.RequiredIssuer)
	}

	// Add required annotations (sorted by key for determinism)
	if len(policy.RequiredAnnotations) > 0 {
		// Extract keys and sort them
		keys := make([]string, 0, len(policy.RequiredAnnotations))
		for k := range policy.RequiredAnnotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Add annotations in sorted key order
		for _, k := range keys {
			fmt.Fprintf(h, "annotation:%s=%s\n", k, policy.RequiredAnnotations[k])
		}
	}

	// Add Rekor settings
	fmt.Fprintf(h, "rekor_enabled:%t\n", policy.RekorEnabled)
	if policy.RekorURL != "" {
		fmt.Fprintf(h, "rekor_url:%s\n", policy.RekorURL)
	}

	// Compute final hash
	hashBytes := h.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

// computeKeyFingerprint generates a cryptographically secure fingerprint for a public key.
// The fingerprint is computed by:
//   1. Serializing the key to DER format using PKIX encoding (standard for public keys)
//   2. Computing the SHA256 hash of the DER bytes
//   3. Encoding the full hash as a 64-character hex string
//
// This ensures:
//   - Identical keys produce identical fingerprints
//   - Different keys produce different fingerprints (collision resistance)
//   - The fingerprint is cryptographically secure and suitable for cache invalidation
//
// The fingerprint is used to detect when public keys change, which should
// invalidate cached verification results.
func computeKeyFingerprint(key crypto.PublicKey) string {
	// Serialize key to DER format (PKIX format for public keys)
	// This is the standard encoding used by x509 certificates
	derBytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		// Fallback for unsupported key types
		// This should rarely happen with standard key types (RSA, ECDSA, Ed25519)
		derBytes = []byte(fmt.Sprintf("%T", key))
	}

	// Compute SHA256 hash of the serialized key
	hashBytes := sha256.Sum256(derBytes)

	// Return full 64-character hex hash for maximum collision resistance
	// Unlike the previous implementation, we use the full hash (not truncated)
	return hex.EncodeToString(hashBytes[:])
}

// PolicyChanged checks if two policies would produce different cache keys.
// This is a convenience function to determine if cached results should be
// invalidated without computing full hashes.
//
// Returns true if the policies are different in ways that affect verification.
func PolicyChanged(old *Policy, new *Policy) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil || new == nil {
		return true
	}

	// Compare hashes - if they differ, policy changed
	oldHash := ComputePolicyHash(old)
	newHash := ComputePolicyHash(new)
	return oldHash != newHash
}

// PolicyHashShort returns a shortened version of the policy hash (first 16 characters).
// This is useful for logging and debugging where the full 64-character hash
// is unnecessarily long.
func PolicyHashShort(policy *Policy) string {
	fullHash := ComputePolicyHash(policy)
	if len(fullHash) >= 16 {
		return fullHash[:16]
	}
	return fullHash
}

// ValidatePolicyForCaching checks if a policy is suitable for caching.
// Some policy configurations may not be cacheable or may require special
// handling. This function validates that the policy can be safely cached.
//
// Returns an error if the policy is not cacheable, nil otherwise.
func ValidatePolicyForCaching(policy *Policy) error {
	if policy == nil {
		return fmt.Errorf("cannot cache with nil policy")
	}

	// Optional mode doesn't need caching since it never fails
	// But we allow it for consistency
	if policy.VerificationMode == VerificationModeOptional {
		// This is fine - we'll cache the result but it's mostly informational
	}

	// Zero TTL would cause immediate expiration
	if policy.CacheTTL <= 0 {
		return fmt.Errorf("policy has zero or negative TTL, caching would be ineffective")
	}

	// Very short TTLs (< 1 second) may cause excessive cache churn
	// Warn but don't fail
	// (In production, you might want to log a warning here)

	return nil
}

// MergePolicyHashes combines multiple policy hashes into a single composite hash.
// This is useful when caching results that depend on multiple policies
// (though this is not common in the current design).
//
// Returns a SHA256 hash of all input hashes concatenated in sorted order.
func MergePolicyHashes(hashes ...string) string {
	if len(hashes) == 0 {
		return "0000000000000000000000000000000000000000000000000000000000000000"
	}

	if len(hashes) == 1 {
		return hashes[0]
	}

	// Sort for determinism
	sorted := make([]string, len(hashes))
	copy(sorted, hashes)
	sort.Strings(sorted)

	// Concatenate and hash
	h := sha256.New()
	h.Write([]byte(strings.Join(sorted, ":")))
	hashBytes := h.Sum(nil)
	return hex.EncodeToString(hashBytes)
}
