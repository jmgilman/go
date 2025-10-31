// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"context"
	"crypto"
	"fmt"
	"os"
	"strings"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/rekor/pkg/generated/client"
	"github.com/sigstore/sigstore/pkg/signature"
)

// policyToCheckOpts converts our Policy to Cosign's CheckOpts.
// This translation layer maps our policy abstraction to Cosign's verification configuration.
//
// Mapping:
// - Public key mode: CheckOpts.SigVerifier from loaded public key
// - Keyless mode: CheckOpts.Identities, CheckOpts.CertOidcIssuer
// - Required annotations: CheckOpts.Annotations
// - Rekor URL: CheckOpts.RekorClient and CheckOpts.RekorPubKeys
//
// Returns CheckOpts configured for verification, or an error if policy is invalid.
func policyToCheckOpts(ctx context.Context, policy *Policy) (*cosign.CheckOpts, error) {
	if policy == nil {
		return nil, fmt.Errorf("policy cannot be nil")
	}

	// Validate policy before converting
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid policy: %w", err)
	}

	checkOpts := &cosign.CheckOpts{
		// Core Cosign settings
		ClaimVerifier: cosign.SimpleClaimVerifier,
		IgnoreSCT:     false, // Verify SCT (Certificate Transparency)
		IgnoreTlog:    !policy.RekorEnabled,
	}

	// Configure based on verification mode (public key vs keyless)
	if policy.IsKeylessMode() {
		// Keyless mode: configure identity and issuer matching
		if err := configureKeylessMode(ctx, checkOpts, policy); err != nil {
			return nil, fmt.Errorf("failed to configure keyless mode: %w", err)
		}
	} else {
		// Public key mode: configure signature verifier
		if err := configurePublicKeyMode(checkOpts, policy); err != nil {
			return nil, fmt.Errorf("failed to configure public key mode: %w", err)
		}
	}

	// Configure Rekor if enabled
	if policy.RekorEnabled {
		if err := configureRekor(checkOpts, policy); err != nil {
			return nil, fmt.Errorf("failed to configure Rekor: %w", err)
		}
	}

	// Configure annotations if required
	// Note: Cosign's CheckOpts.Annotations field expects map[string]interface{}
	// We need to convert our map[string]string to the expected type
	if len(policy.RequiredAnnotations) > 0 {
		annotations := make(map[string]interface{}, len(policy.RequiredAnnotations))
		for k, v := range policy.RequiredAnnotations {
			annotations[k] = v
		}
		checkOpts.Annotations = annotations
	}

	return checkOpts, nil
}

// configureKeylessMode sets up CheckOpts for keyless (OIDC) verification.
// This configures identity/issuer matching using Cosign's native matchers.
func configureKeylessMode(ctx context.Context, checkOpts *cosign.CheckOpts, policy *Policy) error {
	// Configure identity matchers
	// Cosign's Identities field expects a list of identity matchers
	// Each identity can be an exact match or a regex pattern
	if len(policy.AllowedIdentities) > 0 {
		// Convert our glob patterns to Cosign's identity format
		identities := make([]cosign.Identity, 0, len(policy.AllowedIdentities))
		for _, pattern := range policy.AllowedIdentities {
			// Validate pattern length (security check preserved from policy.go)
			if len(pattern) > maxPatternLength {
				return fmt.Errorf("identity pattern too long: %d characters (max: %d)", len(pattern), maxPatternLength)
			}

			// Security: Reject patterns containing null bytes or control characters
			for _, c := range pattern {
				if c == 0 || (c < 32 && c != '\t' && c != '\n' && c != '\r') {
					return fmt.Errorf("identity pattern contains invalid character")
				}
			}

			// For Cosign, we can use the pattern directly or convert glob to regex
			// Cosign supports both subject and issuer matching
			identity := cosign.Identity{
				// Subject is the signer identity (email, URI, etc.)
				Subject: pattern,
			}

			// If a specific issuer is required, add it to each identity
			if policy.RequiredIssuer != "" {
				identity.Issuer = policy.RequiredIssuer
			}

			identities = append(identities, identity)
		}

		checkOpts.Identities = identities
	} else if policy.RequiredIssuer != "" {
		// No specific identities required, but issuer is specified
		// Allow any identity from the required issuer
		checkOpts.Identities = []cosign.Identity{
			{
				// Empty subject matches any identity
				Issuer: policy.RequiredIssuer,
			},
		}
	}

	// Use TUF for Fulcio roots (default Sigstore behavior)
	// This replaces our manual TUF root fetching in keyless.go:verifyCertificateChain
	checkOpts.RootCerts = nil // nil means use TUF to fetch roots automatically

	// Ignore SCT only if explicitly requested (default: verify SCT)
	checkOpts.IgnoreSCT = false

	return nil
}

// configurePublicKeyMode sets up CheckOpts for public key verification.
// This creates a multi-key verifier that can verify with any of the provided public keys.
func configurePublicKeyMode(checkOpts *cosign.CheckOpts, policy *Policy) error {
	if len(policy.PublicKeys) == 0 {
		return fmt.Errorf("public key mode requires at least one public key")
	}

	// Validate all public keys meet minimum strength requirements
	for i, pubKey := range policy.PublicKeys {
		if err := validateKeyStrength(pubKey); err != nil {
			return fmt.Errorf("public key %d failed validation: %w", i, err)
		}
	}

	// For public key mode, we need to create a signature verifier
	// If we have multiple keys, we can use a multi-key verifier
	// Cosign will try each key until one succeeds (OR logic)
	//
	// However, Cosign's CheckOpts.SigVerifier field expects a single verifier.
	// We'll need to handle multiple keys at a higher level by creating
	// multiple CheckOpts or using a composite verifier.
	//
	// For now, we'll use the first key and handle multi-key verification
	// in the calling code (preserving our existing multi-signature logic).
	pubKey := policy.PublicKeys[0]

	// Create a verifier from the public key
	verifier, err := signature.LoadVerifier(pubKey, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("failed to create signature verifier: %w", err)
	}

	checkOpts.SigVerifier = verifier

	// For public key mode, we don't use TUF or Fulcio
	checkOpts.RootCerts = nil
	checkOpts.IgnoreSCT = true  // No SCT in public key mode
	checkOpts.IgnoreTlog = true // Rekor is configured separately if enabled

	return nil
}

// configureRekor sets up Rekor transparency log verification.
// This replaces our manual Rekor client creation in rekor.go.
func configureRekor(checkOpts *cosign.CheckOpts, policy *Policy) error {
	// Get Rekor URL (use default if not specified)
	rekorURL := policy.RekorURL
	if rekorURL == "" {
		// Use default Sigstore Rekor instance
		// Check environment variable first
		if envURL := os.Getenv("REKOR_URL"); envURL != "" {
			rekorURL = envURL
		} else {
			rekorURL = "https://rekor.sigstore.dev"
		}
	}

	// Validate Rekor URL uses HTTPS (security requirement preserved from rekor.go)
	if !strings.HasPrefix(rekorURL, "https://") {
		return fmt.Errorf("Rekor URL must use HTTPS: %s", rekorURL)
	}

	// Parse the host from the URL (remove https:// prefix)
	rekorHost := strings.TrimPrefix(rekorURL, "https://")

	// Create Rekor client
	rekorClient := client.NewHTTPClientWithConfig(nil, &client.TransportConfig{
		Host:     rekorHost,
		BasePath: client.DefaultBasePath,
		Schemes:  []string{"https"},
	})

	checkOpts.RekorClient = rekorClient
	checkOpts.IgnoreTlog = false // Enable transparency log verification

	// Note: RekorPubKeys can be set here if custom Rekor instance is used
	// For the public Sigstore instance, keys are fetched via TUF
	// checkOpts.RekorPubKeys = ...

	return nil
}
