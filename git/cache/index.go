package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

const indexVersion = "1"

// cacheIndex manages the metadata index for all checkouts.
// It provides thread-safe access to checkout metadata with JSON persistence.
type cacheIndex struct {
	Version   string                       `json:"version"`
	Checkouts map[string]*CheckoutMetadata `json:"checkouts"`
	mu        sync.RWMutex
}

// loadOrCreateIndex loads an existing index from disk or creates a new one.
// If the index file doesn't exist, it creates a new empty index.
// If the file exists but is corrupted, it returns an error.
func loadOrCreateIndex(fs billy.Filesystem, path string) (*cacheIndex, error) {
	// Check if index file exists
	if _, err := fs.Stat(path); os.IsNotExist(err) {
		// Create new index
		return &cacheIndex{
			Version:   indexVersion,
			Checkouts: make(map[string]*CheckoutMetadata),
		}, nil
	}

	// Load existing index
	data, err := util.ReadFile(fs, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	var index cacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index file: %w", err)
	}

	// Validate version
	if index.Version != indexVersion {
		return nil, fmt.Errorf("unsupported index version: %s (expected %s)", index.Version, indexVersion)
	}

	// Ensure map is initialized
	if index.Checkouts == nil {
		index.Checkouts = make(map[string]*CheckoutMetadata)
	}

	return &index, nil
}

// save writes the index to disk atomically.
// This method is thread-safe and uses atomic write-to-temp + rename.
func (idx *cacheIndex) save(fs billy.Filesystem, path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// Write to temporary file first (atomic write)
	tmpPath := path + ".tmp"
	tmpFile, err := fs.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary index file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = fs.Remove(tmpPath)
		return fmt.Errorf("failed to write temporary index file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = fs.Remove(tmpPath)
		return fmt.Errorf("failed to close temporary index file: %w", err)
	}

	// Rename to final path (atomic on POSIX systems)
	if err := fs.Rename(tmpPath, path); err != nil {
		// Clean up temp file on error
		_ = fs.Remove(tmpPath)
		return fmt.Errorf("failed to rename index file: %w", err)
	}

	return nil
}

// get retrieves checkout metadata by composite key.
// Returns nil if the key doesn't exist.
func (idx *cacheIndex) get(key string) *CheckoutMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.Checkouts[key]
}

// set stores or updates checkout metadata for a composite key.
func (idx *cacheIndex) set(key string, metadata *CheckoutMetadata) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.Checkouts[key] = metadata
}

// delete removes checkout metadata by composite key.
func (idx *cacheIndex) delete(key string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.Checkouts, key)
}

// updateLastAccess updates the last access time for a checkout.
// If TTL is set, it also updates the expiration time.
func (idx *cacheIndex) updateLastAccess(key string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	metadata, exists := idx.Checkouts[key]
	if !exists {
		return
	}

	metadata.LastAccess = time.Now()

	// Update expiration time if TTL is set
	if metadata.TTL != nil {
		expiresAt := metadata.LastAccess.Add(*metadata.TTL)
		metadata.ExpiresAt = &expiresAt
	}
}

// list returns all checkout metadata.
// Returns a copy to avoid concurrent modification issues.
func (idx *cacheIndex) list() map[string]*CheckoutMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Create a shallow copy
	result := make(map[string]*CheckoutMetadata, len(idx.Checkouts))
	for key, metadata := range idx.Checkouts {
		result[key] = metadata
	}

	return result
}

// filterByURL returns all checkout metadata for a specific URL.
func (idx *cacheIndex) filterByURL(url string) map[string]*CheckoutMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	normalizedURL := normalizeURL(url)
	result := make(map[string]*CheckoutMetadata)

	for key, metadata := range idx.Checkouts {
		if normalizeURL(metadata.URL) == normalizedURL {
			result[key] = metadata
		}
	}

	return result
}
