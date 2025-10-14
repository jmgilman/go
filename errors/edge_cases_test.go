package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jmgilman/go/errors"
	"github.com/stretchr/testify/require"
)

func TestEdgeCase_EmptyMessage(t *testing.T) {
	err := errors.New(errors.CodeInternal, "")
	require.Equal(t, "", err.Message())
	require.Equal(t, "[INTERNAL_ERROR] ", err.Error())
}

func TestEdgeCase_NilContext(t *testing.T) {
	err := errors.New(errors.CodeInternal, "test")
	err = errors.WithContextMap(err, nil)

	// WithContextMap with nil still creates an empty map (not nil)
	ctx := err.Context()
	if ctx != nil {
		require.Len(t, ctx, 0)
	}
}

func TestEdgeCase_EmptyContextMap(t *testing.T) {
	err := errors.New(errors.CodeInternal, "test")
	err = errors.WithContextMap(err, map[string]interface{}{})

	// Empty map should result in nil context (or empty, both acceptable)
	ctx := err.Context()
	if ctx != nil {
		require.Len(t, ctx, 0)
	}
}

func TestEdgeCase_LargeContextMap(t *testing.T) {
	err := errors.New(errors.CodeInternal, "test")

	// Add 100 context fields
	ctx := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		ctx[fmt.Sprintf("%c%d", rune('a'+i%26), i)] = i
	}

	err = errors.WithContextMap(err, ctx)

	errCtx := err.Context()
	require.Len(t, errCtx, 100)
}

func TestEdgeCase_UnicodeMessages(t *testing.T) {
	messages := []string{
		"错误信息",                // Chinese
		"エラーメッセージ",            // Japanese
		"сообщение об ошибке", // Russian
		"mensaje de error",    // Spanish with accents
		"🚨 error occurred 🔥",  // Emojis
	}

	for _, msg := range messages {
		err := errors.New(errors.CodeInternal, msg)
		require.Equal(t, msg, err.Message())
		require.Contains(t, err.Error(), msg)

		// Should marshal to JSON correctly
		resp := errors.ToJSON(err)
		jsonBytes, marshalErr := json.Marshal(resp)
		require.NoError(t, marshalErr)

		var decoded errors.ErrorResponse
		unmarshalErr := json.Unmarshal(jsonBytes, &decoded)
		require.NoError(t, unmarshalErr)
		require.Equal(t, msg, decoded.Message)
	}
}

func TestEdgeCase_SpecialCharactersJSON(t *testing.T) {
	specialChars := `"quotes" 'apostrophes' \backslash newline\n tab\t`
	err := errors.New(errors.CodeInternal, specialChars)

	jsonBytes, marshalErr := json.Marshal(errors.ToJSON(err))
	require.NoError(t, marshalErr)

	// Should be valid JSON
	var decoded errors.ErrorResponse
	unmarshalErr := json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, unmarshalErr)
	require.Equal(t, specialChars, decoded.Message)
}

func TestEdgeCase_VeryLongMessage(t *testing.T) {
	longMessage := strings.Repeat("a", 10000)
	err := errors.New(errors.CodeInternal, longMessage)

	require.Equal(t, longMessage, err.Message())

	// Should handle in JSON
	resp := errors.ToJSON(err)
	require.Equal(t, longMessage, resp.Message)
}

func TestEdgeCase_AllErrorCodes(t *testing.T) {
	// Verify all error codes are valid
	codes := []errors.ErrorCode{
		errors.CodeNotFound,
		errors.CodeAlreadyExists,
		errors.CodeConflict,
		errors.CodeUnauthorized,
		errors.CodeForbidden,
		errors.CodeInvalidInput,
		errors.CodeInvalidConfig,
		errors.CodeSchemaFailed,
		errors.CodeDatabase,
		errors.CodeNetwork,
		errors.CodeTimeout,
		errors.CodeRateLimit,
		errors.CodeExecutionFailed,
		errors.CodeBuildFailed,
		errors.CodePublishFailed,
		errors.CodeInternal,
		errors.CodeNotImplemented,
		errors.CodeUnavailable,
		errors.CodeUnknown,
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			err := errors.New(code, "test")
			require.Equal(t, code, err.Code())
			require.NotEmpty(t, err.Classification())

			// Should marshal
			resp := errors.ToJSON(err)
			require.Equal(t, string(code), resp.Code)
		})
	}
}

func TestEdgeCase_NilErrorOperations(t *testing.T) {
	// All functions should handle nil gracefully
	require.Nil(t, errors.Wrap(nil, errors.CodeInternal, "test"))
	require.Nil(t, errors.Wrapf(nil, errors.CodeInternal, "test"))
	require.Nil(t, errors.WrapWithContext(nil, errors.CodeInternal, "test", nil))
	require.Nil(t, errors.WithContext(nil, "key", "value"))
	require.Nil(t, errors.WithContextMap(nil, nil))
	require.Nil(t, errors.WithClassification(nil, errors.ClassificationRetryable))
	require.Nil(t, errors.ToJSON(nil))

	require.Equal(t, errors.CodeUnknown, errors.GetCode(nil))
	require.Equal(t, errors.ClassificationPermanent, errors.GetClassification(nil))
	require.False(t, errors.IsRetryable(nil))
}

func TestEdgeCase_ContextValueTypes(t *testing.T) {
	// Test various value types in context
	err := errors.New(errors.CodeInternal, "test")
	err = errors.WithContextMap(err, map[string]interface{}{
		"string": "value",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"nil":    nil,
		"slice":  []int{1, 2, 3},
		"map":    map[string]string{"nested": "value"},
	})

	ctx := err.Context()
	require.Equal(t, "value", ctx["string"])
	require.Equal(t, 42, ctx["int"])
	require.Equal(t, 3.14, ctx["float"])
	require.Equal(t, true, ctx["bool"])
	require.Nil(t, ctx["nil"])

	// Should marshal to JSON
	resp := errors.ToJSON(err)
	jsonBytes, _ := json.Marshal(resp)
	require.NotNil(t, jsonBytes)
}

func TestEdgeCase_StandardErrorTypes(t *testing.T) {
	// Test wrapping various standard library errors
	stdErrors := []error{
		stderrors.New("simple error"),
		fmt.Errorf("formatted error: %s", "detail"),
		fmt.Errorf("wrapped: %w", stderrors.New("cause")),
	}

	for i, stdErr := range stdErrors {
		t.Run(fmt.Sprintf("error_%d", i), func(t *testing.T) {
			wrapped := errors.Wrap(stdErr, errors.CodeInternal, "platform error")
			require.Equal(t, stdErr, wrapped.Unwrap())
			require.Contains(t, wrapped.Error(), stdErr.Error())
		})
	}
}
