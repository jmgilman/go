package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// tagCacheEntry wraps a tag mapping with cache metadata for TTL management.
type tagCacheEntry struct {
	Mapping    *TagMapping `json:"mapping"`
	CreatedAt  time.Time   `json:"created_at"`
	AccessedAt time.Time   `json:"accessed_at"`
}

// tagCache implements caching for OCI tag-to-digest mappings.
// Tags are mutable references that can change frequently, requiring TTL-based
// caching with history tracking for efficient resolution.
type tagCache struct {
	storage *Storage
	config  TagResolverConfig
}

// NewTagCache creates a new tag cache instance.
//
//nolint:revive // Returns unexported type for encapsulation, consistent with codebase patterns
func NewTagCache(storage *Storage, config TagResolverConfig) *tagCache {
	if err := config.Validate(); err != nil {
		// Set defaults if validation fails due to unset values
		config.SetDefaults()
	}

	return &tagCache{
		storage: storage,
		config:  config,
	}
}

// GetTagMapping retrieves the current digest for a tag reference.
// Returns the tag mapping or an error if not found or expired.
func (tc *tagCache) GetTagMapping(ctx context.Context, reference string) (*TagMapping, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Validate reference format
	if !isValidReference(reference) {
		return nil, fmt.Errorf("invalid reference format: %s", reference)
	}

	// Read tag entry from storage
	entry, err := tc.readTagEntry(ctx, reference)
	if err != nil {
		return nil, err
	}

	// Check if entry has expired
	if tc.isExpired(entry) {
		// Remove expired entry
		_ = tc.removeTagEntry(ctx, reference) // Ignore error in cleanup
		return nil, fmt.Errorf("tag mapping not found in cache: %s", reference)
	}

	// Update access time and increment access count
	entry.AccessedAt = time.Now()
	entry.Mapping.AccessCount++
	// Ignore errors when updating access metadata to avoid failing the read operation
	_ = tc.writeTagEntry(ctx, reference, entry)

	return entry.Mapping, nil
}

// PutTagMapping stores or updates a tag-to-digest mapping.
// Creates history entries when updating existing mappings.
func (tc *tagCache) PutTagMapping(ctx context.Context, reference, digest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Validate inputs
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}
	if digest == "" {
		return fmt.Errorf("digest cannot be empty")
	}

	// Validate reference format
	if !isValidReference(reference) {
		return fmt.Errorf("invalid reference format: %s", reference)
	}

	// Validate digest format
	if !isValidDigest(digest) {
		return fmt.Errorf("invalid digest format: %s", digest)
	}

	now := time.Now()

	// Check if mapping already exists
	existingEntry, err := tc.readTagEntry(ctx, reference)
	if err == nil && !tc.isExpired(existingEntry) {
		// Update existing mapping
		if tc.config.EnableHistory && existingEntry.Mapping.Digest != digest {
			// Add current mapping to history before updating
			historyEntry := TagHistoryEntry{
				Digest:    existingEntry.Mapping.Digest,
				ChangedAt: now,
			}

			// Append to history and maintain max size (chronological order: oldest first)
			//nolint:gocritic // append result assigned to new variable for size management
			newHistory := append(existingEntry.Mapping.History, historyEntry)
			if len(newHistory) > tc.config.MaxHistorySize {
				// Remove oldest entries if we exceed max size
				newHistory = newHistory[len(newHistory)-tc.config.MaxHistorySize:]
			}
			existingEntry.Mapping.History = newHistory
		}

		// Update the mapping
		existingEntry.Mapping.Digest = digest
		existingEntry.Mapping.UpdatedAt = now
		existingEntry.AccessedAt = now

		// Write updated entry
		if err := tc.writeTagEntry(ctx, reference, existingEntry); err != nil {
			return fmt.Errorf("failed to update tag entry to storage: %w", err)
		}

		return nil
	}

	// Create new mapping
	mapping := &TagMapping{
		Reference:   reference,
		Digest:      digest,
		CreatedAt:   now,
		UpdatedAt:   now,
		AccessCount: 0,
		History:     []TagHistoryEntry{},
	}

	entry := &tagCacheEntry{
		Mapping:    mapping,
		CreatedAt:  now,
		AccessedAt: now,
	}

	// Store new entry
	if err := tc.writeTagEntry(ctx, reference, entry); err != nil {
		return fmt.Errorf("failed to write tag entry to storage: %w", err)
	}

	return nil
}

// HasTagMapping checks if a tag mapping exists and is not expired.
// This is more efficient than GetTagMapping when only existence matters.
func (tc *tagCache) HasTagMapping(ctx context.Context, reference string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	if !isValidReference(reference) {
		return false, fmt.Errorf("invalid reference format: %s", reference)
	}

	// Try to read the entry
	entry, err := tc.readTagEntry(ctx, reference)
	if err != nil {
		return false, nil // Treat read errors as "not found"
	}

	// Check if expired
	if tc.isExpired(entry) {
		// Remove expired entry
		_ = tc.removeTagEntry(ctx, reference) // Ignore error in cleanup
		return false, nil
	}

	return true, nil
}

// DeleteTagMapping removes a tag mapping from the cache.
// Returns nil if the mapping doesn't exist (idempotent operation).
func (tc *tagCache) DeleteTagMapping(ctx context.Context, reference string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if !isValidReference(reference) {
		return fmt.Errorf("invalid reference format: %s", reference)
	}

	return tc.removeTagEntry(ctx, reference)
}

// GetTagHistory retrieves the history of digests for a tag reference.
// Returns a slice of historical entries in chronological order (oldest first).
func (tc *tagCache) GetTagHistory(
	ctx context.Context,
	reference string,
) ([]TagHistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if !isValidReference(reference) {
		return nil, fmt.Errorf("invalid reference format: %s", reference)
	}

	// Read the current entry
	entry, err := tc.readTagEntry(ctx, reference)
	if err != nil {
		return []TagHistoryEntry{}, nil // Return empty history if not found
	}

	// Check if expired
	if tc.isExpired(entry) {
		// Remove expired entry
		_ = tc.removeTagEntry(ctx, reference) // Ignore error in cleanup
		return []TagHistoryEntry{}, nil
	}

	// Return a copy of the history
	history := make([]TagHistoryEntry, len(entry.Mapping.History))
	copy(history, entry.Mapping.History)

	return history, nil
}

// tagPath returns the storage path for a tag reference.
func (tc *tagCache) tagPath(reference string) string {
	// Create a safe filename from the reference
	// Replace problematic characters with underscores
	safeName := strings.ReplaceAll(reference, "/", "_")
	safeName = strings.ReplaceAll(safeName, ":", "_")
	safeName = strings.ReplaceAll(safeName, "@", "_")

	// Use safe name as filename directly
	// Format: tags/safe_reference_name
	return fmt.Sprintf("tags/%s", safeName)
}

// readTagEntry reads a tag entry from storage.
func (tc *tagCache) readTagEntry(ctx context.Context, reference string) (*tagCacheEntry, error) {
	path := tc.tagPath(reference)
	data, err := tc.storage.ReadWithIntegrity(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tag entry from storage: %w", err)
	}

	var entry tagCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tag entry: %w", err)
	}

	return &entry, nil
}

// writeTagEntry writes a tag entry to storage.
func (tc *tagCache) writeTagEntry(
	ctx context.Context,
	reference string,
	entry *tagCacheEntry,
) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal tag entry: %w", err)
	}

	path := tc.tagPath(reference)
	if err := tc.storage.WriteAtomically(ctx, path, data); err != nil {
		return fmt.Errorf("failed to write tag entry to storage: %w", err)
	}

	return nil
}

// removeTagEntry removes a tag entry from storage.
func (tc *tagCache) removeTagEntry(ctx context.Context, reference string) error {
	path := tc.tagPath(reference)
	return tc.storage.Remove(ctx, path)
}

// isExpired checks if a tag entry has expired based on the cache TTL.
func (tc *tagCache) isExpired(entry *tagCacheEntry) bool {
	ttl := tc.config.DefaultTTL
	if ttl <= 0 {
		return false // No expiration if TTL is disabled
	}

	return time.Since(entry.CreatedAt) > ttl
}

// isValidReference checks if a reference string has valid format.
// Basic validation for OCI reference format.
func isValidReference(reference string) bool {
	if reference == "" {
		return false
	}

	// Handle cases without explicit registry (default to docker.io)
	var lastPart string
	if strings.Contains(reference, "/") {
		parts := strings.Split(reference, "/")
		lastPart = parts[len(parts)-1]
	} else {
		// No registry specified, treat the whole reference as repository:tag
		lastPart = reference
	}

	switch {
	case strings.Contains(lastPart, ":"):
		// Tag format: repository:tag
		tagParts := strings.Split(lastPart, ":")
		if len(tagParts) != 2 || tagParts[1] == "" {
			return false
		}
	case strings.Contains(lastPart, "@"):
		// Digest format: repository@digest
		digestParts := strings.Split(lastPart, "@")
		if len(digestParts) != 2 || !isValidDigest(digestParts[1]) {
			return false
		}
	default:
		// Neither tag nor digest - invalid
		return false
	}

	return true
}
