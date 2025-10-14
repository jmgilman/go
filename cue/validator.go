package cue

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/token"
)

// ValidationOptions configures validation behavior.
type ValidationOptions struct {
	// Concrete requires all values to be concrete (fully specified).
	// If true, incomplete values will cause validation to fail.
	Concrete bool

	// Final resolves default values before validation.
	Final bool

	// All reports all errors instead of stopping at the first one.
	All bool
}

// DefaultValidationOptions returns sensible default validation options.
// By default, we require concrete values and finalize defaults.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Concrete: true,
		Final:    true,
		All:      true,
	}
}

// ValidationIssue represents a single validation error with structured information.
type ValidationIssue struct {
	// Path is the field path where the error occurred (e.g., ["user", "age"]).
	Path []string

	// Message is the human-readable error message.
	Message string

	// Position is the source position if available.
	Position token.Pos
}

// Validate validates a CUE value against a schema using default options.
// Returns nil if validation succeeds, PlatformError with detailed messages if it fails.
// Both schema and data are generic cue.Value - no coupling to specific schema packages.
//
// This is a convenience wrapper around ValidateWithOptions that uses DefaultValidationOptions().
func Validate(ctx context.Context, schema cue.Value, data cue.Value) error {
	return ValidateWithOptions(ctx, schema, data, DefaultValidationOptions())
}

// ValidateWithOptions validates a CUE value against a schema with custom options.
// Returns nil if validation succeeds, PlatformError with detailed messages if it fails.
//
// The validation process:
// 1. Check for basic errors in schema and data
// 2. Unify the schema and data values
// 3. Validate the unified result with specified options
// 4. Extract structured error information on failure
//
// Returns CodeCUEValidationFailed on validation failure.
func ValidateWithOptions(ctx context.Context, schema cue.Value, data cue.Value, opts ValidationOptions) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return wrapValidationErrorWithContext(err, "context cancelled", nil)
	}

	// Check if schema is valid
	if err := schema.Err(); err != nil {
		details := cueerrors.Details(err, nil)
		issues := extractValidationIssues(err)

		return wrapValidationErrorWithContext(
			err,
			"schema is invalid",
			makeContext(
				"schema_error", details,
				"issues", issues,
			),
		)
	}

	// Check if data is valid
	if err := data.Err(); err != nil {
		details := cueerrors.Details(err, nil)
		issues := extractValidationIssues(err)

		return wrapValidationErrorWithContext(
			err,
			"data is invalid",
			makeContext(
				"data_error", details,
				"issues", issues,
			),
		)
	}

	// Unify schema and data
	unified := schema.Unify(data)

	// Build validation options for CUE
	var cueOpts []cue.Option
	if opts.Concrete {
		cueOpts = append(cueOpts, cue.Concrete(true))
	}
	if opts.Final {
		cueOpts = append(cueOpts, cue.Final())
	}
	if opts.All {
		cueOpts = append(cueOpts, cue.All())
	}

	// Validate the unified result
	// Note: We skip the unified.Err() check and go straight to Validate()
	// so that the All option can collect all errors at once.
	if err := unified.Validate(cueOpts...); err != nil {
		details := cueerrors.Details(err, nil)
		issues := extractValidationIssues(err)
		positions := cueerrors.Positions(err)

		return wrapValidationErrorWithContext(
			err,
			"validation failed",
			makeContext(
				"details", details,
				"issues", issues,
				"positions", positions,
			),
		)
	}

	return nil
}

// extractValidationIssues extracts structured validation issues from a CUE error.
// Uses the cue/errors package to properly parse error information.
func extractValidationIssues(err error) []ValidationIssue {
	if err == nil {
		return nil
	}

	var issues []ValidationIssue
	for _, e := range cueerrors.Errors(err) {
		// Get the formatted message
		fmtStr, args := e.Msg()
		message := fmt.Sprintf(fmtStr, args...)

		// Get the field path
		path := e.Path()

		// Get position if available
		var pos token.Pos
		positions := e.InputPositions()
		if len(positions) > 0 {
			pos = positions[0]
		}

		issues = append(issues, ValidationIssue{
			Path:     path,
			Message:  message,
			Position: pos,
		})
	}

	return issues
}

// ValidateConstraint validates that a value satisfies a specific constraint using default options.
// This is a convenience wrapper around ValidateConstraintWithOptions.
func ValidateConstraint(ctx context.Context, value cue.Value, constraint cue.Value) error {
	return ValidateConstraintWithOptions(ctx, value, constraint, DefaultValidationOptions())
}

// ValidateConstraintWithOptions validates that a value satisfies a specific constraint.
// This is a helper function for validating individual constraints with custom options.
// Returns nil if the constraint is satisfied.
func ValidateConstraintWithOptions(ctx context.Context, value cue.Value, constraint cue.Value, opts ValidationOptions) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return wrapValidationErrorWithContext(err, "context cancelled", nil)
	}

	// Unify the value with the constraint
	unified := value.Unify(constraint)

	// Build validation options for CUE
	var cueOpts []cue.Option
	if opts.Concrete {
		cueOpts = append(cueOpts, cue.Concrete(true))
	}
	if opts.Final {
		cueOpts = append(cueOpts, cue.Final())
	}
	if opts.All {
		cueOpts = append(cueOpts, cue.All())
	}

	// Validate the unified result
	// Note: We skip the unified.Err() check and go straight to Validate()
	// so that the All option can collect all errors at once.
	if err := unified.Validate(cueOpts...); err != nil {
		details := cueerrors.Details(err, nil)
		issues := extractValidationIssues(err)
		positions := cueerrors.Positions(err)

		return wrapValidationErrorWithContext(
			err,
			"constraint validation failed",
			makeContext(
				"details", details,
				"issues", issues,
				"positions", positions,
			),
		)
	}

	return nil
}
