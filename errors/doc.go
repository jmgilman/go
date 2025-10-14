// Package errors provides structured error handling.
//
// This package extends Go's standard error handling with error codes, classification
// (retryable vs permanent), context metadata, and JSON serialization. It maintains
// full compatibility with the standard library errors package (errors.Is, errors.As,
// errors.Unwrap).
//
// # Features
//
//   - Structured error codes for consistent categorization
//   - Error classification for intelligent retry logic (retryable vs permanent)
//   - Context metadata attachment for debugging
//   - Error wrapping that preserves the error chain
//   - JSON serialization for API responses
//   - Zero dependencies (Layer 0 library)
//
// # Design Principles
//
//   - Standard library compatibility (errors.Is, errors.As, errors.Unwrap)
//   - Immutability (errors are immutable once created)
//   - Type safety (strong types for codes and classifications)
//   - Simplicity (minimal API surface, easy to use correctly)
//   - Performance (optimized for error creation in hot paths)
//
// # Quick Start
//
// Creating errors:
//
//	// Simple error
//	err := errors.New(errors.CodeNotFound, "user not found")
//
//	// Formatted error
//	err := errors.Newf(errors.CodeInvalidInput, "invalid age: %d", age)
//
// Wrapping errors:
//
//	result, err := repo.Query(ctx, id)
//	if err != nil {
//	    return errors.Wrap(err, errors.CodeDatabase, "failed to query user")
//	}
//
// Adding context:
//
//	err := errors.New(errors.CodeBuildFailed, "build failed")
//	err = errors.WithContext(err, "project", "api")
//	err = errors.WithContext(err, "phase", "test")
//
// Retry logic:
//
//	if errors.IsRetryable(err) {
//	    // Implement retry with backoff
//	    time.Sleep(backoff)
//	    return retry(operation)
//	}
//
// JSON serialization:
//
//	func handleError(w http.ResponseWriter, err error) {
//	    response := errors.ToJSON(err)
//	    w.Header().Set("Content-Type", "application/json")
//	    w.WriteHeader(getHTTPStatus(errors.GetCode(err)))
//	    json.NewEncoder(w).Encode(response)
//	}
//
// # Error Codes
//
// The library provides predefined error codes for all common platform scenarios:
//
//   - Resource errors: CodeNotFound, CodeAlreadyExists, CodeConflict
//   - Permission errors: CodeUnauthorized, CodeForbidden
//   - Validation errors: CodeInvalidInput, CodeInvalidConfig, CodeSchemaFailed
//   - Infrastructure errors: CodeDatabase, CodeNetwork, CodeTimeout, CodeRateLimit
//   - Execution errors: CodeExecutionFailed, CodeBuildFailed, CodePublishFailed
//   - System errors: CodeInternal, CodeNotImplemented, CodeUnavailable
//   - Generic: CodeUnknown
//
// Each error code has a default classification (retryable or permanent) that can
// be overridden when needed.
//
// # Error Classification
//
// Errors are classified as either retryable or permanent:
//
//   - Retryable: Temporary failures (network, timeout, rate limit, transient DB issues)
//   - Permanent: Logic errors (validation, not found, permission denied)
//
// Use errors.IsRetryable(err) to make retry decisions. The classification is
// preserved when wrapping errors and can be overridden with WithClassification.
//
// # Standard Library Compatibility
//
// PlatformError implements the error interface and works seamlessly with standard
// library error functions:
//
//	// errors.Is traverses the error chain
//	if errors.Is(err, sql.ErrNoRows) {
//	    // Handle no rows
//	}
//
//	// errors.As finds typed errors in the chain
//	var platformErr errors.PlatformError
//	if errors.As(err, &platformErr) {
//	    code := platformErr.Code()
//	}
//
//	// errors.Unwrap retrieves the wrapped error
//	cause := errors.Unwrap(err)
//
// # Context Metadata
//
// Attach debugging context to errors without exposing sensitive information:
//
//	err := errors.New(errors.CodeBuildFailed, "build failed")
//	err = errors.WithContextMap(err, map[string]interface{}{
//	    "project":    "api",
//	    "phase":      "test",
//	    "exit_code":  1,
//	    "duration":   "2m30s",
//	})
//
// Context is included in JSON serialization but not in error chains exposed
// to external callers (security).
//
// # Best Practices
//
//   - Always wrap errors with context: errors.Wrap(err, code, message)
//   - Use specific error codes, not CodeUnknown
//   - Don't include sensitive data in error messages or context
//   - Use IsRetryable for retry decisions, not specific codes
//   - Add context at each layer of the call stack
//   - Preserve classification when wrapping (automatic)
//   - Use ToJSON for API responses to prevent information leakage
//
// # Performance
//
// The library is optimized for minimal overhead:
//
//   - Error creation: <10μs
//   - Error wrapping: <5μs
//   - Context attachment: <2μs
//   - Classification lookup: O(1)
//
// See benchmarks for detailed performance characteristics.
package errors
