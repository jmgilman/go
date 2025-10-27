package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/jmgilman/go/exec"
)

// WorktreeOperations defines operations for managing git worktrees.
// This interface abstracts the underlying implementation (git CLI) to enable testing
// and follows the same pattern as RemoteOperations for consistency.
//
// All operations require a real OS filesystem and will return an error if used
// with memory-based filesystems (like memfs), since they shell out to the git CLI.
type WorktreeOperations interface {
	// Add creates a new worktree at the specified path.
	// The ref parameter specifies which commit/branch to checkout in the worktree.
	// Options control creation behavior (force, detach, create branch).
	Add(ctx context.Context, path string, ref string, opts WorktreeOptions) error

	// List returns information about all worktrees associated with the repository.
	// This includes the main worktree and all linked worktrees.
	List(ctx context.Context) ([]WorktreeInfo, error)

	// Remove deletes a worktree.
	// If force is true, the worktree is removed even if it has uncommitted changes.
	Remove(ctx context.Context, path string, force bool) error

	// Lock locks a worktree to prevent it from being pruned.
	// The reason parameter is optional and explains why the worktree is locked.
	Lock(ctx context.Context, path string, reason string) error

	// Unlock unlocks a worktree, allowing it to be pruned.
	Unlock(ctx context.Context, path string) error

	// Prune removes stale worktree administrative data.
	// This cleans up worktree metadata for worktrees that have been manually deleted.
	Prune(ctx context.Context) error
}

// WorktreeInfo contains information about a worktree returned by List.
type WorktreeInfo struct {
	// Path is the absolute filesystem path to the worktree
	Path string

	// Head is the commit hash that the worktree is currently at
	Head string

	// Branch is the name of the checked out branch (empty if detached HEAD)
	Branch string

	// IsLocked indicates if the worktree is locked
	IsLocked bool

	// Reason contains the lock reason if the worktree is locked
	Reason string
}

// defaultWorktreeOps is the default implementation of WorktreeOperations
// that uses the git CLI via the exec module.
type defaultWorktreeOps struct {
	command  *exec.Command
	repoPath string
	fs       billy.Filesystem
}

// newDefaultWorktreeOps creates a new default WorktreeOperations implementation.
// If command is nil, a new default command is created.
func newDefaultWorktreeOps(command *exec.Command, repoPath string, fs billy.Filesystem) WorktreeOperations {
	if command == nil {
		command = exec.New()
	}
	return &defaultWorktreeOps{
		command:  command,
		repoPath: repoPath,
		fs:       fs,
	}
}

// Add creates a new worktree using 'git worktree add'.
func (w *defaultWorktreeOps) Add(ctx context.Context, path string, ref string, opts WorktreeOptions) error {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Build git command args
	git := exec.NewWrapper(w.command, "git")
	args := []string{"worktree", "add"}

	// Add options
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Detach {
		args = append(args, "--detach")
	}
	if opts.CreateBranch != "" {
		args = append(args, "-b", opts.CreateBranch)
	}

	// Add path and ref
	args = append(args, path)
	if ref != "" {
		args = append(args, ref)
	}

	// Execute command
	_, err := git.WithDir(w.repoPath).WithContext(ctx).Run(args...)
	if err != nil {
		return mapWorktreeExecError(err, "failed to add worktree")
	}

	return nil
}

// List returns information about all worktrees using 'git worktree list --porcelain'.
func (w *defaultWorktreeOps) List(ctx context.Context) ([]WorktreeInfo, error) {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return nil, wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Execute git worktree list
	git := exec.NewWrapper(w.command, "git")
	result, err := git.WithDir(w.repoPath).WithContext(ctx).Run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, mapWorktreeExecError(err, "failed to list worktrees")
	}

	// Parse the porcelain output
	return parseWorktreeListPorcelain(result.Stdout)
}

// Remove removes a worktree using 'git worktree remove'.
func (w *defaultWorktreeOps) Remove(ctx context.Context, path string, force bool) error {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Build command args
	git := exec.NewWrapper(w.command, "git")
	args := []string{"worktree", "remove"}

	if force {
		args = append(args, "--force")
	}

	args = append(args, path)

	// Execute command
	_, err := git.WithDir(w.repoPath).WithContext(ctx).Run(args...)
	if err != nil {
		return mapWorktreeExecError(err, "failed to remove worktree")
	}

	return nil
}

// Lock locks a worktree using 'git worktree lock'.
func (w *defaultWorktreeOps) Lock(ctx context.Context, path string, reason string) error {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Build command args
	git := exec.NewWrapper(w.command, "git")
	args := []string{"worktree", "lock"}

	if reason != "" {
		args = append(args, "--reason", reason)
	}

	args = append(args, path)

	// Execute command
	_, err := git.WithDir(w.repoPath).WithContext(ctx).Run(args...)
	if err != nil {
		return mapWorktreeExecError(err, "failed to lock worktree")
	}

	return nil
}

// Unlock unlocks a worktree using 'git worktree unlock'.
func (w *defaultWorktreeOps) Unlock(ctx context.Context, path string) error {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Execute command
	git := exec.NewWrapper(w.command, "git")
	_, err := git.WithDir(w.repoPath).WithContext(ctx).Run("worktree", "unlock", path)
	if err != nil {
		return mapWorktreeExecError(err, "failed to unlock worktree")
	}

	return nil
}

// Prune removes stale worktree data using 'git worktree prune'.
func (w *defaultWorktreeOps) Prune(ctx context.Context) error {
	// Check filesystem type
	if isMemoryFilesystem(w.fs) {
		return wrapError(
			fmt.Errorf("worktree operations not supported with memory filesystem"),
			"memory filesystem detected",
		)
	}

	// Execute command
	git := exec.NewWrapper(w.command, "git")
	_, err := git.WithDir(w.repoPath).WithContext(ctx).Run("worktree", "prune")
	if err != nil {
		return mapWorktreeExecError(err, "failed to prune worktrees")
	}

	return nil
}

// parseWorktreeListPorcelain parses the output of 'git worktree list --porcelain'.
//
// The porcelain format looks like:
//
//	worktree /path/to/worktree
//	HEAD abc123def456...
//	branch refs/heads/main
//
//	worktree /path/to/other
//	HEAD def456abc123...
//	detached
//
// Locked worktrees have a "locked" line and optionally a "locked <reason>" line.
func parseWorktreeListPorcelain(output string) ([]WorktreeInfo, error) {
	var worktrees []WorktreeInfo
	var current *WorktreeInfo

	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Empty line separates worktrees
		if line == "" {
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		// Parse each field
		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			current = &WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if current != nil {
			if strings.HasPrefix(line, "HEAD ") {
				current.Head = strings.TrimPrefix(line, "HEAD ")
			} else if strings.HasPrefix(line, "branch ") {
				branchRef := strings.TrimPrefix(line, "branch ")
				// Extract branch name from refs/heads/branch-name
				if strings.HasPrefix(branchRef, "refs/heads/") {
					current.Branch = strings.TrimPrefix(branchRef, "refs/heads/")
				} else {
					current.Branch = branchRef
				}
			} else if line == "detached" {
				current.Branch = "" // Detached HEAD has no branch
			} else if strings.HasPrefix(line, "locked") {
				current.IsLocked = true
				// Locked line may have a reason: "locked reason goes here"
				if len(line) > 7 { // len("locked ") = 7
					current.Reason = strings.TrimPrefix(line, "locked ")
				}
			}
		}
	}

	// Add the last worktree if there is one
	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees, nil
}

// mapWorktreeExecError converts exec.ExecError to appropriate platform errors.
// It examines the stderr output from git commands and maps common error patterns
// to the error codes defined in errors.go.
func mapWorktreeExecError(err error, context string) error {
	execErr, ok := err.(*exec.ExecError)
	if !ok {
		return wrapError(err, context)
	}

	stderr := execErr.Stderr

	// Map common git worktree error patterns
	if strings.Contains(stderr, "already exists") {
		return wrapError(fmt.Errorf("worktree already exists"), context)
	}
	if strings.Contains(stderr, "not a working tree") || strings.Contains(stderr, "is not a valid path") {
		return wrapError(gogit.ErrRepositoryNotExists, context)
	}
	if strings.Contains(stderr, "locked") || strings.Contains(stderr, "already locked") {
		return wrapError(
			fmt.Errorf("worktree is locked: %s", stderr),
			context,
		)
	}
	if strings.Contains(stderr, "contains modified or untracked files") {
		return wrapError(gogit.ErrWorktreeNotClean, context)
	}
	if strings.Contains(stderr, "permission denied") || strings.Contains(stderr, "Permission denied") {
		return wrapError(
			fmt.Errorf("permission denied: %s", stderr),
			context,
		)
	}

	// For other errors, wrap the exec error with context
	return wrapError(err, context)
}
