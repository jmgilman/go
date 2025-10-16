package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Prune removes checkouts based on the provided strategies.
// Common strategies include expired TTL, last access time, and cache size limits.
//
// If no strategies are provided, defaults to removing expired checkouts.
//
// Examples:
//
//	// Remove expired checkouts (TTL-based)
//	cache.Prune()
//
//	// Remove checkouts not accessed in 7 days
//	cache.Prune(PruneOlderThan(7*24*time.Hour))
//
//	// Multiple strategies (OR logic)
//	cache.Prune(PruneExpired(), PruneOlderThan(30*24*time.Hour))
func (c *RepositoryCache) Prune(strategies ...PruneStrategy) error {
	// Default to PruneExpired if no strategies provided
	if len(strategies) == 0 {
		strategies = []PruneStrategy{PruneExpired()}
	}

	// Check if PruneToSize is among the strategies
	var sizeStrategy *pruneToSize
	var otherStrategies []PruneStrategy
	for _, strategy := range strategies {
		if ps, ok := strategy.(*pruneToSize); ok {
			sizeStrategy = ps
		} else {
			otherStrategies = append(otherStrategies, strategy)
		}
	}

	// Get all checkouts
	allMetadata := c.index.list()

	// Track what to remove
	var toRemove []string

	// Apply standard strategies (OR logic)
	for key, metadata := range allMetadata {
		for _, strategy := range otherStrategies {
			if strategy.ShouldPrune(metadata) {
				toRemove = append(toRemove, key)
				break // Already marked for removal
			}
		}
	}

	// Apply size strategy if present (requires special handling)
	if sizeStrategy != nil {
		sizeToRemove, err := c.applySizeStrategy(sizeStrategy, allMetadata, toRemove)
		if err != nil {
			return fmt.Errorf("failed to apply size strategy: %w", err)
		}
		toRemove = append(toRemove, sizeToRemove...)
	}

	// Remove duplicates from toRemove
	toRemove = uniqueStrings(toRemove)

	// Remove each checkout
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, compositeKey := range toRemove {
		metadata := c.index.get(compositeKey)
		if metadata == nil {
			continue
		}

		// Build checkout path
		normalized := normalizeURL(metadata.URL)
		checkoutPath := filepath.Join(c.checkoutDir, normalized, metadata.Ref, metadata.CacheKey)

		// Remove from filesystem (recursively)
		if err := c.removeAll(checkoutPath); err != nil {
			// Log error but continue with cleanup
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

// applySizeStrategy determines which checkouts to remove to stay under size limit.
// It removes least-recently-accessed checkouts first, but never removes persistent
// checkouts (those without TTL).
func (c *RepositoryCache) applySizeStrategy(strategy *pruneToSize, allMetadata map[string]*CheckoutMetadata, alreadyMarked []string) ([]string, error) {
	// Calculate current total size
	totalSize, err := c.calculateTotalSize()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate total size: %w", err)
	}

	// If already under limit, nothing to do
	if totalSize <= strategy.maxBytes {
		return nil, nil
	}

	// Build list of candidates (only ephemeral checkouts with TTL)
	type candidate struct {
		key        string
		metadata   *CheckoutMetadata
		size       int64
		lastAccess time.Time
	}

	var candidates []candidate
	markedSet := make(map[string]bool)
	for _, key := range alreadyMarked {
		markedSet[key] = true
	}

	for key, metadata := range allMetadata {
		// Skip if already marked for removal
		if markedSet[key] {
			continue
		}

		// Skip persistent checkouts (no TTL)
		if metadata.TTL == nil {
			continue
		}

		// Calculate size of this checkout
		normalized := normalizeURL(metadata.URL)
		checkoutPath := filepath.Join(c.checkoutDir, normalized, metadata.Ref, metadata.CacheKey)
		size, err := c.calculateDirSize(checkoutPath)
		if err != nil {
			// Skip if can't determine size
			continue
		}

		candidates = append(candidates, candidate{
			key:        key,
			metadata:   metadata,
			size:       size,
			lastAccess: metadata.LastAccess,
		})
	}

	// Sort by last access time (oldest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastAccess.Before(candidates[j].lastAccess)
	})

	// Remove candidates until we're under the limit
	var toRemove []string
	currentSize := totalSize

	for _, candidate := range candidates {
		if currentSize <= strategy.maxBytes {
			break
		}

		toRemove = append(toRemove, candidate.key)
		currentSize -= candidate.size
	}

	return toRemove, nil
}

// calculateTotalSize calculates the total disk usage of all checkouts.
func (c *RepositoryCache) calculateTotalSize() (int64, error) {
	var total int64

	allMetadata := c.index.list()
	for _, metadata := range allMetadata {
		normalized := normalizeURL(metadata.URL)
		checkoutPath := filepath.Join(c.checkoutDir, normalized, metadata.Ref, metadata.CacheKey)

		size, err := c.calculateDirSize(checkoutPath)
		if err != nil {
			// Skip if can't determine size
			continue
		}

		total += size
	}

	return total, nil
}

// calculateDirSize calculates the disk usage of a directory recursively.
func (c *RepositoryCache) calculateDirSize(path string) (int64, error) {
	var size int64

	// Check if path exists
	info, err := c.fs.Stat(path)
	if err != nil {
		return 0, err
	}

	// If it's a file, return its size
	if !info.IsDir() {
		return info.Size(), nil
	}

	// If it's a directory, walk it
	err = c.walkDir(path, func(filePath string, info os.FileInfo) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// walkDir walks a directory tree, calling fn for each file.
func (c *RepositoryCache) walkDir(root string, fn func(path string, info os.FileInfo) error) error {
	// Read directory entries
	entries, err := c.fs.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := c.fs.Stat(path)
		if err != nil {
			continue
		}

		// Call fn for this entry
		if err := fn(path, info); err != nil {
			return err
		}

		// Recurse into subdirectories
		if info.IsDir() {
			if err := c.walkDir(path, fn); err != nil {
				return err
			}
		}
	}

	return nil
}

// removeAll removes a path and all its children.
func (c *RepositoryCache) removeAll(path string) error {
	// Check if path exists
	info, err := c.fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone
		}
		return err
	}

	// If it's a file, just remove it
	if !info.IsDir() {
		return c.fs.Remove(path)
	}

	// If it's a directory, remove all children first
	entries, err := c.fs.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		if err := c.removeAll(childPath); err != nil {
			return err
		}
	}

	// Remove the directory itself
	return c.fs.Remove(path)
}

// uniqueStrings returns a deduplicated slice of strings.
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(input))

	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}
