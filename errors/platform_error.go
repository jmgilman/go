package errors

import "fmt"

// platformError is the concrete implementation of PlatformError.
// It is private to enforce construction through package functions.
type platformError struct {
	code           ErrorCode
	classification ErrorClassification
	message        string
	context        map[string]interface{}
	cause          error
}

// Error returns the string representation of the error.
// Format: "[CODE] message" or "[CODE] message: cause" if cause is present.
func (e *platformError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.code, e.message, e.cause)
	}
	return fmt.Sprintf("[%s] %s", e.code, e.message)
}

// Code returns the error code.
func (e *platformError) Code() ErrorCode {
	return e.code
}

// Classification returns the error classification.
func (e *platformError) Classification() ErrorClassification {
	return e.classification
}

// Message returns the error message.
func (e *platformError) Message() string {
	return e.message
}

// Context returns a defensive copy of the context map.
// Returns nil if no context has been attached (maintains immutability).
func (e *platformError) Context() map[string]interface{} {
	if e.context == nil {
		return nil
	}
	ctx := make(map[string]interface{}, len(e.context))
	for k, v := range e.context {
		ctx[k] = v
	}
	return ctx
}

// Unwrap returns the wrapped error for standard library compatibility.
func (e *platformError) Unwrap() error {
	return e.cause
}
