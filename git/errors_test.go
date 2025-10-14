package git

import (
	"errors"
	"fmt"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	platformerrors "github.com/jmgilman/go/errors"
)

func TestClassifyError_RepositoryNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode platformerrors.ErrorCode
	}{
		{
			name:     "ErrRepositoryNotExists",
			err:      gogit.ErrRepositoryNotExists,
			wantCode: platformerrors.CodeNotFound,
		},
		{
			name:     "transport.ErrRepositoryNotFound",
			err:      transport.ErrRepositoryNotFound,
			wantCode: platformerrors.CodeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			// Check that result is a PlatformError
			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("classifyError() did not return PlatformError, got %T", result)
			}

			// Check the error code
			if pe.Code() != tt.wantCode {
				t.Errorf("classifyError() code = %v, want %v", pe.Code(), tt.wantCode)
			}
		})
	}
}

func TestClassifyError_ReferenceNotFound(t *testing.T) {
	err := plumbing.ErrReferenceNotFound
	result := classifyError(err)

	var pe platformerrors.PlatformError
	if !errors.As(result, &pe) {
		t.Fatalf("classifyError() did not return PlatformError, got %T", result)
	}

	if pe.Code() != platformerrors.CodeNotFound {
		t.Errorf("classifyError() code = %v, want %v", pe.Code(), platformerrors.CodeNotFound)
	}
}

func TestClassifyError_AlreadyExists(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode platformerrors.ErrorCode
	}{
		{
			name:     "ErrRepositoryAlreadyExists",
			err:      gogit.ErrRepositoryAlreadyExists,
			wantCode: platformerrors.CodeAlreadyExists,
		},
		{
			name:     "ErrBranchExists",
			err:      gogit.ErrBranchExists,
			wantCode: platformerrors.CodeAlreadyExists,
		},
		{
			name:     "ErrDestinationExists",
			err:      gogit.ErrDestinationExists,
			wantCode: platformerrors.CodeAlreadyExists,
		},
		{
			name:     "ErrSubmoduleAlreadyInitialized",
			err:      gogit.ErrSubmoduleAlreadyInitialized,
			wantCode: platformerrors.CodeAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("classifyError() did not return PlatformError, got %T", result)
			}

			if pe.Code() != tt.wantCode {
				t.Errorf("classifyError() code = %v, want %v", pe.Code(), tt.wantCode)
			}
		})
	}
}

func TestClassifyError_Authentication(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode platformerrors.ErrorCode
	}{
		{
			name:     "ErrAuthenticationRequired",
			err:      transport.ErrAuthenticationRequired,
			wantCode: platformerrors.CodeUnauthorized,
		},
		{
			name:     "ErrAuthorizationFailed",
			err:      transport.ErrAuthorizationFailed,
			wantCode: platformerrors.CodeUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("classifyError() did not return PlatformError, got %T", result)
			}

			if pe.Code() != tt.wantCode {
				t.Errorf("classifyError() code = %v, want %v", pe.Code(), tt.wantCode)
			}
		})
	}
}

func TestClassifyError_Conflict(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode platformerrors.ErrorCode
	}{
		{
			name:     "ErrWorktreeNotClean",
			err:      gogit.ErrWorktreeNotClean,
			wantCode: platformerrors.CodeConflict,
		},
		{
			name:     "ErrEmptyCommit",
			err:      gogit.ErrEmptyCommit,
			wantCode: platformerrors.CodeConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("classifyError() did not return PlatformError, got %T", result)
			}

			if pe.Code() != tt.wantCode {
				t.Errorf("classifyError() code = %v, want %v", pe.Code(), tt.wantCode)
			}
		})
	}
}

func TestClassifyError_InvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode platformerrors.ErrorCode
	}{
		{
			name:     "ErrMissingURL",
			err:      gogit.ErrMissingURL,
			wantCode: platformerrors.CodeInvalidInput,
		},
		{
			name:     "ErrMissingAuthor",
			err:      gogit.ErrMissingAuthor,
			wantCode: platformerrors.CodeInvalidInput,
		},
		{
			name:     "ErrHashOrReference",
			err:      gogit.ErrHashOrReference,
			wantCode: platformerrors.CodeInvalidInput,
		},
		{
			name:     "ErrBranchHashExclusive",
			err:      gogit.ErrBranchHashExclusive,
			wantCode: platformerrors.CodeInvalidInput,
		},
		{
			name:     "ErrMissingName",
			err:      gogit.ErrMissingName,
			wantCode: platformerrors.CodeInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)

			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("classifyError() did not return PlatformError, got %T", result)
			}

			if pe.Code() != tt.wantCode {
				t.Errorf("classifyError() code = %v, want %v", pe.Code(), tt.wantCode)
			}
		})
	}
}

func TestClassifyError_EmptyRemoteRepository(t *testing.T) {
	err := transport.ErrEmptyRemoteRepository
	result := classifyError(err)

	var pe platformerrors.PlatformError
	if !errors.As(result, &pe) {
		t.Fatalf("classifyError() did not return PlatformError, got %T", result)
	}

	if pe.Code() != platformerrors.CodeNotFound {
		t.Errorf("classifyError() code = %v, want %v", pe.Code(), platformerrors.CodeNotFound)
	}
}

func TestClassifyError_UnknownError(t *testing.T) {
	// Test that unknown errors pass through unchanged
	originalErr := errors.New("some unknown error")
	result := classifyError(originalErr)

	if !errors.Is(result, originalErr) {
		t.Errorf("classifyError() should pass through unknown errors unchanged")
	}
}

func TestClassifyError_Nil(t *testing.T) {
	result := classifyError(nil)
	if result != nil {
		t.Errorf("classifyError(nil) = %v, want nil", result)
	}
}

func TestWrapError_WithContext(t *testing.T) {
	err := gogit.ErrRepositoryNotExists
	context := "failed to open repository"

	result := wrapError(err, context)

	// Check that error message includes context
	if result == nil {
		t.Fatal("wrapError() returned nil")
	}

	errMsg := result.Error()
	if errMsg == "" {
		t.Error("wrapError() returned empty error message")
	}

	// Check that we can unwrap to the platform error
	var pe platformerrors.PlatformError
	if !errors.As(result, &pe) {
		t.Errorf("wrapError() result cannot be unwrapped to PlatformError")
	}

	if pe.Code() != platformerrors.CodeNotFound {
		t.Errorf("wrapError() code = %v, want %v", pe.Code(), platformerrors.CodeNotFound)
	}
}

func TestWrapError_PreservesErrorChain(t *testing.T) {
	// Test that errors.Is still works after wrapping
	originalErr := gogit.ErrRepositoryNotExists
	wrapped := wrapError(originalErr, "context")

	// Should be able to identify the platform error
	var pe platformerrors.PlatformError
	if !errors.As(wrapped, &pe) {
		t.Error("wrapError() broke errors.As chain")
	}
}

func TestWrapError_Nil(t *testing.T) {
	result := wrapError(nil, "some context")
	if result != nil {
		t.Errorf("wrapError(nil, _) = %v, want nil", result)
	}
}

func TestWrapError_MultipleWrapping(t *testing.T) {
	// Test that error can be wrapped multiple times
	err := gogit.ErrRepositoryNotExists
	wrapped1 := wrapError(err, "first context")
	wrapped2 := fmt.Errorf("second context: %w", wrapped1)

	// Should still be able to identify the platform error
	var pe platformerrors.PlatformError
	if !errors.As(wrapped2, &pe) {
		t.Error("Multiple wrapping broke errors.As chain")
	}

	if pe.Code() != platformerrors.CodeNotFound {
		t.Errorf("Multiple wrapping changed error code to %v, want %v", pe.Code(), platformerrors.CodeNotFound)
	}
}

func TestWrapError_DifferentErrorTypes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		context     string
		wantCode    platformerrors.ErrorCode
		description string
	}{
		{
			name:        "Repository not found",
			err:         gogit.ErrRepositoryNotExists,
			context:     "opening repository",
			wantCode:    platformerrors.CodeNotFound,
			description: "maps to NOT_FOUND",
		},
		{
			name:        "Authentication failed",
			err:         transport.ErrAuthorizationFailed,
			context:     "fetching from remote",
			wantCode:    platformerrors.CodeUnauthorized,
			description: "maps to UNAUTHORIZED",
		},
		{
			name:        "Invalid input",
			err:         gogit.ErrMissingURL,
			context:     "cloning repository",
			wantCode:    platformerrors.CodeInvalidInput,
			description: "maps to INVALID_INPUT",
		},
		{
			name:        "Conflict",
			err:         gogit.ErrWorktreeNotClean,
			context:     "creating commit",
			wantCode:    platformerrors.CodeConflict,
			description: "maps to CONFLICT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapError(tt.err, tt.context)

			var pe platformerrors.PlatformError
			if !errors.As(result, &pe) {
				t.Fatalf("wrapError() did not return PlatformError, got %T", result)
			}

			if pe.Code() != tt.wantCode {
				t.Errorf("wrapError() code = %v, want %v (%s)", pe.Code(), tt.wantCode, tt.description)
			}
		})
	}
}
