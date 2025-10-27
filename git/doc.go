// Package git provides a clean, type-safe wrapper around go-git for Git repository operations.
//
// This library uses fs/billy for all filesystem operations, wraps go-git types with enhanced
// platform types while providing escape hatches, and organizes functionality into focused,
// maintainable files.
//
// The library targets Worker Service caching workflows (clone → update → worktree creation)
// while supporting bootstrap CLI and discovery needs. It prioritizes simplicity in common
// operations while providing full access to underlying go-git when needed.
//
// # Architecture
//
// The library is built on several key principles:
//
//  1. Thin wrappers over go-git (not reimplementing Git)
//  2. Billy filesystem for all I/O operations
//  3. Escape hatches via Underlying() methods for advanced use cases
//  4. RemoteOperations and WorktreeOperations interfaces enable testing by mocking operations
//  5. Organized by operation type (repository, worktree, branch, tag, commit, remote)
//  6. Worktree operations use git CLI for true linked worktree support
//
// # Core Types
//
// Repository wraps go-git with platform conventions and provides methods for all Git operations.
//
// Worktree represents a Git worktree with path tracking and operations. This library provides
// full support for linked worktrees via git CLI commands, allowing multiple branches to be
// checked out simultaneously in separate working directories.
//
// Commit, Branch, Tag, and Remote are value types for representing Git objects.
//
// # Billy Filesystem Requirement
//
// All repository operations use the go-billy filesystem abstraction. By default, operations
// use the local OS filesystem (osfs), but you can provide custom filesystems for testing
// (memfs) or other specialized use cases.
//
// The filesystem is scoped to the repository path, so all file operations are relative to
// the repository root.
//
// # Worktree Support via Git CLI
//
// Worktree operations (CreateWorktree, ListWorktrees, Remove, Lock, Unlock, PruneWorktrees)
// use git CLI commands to provide true linked worktree support. This requires:
//
//	1. Git must be installed and available in the system PATH
//	2. Operations must use the OS filesystem (osfs) - memory filesystems (memfs) are not supported
//	3. The WorktreeOperations interface can be mocked for testing without git CLI dependency
//
// When a memory filesystem is detected, worktree operations will return an error explaining
// that worktrees require the OS filesystem.
//
// # Factory Functions
//
// Init initializes a new Git repository at the specified path.
//
// Clone clones a repository from a remote URL.
//
// Open opens an existing repository from a path.
//
// All factory functions accept RepositoryOption arguments for customization (filesystem,
// authentication, depth, etc.).
//
// # Authentication
//
// The library provides helper functions for creating authentication:
//
//	// SSH key authentication
//	auth, err := git.SSHKeyFile("git", "/home/user/.ssh/id_rsa", "")
//
//	// SSH key from memory
//	auth, err := git.SSHKeyAuth("git", pemBytes, "")
//
//	// Basic authentication (HTTPS)
//	auth := git.BasicAuth("username", "password")
//
//	// No authentication (public repositories)
//	auth := git.EmptyAuth()
//
//	// Use with operations
//	repo, err := git.Clone(ctx, "https://github.com/org/repo", git.WithAuth(auth))
//
// # Escape Hatches
//
// All wrapper types provide Underlying() methods to access the raw go-git objects
// for advanced operations not covered by this library:
//
//	gogitRepo := repo.Underlying()           // Access go-git repository
//	fs := repo.Filesystem()                  // Access billy filesystem
//	gogitCommit := commit.Underlying()       // Access go-git commit
//	gogitWorktree := worktree.Underlying()   // Access go-git worktree
//
// This allows you to use the full power of go-git when needed while still benefiting
// from the simplified API for common operations.
//
// # RemoteOperations Interface
//
// The RemoteOperations interface abstracts network operations (Clone, Fetch, Push) to
// enable testing without actual network calls. Tests can provide a mock implementation
// using WithRemoteOperations() option.
//
// The default implementation uses go-git's network operations. Custom implementations
// can be used for testing, proxying, or adding custom behavior around network operations.
//
// # Usage Examples
//
// ## Example 1: Initialize New Repository
//
//	package main
//
//	import (
//	    "github.com/jmgilman/go/git"
//	)
//
//	func main() {
//	    // Initialize new repository
//	    repo, err := git.Init("/path/to/repos/my-repo")
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    // Create initial commit
//	    hash, err := repo.CreateCommit(git.CommitOptions{
//	        Author:     "User",
//	        Email:      "user@example.com",
//	        Message:    "Initial commit",
//	        AllowEmpty: true,
//	    })
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    println("Initialized repository with commit:", hash)
//	}
//
// ## Example 2: Clone and Create Worktree
//
//	package main
//
//	import (
//	    "context"
//	    "github.com/go-git/go-git/v5/plumbing"
//	    "github.com/jmgilman/go/git"
//	)
//
//	func main() {
//	    ctx := context.Background()
//
//	    // Clone repository (shallow)
//	    repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	        git.WithDepth(1),
//	        git.WithSingleBranch())
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    // Create linked worktree at specific commit
//	    wt, err := repo.CreateWorktree("/tmp/worktree", git.WorktreeOptions{
//	        Hash: plumbing.NewHash("abc123..."),
//	    })
//	    if err != nil {
//	        panic(err)
//	    }
//	    defer wt.Remove()
//
//	    // Create worktree with new branch
//	    wt2, err := repo.CreateWorktree("/tmp/feature", git.WorktreeOptions{
//	        Branch: plumbing.NewBranchReferenceName("main"),
//	        CreateBranch: "new-feature",
//	    })
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    // Lock worktree to prevent pruning
//	    err = wt2.Lock("Working on feature")
//
//	    // Work with worktrees...
//	    println("Worktree created at:", wt.Path())
//	}
//
// ## Example 3: Update Cached Repository
//
//	func updateCache(ctx context.Context, repoPath string) error {
//	    // Open existing repository
//	    repo, err := git.Open(repoPath)
//	    if err != nil {
//	        return err
//	    }
//
//	    // Fetch updates
//	    auth := git.BasicAuth("user", "token")
//	    err = repo.Fetch(ctx, git.FetchOptions{
//	        Auth: auth,
//	    })
//	    if err != nil {
//	        return err
//	    }
//
//	    return nil
//	}
//
// ## Example 4: Walk Commits Between Tags
//
//	func listChanges(repo *git.Repository, from, to string) error {
//	    for commit, err := range repo.WalkCommits(from, to) {
//	        if err != nil {
//	            return err
//	        }
//	        fmt.Printf("%s: %s (%s)\n",
//	            commit.Hash[:7],
//	            commit.Message,
//	            commit.Author,
//	        )
//	    }
//	    return nil
//	}
//
// ## Example 5: Create Commit (GitOps)
//
//	func updateReleasePointer(ctx context.Context, repo *git.Repository, auth git.Auth) error {
//	    // Modify files using billy filesystem
//	    fs := repo.Filesystem()
//
//	    file, err := fs.Create("release-pointer.yaml")
//	    if err != nil {
//	        return err
//	    }
//	    // ... write content ...
//	    file.Close()
//
//	    // Stage all changes
//	    wt, err := repo.Underlying().Worktree()
//	    if err != nil {
//	        return err
//	    }
//	    _, err = wt.Add("release-pointer.yaml")
//	    if err != nil {
//	        return err
//	    }
//
//	    // Create commit
//	    hash, err := repo.CreateCommit(git.CommitOptions{
//	        Author:  "Platform Bot",
//	        Email:   "bot@platform",
//	        Message: "Update release pointer",
//	    })
//	    if err != nil {
//	        return err
//	    }
//
//	    // Push to remote (needs ctx for network operation)
//	    err = repo.Push(ctx, git.PushOptions{
//	        RemoteName: "origin",
//	        RefSpecs:   []string{"refs/heads/main"},
//	        Auth:       auth,
//	    })
//
//	    return err
//	}
//
// ## Example 6: Using Escape Hatches
//
//	func advancedOperation(repo *git.Repository) error {
//	    // Get underlying go-git repository for advanced operations
//	    gogitRepo := repo.Underlying()
//
//	    // Use go-git API directly for operations not wrapped
//	    // by our library
//	    storer := gogitRepo.Storer
//	    // ... advanced go-git operations ...
//
//	    return nil
//	}
//
// # Error Handling
//
// The library wraps go-git errors with platform error types from the errors library.
// Common error types include:
//
//   - ErrNotFound: Repository, reference, tag, or branch not found
//   - ErrAlreadyExists: Branch, tag, or remote already exists
//   - ErrAuthenticationFailed: Authentication failure
//   - ErrNetwork: Network or connectivity issues
//   - ErrInvalidInput: Invalid reference or bad parameters
//   - ErrConflict: Dirty worktree or merge conflicts
//
// # Context and Cancellation
//
// Network operations (Clone, Fetch, Push, Pull) accept a context.Context parameter
// for cancellation and timeout control:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	repo, err := git.Clone(ctx, "https://github.com/org/repo")
//
// Local operations (branch, tag, commit operations) do not require context as they
// operate purely on the local filesystem.
//
// # Testing
//
// The library provides a testutil sub-package with helpers for testing:
//
//	import "github.com/jmgilman/go/git/testutil"
//
//	// Create in-memory repository
//	repo, fs, err := testutil.NewMemoryRepo()
//
//	// Create test commits
//	hash, err := testutil.CreateTestCommit(repo, "Test commit")
//
//	// Create test files
//	err := testutil.CreateTestFile(fs, "test.txt", "content")
//
// For mocking network and worktree operations in tests:
//
//	mockRemoteOps := &mockRemoteOps{...}
//	mockWorktreeOps := &mockWorktreeOps{...}
//	repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	    git.WithRemoteOperations(mockRemoteOps),
//	    git.WithWorktreeOperations(mockWorktreeOps))
//
// # References
//
// For advanced operations not covered by this library, refer to:
//   - go-git documentation: https://pkg.go.dev/github.com/go-git/go-git/v5
//   - go-billy documentation: https://pkg.go.dev/github.com/go-git/go-billy/v5
package git
