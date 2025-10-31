package signature

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
	orasint "github.com/jmgilman/go/oci/internal/oras"
)

// TestNewPublicKeyVerifier tests creating a public key verifier
func TestNewPublicKeyVerifier(t *testing.T) {
	// Generate a test RSA key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey

	verifier := NewPublicKeyVerifier(pubKey)
	if verifier == nil {
		t.Fatal("NewPublicKeyVerifier returned nil")
	}

	policy := verifier.Policy()
	if len(policy.PublicKeys) != 1 {
		t.Errorf("expected 1 public key, got %d", len(policy.PublicKeys))
	}

	if policy.IsKeylessMode() {
		t.Error("verifier should not be in keyless mode")
	}
}

// TestNewKeylessVerifier tests creating a keyless verifier
func TestNewKeylessVerifier(t *testing.T) {
	verifier := NewKeylessVerifier(
		WithAllowedIdentities("*@example.com"),
		WithRekor(true),
	)

	if verifier == nil {
		t.Fatal("NewKeylessVerifier returned nil")
	}

	policy := verifier.Policy()
	if !policy.IsKeylessMode() {
		t.Error("verifier should be in keyless mode")
	}

	if !policy.RekorEnabled {
		t.Error("Rekor should be enabled")
	}

	if len(policy.AllowedIdentities) != 1 {
		t.Errorf("expected 1 allowed identity, got %d", len(policy.AllowedIdentities))
	}

	if policy.AllowedIdentities[0] != "*@example.com" {
		t.Errorf("expected identity *@example.com, got %s", policy.AllowedIdentities[0])
	}
}

// TestPolicyValidation tests policy validation
func TestPolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		policy  *Policy
		wantErr bool
	}{
		{
			name:    "nil policy",
			policy:  nil,
			wantErr: true,
		},
		{
			name: "valid public key policy",
			policy: &Policy{
				PublicKeys: []crypto.PublicKey{&rsa.PublicKey{}},
			},
			wantErr: false,
		},
		{
			name: "valid keyless policy",
			policy: &Policy{
				AllowedIdentities: []string{"*@example.com"},
			},
			wantErr: false,
		},
		{
			name: "invalid minimum signatures",
			policy: &Policy{
				AllowedIdentities:  []string{"*@example.com"},
				MultiSignatureMode: MultiSignatureModeMinimum,
				MinimumSignatures:  0,
			},
			wantErr: true,
		},
		{
			name: "empty identity pattern",
			policy: &Policy{
				AllowedIdentities: []string{""},
			},
			wantErr: true,
		},
		{
			name: "both public keys and keyless config",
			policy: &Policy{
				PublicKeys:        []crypto.PublicKey{&rsa.PublicKey{}},
				AllowedIdentities: []string{"*@example.com"},
			},
			wantErr: true,
		},
		{
			name:    "neither public keys nor keyless config",
			policy:  &Policy{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPolicyIdentityMatching tests identity pattern matching
func TestPolicyIdentityMatching(t *testing.T) {
	tests := []struct {
		name      string
		patterns  []string
		identity  string
		wantMatch bool
	}{
		{
			name:      "exact match",
			patterns:  []string{"alice@example.com"},
			identity:  "alice@example.com",
			wantMatch: true,
		},
		{
			name:      "wildcard domain",
			patterns:  []string{"*@example.com"},
			identity:  "alice@example.com",
			wantMatch: true,
		},
		{
			name:      "wildcard domain no match",
			patterns:  []string{"*@example.com"},
			identity:  "alice@other.com",
			wantMatch: false,
		},
		{
			name:      "prefix wildcard",
			patterns:  []string{"alice*"},
			identity:  "alice@example.com",
			wantMatch: true,
		},
		{
			name:      "no patterns (accept all)",
			patterns:  []string{},
			identity:  "anyone@anywhere.com",
			wantMatch: true,
		},
		{
			name:      "multiple patterns - first matches",
			patterns:  []string{"*@example.com", "*@other.com"},
			identity:  "alice@example.com",
			wantMatch: true,
		},
		{
			name:      "multiple patterns - second matches",
			patterns:  []string{"*@example.com", "*@other.com"},
			identity:  "bob@other.com",
			wantMatch: true,
		},
		{
			name:      "multiple patterns - no match",
			patterns:  []string{"*@example.com", "*@other.com"},
			identity:  "charlie@third.com",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &Policy{
				AllowedIdentities: tt.patterns,
			}
			got := policy.MatchesIdentity(tt.identity)
			if got != tt.wantMatch {
				t.Errorf("MatchesIdentity(%q) = %v, want %v", tt.identity, got, tt.wantMatch)
			}
		})
	}
}

// TestLoadPublicKeyFromBytes tests loading public keys from byte arrays
func TestLoadPublicKeyFromBytes(t *testing.T) {
	// Generate test keys
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	tests := []struct {
		name    string
		keyData []byte
		wantErr bool
	}{
		{
			name:    "RSA public key PEM",
			keyData: encodePublicKeyPEM(&rsaKey.PublicKey),
			wantErr: false,
		},
		{
			name:    "ECDSA public key PEM",
			keyData: encodePublicKeyPEM(&ecKey.PublicKey),
			wantErr: false,
		},
		{
			name:    "empty data",
			keyData: []byte{},
			wantErr: true,
		},
		{
			name:    "invalid PEM",
			keyData: []byte("-----BEGIN PUBLIC KEY-----\ninvalid\n-----END PUBLIC KEY-----"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := LoadPublicKeyFromBytes(tt.keyData)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPublicKeyFromBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && key == nil {
				t.Error("LoadPublicKeyFromBytes() returned nil key")
			}
		})
	}
}

// TestLoadPublicKeyFromFile tests loading public keys from files
func TestLoadPublicKeyFromFile(t *testing.T) {
	// Generate test key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Create temp directory
	tempDir := t.TempDir()

	// Write key to file
	keyPath := filepath.Join(tempDir, "test.pub")
	keyData := encodePublicKeyPEM(&rsaKey.PublicKey)
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// Test loading
	key, err := LoadPublicKey(keyPath)
	if err != nil {
		t.Errorf("LoadPublicKey() error = %v", err)
	}
	if key == nil {
		t.Error("LoadPublicKey() returned nil key")
	}

	// Test loading non-existent file
	_, err = LoadPublicKey(filepath.Join(tempDir, "nonexistent.pub"))
	if err == nil {
		t.Error("LoadPublicKey() should fail for non-existent file")
	}
}

// Note: TestBuildSignatureReference has been removed because signature reference
// construction is now handled internally by Cosign's VerifyImageSignatures API.
// This was an implementation detail that is no longer exposed.

// TestVerifyInterface tests that CosignVerifier implements the SignatureVerifier interface
func TestVerifyInterface(t *testing.T) {
	// Generate test key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Test with Optional mode so missing signatures don't cause errors
	verifier := NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{&rsaKey.PublicKey},
		WithOptionalMode(true),
	)

	// Verify it implements the interface
	var _ ocibundle.SignatureVerifier = verifier

	// Test Verify method with Optional mode (should not error on missing signature)
	ctx := context.Background()
	descriptor := &orasint.PullDescriptor{
		Digest:    "sha256:abc123",
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Size:      1024,
		Data:      io.NopCloser(strings.NewReader("")),
	}

	err = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0.0", descriptor)
	// Optional mode should allow missing signatures (return nil)
	if err != nil {
		// If we get a network error, that's also acceptable - just skip the test
		if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "not found") {
			t.Errorf("Unexpected error in optional mode: %v", err)
		}
	}

	// Test with Enforce mode (should error on missing signature)
	enforcingVerifier := NewPublicKeyVerifierWithOptions(
		[]crypto.PublicKey{&rsaKey.PublicKey},
		WithEnforceMode(true),
	)

	err = enforcingVerifier.Verify(ctx, "ghcr.io/org/repo:v1.0.0", descriptor)
	if err == nil {
		t.Error("Verify() in enforce mode should return error for missing signature")
	}
	// Should be a signature-related error
	if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "denied") {
		t.Errorf("Expected signature error, got: %v", err)
	}
}

// TestVerifierOptions tests that options are properly applied
func TestVerifierOptions(t *testing.T) {
	t.Run("WithRequireAll", func(t *testing.T) {
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithRequireAll(true),
		)
		policy := verifier.Policy()
		if policy.MultiSignatureMode != MultiSignatureModeAll {
			t.Errorf("expected MultiSignatureModeAll, got %v", policy.MultiSignatureMode)
		}
	})

	t.Run("WithMinimumSignatures", func(t *testing.T) {
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithMinimumSignatures(3),
		)
		policy := verifier.Policy()
		if policy.MultiSignatureMode != MultiSignatureModeMinimum {
			t.Errorf("expected MultiSignatureModeMinimum, got %v", policy.MultiSignatureMode)
		}
		if policy.MinimumSignatures != 3 {
			t.Errorf("expected MinimumSignatures=3, got %d", policy.MinimumSignatures)
		}
	})

	t.Run("WithOptionalMode", func(t *testing.T) {
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithOptionalMode(true),
		)
		policy := verifier.Policy()
		if policy.VerificationMode != VerificationModeOptional {
			t.Errorf("expected VerificationModeOptional, got %v", policy.VerificationMode)
		}
	})

	t.Run("WithEnforceMode", func(t *testing.T) {
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithEnforceMode(true),
		)
		policy := verifier.Policy()
		if policy.VerificationMode != VerificationModeEnforce {
			t.Errorf("expected VerificationModeEnforce, got %v", policy.VerificationMode)
		}
	})

	t.Run("WithRekor", func(t *testing.T) {
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithRekor(true),
		)
		policy := verifier.Policy()
		if !policy.RekorEnabled {
			t.Error("expected Rekor to be enabled")
		}
	})

	t.Run("WithRekorURL", func(t *testing.T) {
		url := "https://rekor.example.com"
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithRekorURL(url),
		)
		policy := verifier.Policy()
		if policy.RekorURL != url {
			t.Errorf("expected RekorURL=%s, got %s", url, policy.RekorURL)
		}
		if !policy.RekorEnabled {
			t.Error("expected Rekor to be enabled when URL is set")
		}
	})

	t.Run("WithRequiredAnnotations", func(t *testing.T) {
		annotations := map[string]string{
			"build": "ci",
			"test":  "passed",
		}
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithRequiredAnnotations(annotations),
		)
		policy := verifier.Policy()
		if len(policy.RequiredAnnotations) != 2 {
			t.Errorf("expected 2 annotations, got %d", len(policy.RequiredAnnotations))
		}
		if policy.RequiredAnnotations["build"] != "ci" {
			t.Error("annotation 'build' not set correctly")
		}
	})

	t.Run("WithCacheTTL", func(t *testing.T) {
		ttl := 30 * time.Minute
		verifier := NewKeylessVerifier(
			WithAllowedIdentities("*@example.com"),
			WithCacheTTL(ttl),
		)
		policy := verifier.Policy()
		if policy.CacheTTL != ttl {
			t.Errorf("expected CacheTTL=%v, got %v", ttl, policy.CacheTTL)
		}
	})
}

// Helper function to encode a public key as PEM
func encodePublicKeyPEM(pubKey crypto.PublicKey) []byte {
	derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		panic(err)
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}
	return pem.EncodeToMemory(block)
}
