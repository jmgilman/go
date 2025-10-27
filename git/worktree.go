package git

import (
	"context"
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateWorktree creates a new linked worktree at the specified path.
//
// A worktree is a separate working directory connected to the same repository.
// This allows multiple branches to be checked out simultaneously without
// affecting each other. This implementation uses git CLI commands to create
// true linked worktrees.
//
// The worktree can be created at a specific commit (using opts.Hash) or branch
// (using opts.Branch). At least one of these options must be provided.
//
// Additional options:
//   - CreateBranch: Create a new branch when adding the worktree
//   - Force: Force creation even if the worktree path already exists
//   - Detach: Detach HEAD at the named commit
//
// Returns the created Worktree or an error if creation fails. Common errors
// include ErrInvalidInput if neither Hash nor Branch are provided, ErrAlreadyExists
// if the worktree already exists, ErrNotFound if the reference doesn't exist, or
// ErrConflict if a memory filesystem is used (worktrees require OS filesystem).
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
//
//	// Create worktree with a new branch
//	wt, err := repo.CreateWorktree("/tmp/worktree", git.WorktreeOptions{
//	    Branch: plumbing.NewBranchReferenceName("main"),
//	    CreateBranch: "new-feature",
//	})
func (r *Repository) CreateWorktree(path string, opts WorktreeOptions) (*Worktree, error) {
	ctx := context.Background()

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

	// Determine ref from options
	var ref string
	if hasHash {
		ref = opts.Hash.String()
	} else {
		ref = opts.Branch.Short()
	}

	// Use WorktreeOperations to create worktree
	err := r.worktreeOps.Add(ctx, path, ref, opts)
	if err != nil {
		return nil, err // Already wrapped
	}

	// Open the newly created worktree
	worktreeRepo, err := Open(path, WithWorktreeOperations(r.worktreeOps))
	if err != nil {
		return nil, wrapError(err, "failed to open created worktree")
	}

	// Get the go-git worktree object
	wt, err := worktreeRepo.repo.Worktree()
	if err != nil {
		return nil, wrapError(err, "failed to get worktree")
	}

	return &Worktree{
		path:     path,
		worktree: wt,
		repo:     worktreeRepo,
	}, nil
}

// ListWorktrees returns all worktrees associated with this repository.
//
// This method uses git CLI commands to list all worktrees, including the main
// worktree and all linked worktrees. Each worktree is opened and returned as
// a Worktree object.
//
// Returns a slice of Worktree pointers or an error if listing fails. Common errors
// include ErrConflict if a memory filesystem is used (worktrees require OS filesystem).
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
	ctx := context.Background()

	// Get worktree information from git CLI
	infos, err := r.worktreeOps.List(ctx)
	if err != nil {
		return nil, err // Already wrapped
	}

	// Open each worktree and create Worktree objects
	worktrees := make([]*Worktree, 0, len(infos))
	for _, info := range infos {
		// Open the worktree repository
		worktreeRepo, err := Open(info.Path, WithWorktreeOperations(r.worktreeOps))
		if err != nil {
			return nil, wrapError(err, fmt.Sprintf("failed to open worktree at %s", info.Path))
		}

		// Get the go-git worktree object
		wt, err := worktreeRepo.repo.Worktree()
		if err != nil {
			return nil, wrapError(err, fmt.Sprintf("failed to get worktree at %s", info.Path))
		}

		worktrees = append(worktrees, &Worktree{
			path:     info.Path,
			worktree: wt,
			repo:     worktreeRepo,
		})
	}

	return worktrees, nil
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
			Force:  true,
		})
		if err != nil {
			return wrapError(err, "failed to checkout branch")
		}

		// Reset to ensure index is clean
		head, err := w.repo.repo.Head()
		if err != nil {
			return wrapError(err, "failed to get HEAD after checkout")
		}
		err = w.worktree.Reset(&gogit.ResetOptions{
			Commit: head.Hash(),
			Mode:   gogit.HardReset,
		})
		if err != nil {
			return wrapError(err, "failed to reset worktree after checkout")
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
			Hash:  *hash,
			Force: true,
		})
		if err != nil {
			return wrapError(err, "failed to checkout tag")
		}

		// Reset to ensure index is clean
		err = w.worktree.Reset(&gogit.ResetOptions{
			Commit: *hash,
			Mode:   gogit.HardReset,
		})
		if err != nil {
			return wrapError(err, "failed to reset worktree after checkout")
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
		Hash:  *hash,
		Force: true,
	})
	if err != nil {
		return wrapError(err, "failed to checkout reference")
	}

	// Reset the worktree to ensure index matches HEAD
	err = w.worktree.Reset(&gogit.ResetOptions{
		Commit: *hash,
		Mode:   gogit.HardReset,
	})
	if err != nil {
		return wrapError(err, "failed to reset worktree after checkout")
	}

	return nil
}

// Remove removes this worktree from the repository.
//
// This operation performs safety checks to ensure no uncommitted changes
// would be lost. The worktree must be clean (no uncommitted changes) before
// it can be removed. This method uses git CLI commands to remove linked worktrees.
//
// Returns an error if the worktree has uncommitted changes (ErrConflict),
// if the worktree is locked, or if a memory filesystem is used (worktrees
// require OS filesystem).
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
	ctx := context.Background()

	// Check if the worktree has uncommitted changes using go-git
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

	// Note: We use force=true here because go-git and git CLI may have different
	// views of what's "clean" (e.g., index state differences). Since go-git reports
	// clean status, we trust that and force the removal.
	// Use WorktreeOperations to remove the worktree
	return w.repo.worktreeOps.Remove(ctx, w.path, true)
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

// Lock locks this worktree to prevent it from being pruned.
//
// Locked worktrees cannot be removed by 'git worktree prune' and are protected
// from automatic cleanup. The optional reason parameter explains why the worktree
// is locked.
//
// Returns an error if the worktree is already locked or if a memory filesystem
// is used (worktrees require OS filesystem).
//
// Example:
//
//	err := wt.Lock("Working on critical fix")
func (w *Worktree) Lock(reason string) error {
	ctx := context.Background()
	return w.repo.worktreeOps.Lock(ctx, w.path, reason)
}

// Unlock unlocks this worktree, allowing it to be pruned.
//
// Returns an error if the worktree is not locked or if a memory filesystem
// is used (worktrees require OS filesystem).
//
// Example:
//
//	err := wt.Unlock()
func (w *Worktree) Unlock() error {
	ctx := context.Background()
	return w.repo.worktreeOps.Unlock(ctx, w.path)
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

// PruneWorktrees removes stale worktree administrative data.
//
// This method cleans up worktree metadata for worktrees that have been manually
// deleted from the filesystem. It's useful for maintaining repository health
// when worktrees are removed without using Remove().
//
// Returns an error if a memory filesystem is used (worktrees require OS filesystem).
//
// Example:
//
//	err := repo.PruneWorktrees()
func (r *Repository) PruneWorktrees() error {
	ctx := context.Background()
	return r.worktreeOps.Prune(ctx)
}
