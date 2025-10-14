package errors

import "fmt"

// New creates a new PlatformError with the given code and message.
// The error classification is determined by the error code using default mappings.
//
// Example:
//
//	err := errors.New(errors.CodeNotFound, "project not found")
func New(code ErrorCode, message string) PlatformError {
	return &platformError{
		code:           code,
		classification: getDefaultClassification(code),
		message:        message,
		context:        nil,
		cause:          nil,
	}
}

// Newf creates a new PlatformError with a formatted message.
// The error classification is determined by the error code using default mappings.
//
// Example:
//
//	err := errors.Newf(errors.CodeInvalidInput, "project name too long: %d characters (max %d)", len(name), maxLen)
func Newf(code ErrorCode, format string, args ...interface{}) PlatformError {
	return &platformError{
		code:           code,
		classification: getDefaultClassification(code),
		message:        fmt.Sprintf(format, args...),
		context:        nil,
		cause:          nil,
	}
}
