package git

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateBranch creates a new local branch from the specified reference (commit, tag, or branch).
//
// The ref parameter can be:
//   - A commit hash (e.g., "abc123...")
//   - A branch name (e.g., "main", "develop")
//   - A tag name (e.g., "v1.0.0")
//   - A reference name (e.g., "refs/heads/main")
//
// The branch is created but not checked out. Use CheckoutBranch to switch to the new branch.
//
// Returns ErrAlreadyExists if a branch with the given name already exists,
// ErrNotFound if the specified reference doesn't exist, or ErrInvalidInput for
// invalid parameters.
//
// Examples:
//
//	// Create branch from HEAD
//	err := repo.CreateBranch("feature-branch", "HEAD")
//
//	// Create branch from specific commit
//	err := repo.CreateBranch("hotfix", "abc123def456")
//
//	// Create branch from another branch
//	err := repo.CreateBranch("new-feature", "develop")
func (r *Repository) CreateBranch(name string, ref string) error {
	if name == "" {
		return wrapError(fmt.Errorf("branch name is required"), "failed to create branch")
	}
	if ref == "" {
		return wrapError(fmt.Errorf("reference is required"), "failed to create branch")
	}

	// Resolve the reference to a hash
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return wrapError(err, fmt.Sprintf("failed to resolve reference %q", ref))
	}

	// Create the branch reference
	branchRef := plumbing.NewBranchReferenceName(name)
	newRef := plumbing.NewHashReference(branchRef, *hash)

	// Check if branch already exists
	if _, err := r.repo.Reference(branchRef, false); err == nil {
		return wrapError(fmt.Errorf("branch %q already exists", name), "failed to create branch")
	}

	// Create the reference
	if err := r.repo.Storer.SetReference(newRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to create branch %q", name))
	}

	return nil
}

// CreateBranchFromRemote creates a local branch that tracks a remote branch.
//
// This is a convenience function that simplifies the common workflow of creating
// a local branch from a remote tracking branch. The remoteBranch parameter should
// be in the format "remote/branch" (e.g., "origin/main", "upstream/develop").
//
// The created local branch will be configured to track the remote branch for
// pull and push operations.
//
// Returns ErrAlreadyExists if the local branch already exists, ErrNotFound if
// the remote branch doesn't exist, or ErrInvalidInput for invalid parameters.
//
// Examples:
//
//	// Create local "main" tracking "origin/main"
//	err := repo.CreateBranchFromRemote("main", "origin/main")
//
//	// Create local "develop" tracking "upstream/develop"
//	err := repo.CreateBranchFromRemote("develop", "upstream/develop")
func (r *Repository) CreateBranchFromRemote(localName, remoteBranch string) error {
	if localName == "" {
		return wrapError(fmt.Errorf("local branch name is required"), "failed to create branch from remote")
	}
	if remoteBranch == "" {
		return wrapError(fmt.Errorf("remote branch name is required"), "failed to create branch from remote")
	}

	// Parse remote name and branch name from remoteBranch (format: "remote/branch")
	parts := strings.SplitN(remoteBranch, "/", 2)
	if len(parts) != 2 {
		return wrapError(
			fmt.Errorf("remote branch must be in format 'remote/branch', got %q", remoteBranch),
			"failed to create branch from remote",
		)
	}
	remoteName := parts[0]
	remoteBranchName := parts[1]

	// Build the remote reference name
	remoteRef := plumbing.NewRemoteReferenceName(remoteName, remoteBranchName)

	// Get the remote reference
	ref, err := r.repo.Reference(remoteRef, false)
	if err != nil {
		return wrapError(err, fmt.Sprintf("failed to find remote branch %q", remoteBranch))
	}

	// Create the local branch
	localRef := plumbing.NewBranchReferenceName(localName)
	newRef := plumbing.NewHashReference(localRef, ref.Hash())

	// Check if branch already exists
	if _, err := r.repo.Reference(localRef, false); err == nil {
		return wrapError(fmt.Errorf("branch %q already exists", localName), "failed to create branch from remote")
	}

	// Create the local branch reference
	if err := r.repo.Storer.SetReference(newRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to create local branch %q", localName))
	}

	// Set up tracking configuration
	cfg, err := r.repo.Config()
	if err != nil {
		return wrapError(err, "failed to read repository config")
	}

	// Add branch tracking configuration
	cfg.Branches[localName] = &config.Branch{
		Name:   localName,
		Remote: remoteName,
		Merge:  plumbing.NewBranchReferenceName(remoteBranchName),
	}

	// Save configuration
	if err := r.repo.Storer.SetConfig(cfg); err != nil {
		return wrapError(err, "failed to save tracking configuration")
	}

	return nil
}

// ListBranches returns all branches in the repository, including both local and remote branches.
//
// Local branches are those in refs/heads/, remote branches are those in refs/remotes/.
// The Branch.IsRemote field indicates whether each branch is remote or local.
//
// Examples:
//
//	branches, err := repo.ListBranches()
//	for _, branch := range branches {
//	    if branch.IsRemote {
//	        fmt.Printf("Remote: %s (%s)\n", branch.Name, branch.Hash)
//	    } else {
//	        fmt.Printf("Local: %s (%s)\n", branch.Name, branch.Hash)
//	    }
//	}
func (r *Repository) ListBranches() ([]Branch, error) {
	var branches []Branch

	// Get all references
	refs, err := r.repo.References()
	if err != nil {
		return nil, wrapError(err, "failed to list references")
	}

	// Iterate through references and filter for branches
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		refName := ref.Name()

		// Check if this is a local branch (refs/heads/*)
		if refName.IsBranch() {
			branches = append(branches, Branch{
				Name:     refName.Short(),
				Hash:     ref.Hash(),
				IsRemote: false,
			})
			return nil
		}

		// Check if this is a remote branch (refs/remotes/*)
		if refName.IsRemote() {
			// For remote branches, the short name is "remote/branch"
			branches = append(branches, Branch{
				Name:     refName.Short(),
				Hash:     ref.Hash(),
				IsRemote: true,
			})
			return nil
		}

		return nil
	})

	if err != nil {
		return nil, wrapError(err, "failed to iterate references")
	}

	return branches, nil
}

// CheckoutBranch switches the working tree to the specified branch.
//
// This updates HEAD to point to the branch and updates the working tree to
// match the branch's commit. Any uncommitted changes may cause conflicts.
//
// Returns ErrNotFound if the branch doesn't exist, ErrConflict if there are
// uncommitted changes that would be overwritten, or other errors for checkout
// failures.
//
// Note: This operation requires a working tree and will fail for bare repositories.
//
// Examples:
//
//	// Checkout an existing branch
//	err := repo.CheckoutBranch("main")
//
//	// Switch to a different branch
//	err := repo.CheckoutBranch("develop")
func (r *Repository) CheckoutBranch(name string) error {
	if name == "" {
		return wrapError(fmt.Errorf("branch name is required"), "failed to checkout branch")
	}

	// Get the worktree
	wt, err := r.repo.Worktree()
	if err != nil {
		return wrapError(err, "failed to get worktree")
	}

	// Build the branch reference name
	branchRef := plumbing.NewBranchReferenceName(name)

	// Checkout the branch
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: branchRef,
	}); err != nil {
		return wrapError(err, fmt.Sprintf("failed to checkout branch %q", name))
	}

	return nil
}

// DeleteBranch deletes a local branch from the repository.
//
// The force parameter determines whether to delete the branch even if it has
// unmerged changes. If force is false and the branch has commits that are not
// merged into the current branch or other branches, the operation will fail.
//
// Returns ErrNotFound if the branch doesn't exist, or ErrConflict if the branch
// has unmerged changes and force is false.
//
// Note: This only deletes local branches (refs/heads/*). To delete remote branches,
// use push operations with appropriate refspecs.
//
// Examples:
//
//	// Delete a merged branch
//	err := repo.DeleteBranch("feature-branch", false)
//
//	// Force delete an unmerged branch
//	err := repo.DeleteBranch("experimental", true)
func (r *Repository) DeleteBranch(name string, force bool) error {
	if name == "" {
		return wrapError(fmt.Errorf("branch name is required"), "failed to delete branch")
	}

	// Build the branch reference name
	branchRef := plumbing.NewBranchReferenceName(name)

	// Check if branch exists
	ref, err := r.repo.Reference(branchRef, false)
	if err != nil {
		return wrapError(err, fmt.Sprintf("failed to find branch %q", name))
	}

	// Check if this is the current branch
	head, err := r.repo.Head()
	if err != nil {
		return wrapError(err, "failed to get HEAD")
	}

	if head.Name() == branchRef {
		return wrapError(
			fmt.Errorf("cannot delete current branch %q", name),
			"failed to delete branch",
		)
	}

	// If not forcing, check if the branch is merged
	if !force {
		// Get the branch commit
		branchCommit, err := r.repo.CommitObject(ref.Hash())
		if err != nil {
			return wrapError(err, fmt.Sprintf("failed to get commit for branch %q", name))
		}

		// Get HEAD commit
		headCommit, err := r.repo.CommitObject(head.Hash())
		if err != nil {
			return wrapError(err, "failed to get HEAD commit")
		}

		// Check if branch commit is an ancestor of HEAD
		isAncestor, err := branchCommit.IsAncestor(headCommit)
		if err != nil {
			return wrapError(err, "failed to check if branch is merged")
		}

		if !isAncestor && branchCommit.Hash != headCommit.Hash {
			return wrapError(
				fmt.Errorf("branch %q has unmerged changes", name),
				"failed to delete branch",
			)
		}
	}

	// Delete the reference
	if err := r.repo.Storer.RemoveReference(branchRef); err != nil {
		return wrapError(err, fmt.Sprintf("failed to delete branch %q", name))
	}

	return nil
}

