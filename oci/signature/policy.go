// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"crypto"
	"fmt"
	"regexp"
	"time"

	"github.com/gobwas/glob"
)

// VerificationMode defines how signature verification should be enforced.
type VerificationMode int

const (
	// VerificationModeOptional allows pull operations to succeed even if
	// signature verification fails. Useful for audit mode during rollout.
	// Verification failures are logged but don't block operations.
	VerificationModeOptional VerificationMode = iota

	// VerificationModeRequired requires signatures to be valid but allows
	// missing signatures. Useful for soft enforcement during migration.
	// Invalid signatures fail the operation, but unsigned artifacts are allowed.
	VerificationModeRequired

	// VerificationModeEnforce requires all artifacts to have valid signatures.
	// Missing or invalid signatures fail the operation.
	// This is the recommended mode for production security.
	VerificationModeEnforce
)

// String returns the string representation of the verification mode.
func (m VerificationMode) String() string {
	switch m {
	case VerificationModeOptional:
		return "optional"
	case VerificationModeRequired:
		return "required"
	case VerificationModeEnforce:
		return "enforce"
	default:
		return "unknown"
	}
}

// MultiSignatureMode defines how multiple signatures should be validated.
type MultiSignatureMode int

const (
	// MultiSignatureModeAny accepts if ANY signature is valid (OR logic).
	// This is the default and most permissive mode.
	// Useful when multiple people can sign artifacts independently.
	MultiSignatureModeAny MultiSignatureMode = iota

	// MultiSignatureModeAll requires ALL signatures to be valid (AND logic).
	// Useful for requiring unanimous approval from multiple signers.
	// All found signatures must verify successfully.
	MultiSignatureModeAll

	// MultiSignatureModeMinimum requires at least N signatures to be valid.
	// Useful for threshold-based approval workflows.
	// The MinimumSignatures field specifies the required count.
	MultiSignatureModeMinimum
)

// String returns the string representation of the multi-signature mode.
func (m MultiSignatureMode) String() string {
	switch m {
	case MultiSignatureModeAny:
		return "any"
	case MultiSignatureModeAll:
		return "all"
	case MultiSignatureModeMinimum:
		return "minimum"
	default:
		return "unknown"
	}
}

const (
	// maxIdentityLength is the maximum allowed length for a signer identity.
	// Prevents DoS attacks via extremely long identity strings.
	maxIdentityLength = 512

	// maxPatternLength is the maximum allowed length for an identity pattern.
	// Prevents DoS attacks via extremely long or complex patterns.
	maxPatternLength = 256
)

// Policy contains all verification settings for signature validation.
// Policies control how signatures are verified and what requirements must be met.
type Policy struct {
	// VerificationMode controls enforcement level (optional, required, enforce).
	VerificationMode VerificationMode

	// MultiSignatureMode controls how multiple signatures are validated (any, all, minimum).
	MultiSignatureMode MultiSignatureMode

	// MinimumSignatures is the minimum number of valid signatures required.
	// Only used when MultiSignatureMode is MultiSignatureModeMinimum.
	MinimumSignatures int

	// PublicKeys contains the public keys for signature verification.
	// Used for traditional public key cryptography mode.
	// If empty, keyless (OIDC) verification is assumed.
	PublicKeys []crypto.PublicKey

	// AllowedIdentities contains patterns for allowed signer identities.
	// Used in keyless verification to match against certificate subjects.
	// Supports glob patterns like "*@example.com" or exact matches.
	AllowedIdentities []string

	// RequiredIssuer is the required OIDC issuer for keyless verification.
	// Examples: "https://github.com/login/oauth", "https://accounts.google.com"
	// If empty, any issuer is accepted.
	RequiredIssuer string

	// RequiredAnnotations are key-value pairs that must be present in the signature.
	// Useful for enforcing build metadata requirements.
	// All specified annotations must match exactly.
	RequiredAnnotations map[string]string

	// RekorEnabled controls whether Rekor transparency log verification is required.
	// When true, signatures must be present in the Rekor log.
	RekorEnabled bool

	// RekorURL is the URL of the Rekor transparency log server.
	// Defaults to the public Sigstore Rekor instance if empty.
	RekorURL string

	// CacheTTL is the time-to-live for cached verification results.
	// Defaults to 1 hour for keyless, 24 hours for public key mode.
	CacheTTL time.Duration
}

// NewPolicy creates a new Policy with default settings.
// Defaults to:
//   - VerificationMode: Required (fail on invalid, allow missing)
//   - MultiSignatureMode: Any (first valid signature passes)
//   - MinimumSignatures: 1
//   - RekorEnabled: false (offline verification)
//   - CacheTTL: 1 hour
func NewPolicy() *Policy {
	return &Policy{
		VerificationMode:   VerificationModeRequired,
		MultiSignatureMode: MultiSignatureModeAny,
		MinimumSignatures:  1,
		RekorEnabled:       false,
		CacheTTL:           time.Hour,
		RequiredAnnotations: make(map[string]string),
	}
}

// Validate checks if the policy configuration is valid.
// Returns an error if the policy has invalid or conflicting settings.
func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("policy cannot be nil")
	}

	// Validate minimum signatures
	if p.MultiSignatureMode == MultiSignatureModeMinimum {
		if p.MinimumSignatures < 1 {
			return fmt.Errorf("minimum signatures must be at least 1, got %d", p.MinimumSignatures)
		}
	}

	// Validate identity patterns
	for _, pattern := range p.AllowedIdentities {
		if pattern == "" {
			return fmt.Errorf("identity pattern cannot be empty")
		}
		// Validate pattern is a valid glob or regex
		if _, err := regexp.Compile(pattern); err != nil {
			// If not a valid regex, check if it's a simple glob pattern
			// Allow patterns like "*@example.com" (these will be handled specially)
			if !isValidGlobPattern(pattern) {
				return fmt.Errorf("invalid identity pattern %q: %w", pattern, err)
			}
		}
	}

	// Validate that either public keys or keyless config is present
	hasPublicKeys := len(p.PublicKeys) > 0
	hasKeylessConfig := len(p.AllowedIdentities) > 0 || p.RequiredIssuer != "" || p.RekorEnabled

	if !hasPublicKeys && !hasKeylessConfig {
		return fmt.Errorf("policy must specify either public keys or keyless configuration (identities, issuer, or rekor)")
	}

	// Public keys and keyless are mutually exclusive
	if hasPublicKeys && hasKeylessConfig {
		return fmt.Errorf("policy cannot specify both public keys and keyless configuration")
	}

	return nil
}

// IsKeylessMode returns true if the policy is configured for keyless verification.
func (p *Policy) IsKeylessMode() bool {
	return len(p.PublicKeys) == 0
}

// MatchesIdentity checks if a given identity matches any allowed pattern.
// Uses proper glob pattern matching with security validations.
func (p *Policy) MatchesIdentity(identity string) bool {
	// Validate identity length to prevent DoS attacks
	if len(identity) > maxIdentityLength {
		return false
	}

	// Security: Reject identities containing null bytes or control characters
	// This prevents injection attacks
	for _, c := range identity {
		if c == 0 || (c < 32 && c != '\t' && c != '\n' && c != '\r') {
			return false
		}
	}

	if len(p.AllowedIdentities) == 0 {
		// No restrictions - accept any identity
		return true
	}

	for _, pattern := range p.AllowedIdentities {
		if matchesGlobPattern(pattern, identity) {
			return true
		}
	}

	return false
}

// isValidGlobPattern checks if a string is a valid glob pattern.
// Validates pattern length and attempts to compile with glob library.
func isValidGlobPattern(pattern string) bool {
	// Validate pattern length to prevent DoS attacks
	if len(pattern) > maxPatternLength {
		return false
	}

	// Security: Reject patterns containing null bytes or control characters
	for _, c := range pattern {
		if c == 0 || (c < 32 && c != '\t' && c != '\n' && c != '\r') {
			return false
		}
	}

	// Attempt to compile the pattern to verify syntax
	_, err := glob.Compile(pattern)
	return err == nil
}

// matchesGlobPattern checks if a string matches a glob pattern using the glob library.
// Provides proper glob matching with support for *, ?, [abc], {a,b,c} patterns.
func matchesGlobPattern(pattern, str string) bool {
	// Validate pattern length before compiling
	if len(pattern) > maxPatternLength {
		return false
	}

	// Compile the glob pattern
	// We compile on each match for safety (patterns come from policy configuration)
	// In a high-performance scenario, you could cache compiled patterns
	g, err := glob.Compile(pattern)
	if err != nil {
		// Invalid pattern - reject
		return false
	}

	// Match the identity against the pattern
	return g.Match(str)
}
