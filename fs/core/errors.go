package core

import (
	"errors"
	"io/fs"
)

var (
	// ErrNotExist is returned when a file or directory does not exist.
	// Re-exported from io/fs for convenience.
	ErrNotExist = fs.ErrNotExist

	// ErrExist is returned when a file or directory already exists.
	// Re-exported from io/fs for convenience.
	ErrExist = fs.ErrExist

	// ErrPermission is returned when permission is denied.
	// Re-exported from io/fs for convenience.
	ErrPermission = fs.ErrPermission

	// ErrClosed is returned when an operation is performed on a closed file.
	// Re-exported from io/fs for convenience.
	ErrClosed = fs.ErrClosed

	// ErrUnsupported is returned when an operation is not supported by the provider.
	// For example, symlink operations on S3 providers or metadata operations on
	// cloud storage backends.
	ErrUnsupported = errors.New("operation not supported")
)
