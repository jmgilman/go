package cache

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jmgilman/go/git"
)

// getOrCreateBareRepo returns a bare repository from cache or creates it.
//
// This method:
// 1. Checks if the bare repo is already in memory cache
// 2. If not in memory, checks if it exists on disk and opens it
// 3. If not on disk, clones it from the remote URL
//
// Bare repositories are the single source of truth for Git objects and are
// shared across all checkouts of the same repository.
//
// The method is thread-safe and uses the cache's mutex for synchronization.
func (c *RepositoryCache) getOrCreateBareRepo(ctx context.Context, url string, opts *cacheOptions) (*git.Repository, error) {
	normalized := normalizeURL(url)

	// Check in-memory cache first (read lock)
	c.mu.RLock()
	if repo, exists := c.bare[normalized]; exists {
		c.mu.RUnlock()
		return repo, nil
	}
	c.mu.RUnlock()

	// Not in memory, acquire write lock to create/open
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if repo, exists := c.bare[normalized]; exists {
		return repo, nil
	}

	// Determine bare repository path
	barePath := filepath.Join(c.bareDir, normalized+".git")

	// Check if bare repo exists on disk
	if _, err := c.fs.Stat(barePath); err == nil {
		// Bare repo exists, open it
		repo, err := git.Open(barePath, git.WithFilesystem(c.fs))
		if err != nil {
			return nil, fmt.Errorf("failed to open bare repository at %s: %w", barePath, err)
		}

		// Cache in memory
		c.bare[normalized] = repo
		c.barePaths[normalized] = barePath
		return repo, nil
	}

	// Bare repo doesn't exist, clone it
	repo, err := c.cloneBareRepo(ctx, url, barePath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone bare repository: %w", err)
	}

	// Cache in memory
	c.bare[normalized] = repo
	c.barePaths[normalized] = barePath
	return repo, nil
}

// cloneBareRepo clones a remote repository as a bare repository.
//
// This creates a new bare repository at the specified path by:
// 1. Initializing a bare repository
// 2. Adding the remote as "origin"
// 3. Fetching all refs from the remote
func (c *RepositoryCache) cloneBareRepo(ctx context.Context, url, barePath string, opts *cacheOptions) (*git.Repository, error) {
	// Initialize bare repository at the specified path
	repo, err := git.Init(barePath, git.WithFilesystem(c.fs), git.WithBare())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bare repository: %w", err)
	}

	// Add remote as "origin"
	if err := repo.AddRemote(git.RemoteOptions{
		Name: "origin",
		URL:  url,
	}); err != nil {
		return nil, fmt.Errorf("failed to add remote: %w", err)
	}

	// Build fetch options
	fetchOpts := git.FetchOptions{
		RemoteName: "origin",
	}

	// Add authentication if provided
	if opts.auth != nil {
		fetchOpts.Auth = opts.auth
	}

	// Add depth if specified (shallow clone)
	if opts.depth > 0 {
		fetchOpts.Depth = opts.depth
	}

	// Fetch all refs from remote
	if err := repo.Fetch(ctx, fetchOpts); err != nil {
		return nil, fmt.Errorf("failed to fetch from remote %s: %w", url, err)
	}

	// When fetching from local paths, refs are stored as refs/remotes/origin/*
	// Copy them to refs/heads/* so they can be used as local branches
	underlying := repo.Underlying()
	refs, err := underlying.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		// Copy refs/remotes/origin/* to refs/heads/*
		if ref.Name().IsRemote() && ref.Name().String() == "refs/remotes/origin/master" {
			localRef := plumbing.NewHashReference("refs/heads/master", ref.Hash())
			if err := underlying.Storer.SetReference(localRef); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy remote refs: %w", err)
	}

	// Set HEAD to point to refs/heads/master if it now exists
	if _, err := underlying.Reference("refs/heads/master", true); err == nil {
		headRef := plumbing.NewSymbolicReference("HEAD", "refs/heads/master")
		if err := underlying.Storer.SetReference(headRef); err != nil {
			// Non-fatal - continue even if setting HEAD fails
			_ = err
		}
	}

	return repo, nil
}

// updateBareRepo fetches the latest changes from the remote.
//
// This method updates an existing bare repository by fetching from its
// remote origin. It requires authentication if the remote requires it.
func (c *RepositoryCache) updateBareRepo(ctx context.Context, repo *git.Repository, opts *cacheOptions) error {
	// Build fetch options
	fetchOpts := git.FetchOptions{
		RemoteName: "origin",
	}

	// Add authentication if provided
	if opts.auth != nil {
		fetchOpts.Auth = opts.auth
	}

	// Add depth if specified (for deepening shallow clones)
	if opts.depth > 0 {
		fetchOpts.Depth = opts.depth
	}

	// Fetch from remote
	if err := repo.Fetch(ctx, fetchOpts); err != nil {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}

	// Update local branches from remote refs
	// When fetching, refs are stored as refs/remotes/origin/*
	// We need to update refs/heads/* to match
	underlying := repo.Underlying()
	refs, err := underlying.References()
	if err != nil {
		return fmt.Errorf("failed to get references: %w", err)
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		// Copy refs/remotes/origin/* to refs/heads/*
		if ref.Name().IsRemote() && ref.Name().String() == "refs/remotes/origin/master" {
			localRef := plumbing.NewHashReference("refs/heads/master", ref.Hash())
			if err := underlying.Storer.SetReference(localRef); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update local refs: %w", err)
	}

	return nil
}
