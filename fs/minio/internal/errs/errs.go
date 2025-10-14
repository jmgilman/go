// Package errs provides error handling utilities for the minio filesystem.
package errs

import (
	"fmt"
	"io/fs"

	"github.com/minio/minio-go/v7"
)

// Translate converts MinIO errors to stdlib fs errors.
func Translate(err error) error {
	if err == nil {
		return nil
	}

	// Check MinIO error responses
	errResp := minio.ToErrorResponse(err)

	switch errResp.Code {
	case "NoSuchKey":
		return fs.ErrNotExist
	case "NoSuchBucket":
		return fs.ErrNotExist
	case "AccessDenied":
		return fs.ErrPermission
	}

	// Return wrapped error with context for other errors
	return fmt.Errorf("minio: %w", err)
}

// PathError wraps an error in a fs.PathError for the given operation and path.
// If the error is nil, returns nil.
func PathError(op, path string, err error) error {
	if err == nil {
		return nil
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// PathErrorf creates a fs.PathError with a formatted error message.
func PathErrorf(op, path, format string, args ...interface{}) error {
	return &fs.PathError{Op: op, Path: path, Err: fmt.Errorf(format, args...)}
}
