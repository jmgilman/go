package errors

// ErrorClassification indicates whether an error should trigger a retry.
// This is used by platform services to determine if an operation should be retried
// or if it represents a permanent failure.
type ErrorClassification string

const (
	// ClassificationRetryable indicates temporary failures that may succeed on retry.
	// Examples: network timeouts, rate limits, transient database issues.
	ClassificationRetryable ErrorClassification = "RETRYABLE"

	// ClassificationPermanent indicates failures that will not succeed on retry.
	// Examples: validation errors, permission denials, resource not found.
	ClassificationPermanent ErrorClassification = "PERMANENT"
)

// IsRetryable returns true if the classification indicates retry should be attempted.
func (c ErrorClassification) IsRetryable() bool {
	return c == ClassificationRetryable
}

// defaultClassifications maps error codes to their default classification.
// This determines the default retry behavior for each error type.
var defaultClassifications = map[ErrorCode]ErrorClassification{
	// Retryable errors (temporary failures)
	CodeTimeout:     ClassificationRetryable,
	CodeNetwork:     ClassificationRetryable,
	CodeRateLimit:   ClassificationRetryable,
	CodeUnavailable: ClassificationRetryable,
	CodeDatabase:    ClassificationRetryable, // Transient DB issues

	// Permanent errors (will not succeed on retry)
	CodeNotFound:       ClassificationPermanent,
	CodeAlreadyExists:  ClassificationPermanent,
	CodeConflict:       ClassificationPermanent,
	CodeUnauthorized:   ClassificationPermanent,
	CodeForbidden:      ClassificationPermanent,
	CodeInvalidInput:   ClassificationPermanent,
	CodeInvalidConfig:  ClassificationPermanent,
	CodeSchemaFailed:   ClassificationPermanent,
	CodeNotImplemented: ClassificationPermanent,

	// Execution errors (permanent by default, but context-dependent)
	CodeExecutionFailed: ClassificationPermanent,
	CodeBuildFailed:     ClassificationPermanent,
	CodePublishFailed:   ClassificationPermanent,

	// CUE errors (permanent - user configuration issues)
	CodeCUELoadFailed:       ClassificationPermanent,
	CodeCUEBuildFailed:      ClassificationPermanent,
	CodeCUEValidationFailed: ClassificationPermanent,
	CodeCUEDecodeFailed:     ClassificationPermanent,
	CodeCUEEncodeFailed:     ClassificationPermanent,

	// Schema errors (permanent - version incompatibility)
	CodeSchemaVersionIncompatible: ClassificationPermanent,

	// System errors (often permanent, but may be transient)
	CodeInternal: ClassificationPermanent,
	CodeUnknown:  ClassificationPermanent,
}

// getDefaultClassification returns the default classification for an error code.
// Returns ClassificationPermanent if the code is not in the map (safe default).
func getDefaultClassification(code ErrorCode) ErrorClassification {
	if class, ok := defaultClassifications[code]; ok {
		return class
	}
	return ClassificationPermanent // Safe default
}
