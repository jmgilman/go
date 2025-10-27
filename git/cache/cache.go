package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/jmgilman/go/git"
)

// NewRepositoryCache creates a new repository cache at the specified base path.
// The cache manages bare repositories and checkouts with metadata tracking.
//
// The basePath typically points to ~/.cache/git or a similar cache directory.
// The cache creates two subdirectories (bare/, checkouts/) and an index.json file.
//
// By default, NewRepositoryCache uses the local filesystem (osfs). A custom
// filesystem can be provided via WithFilesystem for testing purposes.
//
// Example:
//
//	cache, err := cache.NewRepositoryCache("~/.cache/git")
//
//	// With custom filesystem (for testing)
//	cache, err := cache.NewRepositoryCache("/cache/path",
//	    cache.WithFilesystem(memfs.New()))
func NewRepositoryCache(basePath string, opts ...RepositoryCacheOption) (*RepositoryCache, error) {
	// Apply options with defaults
	options := &repositoryCacheOptions{
		fs: osfs.New("/"), // Default to root filesystem
	}
	for _, opt := range opts {
		opt(options)
	}

	fs := options.fs

	// Create directory structure
	bareDir := filepath.Join(basePath, "bare")
	checkoutDir := filepath.Join(basePath, "checkouts")
	indexPath := filepath.Join(basePath, "index.json")

	// Create directories if they don't exist
	if err := fs.MkdirAll(bareDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create bare directory: %w", err)
	}
	if err := fs.MkdirAll(checkoutDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create checkouts directory: %w", err)
	}

	// Create the cache instance
	cache := &RepositoryCache{
		basePath:    basePath,
		bareDir:     bareDir,
		checkoutDir: checkoutDir,
		indexPath:   indexPath,
		fs:          fs,
		bare:        make(map[string]*git.Repository),
		barePaths:   make(map[string]string),
		checkouts:   make(map[string]*git.Repository),
	}

	// Load or create the index
	index, err := loadOrCreateIndex(fs, indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	cache.index = index

	return cache, nil
}

// Clear removes all cached data for a specific URL.
// This includes the bare cache, all checkouts for that URL, and index entries.
//
// Example:
//
//	cache.Clear("https://github.com/my/repo")
func (c *RepositoryCache) Clear(url string) error {
	normalized := normalizeURL(url)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove bare repository
	barePath := filepath.Join(c.bareDir, normalized+".git")
	if err := c.removeAll(barePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove bare repository: %w", err)
	}

	// Remove from in-memory bare cache
	delete(c.bare, normalized)
	delete(c.barePaths, normalized)

	// Find and remove all checkouts for this URL
	allMetadata := c.index.filterByURL(url)
	for key, metadata := range allMetadata {
		// Build checkout path
		checkoutPath := filepath.Join(c.checkoutDir, normalized, metadata.Ref, metadata.CacheKey)

		// Remove from filesystem
		if err := c.removeAll(checkoutPath); err != nil && !os.IsNotExist(err) {
			// Continue even if removal fails
			_ = err
		}

		// Remove from in-memory checkout cache
		delete(c.checkouts, key)

		// Remove from index
		c.index.delete(key)
	}

	// Save index
	if err := c.index.save(c.fs, c.indexPath); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

// ClearAll removes all cached repositories and resets the cache.
// This removes all bare repositories, checkouts, and the index.
func (c *RepositoryCache) ClearAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove all bare repositories
	if err := c.removeAll(c.bareDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove bare repositories: %w", err)
	}

	// Recreate bare directory
	if err := c.fs.MkdirAll(c.bareDir, 0o755); err != nil {
		return fmt.Errorf("failed to recreate bare directory: %w", err)
	}

	// Remove all checkouts
	if err := c.removeAll(c.checkoutDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove checkouts: %w", err)
	}

	// Recreate checkouts directory
	if err := c.fs.MkdirAll(c.checkoutDir, 0o755); err != nil {
		return fmt.Errorf("failed to recreate checkouts directory: %w", err)
	}

	// Clear in-memory caches
	c.bare = make(map[string]*git.Repository)
	c.barePaths = make(map[string]string)
	c.checkouts = make(map[string]*git.Repository)

	// Reset index
	c.index = &cacheIndex{
		Version:   indexVersion,
		Checkouts: make(map[string]*CheckoutMetadata),
	}

	// Save empty index
	if err := c.index.save(c.fs, c.indexPath); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

// Stats returns statistics about the cache (entries, disk usage, etc.).
func (c *RepositoryCache) Stats() (*CacheStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := &CacheStats{
		BareRepos: len(c.bare),
		Checkouts: len(c.index.Checkouts),
	}

	// Calculate sizes
	bareSize, err := c.calculateDirSize(c.bareDir)
	if err == nil {
		stats.BareSize = bareSize
	}

	checkoutsSize, err := c.calculateDirSize(c.checkoutDir)
	if err == nil {
		stats.CheckoutsSize = checkoutsSize
	}

	stats.TotalSize = stats.BareSize + stats.CheckoutsSize

	// Find oldest and newest checkouts
	allMetadata := c.index.list()
	for _, metadata := range allMetadata {
		if stats.OldestCheckout == nil || metadata.CreatedAt.Before(*stats.OldestCheckout) {
			t := metadata.CreatedAt
			stats.OldestCheckout = &t
		}

		if stats.NewestCheckout == nil || metadata.CreatedAt.After(*stats.NewestCheckout) {
			t := metadata.CreatedAt
			stats.NewestCheckout = &t
		}
	}

	return stats, nil
}
