package errors

// PlatformError extends the standard error interface with structured information
// for consistent error handling.
//
// PlatformError provides error codes for categorization, classification for
// retry logic, contextual metadata, and compatibility with standard library
// error handling (errors.Is, errors.As, errors.Unwrap).
type PlatformError interface {
	error

	// Code returns the error code identifying the type of error.
	Code() ErrorCode

	// Classification returns whether the error is retryable or permanent.
	Classification() ErrorClassification

	// Message returns the human-readable error message.
	Message() string

	// Context returns attached metadata as a read-only map.
	// Returns nil if no context has been attached.
	Context() map[string]interface{}

	// Unwrap returns the wrapped error for errors.Is and errors.As compatibility.
	// Returns nil if this error does not wrap another error.
	Unwrap() error
}
