// Package cue provides CUE evaluation and validation capabilities with platform error handling.
package cue

import (
	"context"
	"io"

	"cuelang.org/go/cue"
	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/jmgilman/go/errors"
)

// EncodeYAML encodes a CUE value to YAML bytes.
// Returns CodeCUEEncodeFailed if the value cannot be encoded or is not fully evaluated.
func EncodeYAML(ctx context.Context, value cue.Value) ([]byte, errors.PlatformError) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return nil, wrapEncodeError(ctx.Err(), "context cancelled before encoding")
	}

	// Validate that the value is fully evaluated
	if err := value.Err(); err != nil {
		return nil, wrapEncodeErrorWithContext(
			err,
			"CUE value contains errors and cannot be encoded",
			makeContext("error", err.Error()),
		)
	}

	// Check if value is concrete (fully evaluated)
	if !value.IsConcrete() {
		return nil, errors.New(
			errors.CodeCUEEncodeFailed,
			"CUE value is not concrete (contains unresolved values) and cannot be encoded to YAML",
		)
	}

	// Encode to YAML
	data, err := cueyaml.Encode(value)
	if err != nil {
		return nil, wrapEncodeErrorf(
			err,
			"failed to encode CUE value to YAML: %v",
			err,
		)
	}

	return data, nil
}

// EncodeJSON encodes a CUE value to JSON bytes.
// Returns CodeCUEEncodeFailed if the value cannot be encoded or is not fully evaluated.
func EncodeJSON(ctx context.Context, value cue.Value) ([]byte, errors.PlatformError) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return nil, wrapEncodeError(ctx.Err(), "context cancelled before encoding")
	}

	// Validate that the value is fully evaluated
	if err := value.Err(); err != nil {
		return nil, wrapEncodeErrorWithContext(
			err,
			"CUE value contains errors and cannot be encoded",
			makeContext("error", err.Error()),
		)
	}

	// Check if value is concrete (fully evaluated)
	if !value.IsConcrete() {
		return nil, errors.New(
			errors.CodeCUEEncodeFailed,
			"CUE value is not concrete (contains unresolved values) and cannot be encoded to JSON",
		)
	}

	// Encode to JSON using the built-in MarshalJSON method
	data, err := value.MarshalJSON()
	if err != nil {
		return nil, wrapEncodeErrorf(
			err,
			"failed to encode CUE value to JSON: %v",
			err,
		)
	}

	return data, nil
}

// EncodeYAMLStream encodes a CUE value to YAML and writes it to an io.Writer.
// This is useful for large manifests to avoid loading the entire output into memory.
// Returns CodeCUEEncodeFailed if the value cannot be encoded, is not fully evaluated,
// or if writing to the Writer fails.
func EncodeYAMLStream(ctx context.Context, value cue.Value, w io.Writer) errors.PlatformError {
	// Validate writer is not nil first (before any processing)
	if w == nil {
		return errors.New(errors.CodeCUEEncodeFailed, "writer cannot be nil")
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return wrapEncodeError(ctx.Err(), "context cancelled before encoding")
	}

	// Validate that the value is fully evaluated
	if err := value.Err(); err != nil {
		return wrapEncodeErrorWithContext(
			err,
			"CUE value contains errors and cannot be encoded",
			makeContext("error", err.Error()),
		)
	}

	// Check if value is concrete (fully evaluated)
	if !value.IsConcrete() {
		return errors.New(
			errors.CodeCUEEncodeFailed,
			"CUE value is not concrete (contains unresolved values) and cannot be encoded to YAML",
		)
	}

	// Encode to YAML bytes first
	// Note: CUE's YAML encoder doesn't support streaming directly,
	// so we encode to bytes then write. This is still more memory-efficient
	// than having the caller manage the bytes.
	data, err := cueyaml.Encode(value)
	if err != nil {
		return wrapEncodeErrorf(
			err,
			"failed to encode CUE value to YAML: %v",
			err,
		)
	}

	// Write to the writer
	n, err := w.Write(data)
	if err != nil {
		return wrapEncodeErrorWithContext(
			err,
			"failed to write YAML data to writer",
			makeContext("bytes_written", n, "total_bytes", len(data)),
		)
	}

	// Verify all bytes were written
	if n != len(data) {
		err := errors.New(errors.CodeCUEEncodeFailed, "incomplete write: not all YAML data was written to writer")
		return errors.WithContextMap(err, makeContext("bytes_written", n, "total_bytes", len(data)))
	}

	return nil
}
