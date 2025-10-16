package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jmgilman/go/git"
)

// GetCheckout returns a path to a working tree suitable for symlinking or temporary use.
//
// The checkout is identified by a composite key: (url + ref + cacheKey).
// - Same composite key = reuses existing checkout (if it exists)
// - Different cache key = creates isolated checkout
// - Different ref = creates separate checkout for that ref
//
// If the repository doesn't exist in cache, it's cloned. If WithUpdate() is
// specified, the cache is refreshed from the remote before returning.
//
// The cacheKey controls checkout lifecycle:
// - Stable key (e.g., "team-docs") = persistent, reused across calls
// - Unique key (e.g., UUID) = ephemeral, intended for single use
//
// Ephemeral checkouts should specify WithTTL() to enable automatic cleanup.
//
// The returned path remains valid across calls and can be safely symlinked.
// Symlinks are preserved during updates via in-place refresh.
//
// Examples:
//
//	// Persistent checkout with stable key
//	path, _ := cache.GetCheckout(ctx, "https://github.com/my/docs", "team-docs",
//	    WithRef("main"))
//	os.Symlink(filepath.Join(path, "docs"), ".sow/refs/team-docs")
//
//	// Ephemeral checkout with unique key and TTL
//	cacheKey := uuid.New().String()
//	path, _ := cache.GetCheckout(ctx, "https://github.com/my/repo", cacheKey,
//	    WithRef("main"),
//	    WithTTL(1*time.Hour))
//	defer cache.RemoveCheckout(url, cacheKey)
//
//	// Force fresh update
//	path, _ := cache.GetCheckout(ctx, url, "build",
//	    WithUpdate(),
//	    WithAuth(auth))
func (c *RepositoryCache) GetCheckout(ctx context.Context, url, cacheKey string, opts ...CacheOption) (string, error) {
	// Apply options with defaults
	options := &cacheOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Get or create bare repository (Tier 1)
	bareRepo, err := c.getOrCreateBareRepo(ctx, url, options)
	if err != nil {
		return "", fmt.Errorf("failed to get bare repository: %w", err)
	}

	// Update bare repo if requested
	if options.update {
		if err := c.updateBareRepo(ctx, bareRepo, options); err != nil {
			return "", fmt.Errorf("failed to update bare repository: %w", err)
		}
	}

	// Determine ref to use
	ref := options.ref
	if ref == "" {
		// Get default branch if no ref specified
		head, err := bareRepo.Underlying().Head()
		if err != nil {
			return "", fmt.Errorf("failed to get default branch: %w", err)
		}
		ref = head.Name().Short()
	}

	// Create composite key
	compositeKey := makeCompositeKey(url, ref, cacheKey)

	// Get or create checkout (Tier 2)
	checkoutPath, err := c.getOrCreateCheckout(ctx, url, ref, cacheKey, compositeKey, bareRepo, options)
	if err != nil {
		return "", fmt.Errorf("failed to get checkout: %w", err)
	}

	// Update metadata index
	c.updateCheckoutMetadata(url, ref, cacheKey, compositeKey, options)

	// Save index
	if err := c.index.save(c.fs, c.indexPath); err != nil {
		return "", fmt.Errorf("failed to save index: %w", err)
	}

	return checkoutPath, nil
}

// getOrCreateCheckout returns a checkout from cache or creates it.
func (c *RepositoryCache) getOrCreateCheckout(
	ctx context.Context,
	url, ref, cacheKey, compositeKey string,
	bareRepo *git.Repository,
	opts *cacheOptions,
) (string, error) {
	normalized := normalizeURL(url)
	checkoutPath := filepath.Join(c.checkoutDir, normalized, ref, cacheKey)

	// Check in-memory cache first (read lock)
	c.mu.RLock()
	if repo, exists := c.checkouts[compositeKey]; exists {
		c.mu.RUnlock()

		// If update requested, refresh the checkout
		if opts.update {
			if err := c.refreshCheckout(ctx, repo, bareRepo, ref); err != nil {
				return "", fmt.Errorf("failed to refresh checkout: %w", err)
			}
		}

		return checkoutPath, nil
	}
	c.mu.RUnlock()

	// Not in memory, acquire write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if repo, exists := c.checkouts[compositeKey]; exists {
		// If update requested, refresh the checkout
		if opts.update {
			if err := c.refreshCheckout(ctx, repo, bareRepo, ref); err != nil {
				return "", fmt.Errorf("failed to refresh checkout: %w", err)
			}
		}

		return checkoutPath, nil
	}

	// Check if checkout exists on disk
	if _, err := c.fs.Stat(checkoutPath); err == nil {
		// Checkout exists on disk, open it
		repo, err := git.Open(checkoutPath, git.WithFilesystem(c.fs))
		if err != nil {
			return "", fmt.Errorf("failed to open checkout at %s: %w", checkoutPath, err)
		}

		// If update requested, refresh it
		if opts.update {
			if err := c.refreshCheckout(ctx, repo, bareRepo, ref); err != nil {
				return "", fmt.Errorf("failed to refresh checkout: %w", err)
			}
		}

		// Cache in memory
		c.checkouts[compositeKey] = repo
		return checkoutPath, nil
	}

	// Checkout doesn't exist, create it
	repo, err := c.createCheckout(ctx, checkoutPath, bareRepo, ref)
	if err != nil {
		return "", fmt.Errorf("failed to create checkout: %w", err)
	}

	// Cache in memory
	c.checkouts[compositeKey] = repo
	return checkoutPath, nil
}

// createCheckout creates a new checkout from a bare repository.
//
// This creates a new working tree by cloning from the local bare repository.
// The checkout tracks the bare repository as its remote, allowing fast updates.
func (c *RepositoryCache) createCheckout(ctx context.Context, checkoutPath string, bareRepo *git.Repository, ref string) (*git.Repository, error) {
	// Get the URL from the bare repo's remote
	remotes, err := bareRepo.ListRemotes()
	if err != nil {
		return nil, fmt.Errorf("failed to list remotes: %w", err)
	}

	if len(remotes) == 0 || len(remotes[0].URLs) == 0 {
		return nil, fmt.Errorf("bare repository has no remotes configured")
	}

	originalURL := remotes[0].URLs[0]

	// Initialize a new repository at the checkout path
	repo, err := git.Init(checkoutPath, git.WithFilesystem(c.fs))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize checkout repository: %w", err)
	}

	// Add the original remote URL (the checkout will use the bare cache for actual fetches)
	if err := repo.AddRemote(git.RemoteOptions{
		Name: "origin",
		URL:  originalURL,
	}); err != nil {
		return nil, fmt.Errorf("failed to add remote: %w", err)
	}

	// Get underlying repositories for direct manipulation
	checkoutUnderlying := repo.Underlying()
	bareUnderlying := bareRepo.Underlying()

	// Copy all objects from bare repo to checkout
	// This is necessary because the checkout needs access to git objects
	objectIter, err := bareUnderlying.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return nil, fmt.Errorf("failed to get objects: %w", err)
	}

	err = objectIter.ForEach(func(obj plumbing.EncodedObject) error {
		_, err := checkoutUnderlying.Storer.SetEncodedObject(obj)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy objects: %w", err)
	}

	// Copy all references from bare to checkout
	refIter, err := bareUnderlying.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		return checkoutUnderlying.Storer.SetReference(ref)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy references: %w", err)
	}

	// Checkout the specified ref
	worktree, err := repo.Underlying().Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Try checking out as a branch first
	branchRef := plumbing.NewBranchReferenceName(ref)
	err = worktree.Checkout(&gogit.CheckoutOptions{
		Branch: branchRef,
	})
	if err != nil {
		// If branch checkout fails, try as a hash or tag
		hash, resolveErr := bareUnderlying.ResolveRevision(plumbing.Revision(ref))
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to resolve ref %s: %w", ref, resolveErr)
		}

		err = worktree.Checkout(&gogit.CheckoutOptions{
			Hash: *hash,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to checkout ref %s: %w", ref, err)
		}
	}

	return repo, nil
}

// refreshCheckout updates a checkout from its bare repository.
//
// This fetches the latest changes from the bare repository and resets the
// working tree to match. This preserves symlinks and updates files in place.
func (c *RepositoryCache) refreshCheckout(ctx context.Context, checkout, bareRepo *git.Repository, ref string) error {
	// Copy updated references and objects from bare repo to checkout
	checkoutUnderlying := checkout.Underlying()
	bareUnderlying := bareRepo.Underlying()

	// Copy all objects from bare repo to checkout
	// This ensures new commits/trees/blobs are available
	objectIter, err := bareUnderlying.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return fmt.Errorf("failed to get objects: %w", err)
	}

	err = objectIter.ForEach(func(obj plumbing.EncodedObject) error {
		_, err := checkoutUnderlying.Storer.SetEncodedObject(obj)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to copy objects: %w", err)
	}

	// Get all references from bare repo
	refIter, err := bareUnderlying.References()
	if err != nil {
		return fmt.Errorf("failed to get references: %w", err)
	}

	// Copy references to checkout
	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		return checkoutUnderlying.Storer.SetReference(ref)
	})
	if err != nil {
		return fmt.Errorf("failed to copy references: %w", err)
	}

	// Get worktree and reset to the specified ref
	worktree, err := checkoutUnderlying.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Resolve the ref to a hash
	hash, err := bareUnderlying.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("failed to resolve ref %s: %w", ref, err)
	}

	// Hard reset to the ref
	err = worktree.Reset(&gogit.ResetOptions{
		Commit: *hash,
		Mode:   gogit.HardReset,
	})
	if err != nil {
		return fmt.Errorf("failed to reset to %s: %w", ref, err)
	}

	return nil
}

// updateCheckoutMetadata updates or creates metadata for a checkout.
func (c *RepositoryCache) updateCheckoutMetadata(url, ref, cacheKey, compositeKey string, opts *cacheOptions) {
	// Check if metadata already exists
	metadata := c.index.get(compositeKey)
	if metadata != nil {
		// Update existing metadata
		c.index.updateLastAccess(compositeKey)
		return
	}

	// Create new metadata
	now := time.Now()
	metadata = &CheckoutMetadata{
		URL:        url,
		Ref:        ref,
		CacheKey:   cacheKey,
		CreatedAt:  now,
		LastAccess: now,
		TTL:        opts.ttl,
	}

	// Calculate expiration time if TTL is set
	if opts.ttl != nil {
		expiresAt := now.Add(*opts.ttl)
		metadata.ExpiresAt = &expiresAt
	}

	c.index.set(compositeKey, metadata)
}

// RemoveCheckout removes a specific checkout by URL and cache key.
// This removes the checkout directory, updates the index, and removes it from
// the in-memory cache.
//
// Example:
//
//	cache.RemoveCheckout("https://github.com/my/repo", "build-abc123")
func (c *RepositoryCache) RemoveCheckout(url, cacheKey string) error {
	// We need to find all checkouts matching this URL and cache key
	// (there could be multiple if different refs were used)
	allMetadata := c.index.list()

	normalized := normalizeURL(url)
	var toRemove []string

	for key, metadata := range allMetadata {
		if normalizeURL(metadata.URL) == normalized && metadata.CacheKey == cacheKey {
			toRemove = append(toRemove, key)
		}
	}

	if len(toRemove) == 0 {
		return fmt.Errorf("no checkout found for URL %s with cache key %s", url, cacheKey)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove each matching checkout
	for _, compositeKey := range toRemove {
		metadata := c.index.get(compositeKey)
		if metadata == nil {
			continue
		}

		// Build checkout path
		checkoutPath := filepath.Join(c.checkoutDir, normalized, metadata.Ref, metadata.CacheKey)

		// Remove from filesystem (recursively)
		if err := c.removeAll(checkoutPath); err != nil {
			// Log error but continue with cleanup
			// Note: In production, you might want better error handling
			_ = err
		}

		// Remove from in-memory cache
		delete(c.checkouts, compositeKey)

		// Remove from index
		c.index.delete(compositeKey)
	}

	// Save index
	if err := c.index.save(c.fs, c.indexPath); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}
