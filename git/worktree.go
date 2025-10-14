package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateWorktree creates a new worktree at the specified path.
//
// A worktree is a separate working directory connected to the same repository.
// This allows multiple branches to be checked out simultaneously without
// affecting each other.
//
// Note: go-git v5 does not support linked worktrees (git worktree add).
// This method returns the main worktree and checks it out to the specified
// reference. For true parallel worktrees, consider using separate clones.
//
// The worktree can be created at a specific commit (using opts.Hash) or branch
// (using opts.Branch). Exactly one of these options must be provided.
//
// Returns the created Worktree or an error if creation fails. Common errors
// include ErrInvalidInput if both Hash and Branch are provided or neither is
// provided, ErrNotFound if the reference doesn't exist, or filesystem errors.
//
// Examples:
//
//	// Create worktree at a specific commit
//	wt, err := repo.CreateWorktree("/tmp/worktree", git.WorktreeOptions{
//	    Hash: plumbing.NewHash("abc123..."),
//	})
//
//	// Create worktree on a branch
//	wt, err := repo.CreateWorktree("/tmp/worktree", git.WorktreeOptions{
//	    Branch: plumbing.NewBranchReferenceName("feature"),
//	})
func (r *Repository) CreateWorktree(path string, opts WorktreeOptions) (*Worktree, error) {
	// Validate that exactly one of Hash or Branch is specified
	hasHash := !opts.Hash.IsZero()
	hasBranch := opts.Branch != ""

	if hasHash && hasBranch {
		return nil, wrapError(
			fmt.Errorf("both Hash and Branch specified"),
			"invalid worktree options",
		)
	}

	if !hasHash && !hasBranch {
		return nil, wrapError(
			fmt.Errorf("neither Hash nor Branch specified"),
			"invalid worktree options",
		)
	}

	// Get the main worktree
	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, wrapError(err, "failed to get repository worktree")
	}

	// Build checkout options based on what was provided
	checkoutOpts := &gogit.CheckoutOptions{}

	if hasHash {
		checkoutOpts.Hash = opts.Hash
	} else {
		checkoutOpts.Branch = opts.Branch
	}

	// Checkout the specified reference
	err = wt.Checkout(checkoutOpts)
	if err != nil {
		return nil, wrapError(err, "failed to checkout reference")
	}

	return &Worktree{
		path:     path,
		worktree: wt,
		repo:     r,
	}, nil
}

// ListWorktrees returns all worktrees associated with this repository.
//
// Note: go-git v5 does not support linked worktrees (git worktree add).
// This method returns only the main worktree.
//
// Returns a slice of Worktree pointers or an error if listing fails.
//
// Example:
//
//	worktrees, err := repo.ListWorktrees()
//	if err != nil {
//	    return err
//	}
//	for _, wt := range worktrees {
//	    fmt.Printf("Worktree at: %s\n", wt.Path())
//	}
func (r *Repository) ListWorktrees() ([]*Worktree, error) {
	// Get the main worktree
	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, wrapError(err, "failed to get worktree")
	}

	// Return only the main worktree since go-git doesn't support linked worktrees
	return []*Worktree{
		{
			path:     r.path,
			worktree: wt,
			repo:     r,
		},
	}, nil
}

// Checkout updates the worktree to the specified reference.
//
// The ref parameter can be a commit hash, branch name, or tag name. This
// operation updates the working directory to match the specified reference.
//
// When checking out a branch by name, the HEAD will be set to that branch.
// When checking out by hash or tag, HEAD will be detached.
//
// Returns an error if checkout fails. Common errors include ErrNotFound if
// the reference doesn't exist, ErrConflict if there are uncommitted changes
// that would be overwritten, or filesystem errors.
//
// Examples:
//
//	// Checkout a branch
//	err := wt.Checkout("main")
//
//	// Checkout a specific commit
//	err := wt.Checkout("abc123...")
//
//	// Checkout a tag
//	err := wt.Checkout("v1.0.0")
func (w *Worktree) Checkout(ref string) error {
	// Try as a branch name first
	branchRef := plumbing.NewBranchReferenceName(ref)
	_, err := w.repo.repo.Reference(branchRef, true)
	
	if err == nil {
		// It's a branch, checkout with branch reference to keep HEAD attached
		err = w.worktree.Checkout(&gogit.CheckoutOptions{
			Branch: branchRef,
		})
		if err != nil {
			return wrapError(err, "failed to checkout branch")
		}
		return nil
	}

	// Try as a tag
	tagRef := plumbing.NewTagReferenceName(ref)
	_, err = w.repo.repo.Reference(tagRef, true)
	
	if err == nil {
		// It's a tag, resolve and checkout by hash (detached HEAD)
		hash, err := w.repo.repo.ResolveRevision(plumbing.Revision(tagRef))
		if err != nil {
			return wrapError(err, "failed to resolve tag")
		}
		err = w.worktree.Checkout(&gogit.CheckoutOptions{
			Hash: *hash,
		})
		if err != nil {
			return wrapError(err, "failed to checkout tag")
		}
		return nil
	}

	// Try as a commit hash directly
	hash, err := w.repo.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return wrapError(err, "failed to resolve reference")
	}

	// Checkout by hash (detached HEAD)
	err = w.worktree.Checkout(&gogit.CheckoutOptions{
		Hash: *hash,
	})
	if err != nil {
		return wrapError(err, "failed to checkout reference")
	}

	return nil
}

// Remove removes this worktree from the repository.
//
// This operation performs safety checks to ensure no uncommitted changes
// would be lost. The worktree must be clean (no uncommitted changes) before
// it can be removed.
//
// Note: go-git v5 does not support linked worktrees (git worktree add), so
// this method only validates that the worktree is clean. Actual cleanup of
// worktree files should be handled externally if needed.
//
// Returns an error if the worktree has uncommitted changes (ErrConflict) or
// if the status cannot be determined.
//
// Example:
//
//	wt, err := repo.CreateWorktree("/tmp/worktree", opts)
//	if err != nil {
//	    return err
//	}
//	defer wt.Remove()
//	// ... use worktree ...
func (w *Worktree) Remove() error {
	// Check if the worktree has uncommitted changes
	status, err := w.worktree.Status()
	if err != nil {
		return wrapError(err, "failed to get worktree status")
	}

	// If there are any changes, refuse to remove
	if !status.IsClean() {
		return wrapError(
			gogit.ErrWorktreeNotClean,
			"worktree has uncommitted changes",
		)
	}

	// Since go-git doesn't support linked worktrees, there's no worktree-specific
	// cleanup to perform. The main worktree cleanup would be handled by removing
	// the entire repository, which is outside the scope of this method.
	return nil
}

// Path returns the filesystem path of this worktree.
//
// This is the absolute path to the worktree's working directory where
// files can be accessed and modified.
//
// Example:
//
//	wt, err := repo.CreateWorktree("/tmp/worktree", opts)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Worktree created at: %s\n", wt.Path())
func (w *Worktree) Path() string {
	return w.path
}

// Underlying returns the underlying go-git Worktree for advanced operations
// not covered by this wrapper. This escape hatch allows direct access to
// go-git's full worktree API when needed.
//
// The returned *gogit.Worktree can be used for any go-git worktree operation.
// Changes made through the underlying worktree will be reflected in this wrapper.
//
// Example:
//
//	// Access go-git's worktree for advanced operations
//	gogitWt := wt.Underlying()
//	status, err := gogitWt.Status()
func (w *Worktree) Underlying() *gogit.Worktree {
	return w.worktree
}
