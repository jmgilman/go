package errors

import (
	stderrors "errors"
)

// Is reports whether any error in err's chain matches target.
// This is a convenience wrapper around the standard library errors.Is.
//
// Example:
//
//	var ErrNotFound = errors.New(errors.CodeNotFound, "not found")
//	if errors.Is(err, ErrNotFound) {
//	    // Handle not found case
//	}
func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

// As finds the first error in err's chain that matches target.
// This is a convenience wrapper around the standard library errors.As.
//
// Example:
//
//	var platformErr PlatformError
//	if errors.As(err, &platformErr) {
//	    code := platformErr.Code()
//	}
func As(err error, target interface{}) bool {
	return stderrors.As(err, target)
}

// GetCode extracts the ErrorCode from an error.
// Returns CodeUnknown if the error is nil or not a PlatformError.
//
// This function handles the error chain and will extract the code from
// the outermost PlatformError in the chain.
//
// Example:
//
//	if errors.GetCode(err) == errors.CodeNotFound {
//	    // Handle not found
//	}
func GetCode(err error) ErrorCode {
	if err == nil {
		return CodeUnknown
	}

	var platformErr PlatformError
	if stderrors.As(err, &platformErr) {
		return platformErr.Code()
	}

	return CodeUnknown
}

// GetClassification extracts the ErrorClassification from an error.
// Returns ClassificationPermanent if the error is nil or not a PlatformError.
// This is a safe default that prevents inappropriate retry attempts.
//
// This function handles the error chain and will extract the classification
// from the outermost PlatformError in the chain.
//
// Example:
//
//	classification := errors.GetClassification(err)
//	if classification == errors.ClassificationRetryable {
//	    // Retry logic
//	}
func GetClassification(err error) ErrorClassification {
	if err == nil {
		return ClassificationPermanent
	}

	var platformErr PlatformError
	if stderrors.As(err, &platformErr) {
		return platformErr.Classification()
	}

	return ClassificationPermanent
}

// IsRetryable returns true if the error is classified as retryable.
// Returns false if the error is nil or not a PlatformError (safe default).
//
// This is the primary function for making retry decisions in the platform.
// It provides a simple boolean check to determine if an operation should
// be retried after a failure.
//
// Example:
//
//	if errors.IsRetryable(err) {
//	    // Implement retry with backoff
//	    time.Sleep(backoff)
//	    return retry(operation)
//	}
func IsRetryable(err error) bool {
	return GetClassification(err).IsRetryable()
}
