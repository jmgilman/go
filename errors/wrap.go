package errors

import (
	"errors"
	"fmt"
)

// Wrap wraps an error with additional context while preserving the original error.
// The wrapped error is accessible via Unwrap() and compatible with errors.Is and errors.As.
//
// If the wrapped error is a PlatformError, its classification is preserved.
// Otherwise, the default classification for the error code is used.
//
// Returns nil if err is nil.
//
// Example:
//
//	result, err := repo.Clone(ctx, url)
//	if err != nil {
//	    return errors.Wrap(err, errors.CodeNetwork, "failed to clone repository")
//	}
func Wrap(err error, code ErrorCode, message string) PlatformError {
	if err == nil {
		return nil
	}

	// Preserve classification if wrapping a PlatformError
	classification := getDefaultClassification(code)
	var platformErr PlatformError
	if errors.As(err, &platformErr) {
		classification = platformErr.Classification()
	}

	return &platformError{
		code:           code,
		classification: classification,
		message:        message,
		context:        nil,
		cause:          err,
	}
}

// Wrapf wraps an error with a formatted message while preserving the original error.
//
// Returns nil if err is nil.
//
// Example:
//
//	if err := validate(input); err != nil {
//	    return errors.Wrapf(err, errors.CodeInvalidInput, "validation failed for field %s", fieldName)
//	}
func Wrapf(err error, code ErrorCode, format string, args ...interface{}) PlatformError {
	if err == nil {
		return nil
	}

	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// WrapWithContext wraps an error and attaches context metadata in a single operation.
// The context map is copied to prevent external mutation.
//
// Returns nil if err is nil.
//
// Example:
//
//	if err := build(ctx); err != nil {
//	    return errors.WrapWithContext(err, errors.CodeBuildFailed, "build failed", map[string]interface{}{
//	        "project": projectName,
//	        "phase":   "test",
//	    })
//	}
func WrapWithContext(err error, code ErrorCode, message string, ctx map[string]interface{}) PlatformError {
	if err == nil {
		return nil
	}

	// Preserve classification if wrapping a PlatformError
	classification := getDefaultClassification(code)
	var platformErr PlatformError
	if errors.As(err, &platformErr) {
		classification = platformErr.Classification()
	}

	// Create defensive copy of context
	var contextCopy map[string]interface{}
	if ctx != nil {
		contextCopy = make(map[string]interface{}, len(ctx))
		for k, v := range ctx {
			contextCopy[k] = v
		}
	}

	return &platformError{
		code:           code,
		classification: classification,
		message:        message,
		context:        contextCopy,
		cause:          err,
	}
}
