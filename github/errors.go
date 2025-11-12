package github

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jmgilman/go/errors"
)

// GitHub-specific error codes (use existing codes from errors library).
// These are convenience aliases for readability in GitHub context.
const (
	// ErrCodeNotFound indicates a requested resource was not found.
	ErrCodeNotFound = errors.CodeNotFound

	// ErrCodeAuthenticationFailed indicates authentication failure.
	ErrCodeAuthenticationFailed = errors.CodeUnauthorized

	// ErrCodePermissionDenied indicates insufficient permissions.
	ErrCodePermissionDenied = errors.CodeForbidden

	// ErrCodeRateLimited indicates rate limit exceeded.
	ErrCodeRateLimited = errors.CodeRateLimit

	// ErrCodeInvalidInput indicates invalid parameters or malformed data.
	ErrCodeInvalidInput = errors.CodeInvalidInput

	// ErrCodeConflict indicates a conflict (e.g., resource already exists).
	ErrCodeConflict = errors.CodeConflict

	// ErrCodeNetwork indicates network-related errors.
	ErrCodeNetwork = errors.CodeNetwork

	// ErrCodeInternal indicates internal errors.
	ErrCodeInternal = errors.CodeInternal
)

// WrapHTTPError wraps an error based on HTTP status code from GitHub API.
func WrapHTTPError(err error, statusCode int, message string) error {
	if err == nil {
		return nil
	}

	var code errors.ErrorCode
	switch statusCode {
	case http.StatusNotFound:
		code = errors.CodeNotFound
	case http.StatusUnauthorized:
		code = errors.CodeUnauthorized
	case http.StatusForbidden:
		code = errors.CodeForbidden
	case http.StatusConflict:
		code = errors.CodeConflict
	case http.StatusUnprocessableEntity:
		code = errors.CodeInvalidInput
	case http.StatusBadRequest:
		code = errors.CodeInvalidInput
	case http.StatusTooManyRequests:
		code = errors.CodeRateLimit
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		code = errors.CodeNetwork
	default:
		if statusCode >= 500 {
			code = errors.CodeNetwork
		} else {
			code = errors.CodeInternal
		}
	}

	return errors.Wrap(err, code, message)
}

// wrapCLIError wraps errors from gh CLI execution.
func wrapCLIError(err error, _ string, stderr string) error {
	if err == nil {
		return nil
	}

	// Try to determine error type from stderr
	msg := stderr
	if msg == "" {
		msg = "gh CLI command failed"
	}

	var code errors.ErrorCode

	// Check for common error patterns in stderr
	switch {
	case contains(stderr, "not found", "could not find", "no such"):
		code = errors.CodeNotFound
	case contains(stderr, "authentication failed", "not logged in", "unauthorized"):
		code = errors.CodeUnauthorized
	case contains(stderr, "forbidden", "permission denied"):
		code = errors.CodeForbidden
	case contains(stderr, "rate limit"):
		code = errors.CodeRateLimit
	case contains(stderr, "invalid", "malformed", "bad request"):
		code = errors.CodeInvalidInput
	case contains(stderr, "conflict", "already exists"):
		code = errors.CodeConflict
	case contains(stderr, "network", "connection", "timeout"):
		code = errors.CodeNetwork
	default:
		code = errors.CodeInternal
	}

	return errors.Wrap(err, code, msg)
}

// contains checks if any of the patterns exist in the text (case-insensitive).
func contains(text string, patterns ...string) bool {
	lowText := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(lowText, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// newNotFoundError creates a not found error with context.
func newNotFoundError(resourceType, identifier string) error {
	err := errors.New(
		errors.CodeNotFound,
		fmt.Sprintf("%s not found: %s", resourceType, identifier),
	)
	err = errors.WithContext(err, "resource_type", resourceType)
	err = errors.WithContext(err, "identifier", identifier)
	return err
}

// newInvalidInputError creates an invalid input error with context.
func newInvalidInputError(field, reason string) error {
	err := errors.New(
		errors.CodeInvalidInput,
		fmt.Sprintf("invalid %s: %s", field, reason),
	)
	err = errors.WithContext(err, "field", field)
	err = errors.WithContext(err, "reason", reason)
	return err
}

// newAuthenticationFailedError creates an authentication failed error with context.
func newAuthenticationFailedError(message string, cause error) error {
	if cause != nil {
		return errors.Wrap(cause, errors.CodeUnauthorized, message)
	}
	return errors.New(errors.CodeUnauthorized, message)
}
