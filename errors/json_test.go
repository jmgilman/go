package errors

import (
	"encoding/json"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToJSON(t *testing.T) {
	err := New(CodeNotFound, "resource not found")
	resp := ToJSON(err)

	require.NotNil(t, resp)
	require.Equal(t, "NOT_FOUND", resp.Code)
	require.Equal(t, "resource not found", resp.Message)
	require.Equal(t, "PERMANENT", resp.Classification)
	require.Nil(t, resp.Context)
}

func TestToJSON_PlatformError(t *testing.T) {
	err := New(CodeBuildFailed, "build failed")
	err = WithContext(err, "project", "api")
	err = WithContext(err, "phase", "test")

	resp := ToJSON(err)

	require.Equal(t, "BUILD_FAILED", resp.Code)
	require.Equal(t, "build failed", resp.Message)
	require.Equal(t, "PERMANENT", resp.Classification)
	require.NotNil(t, resp.Context)
	require.Equal(t, "api", resp.Context["project"])
	require.Equal(t, "test", resp.Context["phase"])
}

func TestToJSON_StandardError(t *testing.T) {
	stdErr := stderrors.New("something went wrong")
	resp := ToJSON(stdErr)

	require.Equal(t, "UNKNOWN", resp.Code)
	require.Equal(t, "something went wrong", resp.Message)
	require.Equal(t, "PERMANENT", resp.Classification)
}

func TestToJSON_NilError(t *testing.T) {
	resp := ToJSON(nil)
	require.Nil(t, resp)
}

func TestToJSON_WithContext(t *testing.T) {
	err := New(CodeExecutionFailed, "execution failed")
	err = WithContextMap(err, map[string]interface{}{
		"command":  "earthly",
		"duration": 1234,
	})

	resp := ToJSON(err)

	require.NotNil(t, resp.Context)
	require.Equal(t, "earthly", resp.Context["command"])
	require.Equal(t, 1234, resp.Context["duration"])
}

func TestToJSON_OmitEmptyContext(t *testing.T) {
	err := New(CodeInternal, "internal error")
	resp := ToJSON(err)

	jsonBytes, _ := json.Marshal(resp)
	jsonStr := string(jsonBytes)

	// Context field should not be in JSON
	require.NotContains(t, jsonStr, "context")
}

func TestToJSON_NoWrappedErrors(t *testing.T) {
	cause := stderrors.New("original cause")
	err1 := Wrap(cause, CodeDatabase, "database error")
	err2 := Wrap(err1, CodeInternal, "internal error")

	resp := ToJSON(err2)

	// Only outermost error exposed
	require.Equal(t, "INTERNAL_ERROR", resp.Code)
	require.Equal(t, "internal error", resp.Message)

	// Cause not in JSON
	jsonBytes, _ := json.Marshal(resp)
	jsonStr := string(jsonBytes)
	require.NotContains(t, jsonStr, "database error")
	require.NotContains(t, jsonStr, "original cause")
}

func TestMarshalJSON(t *testing.T) {
	err := New(CodeInvalidInput, "invalid input")
	err = WithContext(err, "field", "email")

	jsonBytes, marshalErr := json.Marshal(err)
	require.NoError(t, marshalErr)

	var resp ErrorResponse
	unmarshalErr := json.Unmarshal(jsonBytes, &resp)
	require.NoError(t, unmarshalErr)

	require.Equal(t, "INVALID_INPUT", resp.Code)
	require.Equal(t, "invalid input", resp.Message)
	require.Equal(t, "PERMANENT", resp.Classification)
	require.Equal(t, "email", resp.Context["field"])
}

func TestMarshalJSON_RoundTrip(t *testing.T) {
	original := New(CodeTimeout, "request timeout")
	original = WithContextMap(original, map[string]interface{}{
		"endpoint": "/api/v1/resource",
		"duration": 30000,
	})

	// Marshal
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var resp ErrorResponse
	err = json.Unmarshal(jsonBytes, &resp)
	require.NoError(t, err)

	// Verify
	require.Equal(t, "TIMEOUT", resp.Code)
	require.Equal(t, "request timeout", resp.Message)
	require.Equal(t, "RETRYABLE", resp.Classification)
	require.Equal(t, "/api/v1/resource", resp.Context["endpoint"])
	require.Equal(t, float64(30000), resp.Context["duration"]) // JSON numbers are float64
}

func TestJSON_Structure(t *testing.T) {
	err := New(CodeNotFound, "not found")
	err = WithContext(err, "id", "123")

	jsonBytes, _ := json.Marshal(err)
	jsonStr := string(jsonBytes)

	// Verify expected structure
	require.Contains(t, jsonStr, `"code":"NOT_FOUND"`)
	require.Contains(t, jsonStr, `"message":"not found"`)
	require.Contains(t, jsonStr, `"classification":"PERMANENT"`)
	require.Contains(t, jsonStr, `"context":{"id":"123"}`)
}
