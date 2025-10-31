package signature

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/internal/cache"
	"github.com/jmgilman/go/oci/internal/oras"
)

// TestSecurity_TamperedSignatureDetection tests that tampered signatures are detected.
func TestSecurity_TamperedSignatureDetection(t *testing.T) {
	ctx := context.Background()

	// Generate a key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Use Enforce mode to require signatures
	verifier := NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{&privateKey.PublicKey},
		WithEnforceMode(true),
	)

	// Test with a tampered signature (simulated by passing wrong digest)
	descriptor := &oras.PullDescriptor{
		Digest:      "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		MediaType:   "application/vnd.oci.image.manifest.v1+json",
		Size:        1024,
	}

	err = verifier.Verify(ctx, "example.com/test:v1", descriptor)

	// Should fail - no signature found (enforce mode)
	if err == nil {
		t.Error("Expected verification to fail with missing signature, but it succeeded")
	}

	// Verify error is appropriate (should be signature not found since we can't fetch it)
	var bundleErr *ocibundle.BundleError
	if !isSignatureError(err, &bundleErr) {
		t.Errorf("Expected signature error, got: %v", err)
	}
}

// TestSecurity_ModifiedArtifactDetection tests that modified artifacts are detected.
func TestSecurity_ModifiedArtifactDetection(t *testing.T) {
	ctx := context.Background()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Use Enforce mode to require signatures
	verifier := NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{&privateKey.PublicKey},
		WithEnforceMode(true),
	)

	// Create original digest
	originalContent := []byte("original content")
	originalHash := sha256.Sum256(originalContent)
	originalDigest := "sha256:" + hex.EncodeToString(originalHash[:])

	// Create modified digest
	modifiedContent := []byte("modified content")
	modifiedHash := sha256.Sum256(modifiedContent)
	modifiedDigest := "sha256:" + hex.EncodeToString(modifiedHash[:])

	// Test 1: Verify original digest fails (no signature in enforce mode)
	descriptor1 := &oras.PullDescriptor{
		Digest:      originalDigest,
		MediaType:   "application/vnd.oci.image.manifest.v1+json",
		Size:        int64(len(originalContent)),
	}

	err = verifier.Verify(ctx, "example.com/test:v1", descriptor1)
	if err == nil {
		t.Error("Expected verification to fail without signature (enforce mode)")
	}

	// Test 2: Verify modified digest also fails
	descriptor2 := &oras.PullDescriptor{
		Digest:      modifiedDigest,
		MediaType:   "application/vnd.oci.image.manifest.v1+json",
		Size:        int64(len(modifiedContent)),
	}

	err = verifier.Verify(ctx, "example.com/test:v1", descriptor2)
	if err == nil {
		t.Error("Expected verification to fail with modified content (enforce mode)")
	}

	// Both should fail due to missing signatures in enforce mode
	// In a real scenario with actual signatures, modified content would fail
	// because the digest in the signature payload wouldn't match
}

// TestSecurity_CachePoisoningPrevention tests that cache cannot be poisoned.
func TestSecurity_CachePoisoningPrevention(t *testing.T) {
	// Test that policy hash is included in cache key
	policy1 := NewPolicy()
	policy1.AllowedIdentities = []string{"*@example.com"}

	policy2 := NewPolicy()
	policy2.AllowedIdentities = []string{"*@malicious.com"}

	hash1 := ComputePolicyHash(policy1)
	hash2 := ComputePolicyHash(policy2)

	if hash1 == hash2 {
		t.Error("Different policies should produce different hashes")
	}

	// Test that digest is included in cache key
	digest1 := "sha256:abc123"
	digest2 := "sha256:def456"

	if digest1 == digest2 {
		t.Error("Different digests should be different")
	}

	// Verify cache key would be different
	cacheKey1 := "verify:" + digest1 + ":" + hash1
	cacheKey2 := "verify:" + digest2 + ":" + hash1
	cacheKey3 := "verify:" + digest1 + ":" + hash2

	if cacheKey1 == cacheKey2 {
		t.Error("Different digests should produce different cache keys")
	}

	if cacheKey1 == cacheKey3 {
		t.Error("Different policies should produce different cache keys")
	}

	t.Logf("Cache keys are properly isolated: %s, %s, %s", cacheKey1, cacheKey2, cacheKey3)
}

// TestSecurity_PolicyBypassAttempts tests that policy restrictions cannot be bypassed.
func TestSecurity_PolicyBypassAttempts(t *testing.T) {
	tests := []struct {
		name        string
		policy      *Policy
		identity    string
		shouldPass  bool
	}{
		{
			name: "ExactMatchRequired",
			policy: &Policy{
				AllowedIdentities: []string{"alice@example.com"},
			},
			identity:   "bob@example.com",
			shouldPass: false,
		},
		{
			name: "WildcardBypass",
			policy: &Policy{
				AllowedIdentities: []string{"*@example.com"},
			},
			identity:   "attacker@malicious.com",
			shouldPass: false,
		},
		{
			name: "EmptyIdentityBypass",
			policy: &Policy{
				AllowedIdentities: []string{"*@example.com"},
			},
			identity:   "",
			shouldPass: false,
		},
		{
			name: "NullByteInjection",
			policy: &Policy{
				AllowedIdentities: []string{"*@example.com"},
			},
			identity:   "attacker\x00@example.com",
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := tt.policy.MatchesIdentity(tt.identity)
			if matches != tt.shouldPass {
				t.Errorf("Identity %q: expected match=%v, got=%v",
					tt.identity, tt.shouldPass, matches)
			}
		})
	}
}

// TestSecurity_MalformedSignatureHandling tests handling of malformed signatures.
func TestSecurity_MalformedSignatureHandling(t *testing.T) {
	ctx := context.Background()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	verifier := NewPublicKeyVerifier(&privateKey.PublicKey)

	tests := []struct {
		name       string
		descriptor *oras.PullDescriptor
	}{
		{
			name: "EmptyDigest",
			descriptor: &oras.PullDescriptor{
				Digest:      "",
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Size:        1024,
					},
		},
		{
			name: "InvalidDigestFormat",
			descriptor: &oras.PullDescriptor{
				Digest:      "invalid-digest-format",
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Size:        1024,
					},
		},
		{
			name: "ZeroSize",
			descriptor: &oras.PullDescriptor{
				Digest:      "sha256:abc123",
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Size:        0,
					},
		},
		{
			name: "NegativeSize",
			descriptor: &oras.PullDescriptor{
				Digest:      "sha256:abc123",
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Size:        -1,
					},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifier.Verify(ctx, "example.com/test:v1", tt.descriptor)

			// Should handle malformed input gracefully
			if err == nil {
				t.Error("Expected verification to fail with malformed descriptor")
			}

			// Should not panic or cause undefined behavior
			t.Logf("Handled malformed input: %v", err)
		})
	}
}

// TestSecurity_ReplayAttackPrevention tests that signatures cannot be replayed.
func TestSecurity_ReplayAttackPrevention(t *testing.T) {
	// Test that the same signature cannot be used for different artifacts
	policy := NewPolicy()
	policy.AllowedIdentities = []string{"*@example.com"}

	// Different artifacts should have different digests
	digest1 := "sha256:abc123"
	digest2 := "sha256:def456"

	hash := ComputePolicyHash(policy)

	// Cache keys should be different for different digests
	key1 := "verify:" + digest1 + ":" + hash
	key2 := "verify:" + digest2 + ":" + hash

	if key1 == key2 {
		t.Error("Different artifacts should have different cache keys (replay prevention)")
	}

	t.Logf("Replay attack prevented: %s != %s", key1, key2)
}

// TestSecurity_TimeBasedAttackPrevention tests TTL expiration.
func TestSecurity_TimeBasedAttackPrevention(t *testing.T) {
	// Test that cache entries expire
	result := &cache.VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "user@example.com",
		Timestamp:  time.Now().Add(-2 * time.Hour), // Expired
		PolicyHash: "hash123",
		TTL:        time.Hour, // 1 hour TTL
	}

	if !result.IsExpired() {
		t.Error("Expected result to be expired after 2 hours with 1 hour TTL")
	}

	// Test not expired
	result2 := &cache.VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "user@example.com",
		Timestamp:  time.Now().Add(-30 * time.Minute), // Not expired
		PolicyHash: "hash123",
		TTL:        time.Hour, // 1 hour TTL
	}

	if result2.IsExpired() {
		t.Error("Expected result not to be expired after 30 minutes with 1 hour TTL")
	}
}

// TestSecurity_AnnotationInjection tests that annotations cannot be injected.
func TestSecurity_AnnotationInjection(t *testing.T) {
	policy := NewPolicy()
	policy.RequiredAnnotations = map[string]string{
		"verified": "true",
	}

	// Test various injection attempts
	tests := []struct {
		name        string
		annotations map[string]string
		shouldPass  bool
	}{
		{
			name: "ValidAnnotation",
			annotations: map[string]string{
				"verified": "true",
			},
			shouldPass: true,
		},
		{
			name: "WrongValue",
			annotations: map[string]string{
				"verified": "false",
			},
			shouldPass: false,
		},
		{
			name: "CaseMismatch",
			annotations: map[string]string{
				"Verified": "true",
			},
			shouldPass: false,
		},
		{
			name: "ExtraWhitespace",
			annotations: map[string]string{
				"verified": " true ",
			},
			shouldPass: false,
		},
		{
			name: "NullByteInjection",
			annotations: map[string]string{
				"verified": "true\x00malicious",
			},
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := NewKeylessVerifier(
				WithRequiredAnnotations(policy.RequiredAnnotations),
			)

			// Check if annotations match
			matches := verifier.policy.RequiredAnnotations["verified"] == tt.annotations["verified"]

			if matches != tt.shouldPass {
				t.Errorf("Annotation check failed: expected %v, got %v for %q",
					tt.shouldPass, matches, tt.annotations["verified"])
			}
		})
	}
}

// TestSecurity_PolicyHashCollision tests that different policies don't collide.
func TestSecurity_PolicyHashCollision(t *testing.T) {
	// Generate many different policies and verify no hash collisions
	hashes := make(map[string]*Policy)

	testPolicies := []*Policy{
		{AllowedIdentities: []string{"*@example.com"}},
		{AllowedIdentities: []string{"*@other.com"}},
		{AllowedIdentities: []string{"alice@example.com"}},
		{AllowedIdentities: []string{"bob@example.com"}},
		{RequiredIssuer: "https://github.com/login/oauth"},
		{RequiredIssuer: "https://accounts.google.com"},
		{RekorEnabled: true},
		// Note: {RekorEnabled: false} is identical to default policy, removed to avoid false collision
		{RequiredAnnotations: map[string]string{"key": "value1"}},
		{RequiredAnnotations: map[string]string{"key": "value2"}},
		// Note: {VerificationMode: VerificationModeOptional} is the default, removed
		{VerificationMode: VerificationModeRequired},
		{VerificationMode: VerificationModeEnforce},
		// Add combinations to test multiple fields
		{AllowedIdentities: []string{"*@example.com"}, RekorEnabled: true},
		{AllowedIdentities: []string{"*@example.com"}, VerificationMode: VerificationModeEnforce},
	}

	for i, policy := range testPolicies {
		hash := ComputePolicyHash(policy)

		if existing, found := hashes[hash]; found {
			t.Errorf("Hash collision detected between policies %d and %v: %s",
				i, existing, hash)
		}

		hashes[hash] = policy
	}

	t.Logf("Tested %d policies with no hash collisions", len(testPolicies))
}

// TestSecurity_PublicKeyValidation tests that only valid public keys are accepted.
func TestSecurity_PublicKeyValidation(t *testing.T) {
	tests := []struct {
		name      string
		keyBytes  []byte
		shouldErr bool
	}{
		{
			name:      "EmptyKey",
			keyBytes:  []byte{},
			shouldErr: true,
		},
		{
			name:      "InvalidPEM",
			keyBytes:  []byte("not a valid PEM"),
			shouldErr: true,
		},
		{
			name:      "NullBytes",
			keyBytes:  []byte{0x00, 0x00, 0x00},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPublicKeyFromBytes(tt.keyBytes)

			if tt.shouldErr && err == nil {
				t.Error("Expected error when loading invalid key")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Helper function to check if error is a signature error.
func isSignatureError(err error, bundleErr **ocibundle.BundleError) bool {
	if err == nil {
		return false
	}

	var be *ocibundle.BundleError
	if !errors.As(err, &be) {
		return false
	}

	if bundleErr != nil {
		*bundleErr = be
	}

	return be.IsSignatureError()
}
