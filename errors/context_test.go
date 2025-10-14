package errors

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithContext(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "project", "api")

	ctx := err.Context()
	require.NotNil(t, ctx)
	require.Equal(t, "api", ctx["project"])
}

func TestWithContext_Chaining(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "project", "api")
	err = WithContext(err, "phase", "test")
	err = WithContext(err, "exit_code", 1)

	ctx := err.Context()
	require.Len(t, ctx, 3)
	require.Equal(t, "api", ctx["project"])
	require.Equal(t, "test", ctx["phase"])
	require.Equal(t, 1, ctx["exit_code"])
}

func TestWithContext_StandardError(t *testing.T) {
	stdErr := stderrors.New("standard error")
	err := WithContext(stdErr, "key", "value")

	require.Equal(t, CodeUnknown, err.Code())
	require.Equal(t, ClassificationPermanent, err.Classification())
	require.Equal(t, stdErr, err.Unwrap())

	ctx := err.Context()
	require.Equal(t, "value", ctx["key"])
}

func TestWithContext_NilError(t *testing.T) {
	err := WithContext(nil, "key", "value")
	require.Nil(t, err)
}

func TestWithContext_Immutability(t *testing.T) {
	original := New(CodeInternal, "internal")
	modified := WithContext(original, "key", "value")

	// Original unchanged
	require.Nil(t, original.Context())

	// Modified has context
	require.NotNil(t, modified.Context())
}

func TestWithContextMap(t *testing.T) {
	err := New(CodeExecutionFailed, "execution failed")
	ctx := map[string]interface{}{
		"command": "earthly",
		"target":  "+test",
	}

	err = WithContextMap(err, ctx)

	errCtx := err.Context()
	require.Equal(t, "earthly", errCtx["command"])
	require.Equal(t, "+test", errCtx["target"])
}

func TestWithContextMap_Merge(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "project", "api")

	err = WithContextMap(err, map[string]interface{}{
		"phase":     "test",
		"exit_code": 1,
	})

	ctx := err.Context()
	require.Len(t, ctx, 3)
	require.Equal(t, "api", ctx["project"])
	require.Equal(t, "test", ctx["phase"])
	require.Equal(t, 1, ctx["exit_code"])
}

func TestWithContextMap_Override(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "key", "original")

	err = WithContextMap(err, map[string]interface{}{
		"key": "overridden",
	})

	ctx := err.Context()
	require.Equal(t, "overridden", ctx["key"])
}

func TestWithContextMap_DefensiveCopy(t *testing.T) {
	err := New(CodeInternal, "internal")
	ctx := map[string]interface{}{"key": "value"}

	err = WithContextMap(err, ctx)

	// Mutate original map
	ctx["key"] = "modified"
	ctx["new"] = "value"

	// Verify error context unchanged
	errCtx := err.Context()
	require.Equal(t, "value", errCtx["key"])
	require.NotContains(t, errCtx, "new")
}

func TestWithClassification(t *testing.T) {
	// Create permanent error
	err := New(CodeNotFound, "not found")
	require.Equal(t, ClassificationPermanent, err.Classification())

	// Override to retryable
	err = WithClassification(err, ClassificationRetryable)
	require.Equal(t, ClassificationRetryable, err.Classification())
}

func TestWithClassification_StandardError(t *testing.T) {
	stdErr := stderrors.New("standard error")
	err := WithClassification(stdErr, ClassificationRetryable)

	require.Equal(t, CodeUnknown, err.Code())
	require.Equal(t, ClassificationRetryable, err.Classification())
}

func TestWithClassification_PreservesContext(t *testing.T) {
	err := New(CodeInternal, "internal")
	err = WithContext(err, "key", "value")
	err = WithClassification(err, ClassificationRetryable)

	require.Equal(t, "value", err.Context()["key"])
}
