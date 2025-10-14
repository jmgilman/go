package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// RemoteOperations defines the interface for Git remote network operations.
// This interface allows for testing by enabling mock implementations that
// don't require actual network access.
//
// The default implementation (defaultRemoteOps) delegates to go-git's
// network operations. Tests can replace the package-level remoteOps variable
// with a mock implementation to avoid network calls.
type RemoteOperations interface {
	// Clone clones a remote repository to the local filesystem.
	Clone(ctx context.Context, fs billy.Filesystem, opts CloneOptions) (*Repository, error)

	// Fetch downloads objects and refs from the remote repository.
	Fetch(ctx context.Context, repo *Repository, opts FetchOptions) error

	// Push uploads objects and refs to the remote repository.
	Push(ctx context.Context, repo *Repository, opts PushOptions) error
}

// defaultRemoteOps is the default implementation of RemoteOperations that
// uses go-git's network operations to interact with remote repositories.
type defaultRemoteOps struct{}

// Clone implements RemoteOperations.Clone using go-git's PlainClone.
// It creates a new repository by cloning from the remote URL specified in opts.
func (d *defaultRemoteOps) Clone(ctx context.Context, fs billy.Filesystem, opts CloneOptions) (*Repository, error) {
	// Create clone path based on repository name if not specified
	// For now, we'll use a default ".git" path
	path := ".git"

	// Create directory structure
	if err := fs.MkdirAll(path, 0o755); err != nil {
		return nil, wrapError(err, "failed to create clone directory")
	}

	// Create a filesystem scoped to the repository path
	scopedFs, err := fs.Chroot(path)
	if err != nil {
		return nil, wrapError(err, "failed to scope filesystem to path")
	}

	// For non-bare clones, we need separate storage and worktree filesystems
	// Create storage in .git subdirectory
	dotGitFs, err := scopedFs.Chroot(".git")
	if err != nil {
		return nil, wrapError(err, "failed to create .git filesystem")
	}

	storage := filesystem.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

	// Convert our options to go-git clone options
	cloneOpts := &gogit.CloneOptions{
		URL: opts.URL,
	}

	// Set authentication if provided
	if opts.Auth != nil {
		auth, ok := opts.Auth.(transport.AuthMethod)
		if !ok {
			return nil, wrapError(fmt.Errorf("invalid auth type"), "failed to convert auth")
		}
		cloneOpts.Auth = auth
	}

	// Set depth for shallow clones
	if opts.Depth > 0 {
		cloneOpts.Depth = opts.Depth
	}

	// Set single branch option
	if opts.SingleBranch {
		cloneOpts.SingleBranch = true
	}

	// Set reference name if provided
	if opts.ReferenceName != "" {
		cloneOpts.ReferenceName = opts.ReferenceName
	}

	// Perform the clone
	repo, err := gogit.CloneContext(ctx, storage, scopedFs, cloneOpts)
	if err != nil {
		return nil, wrapError(err, "failed to clone repository")
	}

	return &Repository{
		path: path,
		repo: repo,
		fs:   scopedFs,
	}, nil
}

// Fetch implements RemoteOperations.Fetch using go-git's Fetch.
// It downloads objects and refs from the remote repository.
func (d *defaultRemoteOps) Fetch(ctx context.Context, repo *Repository, opts FetchOptions) error {
	// Set default remote name if not provided
	remoteName := opts.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}

	// Convert our options to go-git fetch options
	fetchOpts := &gogit.FetchOptions{
		RemoteName: remoteName,
	}

	// Set authentication if provided
	if opts.Auth != nil {
		auth, ok := opts.Auth.(transport.AuthMethod)
		if !ok {
			return wrapError(fmt.Errorf("invalid auth type"), "failed to convert auth")
		}
		fetchOpts.Auth = auth
	}

	// Set depth for shallow fetches
	if opts.Depth > 0 {
		fetchOpts.Depth = opts.Depth
	}

	// Perform the fetch
	err := repo.repo.FetchContext(ctx, fetchOpts)
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return wrapError(err, "failed to fetch from remote")
	}

	return nil
}

// Push implements RemoteOperations.Push using go-git's Push.
// It uploads objects and refs to the remote repository.
func (d *defaultRemoteOps) Push(ctx context.Context, repo *Repository, opts PushOptions) error {
	// Set default remote name if not provided
	remoteName := opts.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}

	// Convert our options to go-git push options
	pushOpts := &gogit.PushOptions{
		RemoteName: remoteName,
	}

	// Set authentication if provided
	if opts.Auth != nil {
		auth, ok := opts.Auth.(transport.AuthMethod)
		if !ok {
			return wrapError(fmt.Errorf("invalid auth type"), "failed to convert auth")
		}
		pushOpts.Auth = auth
	}

	// Set force option
	if opts.Force {
		pushOpts.Force = true
	}

	// Set refspecs if provided
	if len(opts.RefSpecs) > 0 {
		for _, refSpec := range opts.RefSpecs {
			pushOpts.RefSpecs = append(pushOpts.RefSpecs, config.RefSpec(refSpec))
		}
	}

	// Perform the push
	err := repo.repo.PushContext(ctx, pushOpts)
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return wrapError(err, "failed to push to remote")
	}

	return nil
}

// remoteOps is the package-level variable that holds the RemoteOperations
// implementation. By default, it uses defaultRemoteOps which performs actual
// network operations. Tests can replace this with a mock implementation.
var remoteOps RemoteOperations = &defaultRemoteOps{}

// CloneWithOptions is a legacy function that clones using CloneOptions.
// Deprecated: Use Clone with functional options instead:
//
//	git.Clone(ctx, url, git.WithAuth(auth), git.WithFilesystem(fs))
func CloneWithOptions(ctx context.Context, fs billy.Filesystem, opts CloneOptions) (*Repository, error) {
	//nolint:wrapcheck // Errors from remoteOps are already wrapped in their implementations
	return remoteOps.Clone(ctx, fs, opts)
}

// ListRemotes returns all configured remotes for this repository.
// Each remote includes its name and configured URLs.
//
// Example:
//
//	remotes, err := repo.ListRemotes()
//	for _, remote := range remotes {
//	    fmt.Printf("Remote %s: %v\n", remote.Name, remote.URLs)
//	}
func (r *Repository) ListRemotes() ([]Remote, error) {
	remotes, err := r.repo.Remotes()
	if err != nil {
		return nil, wrapError(err, "failed to list remotes")
	}

	result := make([]Remote, 0, len(remotes))
	for _, remote := range remotes {
		cfg := remote.Config()
		result = append(result, Remote{
			Name: cfg.Name,
			URLs: cfg.URLs,
		})
	}

	return result, nil
}

// AddRemote adds a new remote to the repository configuration.
// The remote name must be unique within the repository.
//
// Parameters:
//   - opts: remote options including name and URL
//
// Returns an error if the remote already exists (ErrAlreadyExists) or if
// the configuration is invalid (ErrInvalidInput).
//
// Example:
//
//	err := repo.AddRemote(git.RemoteOptions{
//	    Name: "upstream",
//	    URL:  "https://github.com/upstream/repo",
//	})
func (r *Repository) AddRemote(opts RemoteOptions) error {
	_, err := r.repo.CreateRemote(&config.RemoteConfig{
		Name: opts.Name,
		URLs: []string{opts.URL},
	})
	if err != nil {
		return wrapError(err, "failed to add remote")
	}

	return nil
}

// RemoveRemote removes a remote from the repository configuration.
//
// Parameters:
//   - name: name of the remote to remove
//
// Returns an error if the remote doesn't exist (ErrNotFound).
//
// Example:
//
//	err := repo.RemoveRemote("upstream")
func (r *Repository) RemoveRemote(name string) error {
	err := r.repo.DeleteRemote(name)
	if err != nil {
		return wrapError(err, "failed to remove remote")
	}

	return nil
}

// Fetch downloads objects and refs from the remote repository.
// It updates remote-tracking branches but doesn't modify the working tree.
//
// The fetch operation uses the package-level remoteOps interface, which allows
// tests to mock network operations.
//
// Parameters:
//   - ctx: context for cancellation and timeout
//   - opts: fetch options including remote name, authentication, depth, etc.
//
// Returns an error if fetching fails. Common errors include ErrNotFound if
// the remote doesn't exist, ErrUnauthorized for authentication failures, or
// network errors.
//
// Example:
//
//	err := repo.Fetch(ctx, git.FetchOptions{
//	    RemoteName: "origin",
//	    Auth:       auth,
//	})
func (r *Repository) Fetch(ctx context.Context, opts FetchOptions) error {
	//nolint:wrapcheck // Errors from remoteOps are already wrapped in their implementations
	return remoteOps.Fetch(ctx, r, opts)
}

// Push uploads objects and refs to the remote repository.
// It updates the remote branches with local changes.
//
// The push operation uses the package-level remoteOps interface, which allows
// tests to mock network operations.
//
// Parameters:
//   - ctx: context for cancellation and timeout
//   - opts: push options including remote name, authentication, refspecs, etc.
//
// Returns an error if pushing fails. Common errors include ErrUnauthorized for
// authentication failures, ErrConflict for non-fast-forward updates (unless
// Force is true), or network errors.
//
// Example:
//
//	err := repo.Push(ctx, git.PushOptions{
//	    RemoteName: "origin",
//	    RefSpecs:   []string{"refs/heads/main:refs/heads/main"},
//	    Auth:       auth,
//	})
func (r *Repository) Push(ctx context.Context, opts PushOptions) error {
	//nolint:wrapcheck // Errors from remoteOps are already wrapped in their implementations
	return remoteOps.Push(ctx, r, opts)
}

// Pull is a convenience method that combines Fetch with a working tree update.
// It fetches changes from the remote and updates the current branch's working
// tree to match the remote tracking branch.
//
// This operation is equivalent to: Fetch + merge/rebase of the remote tracking
// branch into the current branch. The implementation uses go-git's Pull which
// performs a fetch followed by a merge.
//
// Parameters:
//   - ctx: context for cancellation and timeout
//   - opts: pull options including remote name and authentication
//
// Returns an error if pulling fails. Common errors include ErrNotFound if
// the remote doesn't exist, ErrUnauthorized for authentication failures,
// ErrConflict for merge conflicts, or network errors.
//
// Note: This method requires a non-bare repository with a working tree.
//
// Example:
//
//	err := repo.Pull(ctx, git.PullOptions{
//	    RemoteName: "origin",
//	    Auth:       auth,
//	})
func (r *Repository) Pull(ctx context.Context, opts PullOptions) error {
	// Get the worktree
	wt, err := r.repo.Worktree()
	if err != nil {
		return wrapError(err, "failed to get worktree")
	}

	// Set default remote name if not provided
	remoteName := opts.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}

	// Convert our options to go-git pull options
	pullOpts := &gogit.PullOptions{
		RemoteName: remoteName,
	}

	// Set authentication if provided
	if opts.Auth != nil {
		auth, ok := opts.Auth.(transport.AuthMethod)
		if !ok {
			return wrapError(fmt.Errorf("invalid auth type"), "failed to convert auth")
		}
		pullOpts.Auth = auth
	}

	// Perform the pull (fetch + merge)
	err = wt.PullContext(ctx, pullOpts)
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return wrapError(err, "failed to pull from remote")
	}

	return nil
}
