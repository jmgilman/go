package cache

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
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

// createCheckout creates a new checkout from a bare repository using git alternates.
//
// This creates a new working tree by cloning from the local bare repository with
// the Shared option, which creates .git/objects/info/alternates to reference the
// bare repository's object database. This avoids duplicating objects.
func (c *RepositoryCache) createCheckout(ctx context.Context, checkoutPath string, bareRepo *git.Repository, ref string) (*git.Repository, error) {
	// Get bare repo path from our tracking map
	normalized := normalizeURL("")
	for url, repo := range c.bare {
		if repo == bareRepo {
			normalized = url
			break
		}
	}

	barePath, ok := c.barePaths[normalized]
	if !ok {
		return nil, fmt.Errorf("bare repository path not found in cache")
	}

	// Create checkout directory
	if err := c.fs.MkdirAll(checkoutPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create checkout directory: %w", err)
	}

	// Create a filesystem scoped to the checkout path
	scopedFs, err := c.fs.Chroot(checkoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scope filesystem to checkout path: %w", err)
	}

	// Create storage in .git subdirectory
	dotGitFs, err := scopedFs.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("failed to create .git filesystem: %w", err)
	}

	storage := filesystem.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

	// Clone from local bare repo with Shared option (creates alternates)
	_, err = gogit.CloneContext(ctx, storage, scopedFs, &gogit.CloneOptions{
		URL:    barePath,
		Shared: true, // Creates .git/objects/info/alternates
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone with alternates: %w", err)
	}

	// Open the newly created repository using our wrapper
	repo, err := git.Open(checkoutPath, git.WithFilesystem(c.fs))
	if err != nil {
		return nil, fmt.Errorf("failed to open cloned checkout: %w", err)
	}

	// Checkout the specified ref
	worktree, err := repo.Underlying().Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	bareUnderlying := bareRepo.Underlying()

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
// With git alternates, objects are already shared between the bare repo and checkouts,
// so we only need to resolve the ref in the bare repo and reset the checkout's working tree.
// No object or ref copying is needed.
func (c *RepositoryCache) refreshCheckout(ctx context.Context, checkout, bareRepo *git.Repository, ref string) error {
	// Resolve ref in bare repo (source of truth)
	bareUnderlying := bareRepo.Underlying()
	hash, err := bareUnderlying.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("failed to resolve ref %s in bare repo: %w", ref, err)
	}

	// Get checkout path
	checkoutPath := checkout.Filesystem().Root()

	// Open the checkout with filesystem that allows alternates to work
	// We need to use the root filesystem (c.fs) as the AlternatesFS
	checkoutFs, err := c.fs.Chroot(checkoutPath)
	if err != nil {
		return fmt.Errorf("failed to chroot to checkout: %w", err)
	}

	dotGitFs, err := checkoutFs.Chroot(".git")
	if err != nil {
		return fmt.Errorf("failed to chroot to .git: %w", err)
	}

	// Create storage with AlternatesFS set to root filesystem
	// This allows go-git to access alternates outside the repository
	storage := filesystem.NewStorageWithOptions(dotGitFs, cache.NewObjectLRUDefault(),
		filesystem.Options{
			AlternatesFS: c.fs, // Use root filesystem to access alternates
		})

	// Open repository with the configured storage
	freshRepo, err := gogit.Open(storage, checkoutFs)
	if err != nil {
		return fmt.Errorf("failed to open checkout with alternates support: %w", err)
	}

	// Reset checkout working tree
	worktree, err := freshRepo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Reset(&gogit.ResetOptions{
		Commit: *hash,
		Mode:   gogit.HardReset,
	})
	if err != nil {
		return fmt.Errorf("failed to reset to %s (%s): %w", ref, hash, err)
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
