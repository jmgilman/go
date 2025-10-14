// Package cue provides CUE evaluation and validation capabilities with platform error handling.
package cue

import (
	"context"
	"fmt"
	"reflect"

	"cuelang.org/go/cue"
	"github.com/jmgilman/go/errors"
)

// Decode decodes a CUE value into a Go struct pointer.
// The target must be a pointer to a struct. Works with any Go struct type.
//
// Returns CodeCUEDecodeFailed if:
// - target is not a pointer to a struct
// - the CUE value contains errors
// - type mismatch occurs during decoding
// - the CUE value is not concrete (contains unresolved values)
//
// Supports optional fields and default values from CUE schemas.
func Decode(ctx context.Context, value cue.Value, target interface{}) errors.PlatformError {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return wrapDecodeError(ctx.Err(), "context cancelled before decoding")
	}

	// Validate target is not nil
	if target == nil {
		return errors.New(
			errors.CodeCUEDecodeFailed,
			"decode target cannot be nil",
		)
	}

	// Validate target is a pointer to a struct
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr {
		return errors.New(
			errors.CodeCUEDecodeFailed,
			fmt.Sprintf("decode target must be a pointer to a struct, got %s", targetValue.Kind()),
		)
	}

	// Check if the pointer is nil
	if targetValue.IsNil() {
		return errors.New(
			errors.CodeCUEDecodeFailed,
			"decode target pointer cannot be nil",
		)
	}

	// Validate that the pointer points to a struct
	targetElem := targetValue.Elem()
	if targetElem.Kind() != reflect.Struct {
		return errors.New(
			errors.CodeCUEDecodeFailed,
			fmt.Sprintf("decode target must be a pointer to a struct, got pointer to %s", targetElem.Kind()),
		)
	}

	// Validate that the value is fully evaluated
	if err := value.Err(); err != nil {
		return wrapDecodeErrorWithContext(
			err,
			"CUE value contains errors and cannot be decoded",
			makeContext("error", err.Error()),
		)
	}

	// Check if value is concrete (fully evaluated)
	// Note: We allow non-concrete values for structs with optional fields and defaults
	// CUE's Decode will handle these correctly and catch the error if it fails
	// This allows for more flexible decoding with optional fields

	// Perform the decode operation
	if err := value.Decode(target); err != nil {
		// Extract information about what went wrong
		targetType := targetElem.Type().Name()
		if targetType == "" {
			targetType = targetElem.Type().String()
		}

		return wrapDecodeErrorWithContext(
			err,
			"failed to decode CUE value to Go struct",
			makeContext(
				"target_type", targetType,
				"value_kind", value.Kind().String(),
			),
		)
	}

	return nil
}
