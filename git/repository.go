package git

import (
	"context"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Init creates a new Git repository at the specified path.
//
// By default, Init creates a standard (non-bare) repository using the local
// filesystem rooted at the specified path. This behavior can be customized
// using RepositoryOption functions.
//
// Returns the initialized Repository or an error if initialization fails.
// Common errors include ErrAlreadyExists if a repository already exists at
// the specified path, or filesystem errors if the path cannot be created.
//
// Examples:
//
//	// Create a standard repository
//	repo, err := git.Init("/path/to/repo")
//
//	// Create a bare repository
//	repo, err := git.Init("/path/to/repo.git", git.WithBare())
//
//	// Create repository with custom filesystem (for testing)
//	repo, err := git.Init("/path/to/repo", git.WithFilesystem(memfs.New()))
func Init(path string, opts ...RepositoryOption) (*Repository, error) {
	// Apply options with defaults
	options := &repositoryOptions{
		fs:   osfs.New(path),
		bare: false,
	}
	for _, opt := range opts {
		opt(options)
	}

	fs := options.fs
	// Create directory structure
	if err := fs.MkdirAll(path, 0o755); err != nil {
		return nil, wrapError(err, "failed to create repository directory")
	}

	// Create a filesystem scoped to the repository path
	scopedFs, err := fs.Chroot(path)
	if err != nil {
		return nil, wrapError(err, "failed to scope filesystem to path")
	}

	// For non-bare repositories, we need separate storage and worktree filesystems
	if !options.bare {
		// Create storage in .git subdirectory
		dotGitFs, err := scopedFs.Chroot(".git")
		if err != nil {
			return nil, wrapError(err, "failed to create .git filesystem")
		}
		
		storage := filesystem.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

		// Initialize with worktree
		repo, err := gogit.Init(storage, scopedFs)
		if err != nil {
			return nil, wrapError(err, "failed to initialize repository")
		}

		return &Repository{
			path: path,
			repo: repo,
			fs:   scopedFs,
		}, nil
	}

	// For bare repositories, storage is in the root
	storage := filesystem.NewStorage(scopedFs, cache.NewObjectLRUDefault())
	
	// Initialize without worktree (bare repository)
	repo, err := gogit.Init(storage, nil)
	if err != nil {
		return nil, wrapError(err, "failed to initialize bare repository")
	}

	return &Repository{
		path: path,
		repo: repo,
		fs:   scopedFs,
	}, nil
}

// Open opens an existing Git repository at the specified path.
//
// By default, Open uses the local filesystem rooted at the specified path.
// This behavior can be customized using RepositoryOption functions.
//
// Returns the opened Repository or an error if opening fails. Common errors
// include ErrNotFound if no repository exists at the path, or filesystem
// errors if the path cannot be accessed.
//
// Examples:
//
//	// Open a repository from the local filesystem
//	repo, err := git.Open("/path/to/repo")
//
//	// Open with custom filesystem (for testing)
//	repo, err := git.Open("/path/to/repo", git.WithFilesystem(memfs.New()))
func Open(path string, opts ...RepositoryOption) (*Repository, error) {
	// Apply options with defaults
	options := &repositoryOptions{
		fs: osfs.New(path),
	}
	for _, opt := range opts {
		opt(options)
	}

	fs := options.fs
	// Create a filesystem scoped to the repository path
	scopedFs, err := fs.Chroot(path)
	if err != nil {
		return nil, wrapError(err, "failed to scope filesystem to path")
	}

	// Check if this is a standard repository (has .git directory) or bare
	dotGitStat, dotGitErr := scopedFs.Stat(".git")
	
	var repo *gogit.Repository
	
	if dotGitErr == nil && dotGitStat.IsDir() {
		// Standard repository with .git directory
		dotGitFs, err := scopedFs.Chroot(".git")
		if err != nil {
			return nil, wrapError(err, "failed to scope filesystem to .git")
		}
		
		storage := filesystem.NewStorage(dotGitFs, cache.NewObjectLRUDefault())
		
		// Open with worktree
		repo, err = gogit.Open(storage, scopedFs)
		if err != nil {
			return nil, wrapError(err, "failed to open repository")
		}
	} else {
		// Bare repository (no .git directory, objects etc. are in root)
		storage := filesystem.NewStorage(scopedFs, cache.NewObjectLRUDefault())
		
		// Open without worktree
		repo, err = gogit.Open(storage, nil)
		if err != nil {
			return nil, wrapError(err, "failed to open repository")
		}
	}

	return &Repository{
		path: path,
		repo: repo,
		fs:   scopedFs,
	}, nil
}

// Underlying returns the underlying go-git Repository for advanced operations
// not covered by this wrapper. This escape hatch allows direct access to
// go-git's full API when needed.
//
// The returned *gogit.Repository can be used for any go-git operation. Changes
// made through the underlying repository will be reflected in this wrapper.
func (r *Repository) Underlying() *gogit.Repository {
	return r.repo
}

// Filesystem returns the billy.Filesystem associated with this repository.
// This filesystem can be used for file operations within the repository's
// working tree or for accessing repository data.
//
// For standard (non-bare) repositories, this returns a filesystem scoped to
// the working tree. For bare repositories, it returns a filesystem scoped to
// the repository directory itself.
func (r *Repository) Filesystem() billy.Filesystem {
	return r.fs
}

// Clone clones a remote repository to the local filesystem.
//
// By default, Clone creates a full clone using the local filesystem rooted at
// a generated path based on the repository name, and uses the internal
// RemoteOperations implementation for network operations.
//
// The behavior can be customized using RepositoryOption functions to set
// authentication, shallow clone depth, filesystem location, and mock network
// operations for testing.
//
// Returns the cloned Repository or an error if cloning fails. Common errors
// include ErrNotFound if the remote repository doesn't exist, ErrUnauthorized
// for authentication failures, or network errors.
//
// Examples:
//
//	// Clone a public repository
//	repo, err := git.Clone(ctx, "https://github.com/org/repo")
//
//	// Clone with authentication
//	auth, _ := git.SSHKeyFile("git", "~/.ssh/id_rsa")
//	repo, err := git.Clone(ctx, "git@github.com:org/repo.git", git.WithAuth(auth))
//
//	// Shallow clone (depth=1)
//	repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	    git.WithDepth(1),
//	    git.WithSingleBranch())
//
//	// Clone with custom filesystem (for testing)
//	repo, err := git.Clone(ctx, "https://github.com/org/repo",
//	    git.WithFilesystem(memfs.New()),
//	    git.WithRemoteOperations(mockOps))
func Clone(ctx context.Context, url string, opts ...RepositoryOption) (*Repository, error) {
	// Apply options with defaults
	options := &repositoryOptions{
		fs:        osfs.New("."), // Default to current directory
		remoteOps: &defaultRemoteOps{},
	}
	for _, opt := range opts {
		opt(options)
	}

	// Build CloneOptions from the repository options
	cloneOpts := CloneOptions{
		URL:           url,
		Auth:          options.auth,
		Depth:         options.depth,
		SingleBranch:  options.singleBranch,
		ReferenceName: options.referenceName,
	}

	// Use the RemoteOperations interface to perform the clone
	//nolint:wrapcheck // Errors from remoteOps are already wrapped in their implementations
	return options.remoteOps.Clone(ctx, options.fs, cloneOpts)
}
