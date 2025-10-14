// Package cue provides CUE evaluation and validation capabilities with platform error handling.
package cue

import (
	"fmt"

	"github.com/jmgilman/go/errors"
)

// wrapLoadError wraps an error with CodeCUELoadFailed.
// Used when loading CUE modules or files fails.
func wrapLoadError(err error, message string) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeCUELoadFailed, message)
}

// wrapLoadErrorf wraps an error with CodeCUELoadFailed and a formatted message.
func wrapLoadErrorf(err error, format string, args ...interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrapf(err, errors.CodeCUELoadFailed, format, args...)
}

// wrapLoadErrorWithContext wraps an error with CodeCUELoadFailed and attaches context metadata.
func wrapLoadErrorWithContext(err error, message string, ctx map[string]interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.WrapWithContext(err, errors.CodeCUELoadFailed, message, ctx)
}

// wrapBuildError wraps an error with CodeCUEBuildFailed.
// Used when CUE build or evaluation fails.
func wrapBuildError(err error, message string) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeCUEBuildFailed, message)
}

// wrapBuildErrorf wraps an error with CodeCUEBuildFailed and a formatted message.
func wrapBuildErrorf(err error, format string, args ...interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrapf(err, errors.CodeCUEBuildFailed, format, args...)
}

// wrapBuildErrorWithContext wraps an error with CodeCUEBuildFailed and attaches context metadata.
func wrapBuildErrorWithContext(err error, message string, ctx map[string]interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.WrapWithContext(err, errors.CodeCUEBuildFailed, message, ctx)
}

// wrapValidationError wraps an error with CodeCUEValidationFailed.
// Used when CUE validation fails.
func wrapValidationError(err error, message string) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeCUEValidationFailed, message)
}

// wrapValidationErrorf wraps an error with CodeCUEValidationFailed and a formatted message.
func wrapValidationErrorf(err error, format string, args ...interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrapf(err, errors.CodeCUEValidationFailed, format, args...)
}

// wrapValidationErrorWithContext wraps an error with CodeCUEValidationFailed and attaches context metadata.
func wrapValidationErrorWithContext(err error, message string, ctx map[string]interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.WrapWithContext(err, errors.CodeCUEValidationFailed, message, ctx)
}

// wrapDecodeError wraps an error with CodeCUEDecodeFailed.
// Used when decoding CUE values to Go structs fails.
func wrapDecodeError(err error, message string) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeCUEDecodeFailed, message)
}

// wrapDecodeErrorf wraps an error with CodeCUEDecodeFailed and a formatted message.
func wrapDecodeErrorf(err error, format string, args ...interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrapf(err, errors.CodeCUEDecodeFailed, format, args...)
}

// wrapDecodeErrorWithContext wraps an error with CodeCUEDecodeFailed and attaches context metadata.
func wrapDecodeErrorWithContext(err error, message string, ctx map[string]interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.WrapWithContext(err, errors.CodeCUEDecodeFailed, message, ctx)
}

// wrapEncodeError wraps an error with CodeCUEEncodeFailed.
// Used when encoding CUE values to YAML/JSON fails.
func wrapEncodeError(err error, message string) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrap(err, errors.CodeCUEEncodeFailed, message)
}

// wrapEncodeErrorf wraps an error with CodeCUEEncodeFailed and a formatted message.
func wrapEncodeErrorf(err error, format string, args ...interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.Wrapf(err, errors.CodeCUEEncodeFailed, format, args...)
}

// wrapEncodeErrorWithContext wraps an error with CodeCUEEncodeFailed and attaches context metadata.
func wrapEncodeErrorWithContext(err error, message string, ctx map[string]interface{}) errors.PlatformError {
	if err == nil {
		return nil
	}
	return errors.WrapWithContext(err, errors.CodeCUEEncodeFailed, message, ctx)
}

// extractErrorContext creates a context map with common error metadata.
// Helper for building context maps for WrapWithContext calls.
func extractErrorContext(kvPairs ...interface{}) map[string]interface{} {
	if len(kvPairs) == 0 {
		return nil
	}

	ctx := make(map[string]interface{})
	for i := 0; i < len(kvPairs)-1; i += 2 {
		key, ok := kvPairs[i].(string)
		if !ok {
			continue
		}
		ctx[key] = kvPairs[i+1]
	}

	if len(ctx) == 0 {
		return nil
	}
	return ctx
}

// makeContext is a convenience helper for creating context maps inline.
// Example: makeContext("path", "/foo/bar", "line", 42).
func makeContext(kvPairs ...interface{}) map[string]interface{} {
	return extractErrorContext(kvPairs...)
}

// formatFieldPath formats a CUE field path for error messages.
// Handles edge cases like empty paths or single-element paths.
func formatFieldPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return fmt.Sprintf("field %s", path)
}
