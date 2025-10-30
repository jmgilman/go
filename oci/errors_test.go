package ocibundle

import (
	"errors"
	"testing"
)

// TestSentinelErrors verifies all sentinel errors are properly defined
func TestSentinelErrors(t *testing.T) {
	// Test that ErrAuthenticationFailed is defined
	if ErrAuthenticationFailed == nil {
		t.Error("ErrAuthenticationFailed should not be nil")
	}
	if ErrAuthenticationFailed.Error() != "authentication failed" {
		t.Errorf(
			"ErrAuthenticationFailed message = %q, want %q",
			ErrAuthenticationFailed.Error(),
			"authentication failed",
		)
	}

	// Test that ErrRegistryUnreachable is defined
	if ErrRegistryUnreachable == nil {
		t.Error("ErrRegistryUnreachable should not be nil")
	}
	if ErrRegistryUnreachable.Error() != "registry unreachable" {
		t.Errorf("ErrRegistryUnreachable message = %q, want %q", ErrRegistryUnreachable.Error(), "registry unreachable")
	}

	// Test that ErrInvalidReference is defined
	if ErrInvalidReference == nil {
		t.Error("ErrInvalidReference should not be nil")
	}
	if ErrInvalidReference.Error() != "invalid OCI reference" {
		t.Errorf("ErrInvalidReference message = %q, want %q", ErrInvalidReference.Error(), "invalid OCI reference")
	}

	// Test that ErrSecurityViolation is defined
	if ErrSecurityViolation == nil {
		t.Error("ErrSecurityViolation should not be nil")
	}
	if ErrSecurityViolation.Error() != "security constraint violated" {
		t.Errorf(
			"ErrSecurityViolation message = %q, want %q",
			ErrSecurityViolation.Error(),
			"security constraint violated",
		)
	}

	// Test that ErrArchiveCorrupted is defined
	if ErrArchiveCorrupted == nil {
		t.Error("ErrArchiveCorrupted should not be nil")
	}
	if ErrArchiveCorrupted.Error() != "archive corrupted or invalid" {
		t.Errorf(
			"ErrArchiveCorrupted message = %q, want %q",
			ErrArchiveCorrupted.Error(),
			"archive corrupted or invalid",
		)
	}
}

// TestBundleErrorStruct verifies BundleError struct exists and implements error interface
func TestBundleErrorStruct(t *testing.T) {
	// Test that BundleError can be created
	err := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       errors.New("underlying error"),
	}

	// Test that it implements error interface
	var _ error = err

	if err.Op != "push" {
		t.Errorf("BundleError.Op = %q, want %q", err.Op, "push")
	}
	if err.Reference != "ghcr.io/org/repo:v1.0.0" {
		t.Errorf("BundleError.Reference = %q, want %q", err.Reference, "ghcr.io/org/repo:v1.0.0")
	}
	if err.Err.Error() != "underlying error" {
		t.Errorf("BundleError.Err = %q, want %q", err.Err.Error(), "underlying error")
	}
}

// TestBundleErrorErrorMethod verifies Error() method
func TestBundleErrorErrorMethod(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := &BundleError{
		Op:        "pull",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       underlyingErr,
	}

	if err.Error() != "underlying error" {
		t.Errorf("BundleError.Error() = %q, want %q", err.Error(), "underlying error")
	}
}

// TestBundleErrorUnwrap verifies Unwrap() method for error wrapping
func TestBundleErrorUnwrap(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       underlyingErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlyingErr { //nolint:errorlint // Direct comparison needed for testing Unwrap method
		t.Errorf("BundleError.Unwrap() = %v, want %v", unwrapped, underlyingErr)
	}
}

// TestBundleErrorWrapping verifies errors.Is() and errors.As() work with BundleError
func TestBundleErrorWrapping(t *testing.T) {
	// Test with direct sentinel error
	bundleErr := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrAuthenticationFailed,
	}

	// Test errors.Is() works
	if !errors.Is(bundleErr, ErrAuthenticationFailed) {
		t.Error("errors.Is() should return true for wrapped ErrAuthenticationFailed")
	}

	// Test with wrapped error chain
	wrappedErr := errors.New("connection failed")
	authErr := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       wrappedErr,
	}

	registryErr := &BundleError{
		Op:        "pull",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       authErr,
	}

	// Test errors.Is() works through error chain
	if !errors.Is(registryErr, wrappedErr) {
		t.Error("errors.Is() should return true for deeply wrapped error")
	}

	// Test errors.As() works
	var target *BundleError
	if !errors.As(registryErr, &target) {
		t.Error("errors.As() should successfully extract BundleError")
	}
	if target.Op != "pull" {
		t.Errorf("Extracted BundleError.Op = %q, want %q", target.Op, "pull")
	}
}

// TestNewBundleError verifies the NewBundleError constructor
func TestNewBundleError(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := NewBundleError("push", "ghcr.io/org/repo:v1.0.0", underlyingErr)

	if err.Op != "push" {
		t.Errorf("NewBundleError.Op = %q, want %q", err.Op, "push")
	}
	if err.Reference != "ghcr.io/org/repo:v1.0.0" {
		t.Errorf("NewBundleError.Reference = %q, want %q", err.Reference, "ghcr.io/org/repo:v1.0.0")
	}
	if err.Err != underlyingErr { //nolint:errorlint // Direct comparison needed for testing error field
		t.Errorf("NewBundleError.Err = %v, want %v", err.Err, underlyingErr)
	}
}

// TestBundleErrorFormatError verifies the FormatError method
func TestBundleErrorFormatError(t *testing.T) {
	underlyingErr := errors.New("authentication failed")
	err := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       underlyingErr,
	}

	expected := "push ghcr.io/org/repo:v1.0.0: authentication failed"
	if err.FormatError() != expected {
		t.Errorf("FormatError() = %q, want %q", err.FormatError(), expected)
	}
}

// TestBundleErrorIsSecurityError verifies the IsSecurityError method
func TestBundleErrorIsSecurityError(t *testing.T) {
	// Test with security error
	securityErr := &BundleError{
		Op:        "extract",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrSecurityViolation,
	}

	if !securityErr.IsSecurityError() {
		t.Error("IsSecurityError() should return true for security violation")
	}

	// Test with wrapped security error
	wrappedSecurityErr := &BundleError{
		Op:        "extract",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       errors.New("path traversal detected"),
	}
	outerErr := &BundleError{
		Op:        "pull",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       wrappedSecurityErr,
	}

	if outerErr.IsSecurityError() {
		t.Error("IsSecurityError() should return false for non-security error")
	}

	// Test with direct security error in chain
	directSecurityErr := &BundleError{
		Op:        "extract",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrSecurityViolation,
	}

	if !directSecurityErr.IsSecurityError() {
		t.Error("IsSecurityError() should return true for direct security error")
	}
}

// TestBundleErrorIsAuthError verifies the IsAuthError method
func TestBundleErrorIsAuthError(t *testing.T) {
	// Test with auth error
	authErr := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrAuthenticationFailed,
	}

	if !authErr.IsAuthError() {
		t.Error("IsAuthError() should return true for authentication failure")
	}

	// Test with non-auth error
	registryErr := &BundleError{
		Op:        "pull",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrRegistryUnreachable,
	}

	if registryErr.IsAuthError() {
		t.Error("IsAuthError() should return false for non-auth error")
	}

	// Test with wrapped auth error
	wrappedAuthErr := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       errors.New("token expired"),
	}
	outerErr := &BundleError{
		Op:        "pull",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       wrappedAuthErr,
	}

	if outerErr.IsAuthError() {
		t.Error("IsAuthError() should return false for wrapped non-auth error")
	}

	// Test with direct auth error in chain
	directAuthErr := &BundleError{
		Op:        "push",
		Reference: "ghcr.io/org/repo:v1.0.0",
		Err:       ErrAuthenticationFailed,
	}

	if !directAuthErr.IsAuthError() {
		t.Error("IsAuthError() should return true for direct auth error")
	}
}
