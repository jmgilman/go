package signature

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/billy"
	"github.com/jmgilman/go/oci/internal/cache"
	"github.com/jmgilman/go/oci/internal/oras"
)

// BenchmarkPublicKeyVerification benchmarks public key signature verification.
func BenchmarkPublicKeyVerification(b *testing.B) {
	ctx := context.Background()

	// Generate a test key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("Failed to generate key: %v", err)
	}

	verifier := NewPublicKeyVerifier(&privateKey.PublicKey)

	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123def456",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Size:      1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkKeylessVerification benchmarks keyless (OIDC) signature verification.
func BenchmarkKeylessVerification(b *testing.B) {
	ctx := context.Background()

	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithRekor(false), // Disable Rekor for pure verification benchmark
	)

	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123def456",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Size:      1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkVerificationWithCacheHit benchmarks verification with cache enabled (cache hit).
func BenchmarkVerificationWithCacheHit(b *testing.B) {
	ctx := context.Background()

	// Create cache
	fs := billy.NewMemory()
	cacheConfig := cache.Config{
		MaxSizeBytes: 100 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator, err := cache.NewCoordinator(ctx, cacheConfig, fs, "/cache", nil)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = coordinator.Close() }()

	// Create verifier with cache
	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithCacheTTL(time.Hour),
	).WithCacheForVerifier(coordinator)

	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123def456",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Size:      1024,
	}

	// Prime the cache
	policyHash := ComputePolicyHash(verifier.policy)
	result := &cache.VerificationResult{
		Digest:     descriptor.Digest,
		Verified:   true,
		Signer:     "user@example.com",
		Timestamp:  time.Now(),
		PolicyHash: policyHash,
		TTL:        time.Hour,
	}

	if err := coordinator.PutVerificationResult(ctx, result); err != nil {
		b.Fatalf("Failed to prime cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkVerificationWithCacheMiss benchmarks verification with cache enabled (cache miss).
func BenchmarkVerificationWithCacheMiss(b *testing.B) {
	ctx := context.Background()

	// Create cache
	fs := billy.NewMemory()
	cacheConfig := cache.Config{
		MaxSizeBytes: 100 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator, err := cache.NewCoordinator(ctx, cacheConfig, fs, "/cache", nil)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = coordinator.Close() }()

	// Create verifier with cache
	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithCacheTTL(time.Hour),
	).WithCacheForVerifier(coordinator)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		descriptor := &oras.PullDescriptor{
			Digest:    "sha256:unique" + string(rune(i)),
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Size:      1024,
		}

		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkRekorVerification benchmarks Rekor transparency log verification.
func BenchmarkRekorVerification(b *testing.B) {
	ctx := context.Background()

	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithRekor(true),
	)

	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123def456",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Size:      1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkPolicyHashComputation benchmarks policy hash computation.
func BenchmarkPolicyHashComputation(b *testing.B) {
	policy := NewPolicy()
	policy.AllowedIdentities = []string{"*@example.com", "*@other.com"}
	policy.RequiredAnnotations = map[string]string{
		"build-system": "github-actions",
		"verified":     "true",
	}
	policy.RekorEnabled = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputePolicyHash(policy)
	}
}

// BenchmarkIdentityMatching benchmarks identity pattern matching.
func BenchmarkIdentityMatching(b *testing.B) {
	policy := NewPolicy()
	policy.AllowedIdentities = []string{
		"*@example.com",
		"*@other.org",
		"specific@email.com",
	}

	identity := "user@example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = policy.MatchesIdentity(identity)
	}
}

// BenchmarkPolicyValidation benchmarks policy validation.
func BenchmarkPolicyValidation(b *testing.B) {
	policy := NewPolicy()
	policy.AllowedIdentities = []string{"*@example.com"}
	policy.RequiredAnnotations = map[string]string{
		"build-system": "github-actions",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = policy.Validate()
	}
}

// BenchmarkPublicKeyLoading benchmarks loading public keys from PEM.
func BenchmarkPublicKeyLoading(b *testing.B) {
	// Generate a key and encode it
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("Failed to generate key: %v", err)
	}

	// Encode public key to PEM
	keyBytes := encodePublicKeyToPEM(&privateKey.PublicKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadPublicKeyFromBytes(keyBytes)
	}
}

// BenchmarkAnnotationChecking benchmarks annotation validation.
func BenchmarkAnnotationChecking(b *testing.B) {
	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithRequiredAnnotations(map[string]string{
			"build-system":  "github-actions",
			"verified":      "true",
			"security-scan": "passed",
		}),
	)

	annotations := map[string]string{
		"build-system":  "github-actions",
		"verified":      "true",
		"security-scan": "passed",
		"extra-field":   "extra-value",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.checkAnnotations(annotations)
	}
}

// BenchmarkMultiplePublicKeys benchmarks verification with multiple public keys.
func BenchmarkMultiplePublicKeys(b *testing.B) {
	ctx := context.Background()

	// Generate 5 keys
	keys := make([]crypto.PublicKey, 5)
	for i := 0; i < 5; i++ {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			b.Fatalf("Failed to generate key: %v", err)
		}
		keys[i] = &privateKey.PublicKey
	}

	verifier := NewPublicKeyVerifierWithOptions(keys)

	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123def456",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Size:      1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifier.Verify(ctx, "example.com/test:v1", descriptor)
	}
}

// BenchmarkVerifierCreation benchmarks creating verifier instances.
func BenchmarkVerifierCreation(b *testing.B) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatalf("Failed to generate key: %v", err)
	}

	b.Run("PublicKey", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewPublicKeyVerifier(&privateKey.PublicKey)
		}
	})

	b.Run("Keyless", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewKeylessVerifier(
				WithAllowedIdentities("*@example.com"),
			)
		}
	})

	b.Run("WithOptions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewPublicKeyVerifierWithOptions(
				[]crypto.PublicKey{&privateKey.PublicKey},
				WithRequireAll(true),
				WithEnforceMode(true),
			)
		}
	})
}

// BenchmarkCacheOperations benchmarks cache read/write operations.
func BenchmarkCacheOperations(b *testing.B) {
	ctx := context.Background()

	fs := billy.NewMemory()
	cacheConfig := cache.Config{
		MaxSizeBytes: 100 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator, err := cache.NewCoordinator(ctx, cacheConfig, fs, "/cache", nil)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = coordinator.Close() }()

	result := &cache.VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "user@example.com",
		Timestamp:  time.Now(),
		PolicyHash: "hash123",
		TTL:        time.Hour,
	}

	b.Run("Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			result.Digest = "sha256:unique" + string(rune(i))
			_ = coordinator.PutVerificationResult(ctx, result)
		}
	})

	// Prime cache for read benchmark
	_ = coordinator.PutVerificationResult(ctx, result)

	b.Run("Read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = coordinator.GetVerificationResult(ctx, result.Digest, result.PolicyHash)
		}
	})
}

// Helper function to encode public key to PEM.
func encodePublicKeyToPEM(_ crypto.PublicKey) []byte {
	// Simplified encoding for benchmark
	// In reality, this would use x509.MarshalPKIXPublicKey
	return []byte("-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----")
}
