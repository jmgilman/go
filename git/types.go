package git

import (
	"time"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repository wraps a go-git repository with platform conventions.
// It stores both the underlying go-git repository and a billy filesystem
// for all I/O operations, providing escape hatches for advanced use cases.
type Repository struct {
	path        string
	repo        *gogit.Repository
	fs          billy.Filesystem
	worktreeOps WorktreeOperations
}

// Worktree wraps a go-git worktree with path tracking and a back-reference
// to its parent Repository.
type Worktree struct {
	path     string
	worktree *gogit.Worktree
	repo     *Repository
}

// Commit is a value type containing formatted commit information.
// It includes an escape hatch to the underlying go-git commit object
// for advanced operations.
type Commit struct {
	Hash      string
	Author    string
	Email     string
	Message   string
	Timestamp time.Time
	raw       *object.Commit //nolint:unused // Will be used by Underlying() method in task 009
}

// Branch is a simple value type representing a Git branch.
type Branch struct {
	Name     string
	Hash     plumbing.Hash
	IsRemote bool
}

// Tag is a simple value type representing a Git tag.
type Tag struct {
	Name    string
	Hash    plumbing.Hash
	Message string // Empty for lightweight tags
}

// Remote is a simple value type representing a Git remote.
type Remote struct {
	Name string
	URLs []string
}

// Auth is an interface for authentication methods.
// It is satisfied by go-git's transport.AuthMethod.
type Auth interface {
	// Marker interface - satisfied by go-git transport.AuthMethod
}

// CloneOptions configures repository cloning operations.
type CloneOptions struct {
	URL           string
	Auth          Auth
	Depth         int                    // 0 for full clone, >0 for shallow clone
	SingleBranch  bool                   // Clone only a single branch
	ReferenceName plumbing.ReferenceName // Branch or tag to clone
}

// FetchOptions configures fetch operations.
type FetchOptions struct {
	RemoteName string // Default: "origin"
	Auth       Auth
	Depth      int // For deepening shallow clones
}

// PullOptions configures pull operations.
type PullOptions struct {
	RemoteName string // Default: "origin"
	Auth       Auth
}

// PushOptions configures push operations.
type PushOptions struct {
	RemoteName string // Default: "origin"
	RefSpecs   []string
	Auth       Auth
	Force      bool
}

// WorktreeOptions configures worktree creation.
type WorktreeOptions struct {
	Hash         plumbing.Hash
	Branch       plumbing.ReferenceName
	CreateBranch string // Create a new branch with this name when adding worktree
	Force        bool   // Force creation even if worktree path already exists
	Detach       bool   // Detach HEAD at named commit
}

// CommitOptions configures commit creation.
type CommitOptions struct {
	Author     string
	Email      string
	Message    string
	AllowEmpty bool
}

// RemoteOptions configures remote management.
type RemoteOptions struct {
	Name string
	URL  string
}

// RepositoryOption configures repository creation operations (Init, Open, Clone).
// Options can be used to customize filesystem, remote operations, and clone behavior.
type RepositoryOption func(*repositoryOptions)

// repositoryOptions holds the configuration for repository creation.
type repositoryOptions struct {
	fs            billy.Filesystem
	remoteOps     RemoteOperations
	worktreeOps   WorktreeOperations
	bare          bool
	auth          Auth
	depth         int
	singleBranch  bool
	referenceName plumbing.ReferenceName
}

// WithFilesystem sets the billy filesystem to use for repository operations.
// If not provided, defaults to osfs.New(path) rooted at the repository path.
//
// Example:
//
//	repo, err := git.Init(ctx, "/path/to/repo", git.WithFilesystem(memfs.New()))
func WithFilesystem(fs billy.Filesystem) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.fs = fs
	}
}

// WithRemoteOperations sets the RemoteOperations implementation to use for
// network operations (Clone, Fetch, Push). If not provided, defaults to the
// internal implementation that uses go-git's network operations.
//
// This option is primarily useful for testing, allowing consumers to mock
// network operations without actual network calls.
//
// Example:
//
//	repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	    git.WithRemoteOperations(mockOps))
func WithRemoteOperations(ops RemoteOperations) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.remoteOps = ops
	}
}

// WithBare creates a bare repository (no working tree).
// Only applicable to Init operations.
//
// Example:
//
//	repo, err := git.Init(ctx, "/path/to/repo.git", git.WithBare())
func WithBare() RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.bare = true
	}
}

// WithAuth sets authentication for Clone operations.
//
// Example:
//
//	auth, _ := git.SSHKeyFile("git", "~/.ssh/id_rsa")
//	repo, err := git.Clone(ctx, "git@github.com:org/repo.git", git.WithAuth(auth))
func WithAuth(auth Auth) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.auth = auth
	}
}

// WithDepth sets the depth for shallow clones.
// A depth of 0 (default) performs a full clone.
//
// Example:
//
//	repo, err := git.Clone(ctx, "https://github.com/org/repo", git.WithDepth(1))
func WithDepth(depth int) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.depth = depth
	}
}

// WithSingleBranch limits the clone to a single branch.
//
// Example:
//
//	repo, err := git.Clone(ctx, "https://github.com/org/repo", git.WithSingleBranch())
func WithSingleBranch() RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.singleBranch = true
	}
}

// WithReferenceName sets the specific branch or tag to clone.
//
// Example:
//
//	repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	    git.WithReferenceName(plumbing.NewBranchReferenceName("develop")))
func WithReferenceName(ref plumbing.ReferenceName) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.referenceName = ref
	}
}

// WithWorktreeOperations sets the WorktreeOperations implementation to use for
// worktree operations (add, list, remove, lock, unlock, prune). If not provided,
// defaults to the git CLI implementation via the exec module.
//
// This option is primarily useful for testing, allowing consumers to mock
// worktree operations without actual git CLI calls.
//
// Example:
//
//	repo, err := git.Open("/path/to/repo",
//	    git.WithWorktreeOperations(mockOps))
func WithWorktreeOperations(ops WorktreeOperations) RepositoryOption {
	return func(opts *repositoryOptions) {
		opts.worktreeOps = ops
	}
}
