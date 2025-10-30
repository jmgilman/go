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
)

// BundleError provides detailed context about OCI bundle operation failures.
// It wraps underlying errors with additional context specific to OCI bundle operations,
// including the operation type and OCI reference being processed.
//
// BundleError implements the error interface and supports error wrapping,
// allowing it to be used with errors.Is() and errors.As() for proper error handling.
type BundleError struct {
	// Op describes the operation that failed (e.g., "push", "pull", "extract").
	Op string

	// Reference is the OCI reference being processed when the error occurred.
	// This could be a full reference like "ghcr.io/org/repo:v1.0.0".
	Reference string

	// Err is the underlying error that caused this BundleError to be created.
	// This preserves the original error context and allows for proper error wrapping.
	Err error
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
