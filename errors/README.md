# errors

Structured error handling library.

## Overview

The `errors` library provides a foundational error handling system with:

- **Structured error codes** - Consistent categorization across the platform
- **Error classification** - Retryable vs permanent for intelligent retry logic
- **Context preservation** - Rich metadata without exposing sensitive data
- **Error wrapping** - Maintain error chains with standard library compatibility
- **JSON serialization** - API-friendly error responses

## Installation

```bash
go get github.com/jmgilman/go/errors
```

## Quick Start

```go
import "github.com/jmgilman/go/errors"

// Create an error
err := errors.New(errors.CodeNotFound, "user not found")

// Wrap an error
if err := db.Query(ctx, id); err != nil {
    return errors.Wrap(err, errors.CodeDatabase, "failed to query user")
}

// Add context
err = errors.WithContext(err, "user_id", id)
err = errors.WithContext(err, "operation", "login")

// Check if retryable
if errors.IsRetryable(err) {
    // Retry with backoff
}

// Serialize for API
response := errors.ToJSON(err)
json.NewEncoder(w).Encode(response)
```

## Features

### Error Codes

25 predefined error codes covering all platform scenarios:

- Resource: `CodeNotFound`, `CodeAlreadyExists`, `CodeConflict`
- Permission: `CodeUnauthorized`, `CodeForbidden`
- Validation: `CodeInvalidInput`, `CodeInvalidConfig`, `CodeSchemaFailed`
- Infrastructure: `CodeDatabase`, `CodeNetwork`, `CodeTimeout`, `CodeRateLimit`
- Execution: `CodeExecutionFailed`, `CodeBuildFailed`, `CodePublishFailed`
- CUE: `CodeCUELoadFailed`, `CodeCUEBuildFailed`, `CodeCUEValidationFailed`, `CodeCUEDecodeFailed`, `CodeCUEEncodeFailed`
- Schema: `CodeSchemaVersionIncompatible`
- System: `CodeInternal`, `CodeNotImplemented`, `CodeUnavailable`
- Generic: `CodeUnknown`

### Classification

Errors are automatically classified:

- **Retryable**: Temporary failures (network, timeout, rate limit)
- **Permanent**: Logic errors (validation, not found, permission denied)

Use `errors.IsRetryable(err)` for retry decisions.

### Context Metadata

Attach debugging information to errors:

```go
err = errors.WithContextMap(err, map[string]interface{}{
    "project": "api",
    "phase": "test",
    "duration": "2m30s",
})
```

### Standard Library Compatibility

Works seamlessly with `errors.Is`, `errors.As`, and `errors.Unwrap`:

```go
if errors.Is(err, sql.ErrNoRows) {
    // Handle no rows
}

var platformErr errors.PlatformError
if errors.As(err, &platformErr) {
    code := platformErr.Code()
}
```

## Documentation

Full documentation: https://pkg.go.dev/github.com/jmgilman/go/errors

## Performance

- Error creation: <10μs
- Error wrapping: <5μs
- Context attachment: <2μs

See benchmarks for detailed performance characteristics.

## License

Apache 2.0
