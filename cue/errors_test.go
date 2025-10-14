package cue

import (
	"errors"
	"testing"

	platformerrors "github.com/jmgilman/go/errors"
)

// mockError is a test error for wrapping tests.
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

// TestWrapLoadError tests the wrapLoadError helper.
func TestWrapLoadError(t *testing.T) {
	t.Run("wraps error with correct code", func(t *testing.T) {
		origErr := &mockError{msg: "original error"}
		wrapped := wrapLoadError(origErr, "failed to load")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, wrapped.Code())
		}

		if wrapped.Message() != "failed to load" {
			t.Errorf("expected message 'failed to load', got %s", wrapped.Message())
		}

		// Check error chain preservation
		if !errors.Is(wrapped, origErr) {
			t.Error("error chain not preserved")
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapLoadError(nil, "message")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapLoadErrorf tests the wrapLoadErrorf helper.
func TestWrapLoadErrorf(t *testing.T) {
	t.Run("wraps error with formatted message", func(t *testing.T) {
		origErr := &mockError{msg: "original error"}
		wrapped := wrapLoadErrorf(origErr, "failed to load module %s", "example")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, wrapped.Code())
		}

		expectedMsg := "failed to load module example"
		if wrapped.Message() != expectedMsg {
			t.Errorf("expected message '%s', got %s", expectedMsg, wrapped.Message())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapLoadErrorf(nil, "format %s", "arg")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapLoadErrorWithContext tests the wrapLoadErrorWithContext helper.
func TestWrapLoadErrorWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		origErr := &mockError{msg: "original error"}
		ctx := map[string]interface{}{
			"module_path": "/foo/bar",
			"line":        42,
		}
		wrapped := wrapLoadErrorWithContext(origErr, "failed to load", ctx)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, wrapped.Code())
		}

		errCtx := wrapped.Context()
		if errCtx == nil {
			t.Fatal("expected non-nil context")
		}

		if errCtx["module_path"] != "/foo/bar" {
			t.Errorf("expected module_path '/foo/bar', got %v", errCtx["module_path"])
		}

		if errCtx["line"] != 42 {
			t.Errorf("expected line 42, got %v", errCtx["line"])
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapLoadErrorWithContext(nil, "message", map[string]interface{}{"key": "value"})
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})

	t.Run("handles nil context", func(t *testing.T) {
		origErr := &mockError{msg: "original error"}
		wrapped := wrapLoadErrorWithContext(origErr, "failed to load", nil)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		// Context should be nil
		if wrapped.Context() != nil {
			t.Error("expected nil context")
		}
	})
}

// TestWrapBuildError tests the wrapBuildError helper.
func TestWrapBuildError(t *testing.T) {
	t.Run("wraps error with correct code", func(t *testing.T) {
		origErr := &mockError{msg: "build error"}
		wrapped := wrapBuildError(origErr, "failed to build")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, wrapped.Code())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapBuildError(nil, "message")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapBuildErrorf tests the wrapBuildErrorf helper.
func TestWrapBuildErrorf(t *testing.T) {
	t.Run("wraps error with formatted message", func(t *testing.T) {
		origErr := &mockError{msg: "build error"}
		wrapped := wrapBuildErrorf(origErr, "failed to build instance %d", 123)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		expectedMsg := "failed to build instance 123"
		if wrapped.Message() != expectedMsg {
			t.Errorf("expected message '%s', got %s", expectedMsg, wrapped.Message())
		}
	})
}

// TestWrapBuildErrorWithContext tests the wrapBuildErrorWithContext helper.
func TestWrapBuildErrorWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		origErr := &mockError{msg: "build error"}
		ctx := map[string]interface{}{
			"instance": "test",
		}
		wrapped := wrapBuildErrorWithContext(origErr, "failed to build", ctx)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, wrapped.Code())
		}
	})
}

// TestWrapValidationError tests the wrapValidationError helper.
func TestWrapValidationError(t *testing.T) {
	t.Run("wraps error with correct code", func(t *testing.T) {
		origErr := &mockError{msg: "validation error"}
		wrapped := wrapValidationError(origErr, "validation failed")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEValidationFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, wrapped.Code())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapValidationError(nil, "message")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapValidationErrorf tests the wrapValidationErrorf helper.
func TestWrapValidationErrorf(t *testing.T) {
	t.Run("wraps error with formatted message", func(t *testing.T) {
		origErr := &mockError{msg: "validation error"}
		wrapped := wrapValidationErrorf(origErr, "validation failed for field %s", "email")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		expectedMsg := "validation failed for field email"
		if wrapped.Message() != expectedMsg {
			t.Errorf("expected message '%s', got %s", expectedMsg, wrapped.Message())
		}
	})
}

// TestWrapValidationErrorWithContext tests the wrapValidationErrorWithContext helper.
func TestWrapValidationErrorWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		origErr := &mockError{msg: "validation error"}
		ctx := map[string]interface{}{
			"field": "email",
			"value": "invalid",
		}
		wrapped := wrapValidationErrorWithContext(origErr, "validation failed", ctx)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEValidationFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, wrapped.Code())
		}

		errCtx := wrapped.Context()
		if errCtx["field"] != "email" {
			t.Errorf("expected field 'email', got %v", errCtx["field"])
		}
	})
}

// TestWrapDecodeError tests the wrapDecodeError helper.
func TestWrapDecodeError(t *testing.T) {
	t.Run("wraps error with correct code", func(t *testing.T) {
		origErr := &mockError{msg: "decode error"}
		wrapped := wrapDecodeError(origErr, "failed to decode")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEDecodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEDecodeFailed, wrapped.Code())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapDecodeError(nil, "message")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapDecodeErrorf tests the wrapDecodeErrorf helper.
func TestWrapDecodeErrorf(t *testing.T) {
	t.Run("wraps error with formatted message", func(t *testing.T) {
		origErr := &mockError{msg: "decode error"}
		wrapped := wrapDecodeErrorf(origErr, "failed to decode type %s", "MyStruct")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		expectedMsg := "failed to decode type MyStruct"
		if wrapped.Message() != expectedMsg {
			t.Errorf("expected message '%s', got %s", expectedMsg, wrapped.Message())
		}
	})
}

// TestWrapDecodeErrorWithContext tests the wrapDecodeErrorWithContext helper.
func TestWrapDecodeErrorWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		origErr := &mockError{msg: "decode error"}
		ctx := map[string]interface{}{
			"type": "MyStruct",
		}
		wrapped := wrapDecodeErrorWithContext(origErr, "failed to decode", ctx)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEDecodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEDecodeFailed, wrapped.Code())
		}
	})
}

// TestWrapEncodeError tests the wrapEncodeError helper.
func TestWrapEncodeError(t *testing.T) {
	t.Run("wraps error with correct code", func(t *testing.T) {
		origErr := &mockError{msg: "encode error"}
		wrapped := wrapEncodeError(origErr, "failed to encode")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEEncodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEEncodeFailed, wrapped.Code())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := wrapEncodeError(nil, "message")
		if wrapped != nil {
			t.Error("expected nil, got non-nil error")
		}
	})
}

// TestWrapEncodeErrorf tests the wrapEncodeErrorf helper.
func TestWrapEncodeErrorf(t *testing.T) {
	t.Run("wraps error with formatted message", func(t *testing.T) {
		origErr := &mockError{msg: "encode error"}
		wrapped := wrapEncodeErrorf(origErr, "failed to encode to %s", "YAML")

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		expectedMsg := "failed to encode to YAML"
		if wrapped.Message() != expectedMsg {
			t.Errorf("expected message '%s', got %s", expectedMsg, wrapped.Message())
		}
	})
}

// TestWrapEncodeErrorWithContext tests the wrapEncodeErrorWithContext helper.
func TestWrapEncodeErrorWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		origErr := &mockError{msg: "encode error"}
		ctx := map[string]interface{}{
			"format": "YAML",
		}
		wrapped := wrapEncodeErrorWithContext(origErr, "failed to encode", ctx)

		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}

		if wrapped.Code() != platformerrors.CodeCUEEncodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEEncodeFailed, wrapped.Code())
		}
	})
}

// TestExtractErrorContext tests the extractErrorContext helper.
func TestExtractErrorContext(t *testing.T) {
	t.Run("creates context from key-value pairs", func(t *testing.T) {
		ctx := extractErrorContext("key1", "value1", "key2", 42, "key3", true)

		if ctx == nil {
			t.Fatal("expected non-nil context")
		}

		if ctx["key1"] != "value1" {
			t.Errorf("expected key1 'value1', got %v", ctx["key1"])
		}

		if ctx["key2"] != 42 {
			t.Errorf("expected key2 42, got %v", ctx["key2"])
		}

		if ctx["key3"] != true {
			t.Errorf("expected key3 true, got %v", ctx["key3"])
		}
	})

	t.Run("returns nil for empty pairs", func(t *testing.T) {
		ctx := extractErrorContext()
		if ctx != nil {
			t.Error("expected nil context for empty pairs")
		}
	})

	t.Run("handles odd number of arguments", func(t *testing.T) {
		ctx := extractErrorContext("key1", "value1", "key2")

		if ctx == nil {
			t.Fatal("expected non-nil context")
		}

		if ctx["key1"] != "value1" {
			t.Errorf("expected key1 'value1', got %v", ctx["key1"])
		}

		// key2 should not be in the map
		if _, exists := ctx["key2"]; exists {
			t.Error("expected key2 to not exist")
		}
	})

	t.Run("skips non-string keys", func(t *testing.T) {
		ctx := extractErrorContext(42, "value1", "key2", "value2")

		if ctx == nil {
			t.Fatal("expected non-nil context")
		}

		// Should only have key2
		if len(ctx) != 1 {
			t.Errorf("expected 1 key, got %d", len(ctx))
		}

		if ctx["key2"] != "value2" {
			t.Errorf("expected key2 'value2', got %v", ctx["key2"])
		}
	})

	t.Run("returns nil when all keys are non-string", func(t *testing.T) {
		ctx := extractErrorContext(42, "value1", 43, "value2")

		if ctx != nil {
			t.Error("expected nil context when all keys are non-string")
		}
	})
}

// TestMakeContext tests the makeContext helper.
func TestMakeContext(t *testing.T) {
	t.Run("creates context from key-value pairs", func(t *testing.T) {
		ctx := makeContext("path", "/foo/bar", "line", 10)

		if ctx == nil {
			t.Fatal("expected non-nil context")
		}

		if ctx["path"] != "/foo/bar" {
			t.Errorf("expected path '/foo/bar', got %v", ctx["path"])
		}

		if ctx["line"] != 10 {
			t.Errorf("expected line 10, got %v", ctx["line"])
		}
	})

	t.Run("returns nil for empty arguments", func(t *testing.T) {
		ctx := makeContext()
		if ctx != nil {
			t.Error("expected nil context")
		}
	})
}

// TestFormatFieldPath tests the formatFieldPath helper.
func TestFormatFieldPath(t *testing.T) {
	t.Run("formats non-empty path", func(t *testing.T) {
		result := formatFieldPath("user.email")
		expected := "field user.email"
		if result != expected {
			t.Errorf("expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("formats empty path", func(t *testing.T) {
		result := formatFieldPath("")
		expected := "<root>"
		if result != expected {
			t.Errorf("expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("formats single element path", func(t *testing.T) {
		result := formatFieldPath("email")
		expected := "field email"
		if result != expected {
			t.Errorf("expected '%s', got '%s'", expected, result)
		}
	})
}

// TestErrorChainPreservation tests that error chains are preserved.
func TestErrorChainPreservation(t *testing.T) {
	t.Run("error chain preserved through multiple wraps", func(t *testing.T) {
		// Create a chain: original -> load wrap -> build wrap
		origErr := &mockError{msg: "original error"}
		loadWrapped := wrapLoadError(origErr, "load failed")
		buildWrapped := wrapBuildError(loadWrapped, "build failed")

		// Should be able to unwrap back to original
		if !errors.Is(buildWrapped, origErr) {
			t.Error("error chain not preserved through multiple wraps")
		}

		if !errors.Is(buildWrapped, loadWrapped) {
			t.Error("error chain not preserved to intermediate wrap")
		}
	})
}

// TestNilErrorHandling tests that all helpers handle nil errors correctly.
func TestNilErrorHandling(t *testing.T) {
	tests := []struct {
		name string
		fn   func() platformerrors.PlatformError
	}{
		{"wrapLoadError", func() platformerrors.PlatformError { return wrapLoadError(nil, "msg") }},
		{"wrapLoadErrorf", func() platformerrors.PlatformError { return wrapLoadErrorf(nil, "msg") }},
		{"wrapLoadErrorWithContext", func() platformerrors.PlatformError {
			return wrapLoadErrorWithContext(nil, "msg", nil)
		}},
		{"wrapBuildError", func() platformerrors.PlatformError { return wrapBuildError(nil, "msg") }},
		{"wrapBuildErrorf", func() platformerrors.PlatformError { return wrapBuildErrorf(nil, "msg") }},
		{"wrapBuildErrorWithContext", func() platformerrors.PlatformError {
			return wrapBuildErrorWithContext(nil, "msg", nil)
		}},
		{"wrapValidationError", func() platformerrors.PlatformError { return wrapValidationError(nil, "msg") }},
		{"wrapValidationErrorf", func() platformerrors.PlatformError { return wrapValidationErrorf(nil, "msg") }},
		{"wrapValidationErrorWithContext", func() platformerrors.PlatformError {
			return wrapValidationErrorWithContext(nil, "msg", nil)
		}},
		{"wrapDecodeError", func() platformerrors.PlatformError { return wrapDecodeError(nil, "msg") }},
		{"wrapDecodeErrorf", func() platformerrors.PlatformError { return wrapDecodeErrorf(nil, "msg") }},
		{"wrapDecodeErrorWithContext", func() platformerrors.PlatformError {
			return wrapDecodeErrorWithContext(nil, "msg", nil)
		}},
		{"wrapEncodeError", func() platformerrors.PlatformError { return wrapEncodeError(nil, "msg") }},
		{"wrapEncodeErrorf", func() platformerrors.PlatformError { return wrapEncodeErrorf(nil, "msg") }},
		{"wrapEncodeErrorWithContext", func() platformerrors.PlatformError {
			return wrapEncodeErrorWithContext(nil, "msg", nil)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn()
			if result != nil {
				t.Errorf("%s: expected nil for nil error, got %v", tt.name, result)
			}
		})
	}
}
