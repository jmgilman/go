package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	err := New(CodeNotFound, "resource not found")

	require.NotNil(t, err)
	require.Equal(t, CodeNotFound, err.Code())
	require.Equal(t, "resource not found", err.Message())
	require.Equal(t, ClassificationPermanent, err.Classification())
	require.Nil(t, err.Context())
	require.Nil(t, err.Unwrap())
}

func TestNew_AllErrorCodes(t *testing.T) {
	codes := []ErrorCode{
		CodeNotFound,
		CodeAlreadyExists,
		CodeConflict,
		CodeUnauthorized,
		CodeForbidden,
		CodeInvalidInput,
		CodeInvalidConfig,
		CodeSchemaFailed,
		CodeDatabase,
		CodeNetwork,
		CodeTimeout,
		CodeRateLimit,
		CodeExecutionFailed,
		CodeBuildFailed,
		CodePublishFailed,
		CodeInternal,
		CodeNotImplemented,
		CodeUnavailable,
		CodeUnknown,
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			err := New(code, "test message")
			require.Equal(t, code, err.Code())
			require.NotEmpty(t, err.Classification())
		})
	}
}

func TestNewf(t *testing.T) {
	err := Newf(CodeInvalidInput, "invalid value: %d (expected %d)", 5, 10)

	require.NotNil(t, err)
	require.Equal(t, CodeInvalidInput, err.Code())
	require.Equal(t, "invalid value: 5 (expected 10)", err.Message())
}

func TestNew_DefaultClassification(t *testing.T) {
	tests := []struct {
		name          string
		code          ErrorCode
		wantRetryable bool
	}{
		{"timeout is retryable", CodeTimeout, true},
		{"network is retryable", CodeNetwork, true},
		{"rate limit is retryable", CodeRateLimit, true},
		{"unavailable is retryable", CodeUnavailable, true},
		{"database is retryable", CodeDatabase, true},
		{"not found is permanent", CodeNotFound, false},
		{"invalid input is permanent", CodeInvalidInput, false},
		{"already exists is permanent", CodeAlreadyExists, false},
		{"conflict is permanent", CodeConflict, false},
		{"unauthorized is permanent", CodeUnauthorized, false},
		{"forbidden is permanent", CodeForbidden, false},
		{"invalid config is permanent", CodeInvalidConfig, false},
		{"schema failed is permanent", CodeSchemaFailed, false},
		{"not implemented is permanent", CodeNotImplemented, false},
		{"execution failed is permanent", CodeExecutionFailed, false},
		{"build failed is permanent", CodeBuildFailed, false},
		{"publish failed is permanent", CodePublishFailed, false},
		{"internal is permanent", CodeInternal, false},
		{"unknown is permanent", CodeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test")
			require.Equal(t, tt.wantRetryable, err.Classification().IsRetryable())
		})
	}
}
