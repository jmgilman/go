package errors

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIs(t *testing.T) {
	sentinel := New(CodeNotFound, "not found")
	wrapped := Wrap(sentinel, CodeDatabase, "query failed")

	// Should find sentinel in chain
	require.True(t, Is(wrapped, sentinel))

	// Should not match different error
	other := New(CodeInvalidInput, "invalid")
	require.False(t, Is(wrapped, other))
}

func TestIs_StandardLibraryCompatibility(t *testing.T) {
	stdErr := stderrors.New("standard sentinel")
	wrapped := Wrap(stdErr, CodeInternal, "internal error")

	// Should work with standard errors.Is
	require.True(t, stderrors.Is(wrapped, stdErr))
	require.True(t, Is(wrapped, stdErr))
}

func TestAs(t *testing.T) {
	err := New(CodeNotFound, "not found")

	var platformErr PlatformError
	require.True(t, As(err, &platformErr))
	require.Equal(t, CodeNotFound, platformErr.Code())
}

func TestAs_StandardLibraryCompatibility(t *testing.T) {
	err := New(CodeInternal, "internal")

	var platformErr PlatformError
	require.True(t, stderrors.As(err, &platformErr))
	require.True(t, As(err, &platformErr))
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{
			name: "platform error",
			err:  New(CodeNotFound, "not found"),
			want: CodeNotFound,
		},
		{
			name: "wrapped platform error",
			err:  Wrap(New(CodeTimeout, "timeout"), CodeDatabase, "db timeout"),
			want: CodeDatabase, // Outermost code
		},
		{
			name: "standard error",
			err:  stderrors.New("standard error"),
			want: CodeUnknown,
		},
		{
			name: "nil error",
			err:  nil,
			want: CodeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCode(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorClassification
	}{
		{
			name: "retryable error",
			err:  New(CodeTimeout, "timeout"),
			want: ClassificationRetryable,
		},
		{
			name: "permanent error",
			err:  New(CodeNotFound, "not found"),
			want: ClassificationPermanent,
		},
		{
			name: "standard error - safe default",
			err:  stderrors.New("standard error"),
			want: ClassificationPermanent,
		},
		{
			name: "nil error - safe default",
			err:  nil,
			want: ClassificationPermanent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetClassification(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "retryable error",
			err:  New(CodeTimeout, "timeout"),
			want: true,
		},
		{
			name: "permanent error",
			err:  New(CodeNotFound, "not found"),
			want: false,
		},
		{
			name: "wrapped retryable error",
			err:  Wrap(New(CodeNetwork, "network error"), CodeDatabase, "db connection"),
			want: true, // Preserves retryable classification
		},
		{
			name: "standard error - safe default",
			err:  stderrors.New("standard error"),
			want: false,
		},
		{
			name: "nil error - safe default",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsRetryable_WithClassificationOverride(t *testing.T) {
	// Start with permanent error
	err := New(CodeNotFound, "not found")
	require.False(t, IsRetryable(err))

	// Override to retryable
	err = WithClassification(err, ClassificationRetryable)
	require.True(t, IsRetryable(err))
}
