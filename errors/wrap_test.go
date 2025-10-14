package errors

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrap(t *testing.T) {
	cause := stderrors.New("original error")
	err := Wrap(cause, CodeDatabase, "operation failed")

	require.NotNil(t, err)
	require.Equal(t, CodeDatabase, err.Code())
	require.Equal(t, "operation failed", err.Message())
	require.Equal(t, cause, err.Unwrap())
}

func TestWrap_NilError(t *testing.T) {
	err := Wrap(nil, CodeNotFound, "test")
	require.Nil(t, err)
}

func TestWrap_PreservesClassification(t *testing.T) {
	// Create retryable error
	original := New(CodeTimeout, "timeout")
	require.True(t, original.Classification().IsRetryable())

	// Wrap with different code
	wrapped := Wrap(original, CodeDatabase, "database timeout")

	// Classification should be preserved from original
	require.True(t, wrapped.Classification().IsRetryable())
}

func TestWrap_StandardError(t *testing.T) {
	stdErr := stderrors.New("standard error")
	wrapped := Wrap(stdErr, CodeInternal, "internal error")

	// Should use default classification
	require.Equal(t, ClassificationPermanent, wrapped.Classification())
	require.Equal(t, stdErr, wrapped.Unwrap())
}

func TestWrap_PreservesClassification_Permanent(t *testing.T) {
	// Create permanent error
	original := New(CodeNotFound, "not found")
	require.False(t, original.Classification().IsRetryable())

	// Wrap with retryable code (but should preserve permanent)
	wrapped := Wrap(original, CodeTimeout, "timeout looking for resource")

	// Classification should be preserved from original (permanent)
	require.False(t, wrapped.Classification().IsRetryable())
}

func TestWrapf(t *testing.T) {
	cause := stderrors.New("connection refused")
	err := Wrapf(cause, CodeNetwork, "failed to connect to %s:%d", "localhost", 5432)

	require.Equal(t, "failed to connect to localhost:5432", err.Message())
	require.Equal(t, cause, err.Unwrap())
}

func TestWrapf_NilError(t *testing.T) {
	err := Wrapf(nil, CodeNotFound, "test %s", "arg")
	require.Nil(t, err)
}

func TestWrapf_Formatting(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []interface{}
		want   string
	}{
		{
			name:   "string formatting",
			format: "user %s not found",
			args:   []interface{}{"john"},
			want:   "user john not found",
		},
		{
			name:   "int formatting",
			format: "invalid port: %d",
			args:   []interface{}{99999},
			want:   "invalid port: 99999",
		},
		{
			name:   "multiple args",
			format: "connection to %s:%d failed after %d attempts",
			args:   []interface{}{"localhost", 5432, 3},
			want:   "connection to localhost:5432 failed after 3 attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := stderrors.New("cause")
			err := Wrapf(cause, CodeNetwork, tt.format, tt.args...)
			require.Equal(t, tt.want, err.Message())
		})
	}
}

func TestWrapWithContext(t *testing.T) {
	cause := stderrors.New("build failed")
	ctx := map[string]interface{}{
		"project": "api",
		"phase":   "test",
	}

	err := WrapWithContext(cause, CodeBuildFailed, "build failed", ctx)

	require.NotNil(t, err)
	require.Equal(t, CodeBuildFailed, err.Code())
	require.Equal(t, cause, err.Unwrap())

	errCtx := err.Context()
	require.Equal(t, "api", errCtx["project"])
	require.Equal(t, "test", errCtx["phase"])
}

func TestWrapWithContext_DefensiveCopy(t *testing.T) {
	cause := stderrors.New("error")
	ctx := map[string]interface{}{"key": "value"}

	err := WrapWithContext(cause, CodeInternal, "test", ctx)

	// Mutate original context
	ctx["key"] = "modified"
	ctx["new"] = "value"

	// Verify error context unchanged
	errCtx := err.Context()
	require.Equal(t, "value", errCtx["key"])
	require.NotContains(t, errCtx, "new")
}

func TestWrapWithContext_NilError(t *testing.T) {
	err := WrapWithContext(nil, CodeNotFound, "test", map[string]interface{}{"key": "value"})
	require.Nil(t, err)
}

func TestWrapWithContext_NilContext(t *testing.T) {
	cause := stderrors.New("error")
	err := WrapWithContext(cause, CodeInternal, "test", nil)

	require.NotNil(t, err)
	require.Nil(t, err.Context())
}

func TestWrapWithContext_EmptyContext(t *testing.T) {
	cause := stderrors.New("error")
	err := WrapWithContext(cause, CodeInternal, "test", map[string]interface{}{})

	require.NotNil(t, err)
	require.NotNil(t, err.Context())
	require.Empty(t, err.Context())
}

func TestWrap_ErrorChain(t *testing.T) {
	// Create a chain of wrapped errors
	root := stderrors.New("root cause")
	level1 := Wrap(root, CodeDatabase, "database error")
	level2 := Wrap(level1, CodeInternal, "internal error")

	// Verify chain integrity
	require.Equal(t, level1, level2.Unwrap())
	require.Equal(t, root, level1.Unwrap())

	// Verify error string includes all levels
	errStr := level2.Error()
	require.Contains(t, errStr, "INTERNAL_ERROR")
	require.Contains(t, errStr, "internal error")
	require.Contains(t, errStr, "database error")
}
