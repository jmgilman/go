package errors

import "errors"

// WithContext adds a single context field to an error.
// Returns a new PlatformError with the context field added.
// Existing context fields are preserved.
//
// If err is not a PlatformError, it is converted to one with CodeUnknown.
// Returns nil if err is nil.
//
// Example:
//
//	err := errors.New(errors.CodeBuildFailed, "build failed")
//	err = errors.WithContext(err, "project", "my-app")
//	err = errors.WithContext(err, "phase", "test")
func WithContext(err error, key string, value interface{}) PlatformError {
	if err == nil {
		return nil
	}

	// Convert to PlatformError if needed
	var platformErr PlatformError
	if !errors.As(err, &platformErr) {
		// Wrap standard error as PlatformError
		platformErr = &platformError{
			code:           CodeUnknown,
			classification: ClassificationPermanent,
			message:        err.Error(),
			context:        nil,
			cause:          err,
		}
	}

	// Create new context with existing fields plus new field
	newContext := make(map[string]interface{})
	if existingCtx := platformErr.Context(); existingCtx != nil {
		for k, v := range existingCtx {
			newContext[k] = v
		}
	}
	newContext[key] = value

	return &platformError{
		code:           platformErr.Code(),
		classification: platformErr.Classification(),
		message:        platformErr.Message(),
		context:        newContext,
		cause:          platformErr.Unwrap(),
	}
}

// WithContextMap adds multiple context fields to an error.
// Returns a new PlatformError with the context fields merged.
// Existing context fields are preserved; new fields override existing ones with the same key.
//
// If err is not a PlatformError, it is converted to one with CodeUnknown.
// Returns nil if err is nil.
//
// Example:
//
//	err := errors.New(errors.CodeExecutionFailed, "execution failed")
//	err = errors.WithContextMap(err, map[string]interface{}{
//	    "command": "earthly",
//	    "target":  "+test",
//	})
func WithContextMap(err error, ctx map[string]interface{}) PlatformError {
	if err == nil {
		return nil
	}

	// Convert to PlatformError if needed
	var platformErr PlatformError
	if !errors.As(err, &platformErr) {
		platformErr = &platformError{
			code:           CodeUnknown,
			classification: ClassificationPermanent,
			message:        err.Error(),
			context:        nil,
			cause:          err,
		}
	}

	// Merge existing context with new context
	newContext := make(map[string]interface{})
	if existingCtx := platformErr.Context(); existingCtx != nil {
		for k, v := range existingCtx {
			newContext[k] = v
		}
	}
	// New fields override existing
	for k, v := range ctx {
		newContext[k] = v
	}

	return &platformError{
		code:           platformErr.Code(),
		classification: platformErr.Classification(),
		message:        platformErr.Message(),
		context:        newContext,
		cause:          platformErr.Unwrap(),
	}
}

// WithClassification overrides the classification of an error.
// Returns a new PlatformError with the specified classification.
//
// This is useful when you need to override the default classification for an error code.
// For example, marking a normally permanent error as retryable in specific circumstances.
//
// If err is not a PlatformError, it is converted to one with CodeUnknown.
// Returns nil if err is nil.
//
// Example:
//
//	err := errors.New(errors.CodeDatabase, "connection failed")
//	// Normally retryable, but mark as permanent for this case
//	err = errors.WithClassification(err, errors.ClassificationPermanent)
func WithClassification(err error, classification ErrorClassification) PlatformError {
	if err == nil {
		return nil
	}

	// Convert to PlatformError if needed
	var platformErr PlatformError
	if !errors.As(err, &platformErr) {
		platformErr = &platformError{
			code:           CodeUnknown,
			classification: ClassificationPermanent,
			message:        err.Error(),
			context:        nil,
			cause:          err,
		}
	}

	// Copy context to preserve immutability
	var newContext map[string]interface{}
	if existingCtx := platformErr.Context(); existingCtx != nil {
		newContext = make(map[string]interface{}, len(existingCtx))
		for k, v := range existingCtx {
			newContext[k] = v
		}
	}

	return &platformError{
		code:           platformErr.Code(),
		classification: classification,
		message:        platformErr.Message(),
		context:        newContext,
		cause:          platformErr.Unwrap(),
	}
}
