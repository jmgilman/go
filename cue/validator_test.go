package cue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	platformerrors "github.com/jmgilman/go/errors"
)

func TestValidate_Success(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema
	schemaSource := `{
		name: string
		age: int & >=0 & <=150
		email: string & =~"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define valid data
	dataSource := `{
		name: "Alice"
		age: 30
		email: "alice@example.com"
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate - should succeed
	err := Validate(ctx, schema, data)
	if err != nil {
		t.Errorf("expected validation to succeed, got error: %v", err)
	}
}

func TestValidate_TypeMismatch(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema expecting an int
	schemaSource := `{
		age: int
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define data with wrong type (string instead of int)
	dataSource := `{
		age: "thirty"
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate - should fail
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for type mismatch")
	}

	// Check error code
	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// Check error message includes validation failure
	errMsg := platformErr.Error()
	if !strings.Contains(errMsg, "validation failed") {
		t.Errorf("expected error message to contain 'validation failed', got: %s", errMsg)
	}
}

func TestValidate_IncompleteValue(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema that expects concrete values
	schemaSource := `{
		age: int
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define data with an incomplete value (unbounded type)
	dataSource := `{
		age: int  // Incomplete - just declares type, not a concrete value
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// With default options (Concrete: true), this should fail
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for incomplete value with Concrete option")
	}

	// Check error code
	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// With Concrete: false, it should succeed
	opts := ValidationOptions{
		Concrete: false,
		Final:    false,
		All:      false,
	}
	err = ValidateWithOptions(ctx, schema, data, opts)
	if err != nil {
		t.Errorf("expected validation to succeed with Concrete: false, got error: %v", err)
	}
}

func TestValidate_ConstraintViolation(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema with constraints
	schemaSource := `{
		age: int & >=0 & <=150
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define data violating the constraint
	dataSource := `{
		age: 200
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate - should fail
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for constraint violation")
	}

	// Check error code
	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// Check that error context includes field path
	errMsg := platformErr.Error()
	if !strings.Contains(errMsg, "age") && !strings.Contains(errMsg, "field") {
		t.Logf("Warning: error message may not include field path: %s", errMsg)
	}
}

func TestValidate_NestedStructures(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema with nested structures
	schemaSource := `{
		person: {
			name: string
			address: {
				street: string
				city: string
				zip: string & =~"^[0-9]{5}$"
			}
		}
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Test 1: Valid nested data
	t.Run("valid nested data", func(t *testing.T) {
		dataSource := `{
			person: {
				name: "Charlie"
				address: {
					street: "123 Main St"
					city: "Springfield"
					zip: "12345"
				}
			}
		}`

		data := cueCtx.CompileString(dataSource)
		if err := data.Err(); err != nil {
			t.Fatalf("failed to compile data: %v", err)
		}

		err := Validate(ctx, schema, data)
		if err != nil {
			t.Errorf("expected validation to succeed for valid nested data, got error: %v", err)
		}
	})

	// Test 2: Invalid nested data (bad zip code)
	t.Run("invalid nested data", func(t *testing.T) {
		dataSource := `{
			person: {
				name: "Charlie"
				address: {
					street: "123 Main St"
					city: "Springfield"
					zip: "ABCDE"
				}
			}
		}`

		data := cueCtx.CompileString(dataSource)
		if err := data.Err(); err != nil {
			t.Fatalf("failed to compile data: %v", err)
		}

		err := Validate(ctx, schema, data)
		if err == nil {
			t.Fatal("expected validation to fail for invalid nested data")
		}

		var platformErr platformerrors.PlatformError
		if !errors.As(err, &platformErr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
		}
	})
}

func TestValidate_InvalidSchema(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Create an invalid schema (conflicting constraints)
	schemaSource := `{
		value: int & string
	}`

	schema := cueCtx.CompileString(schemaSource)
	// Note: schema.Err() will return an error

	// Define some data
	dataSource := `{
		value: 42
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate - should fail because schema is invalid
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for invalid schema")
	}

	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// Check error message mentions schema
	errMsg := platformErr.Error()
	if !strings.Contains(errMsg, "schema") {
		t.Errorf("expected error message to mention 'schema', got: %s", errMsg)
	}
}

func TestValidate_InvalidData(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a valid schema
	schemaSource := `{
		name: string
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Create invalid data (conflicting constraints)
	dataSource := `{
		name: int & string
	}`

	data := cueCtx.CompileString(dataSource)
	// Note: data.Err() will return an error

	// Validate - should fail because data is invalid
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for invalid data")
	}

	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// Check error message mentions data
	errMsg := platformErr.Error()
	if !strings.Contains(errMsg, "data") {
		t.Errorf("expected error message to mention 'data', got: %s", errMsg)
	}
}

func TestValidate_ContextCancellation(t *testing.T) {
	cueCtx := cuecontext.New()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Define simple schema and data
	schemaSource := `{
		value: int
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	dataSource := `{
		value: 42
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate with cancelled context - should fail
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail for cancelled context")
	}

	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}

	// Check error message mentions context
	errMsg := platformErr.Error()
	if !strings.Contains(errMsg, "context") && !strings.Contains(errMsg, "cancel") {
		t.Errorf("expected error message to mention context cancellation, got: %s", errMsg)
	}
}

func TestValidateConstraint_Success(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a value
	value := cueCtx.CompileString("42")
	if err := value.Err(); err != nil {
		t.Fatalf("failed to compile value: %v", err)
	}

	// Define a constraint that the value satisfies
	constraint := cueCtx.CompileString("int & >=0 & <=100")
	if err := constraint.Err(); err != nil {
		t.Fatalf("failed to compile constraint: %v", err)
	}

	// Validate constraint - should succeed
	err := ValidateConstraint(ctx, value, constraint)
	if err != nil {
		t.Errorf("expected constraint validation to succeed, got error: %v", err)
	}
}

func TestValidateConstraint_Failure(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a value
	value := cueCtx.CompileString("200")
	if err := value.Err(); err != nil {
		t.Fatalf("failed to compile value: %v", err)
	}

	// Define a constraint that the value violates
	constraint := cueCtx.CompileString("int & >=0 & <=100")
	if err := constraint.Err(); err != nil {
		t.Fatalf("failed to compile constraint: %v", err)
	}

	// Validate constraint - should fail
	err := ValidateConstraint(ctx, value, constraint)
	if err == nil {
		t.Fatal("expected constraint validation to fail")
	}

	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	if platformErr.Code() != platformerrors.CodeCUEValidationFailed {
		t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, platformErr.Code())
	}
}

func TestValidate_FieldPathsInContext(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema
	schemaSource := `{
		user: {
			name: string
			age: int & >=0 & <=150
		}
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define data with validation error
	dataSource := `{
		user: {
			name: "Dave"
			age: 200
		}
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate - should fail
	err := Validate(ctx, schema, data)
	if err == nil {
		t.Fatal("expected validation to fail")
	}

	// Check that error is a PlatformError
	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	// The error should include field paths in its context or message
	errMsg := platformErr.Error()
	t.Logf("Error message: %s", errMsg)

	// We expect either "age" or "user.age" to appear somewhere in the error
	if !strings.Contains(errMsg, "age") {
		t.Errorf("expected error message to contain field path 'age', got: %s", errMsg)
	}
}

func TestValidateWithOptions_ConcreteOption(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Schema that accepts an int
	schemaSource := `{
		age: int
	}`
	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Data with incomplete value
	dataSource := `{
		age: int
	}`
	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	t.Run("with concrete required", func(t *testing.T) {
		opts := ValidationOptions{
			Concrete: true,
			Final:    false,
			All:      false,
		}
		err := ValidateWithOptions(ctx, schema, data, opts)
		if err == nil {
			t.Fatal("expected validation to fail with Concrete: true for incomplete value")
		}
	})

	t.Run("without concrete required", func(t *testing.T) {
		opts := ValidationOptions{
			Concrete: false,
			Final:    false,
			All:      false,
		}
		err := ValidateWithOptions(ctx, schema, data, opts)
		if err != nil {
			t.Errorf("expected validation to succeed with Concrete: false, got error: %v", err)
		}
	})
}

func TestValidateWithOptions_DefaultValues(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Schema with default value
	schemaSource := `{
		name: string
		age: *25 | int
	}`
	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Data without age specified (should use default)
	dataSource := `{
		name: "Alice"
	}`
	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Should work with Final option to resolve defaults
	opts := ValidationOptions{
		Concrete: true,
		Final:    true,
		All:      false,
	}
	err := ValidateWithOptions(ctx, schema, data, opts)
	if err != nil {
		t.Errorf("expected validation to succeed with defaults, got error: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Define a schema with multiple constraints
	schemaSource := `{
		name: string & =~"^[A-Z].*"
		age: int & >=0 & <=150
		email: string & =~"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
		score: number & >=0 & <=100
	}`

	schema := cueCtx.CompileString(schemaSource)
	if err := schema.Err(); err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	// Define data that violates multiple constraints
	dataSource := `{
		name: "alice"        // Should start with uppercase
		age: 200             // Out of range
		email: "not-an-email" // Invalid format
		score: 150           // Out of range
	}`

	data := cueCtx.CompileString(dataSource)
	if err := data.Err(); err != nil {
		t.Fatalf("failed to compile data: %v", err)
	}

	// Validate with All: true to get all errors
	opts := ValidationOptions{
		Concrete: true,
		Final:    true,
		All:      true,
	}

	err := ValidateWithOptions(ctx, schema, data, opts)
	if err == nil {
		t.Fatal("expected validation to fail for multiple constraint violations")
	}

	// Check that we got a PlatformError
	var platformErr platformerrors.PlatformError
	if !errors.As(err, &platformErr) {
		t.Fatalf("expected PlatformError, got %T", err)
	}

	// The error message should indicate multiple errors
	errMsg := platformErr.Error()
	t.Logf("Error message: %s", errMsg)

	if !strings.Contains(errMsg, "more errors") {
		t.Errorf("expected error message to indicate multiple errors, got: %s", errMsg)
	}

	// Check the structured issues in the error context
	// The issues should be available in the error's context
	errCtx := platformErr.Context()
	if errCtx == nil {
		t.Fatal("expected error context to be present")
	}

	issues, ok := errCtx["issues"].([]ValidationIssue)
	if !ok {
		t.Fatalf("expected 'issues' in context to be []ValidationIssue, got %T", errCtx["issues"])
	}

	// We should have 4 validation issues (one for each field)
	if len(issues) != 4 {
		t.Errorf("expected 4 validation issues, got %d", len(issues))
	}

	// Verify that all expected fields are present in the issues
	fieldsFound := make(map[string]bool)
	for _, issue := range issues {
		if len(issue.Path) > 0 {
			fieldsFound[issue.Path[0]] = true
			t.Logf("Issue: field=%s, message=%s", issue.Path, issue.Message)
		}
	}

	expectedFields := []string{"name", "age", "email", "score"}
	for _, field := range expectedFields {
		if !fieldsFound[field] {
			t.Errorf("expected to find error for field %s, but it was not present in issues", field)
		}
	}
}

func TestValidateConstraintWithOptions(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	t.Run("success with concrete value", func(t *testing.T) {
		value := cueCtx.CompileString("42")
		constraint := cueCtx.CompileString("int & >=0 & <=100")

		opts := ValidationOptions{
			Concrete: true,
			Final:    false,
			All:      false,
		}

		err := ValidateConstraintWithOptions(ctx, value, constraint, opts)
		if err != nil {
			t.Errorf("expected constraint validation to succeed, got error: %v", err)
		}
	})

	t.Run("failure with incomplete value and concrete required", func(t *testing.T) {
		value := cueCtx.CompileString("int")
		constraint := cueCtx.CompileString("int & >=0 & <=100")

		opts := ValidationOptions{
			Concrete: true,
			Final:    false,
			All:      false,
		}

		err := ValidateConstraintWithOptions(ctx, value, constraint, opts)
		if err == nil {
			t.Fatal("expected constraint validation to fail for incomplete value with Concrete: true")
		}
	})
}
