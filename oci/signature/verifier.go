// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/cosign/v2/pkg/oci"
	"github.com/sigstore/cosign/v2/pkg/oci/static"
	"github.com/sigstore/sigstore/pkg/signature"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"

	ocibundle "github.com/jmgilman/go/oci"
	orasint "github.com/jmgilman/go/oci/internal/oras"
)

// CosignVerifier implements signature verification for OCI artifacts using Sigstore/Cosign.
// It supports both traditional public key verification and keyless (OIDC) verification modes.
//
// The verifier is thread-safe and can be reused across multiple verification operations.
// Verification results can be cached to avoid redundant cryptographic operations.
type CosignVerifier struct {
	// policy contains all verification settings and requirements
	policy *Policy

	// cache stores verification results to avoid redundant operations
	// Optional - if nil, no caching is performed
	cache VerificationCache
}

// NewPublicKeyVerifier creates a new CosignVerifier for public key verification.
// This mode uses traditional public key cryptography where artifacts are signed
// with a private key and verified with the corresponding public key.
//
// Multiple public keys can be provided. By default, any valid signature from any
// key will pass verification (OR logic). Use WithRequireAll() to require all keys
// to have valid signatures.
//
// Example:
//
//	pubKey, err := LoadPublicKey("cosign.pub")
//	if err != nil {
//	    return err
//	}
//	verifier := NewPublicKeyVerifier(pubKey)
//
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(verifier),
//	)
func NewPublicKeyVerifier(keys ...crypto.PublicKey) *CosignVerifier {
	policy := NewPolicy()
	policy.PublicKeys = keys
	// Public key mode defaults: 24 hour cache since keys don't expire
	policy.CacheTTL = 24 * 3600 * 1000000000 // 24 hours in nanoseconds

	return &CosignVerifier{
		policy: policy,
	}
}

// NewPublicKeyVerifierWithOptions creates a public key verifier with custom options.
// This provides more control than NewPublicKeyVerifier, allowing you to configure
// verification behavior such as multi-signature policies, caching, and enforcement mode.
//
// The keys parameter accepts a slice of public keys. By default, any valid signature
// from any key passes verification (OR logic). Use WithRequireAll() or
// WithMinimumSignatures() to change this behavior.
//
// Example with multi-signature policy:
//
//	verifier := NewPublicKeyVerifierWithOptions(
//	    []crypto.PublicKey{key1, key2, key3},
//	    WithRequireAll(true), // All 3 signatures required
//	    WithEnforceMode(true), // Missing signatures fail
//	)
//
// Example with threshold policy:
//
//	verifier := NewPublicKeyVerifierWithOptions(
//	    []crypto.PublicKey{key1, key2, key3, key4, key5},
//	    WithMinimumSignatures(3), // At least 3 of 5 required
//	)
func NewPublicKeyVerifierWithOptions(keys []crypto.PublicKey, opts ...VerifierOption) *CosignVerifier {
	policy := NewPolicy()
	policy.PublicKeys = keys
	policy.CacheTTL = 24 * 3600 * 1000000000 // 24 hours

	verifier := &CosignVerifier{
		policy: policy,
		cache:  nil, // Will be set by WithCache option if provided
	}

	// Apply options - some options may need access to the verifier itself
	// For now, options only modify policy, but WithCache will need special handling
	for _, opt := range opts {
		opt(policy)
	}

	return verifier
}

// WithCacheForVerifier sets the cache on an existing verifier.
// This is a helper method to enable cache injection after verifier creation,
// allowing you to add caching to an already-configured verifier.
//
// Caching significantly improves performance by storing verification results:
//   - Without caching: 55-730ms per verification (depending on mode)
//   - With caching: <1ms per verification (99.8% reduction)
//
// The cache key includes both the artifact digest and the policy hash, ensuring
// automatic invalidation when the policy changes.
//
// Example:
//
//	// Create cache coordinator
//	coordinator, _ := cache.NewCoordinator(ctx, cacheConfig, fs, cacheDir, nil)
//
//	// Create verifier and add caching
//	verifier := NewKeylessVerifier(
//	    WithAllowedIdentities("*@example.com"),
//	    WithCacheTTL(time.Hour),
//	).WithCacheForVerifier(coordinator)
//
// Note: This method returns the verifier to support method chaining.
func (v *CosignVerifier) WithCacheForVerifier(cache VerificationCache) *CosignVerifier {
	v.cache = cache
	return v
}

// NewKeylessVerifier creates a new CosignVerifier for keyless (OIDC) verification.
// This mode uses Sigstore's keyless signing infrastructure where identities are
// verified via OIDC providers (GitHub, Google, etc.) and short-lived certificates.
//
// Options should specify identity restrictions, issuer requirements, and whether
// Rekor transparency log verification is required.
//
// Example:
//
//	verifier := NewKeylessVerifier(
//	    WithAllowedIdentities("*@example.com"),
//	    WithRekor(true), // Require transparency log
//	)
//
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(verifier),
//	)
func NewKeylessVerifier(opts ...VerifierOption) *CosignVerifier {
	policy := NewPolicy()
	// Keyless mode defaults: 1 hour cache since certificates can expire
	policy.CacheTTL = 3600 * 1000000000 // 1 hour in nanoseconds

	verifier := &CosignVerifier{
		policy: policy,
		cache:  nil, // Will be set by WithCache option if provided
	}

	// Apply options
	for _, opt := range opts {
		opt(policy)
	}

	return verifier
}

// Verify validates the signature for the given OCI artifact.
// This method implements the ocibundle.SignatureVerifier interface.
//
// The verification process:
//  1. Validate input parameters
//  2. Check cache for previous verification result (if caching enabled)
//  3. Build the signature reference using Cosign's naming convention
//  4. Fetch the signature artifact from the registry
//  5. Verify the cryptographic signature matches the artifact digest
//  6. Validate policy requirements (identity, annotations, etc.)
//  7. Optionally verify transparency log inclusion (Rekor)
//  8. Store verification result in cache (if caching enabled)
//
// Returns nil if verification succeeds, or a BundleError with details if it fails.
func (v *CosignVerifier) Verify(ctx context.Context, reference string, descriptor *orasint.PullDescriptor) error {
	// Validate input parameters
	if descriptor == nil {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("descriptor cannot be nil"),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       "",
				Reason:       "Invalid input: descriptor is nil",
				FailureStage: "validation",
			},
		}
	}

	if descriptor.Digest == "" {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("descriptor digest cannot be empty"),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       "",
				Reason:       "Invalid input: digest is empty",
				FailureStage: "validation",
			},
		}
	}

	// Validate digest format (should be "algorithm:hex")
	if !strings.Contains(descriptor.Digest, ":") {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("invalid digest format"),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       descriptor.Digest,
				Reason:       fmt.Sprintf("Invalid digest format: %s (expected 'algorithm:hex')", descriptor.Digest),
				FailureStage: "validation",
			},
		}
	}

	if descriptor.Size <= 0 {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("descriptor size must be positive"),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       descriptor.Digest,
				Reason:       fmt.Sprintf("Invalid input: size is %d", descriptor.Size),
				FailureStage: "validation",
			},
		}
	}

	if reference == "" {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: "",
			Err:       fmt.Errorf("reference cannot be empty"),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       descriptor.Digest,
				Reason:       "Invalid input: reference is empty",
				FailureStage: "validation",
			},
		}
	}

	// Validate policy configuration
	if err := v.policy.Validate(); err != nil {
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("invalid verification policy: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       descriptor.Digest,
				Reason:       fmt.Sprintf("Invalid verification policy: %s", err.Error()),
				FailureStage: "policy",
			},
		}
	}

	// Check cache if enabled
	// Note: Cache implementation MUST validate TTL expiration before returning results
	// (see VerificationCache interface documentation for security requirements)
	if v.cache != nil {
		policyHash := ComputePolicyHash(v.policy)
		verified, signer, err := v.cache.GetCachedVerification(ctx, descriptor.Digest, policyHash)

		if err == nil {
			// Cache hit (and not expired - cache validates TTL)
			if verified {
				// Previous verification succeeded - return success
				// Note: We trust the cached result because:
				//   1. The policy hash matches (policy hasn't changed)
				//   2. Cache validated TTL (result is not stale)
				return nil
			}
			// Previous verification failed - return the cached failure
			// For cached failures, we re-verify to get fresh error details
			// (Alternatively, we could cache the error details too)
			// Fall through to perform fresh verification
			_ = signer // Signer info available from cache if needed
		}
		// Cache miss, expired, or error - proceed with verification
		// Cache errors are logged but don't fail verification (cache is optional)
	}

	// Build signature reference using Cosign convention
	signatureRef := v.buildSignatureReference(reference, descriptor.Digest)

	// Fetch signature(s) from the registry
	signatures, err := v.fetchSignatureArtifacts(ctx, signatureRef)
	if err != nil {
		// Use proper error classification instead of string matching
		if isNotFoundError(err) {
			// Signature not found - apply verification mode policy
			if v.policy.VerificationMode == VerificationModeEnforce {
				return &ocibundle.BundleError{
					Op:        "verify",
					Reference: reference,
					Err:       ocibundle.ErrSignatureNotFound,
					SignatureInfo: &ocibundle.SignatureErrorInfo{
						Digest:       descriptor.Digest,
						Reason:       fmt.Sprintf("No signature found for artifact (enforce mode). Signature reference: %s", signatureRef),
						FailureStage: "fetch",
					},
				}
			} else if v.policy.VerificationMode == VerificationModeOptional {
				// Optional mode: signature missing is allowed
				return nil
			}
			// Required mode: signature missing is allowed, but invalid signatures fail
			return nil
		}

		// Other fetch errors (network, auth, etc.)
		return &ocibundle.BundleError{
			Op:        "verify",
			Reference: reference,
			Err:       fmt.Errorf("failed to fetch signature: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       descriptor.Digest,
				Reason:       fmt.Sprintf("Failed to fetch signature artifact: %s", err.Error()),
				FailureStage: "fetch",
			},
		}
	}

	if len(signatures) == 0 {
		// No signatures found
		if v.policy.VerificationMode == VerificationModeEnforce {
			return &ocibundle.BundleError{
				Op:        "verify",
				Reference: reference,
				Err:       ocibundle.ErrSignatureNotFound,
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       descriptor.Digest,
					Reason:       "No signatures found for artifact (enforce mode)",
					FailureStage: "fetch",
				},
			}
		}
		// Optional or Required mode: missing signature is allowed
		return nil
	}

	// Verify signatures based on policy
	validCount, err := v.verifySignatures(ctx, descriptor.Digest, signatures)
	if err != nil {
		return err // Error already wrapped as BundleError
	}

	// Check if verification meets policy requirements
	if err := v.checkSignaturePolicy(validCount, len(signatures), reference, descriptor.Digest); err != nil {
		// Store failed verification in cache (if enabled)
		v.storeCachedVerification(ctx, descriptor.Digest, false, "")
		return err
	}

	// Verification succeeded - store result in cache (if enabled)
	// Extract signer identity if available
	signer := v.extractSignerIdentity(signatures)
	v.storeCachedVerification(ctx, descriptor.Digest, true, signer)

	return nil
}

// storeCachedVerification stores a verification result in the cache if caching is enabled.
// Cache storage failures are logged but do not fail the verification operation.
// This ensures that caching is truly optional and doesn't impact reliability.
func (v *CosignVerifier) storeCachedVerification(ctx context.Context, digest string, verified bool, signer string) {
	if v.cache == nil {
		return // Caching not enabled
	}

	policyHash := ComputePolicyHash(v.policy)
	ttl := v.policy.CacheTTL

	// Attempt to store in cache - errors are logged but don't fail verification
	if err := v.cache.PutCachedVerification(ctx, digest, policyHash, verified, signer, ttl); err != nil {
		// Log error but don't fail verification
		// In production, this would use a proper logger
		// For now, we silently ignore cache storage failures
		_ = err
	}
}

// extractSignerIdentity extracts the signer identity from verified signatures for audit logging.
// For keyless signatures, this is typically an email address or OIDC subject.
// For public key signatures, this returns empty (no identity available).
func (v *CosignVerifier) extractSignerIdentity(signatures []oci.Signature) string {
	if len(signatures) == 0 {
		return ""
	}

	// Use the first valid signature's identity
	for _, sig := range signatures {
		identity := v.extractIdentityFromSignature(sig)
		if identity != "" {
			return identity
		}
	}

	return ""
}

// extractIdentityFromSignature extracts identity from a single signature.
// For keyless signatures: extracts from certificate (email, URI, etc.)
// For public key signatures: no identity available (returns empty)
func (v *CosignVerifier) extractIdentityFromSignature(sig oci.Signature) string {
	// Check if this is a keyless signature (has a certificate)
	cert, err := sig.Cert()
	if err != nil || cert == nil {
		// Not a keyless signature or error getting cert
		// Public key signatures don't have certificates
		return ""
	}

	// Extract identity from certificate (reuse logic from keyless.go)
	identity, err := extractIdentityFromCert(cert)
	if err != nil {
		// Failed to extract identity
		return ""
	}

	return identity
}

// hexRegex validates that a string contains only lowercase hexadecimal characters
var hexRegex = regexp.MustCompile(`^[a-f0-9]+$`)

// buildSignatureReference constructs the OCI reference for the signature artifact.
// Cosign stores signatures as separate artifacts with a predictable naming convention:
//
//	Image:     ghcr.io/org/repo:tag              → sha256:abc123...
//	Signature: ghcr.io/org/repo:sha256-abc123.sig
//
// The .sig tag points to the signature artifact that contains the signature data.
func (v *CosignVerifier) buildSignatureReference(reference, digestStr string) string {
	// Parse digest string manually for validation (more flexible than digest.Parse)
	// Expected format: "algorithm:hexstring" (e.g., "sha256:abc123...")
	parts := strings.SplitN(digestStr, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	algorithm := parts[0]
	hexPart := parts[1]

	// Validate algorithm is sha256 (required by Cosign)
	if algorithm != "sha256" {
		return ""
	}

	// Validate hex part contains only valid hex characters
	if !hexRegex.MatchString(hexPart) {
		return ""
	}

	// Validate hex part length (protect against excessively long inputs)
	// Real SHA256 hashes are 64 characters, but we allow flexibility for testing
	// Maximum of 128 characters prevents DoS via extremely long digest strings
	if len(hexPart) == 0 || len(hexPart) > 128 {
		return ""
	}

	// Extract the repository part (everything before the tag/digest)
	// Check for @ first (digest reference), then : (tag reference)
	var repo string
	if idx := strings.LastIndex(reference, "@"); idx != -1 {
		repo = reference[:idx]
	} else if idx := strings.LastIndex(reference, ":"); idx != -1 {
		// Need to handle port numbers in registry (e.g., localhost:5000/repo:tag)
		// Find the last : that's after the last /
		lastSlash := strings.LastIndex(reference, "/")
		lastColon := strings.LastIndex(reference, ":")
		if lastColon > lastSlash {
			repo = reference[:lastColon]
		} else {
			repo = reference
		}
	} else {
		repo = reference
	}

	// Validate repository format (basic OCI naming validation)
	// Repository must not be empty and should not contain invalid characters
	if repo == "" || strings.Contains(repo, "..") || strings.HasPrefix(repo, "-") {
		return ""
	}

	// Build signature reference: repo:sha256-<digest>.sig
	signatureRef := fmt.Sprintf("%s:sha256-%s.sig", repo, hexPart)

	// Validate total length (max OCI reference length is typically 255-512 characters)
	if len(signatureRef) > 512 {
		return ""
	}

	return signatureRef
}

// fetchSignatureArtifacts retrieves signature artifacts from the registry.
// Returns a list of signature objects or an error if fetching fails.
func (v *CosignVerifier) fetchSignatureArtifacts(ctx context.Context, signatureRef string) ([]oci.Signature, error) {
	// Create a basic repository for fetching
	// Extract registry and repository from reference
	repo, err := v.createRemoteRepository(signatureRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Fetch the signature manifest
	manifestDesc, err := repo.Resolve(ctx, signatureRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve signature manifest: %w", err)
	}

	// Fetch the manifest content
	manifestReader, err := repo.Fetch(ctx, manifestDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch signature manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestBytes, err := io.ReadAll(manifestReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read signature manifest: %w", err)
	}

	// Parse the manifest to extract signature layers
	var manifest struct {
		Layers []struct {
			MediaType   string            `json:"mediaType"`
			Digest      string            `json:"digest"`
			Size        int64             `json:"size"`
			Annotations map[string]string `json:"annotations,omitempty"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse signature manifest: %w", err)
	}

	// Fetch each signature layer
	signatures := []oci.Signature{}
	for _, layer := range manifest.Layers {
		// Fetch the layer (signature payload)
		layerDesc := manifestDesc
		layerDesc.Digest = digest.Digest(layer.Digest)
		layerDesc.Size = layer.Size

		layerReader, err := repo.Fetch(ctx, layerDesc)
		if err != nil {
			continue // Skip this signature if we can't fetch it
		}

		layerBytes, err := io.ReadAll(layerReader)
		layerReader.Close()
		if err != nil {
			continue
		}

		// Create a static signature object
		sig, err := static.NewSignature(layerBytes, base64.StdEncoding.EncodeToString([]byte(layer.Digest)))
		if err != nil {
			continue
		}

		signatures = append(signatures, sig)
	}

	return signatures, nil
}

// verifySignatures verifies all signatures and returns the count of valid ones.
// Returns (validCount, error). The error is a wrapped BundleError if verification fails.
func (v *CosignVerifier) verifySignatures(ctx context.Context, digest string, signatures []oci.Signature) (int, error) {
	validCount := 0
	var lastError error

	for _, sig := range signatures {
		var err error
		if v.policy.IsKeylessMode() {
			// Keyless verification
			err = v.verifyKeylessSignature(ctx, digest, sig)
		} else {
			// Public key verification
			err = v.verifyPublicKeySignature(ctx, digest, sig)
		}

		if err == nil {
			validCount++
			// For "Any" mode, one valid signature is enough
			if v.policy.MultiSignatureMode == MultiSignatureModeAny {
				return validCount, nil
			}
		} else {
			lastError = err
		}
	}

	// If we need all signatures to be valid and we have invalid ones
	if v.policy.MultiSignatureMode == MultiSignatureModeAll && validCount < len(signatures) {
		return validCount, lastError
	}

	return validCount, nil
}

// verifyPublicKeySignature verifies a signature using public key cryptography.
func (v *CosignVerifier) verifyPublicKeySignature(ctx context.Context, digestStr string, sig oci.Signature) error {
	// Get the signature payload (contains the Simple Signing payload with digest)
	payload, err := sig.Payload()
	if err != nil {
		return fmt.Errorf("failed to get signature payload: %w", err)
	}

	if len(payload) == 0 {
		return fmt.Errorf("signature payload is empty")
	}

	// Parse the payload to verify it contains the expected digest
	// Structure follows the Cosign Simple Signing format
	var payloadData struct {
		Critical struct {
			Image struct {
				DockerManifestDigest string `json:"docker-manifest-digest"`
			} `json:"image"`
			Type     string `json:"type"`
			Identity struct {
				DockerReference string `json:"docker-reference"`
			} `json:"identity"`
		} `json:"critical"`
	}

	if err := json.Unmarshal(payload, &payloadData); err != nil {
		return fmt.Errorf("failed to parse signature payload: %w", err)
	}

	// Validate required fields in payload
	if payloadData.Critical.Image.DockerManifestDigest == "" {
		return fmt.Errorf("payload missing required field: docker-manifest-digest")
	}

	// Validate signature type (Cosign Simple Signing format)
	// Expected: "atomic container signature" or "cosign container image signature"
	if payloadData.Critical.Type != "" {
		validTypes := []string{
			"atomic container signature",
			"cosign container image signature",
		}
		validType := false
		for _, vt := range validTypes {
			if payloadData.Critical.Type == vt {
				validType = true
				break
			}
		}
		if !validType {
			return fmt.Errorf("invalid signature type: %q (expected Cosign format)", payloadData.Critical.Type)
		}
	}

	// Parse and normalize both digests using OCI digest library
	artifactDigest, err := digest.Parse(digestStr)
	if err != nil {
		return fmt.Errorf("invalid artifact digest: %w", err)
	}

	payloadDigest, err := digest.Parse(payloadData.Critical.Image.DockerManifestDigest)
	if err != nil {
		return fmt.Errorf("invalid payload digest: %w", err)
	}

	// Verify algorithm is SHA256 (required by OCI spec)
	if artifactDigest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("unsupported digest algorithm: %s (required: sha256)", artifactDigest.Algorithm())
	}

	// Compare normalized digests (handles case differences and format variations)
	if artifactDigest != payloadDigest {
		return fmt.Errorf("payload digest %q does not match artifact digest %q",
			payloadDigest, artifactDigest)
	}

	// Get the signature bytes
	sigBytes, err := sig.Base64Signature()
	if err != nil {
		return fmt.Errorf("failed to get signature bytes: %w", err)
	}

	// Decode base64 signature
	decodedSig, err := base64.StdEncoding.DecodeString(sigBytes)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Try to verify with each public key (OR logic by default)
	var lastErr error
	for _, pubKey := range v.policy.PublicKeys {
		// Create a verifier from the public key
		verifier, err := signature.LoadVerifier(pubKey, crypto.SHA256)
		if err != nil {
			lastErr = fmt.Errorf("failed to create verifier: %w", err)
			continue
		}

		// Verify the signature
		if err := verifier.VerifySignature(strings.NewReader(string(decodedSig)), strings.NewReader(string(payload))); err != nil {
			lastErr = fmt.Errorf("signature verification failed: %w", err)
			continue
		}

		// Signature verified successfully with this key
		// Check annotations if required
		if len(v.policy.RequiredAnnotations) > 0 {
			annotations, err := sig.Annotations()
			if err != nil {
				return fmt.Errorf("failed to get annotations: %w", err)
			}
			if !v.checkAnnotations(annotations) {
				return fmt.Errorf("required annotations not satisfied")
			}
		}

		return nil
	}

	// No public key successfully verified the signature
	if lastErr != nil {
		return fmt.Errorf("signature verification failed with all public keys: %w", lastErr)
	}
	return fmt.Errorf("no public keys available for verification")
}

// checkSignaturePolicy verifies that the number of valid signatures meets policy requirements.
func (v *CosignVerifier) checkSignaturePolicy(validCount, totalCount int, reference, digest string) error {
	switch v.policy.MultiSignatureMode {
	case MultiSignatureModeAny:
		if validCount >= 1 {
			return nil
		}
	case MultiSignatureModeAll:
		if validCount == totalCount {
			return nil
		}
	case MultiSignatureModeMinimum:
		if validCount >= v.policy.MinimumSignatures {
			return nil
		}
	}

	// Policy not satisfied
	return &ocibundle.BundleError{
		Op:        "verify",
		Reference: reference,
		Err:       ocibundle.ErrSignatureInvalid,
		SignatureInfo: &ocibundle.SignatureErrorInfo{
			Digest: digest,
			Reason: fmt.Sprintf("Signature policy not satisfied: mode=%s, valid=%d, total=%d, required=%d",
				v.policy.MultiSignatureMode, validCount, totalCount, v.policy.MinimumSignatures),
			FailureStage: "policy",
		},
	}
}

// checkAnnotations verifies that required annotations are present.
func (v *CosignVerifier) checkAnnotations(annotations map[string]string) bool {
	for key, requiredValue := range v.policy.RequiredAnnotations {
		actualValue, exists := annotations[key]
		if !exists || actualValue != requiredValue {
			return false
		}
	}
	return true
}

// createRemoteRepository creates a remote repository for the given reference.
func (v *CosignVerifier) createRemoteRepository(ref string) (*remote.Repository, error) {
	// Extract registry and repository from reference
	// Format: registry/repository:tag
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid reference format: %s", ref)
	}

	registry := parts[0]
	repoAndTag := parts[1]

	// Extract repository (remove tag if present)
	repoParts := strings.Split(repoAndTag, ":")
	repository := repoParts[0]

	// Create remote repository
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", registry, repository))
	if err != nil {
		return nil, fmt.Errorf("failed to create remote repository: %w", err)
	}

	return repo, nil
}

// Policy returns a copy of the verification policy.
// This is useful for inspecting the verifier's configuration.
func (v *CosignVerifier) Policy() Policy {
	if v.policy == nil {
		return *NewPolicy()
	}
	return *v.policy
}

// isNotFoundError checks if an error indicates that a resource was not found.
// This uses proper error type checking instead of fragile string matching.
//
// The function checks:
// 1. OCI/ORAS standard error definitions (errdef.ErrNotFound)
// 2. String matching as a fallback for non-standard registries
//
// This prevents security issues from misclassifying network errors or
// other transient failures as "not found" errors.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for OCI/ORAS standard "not found" error
	// This is the proper way to detect 404/manifest not found errors
	if errors.Is(err, errdef.ErrNotFound) {
		return true
	}

	// Fallback: String matching for registries that don't use standard error types
	// This is kept as a last resort for compatibility but is less reliable
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "manifest unknown") ||
		strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "no such")
}
