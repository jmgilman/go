package errors

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlatformError_Error(t *testing.T) {
	err := New(CodeNotFound, "resource not found")
	want := "[NOT_FOUND] resource not found"
	require.Equal(t, want, err.Error())
}

func TestPlatformError_Error_WithCause(t *testing.T) {
	cause := stderrors.New("connection refused")
	err := Wrap(cause, CodeNetwork, "failed to connect")

	require.Contains(t, err.Error(), "[NETWORK_ERROR]")
	require.Contains(t, err.Error(), "failed to connect")
	require.Contains(t, err.Error(), "connection refused")
}

func TestPlatformError_Code(t *testing.T) {
	tests := []struct {
		name string
		code ErrorCode
	}{
		{"not found", CodeNotFound},
		{"invalid input", CodeInvalidInput},
		{"timeout", CodeTimeout},
		{"network", CodeNetwork},
		{"build failed", CodeBuildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test message")
			require.Equal(t, tt.code, err.Code())
		})
	}
}

func TestPlatformError_Classification(t *testing.T) {
	tests := []struct {
		name          string
		code          ErrorCode
		wantRetryable bool
	}{
		{"timeout is retryable", CodeTimeout, true},
		{"not found is permanent", CodeNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test")
			require.Equal(t, tt.wantRetryable, err.Classification().IsRetryable())
		})
	}
}

func TestPlatformError_Message(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"simple message", "resource not found"},
		{"long message", "this is a very long error message with lots of details"},
		{"empty message", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(CodeNotFound, tt.message)
			require.Equal(t, tt.message, err.Message())
		})
	}
}

func TestPlatformError_Context_DefensiveCopy(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "key", "value")

	// Get context
	ctx := err.Context()
	require.NotNil(t, ctx)
	require.Equal(t, "value", ctx["key"])

	// Mutate returned context
	ctx["key"] = "modified"
	ctx["new_key"] = "new_value"

	// Verify original unchanged
	ctx2 := err.Context()
	require.Equal(t, "value", ctx2["key"])
	require.NotContains(t, ctx2, "new_key")
}

func TestPlatformError_Context_Nil(t *testing.T) {
	err := New(CodeInternal, "internal error")
	require.Nil(t, err.Context())
}

func TestPlatformError_Context_Immutability(t *testing.T) {
	// Create error with context
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "project", "api")
	err = WithContext(err, "phase", "test")

	// Get context and mutate it
	ctx := err.Context()
	ctx["project"] = "modified"
	ctx["new_field"] = "new_value"
	delete(ctx, "phase")

	// Verify error context is unchanged
	ctx2 := err.Context()
	require.Equal(t, "api", ctx2["project"])
	require.Equal(t, "test", ctx2["phase"])
	require.NotContains(t, ctx2, "new_field")
}

func TestPlatformError_Unwrap(t *testing.T) {
	cause := stderrors.New("original error")
	err := Wrap(cause, CodeDatabase, "database error")

	require.Equal(t, cause, err.Unwrap())
}

func TestPlatformError_Unwrap_NoWrap(t *testing.T) {
	err := New(CodeNotFound, "not found")
	require.Nil(t, err.Unwrap())
}

func TestPlatformError_Unwrap_Chain(t *testing.T) {
	// Create error chain
	original := stderrors.New("root cause")
	wrapped1 := Wrap(original, CodeDatabase, "database error")
	wrapped2 := Wrap(wrapped1, CodeInternal, "internal error")

	// Verify unwrap chain
	require.Equal(t, wrapped1, wrapped2.Unwrap())

	// Safely unwrap to check root cause
	var platformErr PlatformError
	require.True(t, stderrors.As(wrapped2.Unwrap(), &platformErr))
	require.Equal(t, original, platformErr.Unwrap())
}
