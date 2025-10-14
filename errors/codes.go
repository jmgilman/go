// Package errors provides a foundational error handling system.
// It extends Go's standard error handling with structured error codes, retry classification,
// context preservation, and API serialization capabilities.
package errors

// ErrorCode represents a specific error condition.
// Error codes are string-based for debuggability and natural JSON serialization.
type ErrorCode string

const (
	// Resource errors.

	// CodeNotFound indicates a requested resource does not exist.
	CodeNotFound ErrorCode = "NOT_FOUND"

	// CodeAlreadyExists indicates a resource already exists and cannot be created again.
	CodeAlreadyExists ErrorCode = "ALREADY_EXISTS"

	// CodeConflict indicates a resource state conflict that prevents the operation.
	CodeConflict ErrorCode = "CONFLICT"

	// Permission errors.

	// CodeUnauthorized indicates the request lacks valid authentication credentials.
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"

	// CodeForbidden indicates the authenticated user lacks permission for the operation.
	CodeForbidden ErrorCode = "FORBIDDEN"

	// Validation errors.

	// CodeInvalidInput indicates the provided input is invalid or malformed.
	CodeInvalidInput ErrorCode = "INVALID_INPUT"

	// CodeInvalidConfig indicates a configuration error prevents the operation.
	CodeInvalidConfig ErrorCode = "INVALID_CONFIGURATION"

	// CodeSchemaFailed indicates the data failed schema validation.
	CodeSchemaFailed ErrorCode = "SCHEMA_VALIDATION_FAILED"

	// Infrastructure errors.

	// CodeDatabase indicates a database operation failed.
	CodeDatabase ErrorCode = "DATABASE_ERROR"

	// CodeNetwork indicates a network operation failed.
	CodeNetwork ErrorCode = "NETWORK_ERROR"

	// CodeTimeout indicates an operation exceeded its time limit.
	CodeTimeout ErrorCode = "TIMEOUT"

	// CodeRateLimit indicates the rate limit has been exceeded.
	CodeRateLimit ErrorCode = "RATE_LIMIT_EXCEEDED"

	// Execution errors.

	// CodeExecutionFailed indicates a general execution failure.
	CodeExecutionFailed ErrorCode = "EXECUTION_FAILED"

	// CodeBuildFailed indicates a build operation failed.
	CodeBuildFailed ErrorCode = "BUILD_FAILED"

	// CodePublishFailed indicates a publish operation failed.
	CodePublishFailed ErrorCode = "PUBLISH_FAILED"

	// CUE errors.

	// CodeCUELoadFailed indicates CUE file/module loading failed.
	CodeCUELoadFailed ErrorCode = "CUE_LOAD_FAILED"

	// CodeCUEBuildFailed indicates CUE build/evaluation failed.
	CodeCUEBuildFailed ErrorCode = "CUE_BUILD_FAILED"

	// CodeCUEValidationFailed indicates CUE validation failed.
	CodeCUEValidationFailed ErrorCode = "CUE_VALIDATION_FAILED"

	// CodeCUEDecodeFailed indicates CUE to Go struct decoding failed.
	CodeCUEDecodeFailed ErrorCode = "CUE_DECODE_FAILED"

	// CodeCUEEncodeFailed indicates CUE to YAML/JSON encoding failed.
	CodeCUEEncodeFailed ErrorCode = "CUE_ENCODE_FAILED"

	// Schema errors.

	// CodeSchemaVersionIncompatible indicates incompatible major schema version.
	// Config major version does not match supported schema major version.
	CodeSchemaVersionIncompatible ErrorCode = "SCHEMA_VERSION_INCOMPATIBLE"

	// System errors.

	// CodeInternal indicates an internal system error occurred.
	CodeInternal ErrorCode = "INTERNAL_ERROR"

	// CodeNotImplemented indicates the requested functionality is not implemented.
	CodeNotImplemented ErrorCode = "NOT_IMPLEMENTED"

	// CodeUnavailable indicates the service is temporarily unavailable.
	CodeUnavailable ErrorCode = "SERVICE_UNAVAILABLE"

	// Generic errors.

	// CodeUnknown indicates an unknown or unclassified error occurred.
	CodeUnknown ErrorCode = "UNKNOWN"
)
