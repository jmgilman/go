package git

import (
	"errors"
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	platformerrors "github.com/jmgilman/go/errors"
)

// wrapError wraps an error with context, classifying it as a platform error type.
// It preserves the original error chain for errors.Is/errors.As compatibility.
// If err is nil, returns nil.
func wrapError(err error, context string) error {
	if err == nil {
		return nil
	}

	// First classify the go-git error to a platform error type
	classified := classifyError(err)

	// Then wrap with context
	return fmt.Errorf("%s: %w", context, classified)
}

// classifyError maps go-git errors to platform error types.
// It uses errors.Is() to match go-git error types and returns
// the appropriate platform error code. Unknown errors are passed
// through unchanged to preserve their original information.
//
//nolint:gocyclo,cyclop // High complexity is acceptable for error classification - each case is a simple mapping
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// Repository not found errors → ErrNotFound
	if errors.Is(err, gogit.ErrRepositoryNotExists) {
		return platformerrors.New(platformerrors.CodeNotFound, "repository does not exist")
	}
	if errors.Is(err, transport.ErrRepositoryNotFound) {
		return platformerrors.New(platformerrors.CodeNotFound, "repository not found")
	}

	// Reference not found errors → ErrNotFound
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return platformerrors.New(platformerrors.CodeNotFound, "reference not found")
	}

	// Repository already exists errors → ErrAlreadyExists
	if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		return platformerrors.New(platformerrors.CodeAlreadyExists, "repository already exists")
	}

	// Remote errors
	if errors.Is(err, gogit.ErrRemoteNotFound) {
		return platformerrors.New(platformerrors.CodeNotFound, "remote not found")
	}
	if errors.Is(err, gogit.ErrRemoteExists) {
		return platformerrors.New(platformerrors.CodeAlreadyExists, "remote already exists")
	}

	// Authentication/Authorization errors → ErrUnauthorized
	if errors.Is(err, transport.ErrAuthenticationRequired) {
		return platformerrors.New(platformerrors.CodeUnauthorized, "authentication required")
	}
	if errors.Is(err, transport.ErrAuthorizationFailed) {
		return platformerrors.New(platformerrors.CodeUnauthorized, "authorization failed")
	}

	// Dirty worktree/conflicts → ErrConflict
	if errors.Is(err, gogit.ErrWorktreeNotClean) {
		return platformerrors.New(platformerrors.CodeConflict, "worktree is not clean")
	}

	// Empty remote repository → ErrNotFound (nothing to fetch)
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return platformerrors.New(platformerrors.CodeNotFound, "remote repository is empty")
	}

	// Invalid input errors → ErrInvalidInput
	if errors.Is(err, gogit.ErrMissingURL) {
		return platformerrors.New(platformerrors.CodeInvalidInput, "URL is required")
	}
	if errors.Is(err, gogit.ErrMissingAuthor) {
		return platformerrors.New(platformerrors.CodeInvalidInput, "author is required")
	}
	if errors.Is(err, gogit.ErrHashOrReference) {
		return platformerrors.New(platformerrors.CodeInvalidInput, "ambiguous options: only one of hash or reference allowed")
	}
	if errors.Is(err, gogit.ErrBranchHashExclusive) {
		return platformerrors.New(platformerrors.CodeInvalidInput, "branch and hash are mutually exclusive")
	}
	if errors.Is(err, gogit.ErrMissingName) {
		return platformerrors.New(platformerrors.CodeInvalidInput, "name is required")
	}

	// Already exists errors → ErrAlreadyExists
	if errors.Is(err, gogit.ErrBranchExists) {
		return platformerrors.New(platformerrors.CodeAlreadyExists, "branch already exists")
	}
	if errors.Is(err, gogit.ErrDestinationExists) {
		return platformerrors.New(platformerrors.CodeAlreadyExists, "destination already exists")
	}
	if errors.Is(err, gogit.ErrSubmoduleAlreadyInitialized) {
		return platformerrors.New(platformerrors.CodeAlreadyExists, "submodule already initialized")
	}

	// Empty commit error → ErrConflict
	if errors.Is(err, gogit.ErrEmptyCommit) {
		return platformerrors.New(platformerrors.CodeConflict, "cannot create empty commit: working tree is clean")
	}

	// Pass through unknown errors unchanged to preserve original information
	return err
}
