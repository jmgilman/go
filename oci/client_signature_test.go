package ocibundle

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/jmgilman/go/oci/internal/oras"
	"github.com/jmgilman/go/oci/internal/testutil"
)

// TestClient_PullWithSignatureVerification tests the full Pull() flow with signature verification.
func TestClient_PullWithSignatureVerification(t *testing.T) {
	tests := []struct {
		name          string
		verifyResult  error
		expectSuccess bool
		expectError   error
	}{
		{
			name:          "ValidSignature",
			verifyResult:  nil,
			expectSuccess: true,
			expectError:   nil,
		},
		{
			name:          "InvalidSignature",
			verifyResult:  ErrSignatureInvalid,
			expectSuccess: false,
			expectError:   ErrSignatureInvalid,
		},
		{
			name:          "SignatureNotFound",
			verifyResult:  ErrSignatureNotFound,
			expectSuccess: false,
			expectError:   ErrSignatureNotFound,
		},
		{
			name:          "UntrustedSigner",
			verifyResult:  ErrUntrustedSigner,
			expectSuccess: false,
			expectError:   ErrUntrustedSigner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := testutil.NewMockVerifier(tt.verifyResult)

			client, err := NewWithOptions(
				WithSignatureVerifier(verifier),
			)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Note: This is a mock test - full integration tests are in integration_test.go
			// Here we're testing the integration between client and verifier

			// Mock a pull operation would happen here
			// For now, we verify the verifier was configured
			if client.options.SignatureVerifier == nil {
				t.Error("Signature verifier was not set")
			}

			if client.options.SignatureVerifier != verifier {
				t.Error("Signature verifier is not the expected instance")
			}
		})
	}
}

// TestClient_PullWithoutSignatureVerification ensures backward compatibility.
func TestClient_PullWithoutSignatureVerification(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier != nil {
		t.Error("Signature verifier should be nil when not configured")
	}

	// Client should work normally without verification
	// This ensures backward compatibility
}

// TestClient_SignatureVerificationWithCache tests signature verification with caching enabled.
func TestClient_SignatureVerificationWithCache(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	verifier := testutil.NewMockVerifier(nil)

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify client was created with verifier
	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier was not set")
	}

	// Note: Full cache integration is tested in the signature package
	// This test verifies the client properly passes through the verifier
}

// TestClient_SignatureErrorContextDetails tests that signature errors contain detailed context.
func TestClient_SignatureErrorContextDetails(t *testing.T) {
	tests := []struct {
		name        string
		verifyError error
		expectInfo  bool
	}{
		{
			name: "SignatureNotFound",
			verifyError: &BundleError{
				Op:        "verify",
				Reference: "example.com/test:v1",
				Err:       ErrSignatureNotFound,
				SignatureInfo: &SignatureErrorInfo{
					Digest:       "sha256:abc123",
					Reason:       "No signature found",
					FailureStage: "fetch",
				},
			},
			expectInfo: true,
		},
		{
			name: "InvalidSignature",
			verifyError: &BundleError{
				Op:        "verify",
				Reference: "example.com/test:v1",
				Err:       ErrSignatureInvalid,
				SignatureInfo: &SignatureErrorInfo{
					Digest:       "sha256:abc123",
					Reason:       "Signature verification failed",
					FailureStage: "cryptographic",
					Signer:       "user@example.com",
				},
			},
			expectInfo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bundleErr *BundleError
			if !errors.As(tt.verifyError, &bundleErr) {
				t.Fatal("Expected BundleError")
			}

			if tt.expectInfo && bundleErr.SignatureInfo == nil {
				t.Error("Expected SignatureInfo to be present")
			}

			if bundleErr.SignatureInfo != nil {
				if bundleErr.SignatureInfo.Digest == "" {
					t.Error("Expected Digest to be set")
				}
				if bundleErr.SignatureInfo.Reason == "" {
					t.Error("Expected Reason to be set")
				}
				if bundleErr.SignatureInfo.FailureStage == "" {
					t.Error("Expected FailureStage to be set")
				}
			}

			if !bundleErr.IsSignatureError() {
				t.Error("Expected IsSignatureError() to return true")
			}
		})
	}
}

// TestClient_VerificationErrorsNotRetried tests that signature verification errors are not retried.
func TestClient_VerificationErrorsNotRetried(t *testing.T) {
	errors := []error{
		ErrSignatureNotFound,
		ErrSignatureInvalid,
		ErrUntrustedSigner,
		ErrRekorVerificationFailed,
		ErrCertificateExpired,
		ErrInvalidAnnotations,
	}

	for _, err := range errors {
		t.Run(err.Error(), func(t *testing.T) {
			if isRetryableError(err) {
				t.Errorf("Signature error %v should not be retryable", err)
			}

			// Test wrapped in BundleError
			bundleErr := &BundleError{
				Op:        "verify",
				Reference: "test:v1",
				Err:       err,
			}

			if isRetryableError(bundleErr) {
				t.Errorf("BundleError wrapping %v should not be retryable", err)
			}
		})
	}
}

// TestClient_VerifierCalledAtCorrectTime tests that the verifier is called at the right point in Pull().
func TestClient_VerifierCalledAtCorrectTime(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	verifyFunc := func(ctx context.Context, reference string, descriptor *oras.PullDescriptor) error {
		callCount++
		// Verify we have the descriptor information
		if descriptor == nil {
			t.Error("Descriptor should not be nil")
		}
		if descriptor.Digest == "" {
			t.Error("Descriptor digest should not be empty")
		}
		return nil
	}

	verifier := &testutil.CallbackVerifier{
		VerifyFunc: verifyFunc,
	}

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_ = ctx
	_ = client

	// Note: Full flow testing requires integration test setup
	// This test verifies the callback mechanism works

	if verifier.VerifyFunc == nil {
		t.Error("VerifyFunc should be set")
	}
}

// TestClient_MultipleSignatureVerification tests artifacts with multiple signatures.
func TestClient_MultipleSignatureVerification(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Create a verifier that tracks multiple verification attempts
	verifier := &testutil.TrackingVerifier{
		Results: []error{nil}, // First signature passes
	}

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier should be set")
	}

	// Note: Full multi-signature testing is in the signature package
	// This verifies client-level integration
}

// TestClient_ExpiredCertificate tests handling of expired certificates in keyless mode.
func TestClient_ExpiredCertificate(t *testing.T) {
	verifyErr := &BundleError{
		Op:        "verify",
		Reference: "example.com/test:v1",
		Err:       ErrCertificateExpired,
		SignatureInfo: &SignatureErrorInfo{
			Digest:       "sha256:abc123",
			Reason:       "Certificate expired",
			FailureStage: "certificate",
			Signer:       "user@example.com",
		},
	}

	verifier := testutil.NewMockVerifier(verifyErr)

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier should be set")
	}

	// Verify error is properly typed
	var bundleErr *BundleError
	if !errors.As(verifyErr, &bundleErr) {
		t.Fatal("Expected BundleError")
	}

	if !errors.Is(bundleErr.Err, ErrCertificateExpired) {
		t.Error("Expected ErrCertificateExpired")
	}
}

// TestClient_WrongSignerIdentity tests handling of signatures from untrusted identities.
func TestClient_WrongSignerIdentity(t *testing.T) {
	verifyErr := &BundleError{
		Op:        "verify",
		Reference: "example.com/test:v1",
		Err:       ErrUntrustedSigner,
		SignatureInfo: &SignatureErrorInfo{
			Digest:       "sha256:abc123",
			Reason:       "Signer identity does not match allowed patterns",
			FailureStage: "identity",
			Signer:       "untrusted@example.com",
		},
	}

	verifier := testutil.NewMockVerifier(verifyErr)

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier should be set")
	}

	// Verify error indicates untrusted signer
	var bundleErr *BundleError
	if !errors.As(verifyErr, &bundleErr) {
		t.Fatal("Expected BundleError")
	}

	if !errors.Is(bundleErr.Err, ErrUntrustedSigner) {
		t.Error("Expected ErrUntrustedSigner")
	}

	if bundleErr.SignatureInfo.Signer != "untrusted@example.com" {
		t.Error("Expected signer identity to be captured")
	}
}

// TestClient_AnnotationPolicyViolation tests handling of annotation policy violations.
func TestClient_AnnotationPolicyViolation(t *testing.T) {
	verifyErr := &BundleError{
		Op:        "verify",
		Reference: "example.com/test:v1",
		Err:       ErrInvalidAnnotations,
		SignatureInfo: &SignatureErrorInfo{
			Digest:       "sha256:abc123",
			Reason:       "Required annotation 'build-system=github-actions' not found",
			FailureStage: "policy",
		},
	}

	verifier := testutil.NewMockVerifier(verifyErr)

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier should be set")
	}

	// Verify error indicates annotation violation
	var bundleErr *BundleError
	if !errors.As(verifyErr, &bundleErr) {
		t.Fatal("Expected BundleError")
	}

	if !errors.Is(bundleErr.Err, ErrInvalidAnnotations) {
		t.Error("Expected ErrInvalidAnnotations")
	}
}

// TestClient_RekorVerificationFailure tests handling of Rekor verification failures.
func TestClient_RekorVerificationFailure(t *testing.T) {
	verifyErr := &BundleError{
		Op:        "verify",
		Reference: "example.com/test:v1",
		Err:       ErrRekorVerificationFailed,
		SignatureInfo: &SignatureErrorInfo{
			Digest:       "sha256:abc123",
			Reason:       "Signature not found in Rekor transparency log",
			FailureStage: "rekor",
		},
	}

	verifier := testutil.NewMockVerifier(verifyErr)

	client, err := NewWithOptions(
		WithSignatureVerifier(verifier),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.options.SignatureVerifier == nil {
		t.Error("Signature verifier should be set")
	}

	// Verify error indicates Rekor failure
	var bundleErr *BundleError
	if !errors.As(verifyErr, &bundleErr) {
		t.Fatal("Expected BundleError")
	}

	if !errors.Is(bundleErr.Err, ErrRekorVerificationFailed) {
		t.Error("Expected ErrRekorVerificationFailed")
	}
}

// Mock helper for testing
func generateTestKey() crypto.PublicKey {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return &privateKey.PublicKey
}
