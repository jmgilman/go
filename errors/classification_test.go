package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorClassification_IsRetryable(t *testing.T) {
	tests := []struct {
		name           string
		classification ErrorClassification
		want           bool
	}{
		{
			name:           "retryable classification",
			classification: ClassificationRetryable,
			want:           true,
		},
		{
			name:           "permanent classification",
			classification: ClassificationPermanent,
			want:           false,
		},
		{
			name:           "unknown classification",
			classification: ErrorClassification("UNKNOWN"),
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.classification.IsRetryable()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetDefaultClassification(t *testing.T) {
	tests := []struct {
		name string
		code ErrorCode
		want ErrorClassification
	}{
		{
			name: "retryable - timeout",
			code: CodeTimeout,
			want: ClassificationRetryable,
		},
		{
			name: "retryable - network",
			code: CodeNetwork,
			want: ClassificationRetryable,
		},
		{
			name: "retryable - rate limit",
			code: CodeRateLimit,
			want: ClassificationRetryable,
		},
		{
			name: "retryable - unavailable",
			code: CodeUnavailable,
			want: ClassificationRetryable,
		},
		{
			name: "retryable - database",
			code: CodeDatabase,
			want: ClassificationRetryable,
		},
		{
			name: "permanent - not found",
			code: CodeNotFound,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - invalid input",
			code: CodeInvalidInput,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - already exists",
			code: CodeAlreadyExists,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - conflict",
			code: CodeConflict,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - unauthorized",
			code: CodeUnauthorized,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - forbidden",
			code: CodeForbidden,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - invalid config",
			code: CodeInvalidConfig,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - schema failed",
			code: CodeSchemaFailed,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - not implemented",
			code: CodeNotImplemented,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - execution failed",
			code: CodeExecutionFailed,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - build failed",
			code: CodeBuildFailed,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - publish failed",
			code: CodePublishFailed,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - internal",
			code: CodeInternal,
			want: ClassificationPermanent,
		},
		{
			name: "permanent - unknown",
			code: CodeUnknown,
			want: ClassificationPermanent,
		},
		{
			name: "unknown code - safe default",
			code: ErrorCode("UNKNOWN_CODE"),
			want: ClassificationPermanent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDefaultClassification(tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}
