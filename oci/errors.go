// Package ocibundle provides OCI bundle distribution functionality.
// This file contains domain-specific error types for OCI bundle operations.
package ocibundle

import (
	"errors"
	"fmt"
)

// Sentinel errors for different failure modes.
// These are used to identify specific types of failures in OCI bundle operations.
// They can be checked using errors.Is() for error handling and testing.
var (
	// ErrAuthenticationFailed indicates that authentication with the OCI registry failed.
	// This could be due to invalid credentials, expired tokens, or authentication service issues.
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrRegistryUnreachable indicates that the OCI registry could not be contacted.
	// This could be due to network issues, registry being down, or DNS resolution failures.
	ErrRegistryUnreachable = errors.New("registry unreachable")

	// ErrInvalidReference indicates that the provided OCI reference is malformed or invalid.
	// This includes invalid registry names, malformed tags, or unsupported reference formats.
	ErrInvalidReference = errors.New("invalid OCI reference")

	// ErrSecurityViolation indicates that a security constraint was violated during operation.
	// This includes path traversal attempts, file size limits exceeded, or other security checks.
	ErrSecurityViolation = errors.New("security constraint violated")

	// ErrArchiveCorrupted indicates that the archive file is corrupted or invalid.
	// This could be due to transmission errors, storage corruption, or unsupported archive formats.
	ErrArchiveCorrupted = errors.New("archive corrupted or invalid")

	// ErrConfigNotFound indicates that the Docker config file could not be found.
	// This occurs when the config file path does not exist or is not readable.
	ErrConfigNotFound = errors.New("docker config file not found")

	// ErrConfigInvalid indicates that the Docker config file is malformed or invalid.
	// This occurs when the config file contains invalid JSON or missing required fields.
	ErrConfigInvalid = errors.New("docker config file is invalid")

	// ErrSignatureNotFound indicates that no signature was found for the OCI artifact.
	// This occurs when signature verification is enabled but the signature artifact
	// does not exist in the registry.
	ErrSignatureNotFound = errors.New("signature not found")

	// ErrSignatureInvalid indicates that signature verification failed.
	// This occurs when a signature exists but the cryptographic verification
	// failed, indicating the artifact may have been tampered with.
	ErrSignatureInvalid = errors.New("signature verification failed")

	// ErrUntrustedSigner indicates that the signature is valid but the signer
	// is not in the allowed list. This occurs when identity-based policies
	// reject the signer's identity or certificate.
	ErrUntrustedSigner = errors.New("untrusted signer")

	// ErrRekorVerificationFailed indicates that transparency log verification failed.
	// This occurs when the signature cannot be verified against the Rekor
	// transparency log, which may indicate the signature was not properly logged.
	ErrRekorVerificationFailed = errors.New("rekor verification failed")

	// ErrCertificateExpired indicates that the certificate used for signing has expired.
	// This occurs when using certificate-based verification with expired certificates.
	ErrCertificateExpired = errors.New("certificate expired")

	// ErrInvalidAnnotations indicates that required annotations are missing or incorrect.
	// This occurs when annotation-based policies are not satisfied by the signature.
	ErrInvalidAnnotations = errors.New("required annotations missing or invalid")
)

// BundleError provides detailed context about OCI bundle operation failures.
// It wraps underlying errors with additional context specific to OCI bundle operations,
// including the operation type and OCI reference being processed.
//
// BundleError implements the error interface and supports error wrapping,
// allowing it to be used with errors.Is() and errors.As() for proper error handling.
type BundleError struct {
	// Op describes the operation that failed (e.g., "push", "pull", "extract", "verify").
	Op string

	// Reference is the OCI reference being processed when the error occurred.
	// This could be a full reference like "ghcr.io/org/repo:v1.0.0".
	Reference string

	// Err is the underlying error that caused this BundleError to be created.
	// This preserves the original error context and allows for proper error wrapping.
	Err error

	// SignatureInfo provides detailed information about signature verification failures.
	// This field is populated when the error is related to signature verification.
	// It will be nil for non-signature errors.
	SignatureInfo *SignatureErrorInfo
}

// Error implements the error interface.
// It returns the error message from the underlying error to maintain compatibility
// with existing error handling code that expects the underlying error message.
func (e *BundleError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error to support error wrapping.
// This allows BundleError to be used with errors.Is() and errors.As()
// for checking error types and extracting wrapped errors.
func (e *BundleError) Unwrap() error {
	return e.Err
}

// NewBundleError creates a new BundleError with the specified context.
// This is a convenience function for creating BundleError instances.
//
// Parameters:
//   - op: The operation that failed (e.g., "push", "pull")
//   - ref: The OCI reference being processed
//   - err: The underlying error
//
// Returns a pointer to the new BundleError.
func NewBundleError(op, ref string, err error) *BundleError {
	return &BundleError{
		Op:        op,
		Reference: ref,
		Err:       err,
	}
}

// FormatError creates a formatted error message with BundleError context.
// This is useful for logging or displaying errors with full context.
//
// The format includes the operation, reference, and underlying error message.
// Example output: "push ghcr.io/org/repo:v1.0.0: authentication failed"
func (e *BundleError) FormatError() string {
	return fmt.Sprintf("%s %s: %s", e.Op, e.Reference, e.Err.Error())
}

// IsSecurityError checks if this error or any wrapped error is a security violation.
// This is a convenience method for quickly identifying security-related errors.
//
// Returns true if ErrSecurityViolation is found in the error chain.
func (e *BundleError) IsSecurityError() bool {
	return errors.Is(e.Err, ErrSecurityViolation)
}

// IsAuthError checks if this error or any wrapped error is an authentication failure.
// This is a convenience method for quickly identifying authentication-related errors.
//
// Returns true if ErrAuthenticationFailed is found in the error chain.
func (e *BundleError) IsAuthError() bool {
	return errors.Is(e.Err, ErrAuthenticationFailed)
}

// IsSignatureError checks if this error or any wrapped error is a signature verification failure.
// This is a convenience method for quickly identifying signature-related errors.
//
// Returns true if any signature-related error is found in the error chain:
//   - ErrSignatureNotFound
//   - ErrSignatureInvalid
//   - ErrUntrustedSigner
//   - ErrRekorVerificationFailed
//   - ErrCertificateExpired
//   - ErrInvalidAnnotations
func (e *BundleError) IsSignatureError() bool {
	return errors.Is(e.Err, ErrSignatureNotFound) ||
		errors.Is(e.Err, ErrSignatureInvalid) ||
		errors.Is(e.Err, ErrUntrustedSigner) ||
		errors.Is(e.Err, ErrRekorVerificationFailed) ||
		errors.Is(e.Err, ErrCertificateExpired) ||
		errors.Is(e.Err, ErrInvalidAnnotations)
}

// SignatureErrorInfo provides detailed context about signature verification failures.
// This information helps users understand why signature verification failed and how
// to remediate the issue.
type SignatureErrorInfo struct {
	// Digest is the content digest of the artifact that failed verification.
	// This is the SHA256 digest used to look up the signature.
	Digest string

	// Signer is the identity of the entity that signed the artifact, if available.
	// For keyless signing, this is typically an email address.
	// For public key signing, this may be empty or a key fingerprint.
	Signer string

	// Reason provides a human-readable explanation of why verification failed.
	// This should be actionable and help users understand what went wrong.
	Reason string

	// FailureStage indicates at which stage of verification the failure occurred.
	// Possible values:
	//   - "fetch": Failed to fetch signature artifact from registry
	//   - "crypto": Cryptographic signature verification failed
	//   - "policy": Signature valid but policy check failed (identity, annotations)
	//   - "rekor": Transparency log verification failed
	FailureStage string
}
