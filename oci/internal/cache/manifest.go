package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// manifestCacheEntry wraps a manifest with cache metadata for TTL management.
type manifestCacheEntry struct {
	Manifest   *ocispec.Manifest `json:"manifest"`
	CreatedAt  time.Time         `json:"created_at"`
	AccessedAt time.Time         `json:"accessed_at"`
}

// manifestCache implements caching for OCI manifest objects.
// Manifests are cached with short TTLs (5 minutes by default) since they
// represent mutable references that can change frequently.
type manifestCache struct {
	storage *Storage
	manager ConfigProvider
}

// NewManifestCache creates a new manifest cache instance.
//
//nolint:revive // Returns unexported type for encapsulation, consistent with codebase patterns
func NewManifestCache(storage *Storage, manager ConfigProvider) *manifestCache {
	return &manifestCache{
		storage: storage,
		manager: manager,
	}
}

// GetManifest retrieves a manifest by its digest.
// Returns the manifest or an error if not found or expired.
func (mc *manifestCache) GetManifest(
	ctx context.Context,
	digest string,
) (*ocispec.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Validate digest format
	if !isValidDigest(digest) {
		return nil, fmt.Errorf("invalid digest format: %s", digest)
	}

	// Read manifest entry from storage
	entry, err := mc.readManifestEntry(ctx, digest)
	if err != nil {
		return nil, err
	}

	// Check if entry has expired
	if mc.isExpired(entry) {
		// Remove expired entry
		_ = mc.removeManifestEntry(ctx, digest) // Ignore error in cleanup
		return nil, fmt.Errorf("manifest not found in cache: %s", digest)
	}

	// Update access time
	entry.AccessedAt = time.Now()
	// Ignore errors when updating access time to avoid failing the read operation
	_ = mc.writeManifestEntry(ctx, digest, entry)

	return entry.Manifest, nil
}

// PutManifest stores a manifest with the given digest.
// Validates the manifest before storage and handles TTL management.
func (mc *manifestCache) PutManifest(
	ctx context.Context,
	digest string,
	manifest *ocispec.Manifest,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Validate inputs
	if digest == "" {
		return fmt.Errorf("digest cannot be empty")
	}
	if manifest == nil {
		return fmt.Errorf("manifest cannot be nil")
	}

	// Validate digest format
	if !isValidDigest(digest) {
		return fmt.Errorf("invalid digest format: %s", digest)
	}

	// Validate manifest structure
	if err := mc.validateManifest(manifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Create cache entry with current timestamp
	now := time.Now()
	entry := &manifestCacheEntry{
		Manifest:   manifest,
		CreatedAt:  now,
		AccessedAt: now,
	}

	// Store manifest entry
	if err := mc.writeManifestEntry(ctx, digest, entry); err != nil {
		return fmt.Errorf("failed to write manifest entry to storage: %w", err)
	}

	return nil
}

// HasManifest checks if a manifest exists in the cache and is not expired.
func (mc *manifestCache) HasManifest(ctx context.Context, digest string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	if !isValidDigest(digest) {
		return false, fmt.Errorf("invalid digest format: %s", digest)
	}

	// Try to read the entry
	entry, err := mc.readManifestEntry(ctx, digest)
	if err != nil {
		return false, nil // Treat read errors as "not found"
	}

	// Check if expired
	if mc.isExpired(entry) {
		// Remove expired entry
		_ = mc.removeManifestEntry(ctx, digest) // Ignore error in cleanup
		return false, nil
	}

	return true, nil
}

// ValidateManifest performs validation of a manifest against a reference.
// This implementation does basic validation and could be extended with
// registry HEAD requests for more thorough validation.
func (mc *manifestCache) ValidateManifest(
	ctx context.Context,
	reference, digest string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	// Basic validation: check if reference and digest are provided
	if reference == "" {
		return false, fmt.Errorf("reference cannot be empty")
	}
	if digest == "" {
		return false, fmt.Errorf("digest cannot be empty")
	}

	// Check if digest format is valid
	if !isValidDigest(digest) {
		return false, fmt.Errorf("invalid digest format: %s", digest)
	}

	// For now, we consider validation successful if the manifest exists in cache
	// In a full implementation, this would make HEAD requests to the registry
	exists, err := mc.HasManifest(ctx, digest)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// validateManifest performs basic validation of a manifest structure.
func (mc *manifestCache) validateManifest(manifest *ocispec.Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}

	// Check schema version
	if manifest.SchemaVersion != 2 {
		return fmt.Errorf("unsupported schema version: %d", manifest.SchemaVersion)
	}

	// Validate media type
	if manifest.MediaType == "" {
		return fmt.Errorf("media type cannot be empty")
	}

	// Validate config descriptor
	if manifest.Config.MediaType == "" {
		return fmt.Errorf("config media type cannot be empty")
	}
	if manifest.Config.Size < 0 {
		return fmt.Errorf("config size cannot be negative: %d", manifest.Config.Size)
	}

	// Validate layer descriptors
	for i, layer := range manifest.Layers {
		if layer.MediaType == "" {
			return fmt.Errorf("layer %d media type cannot be empty", i)
		}
		if layer.Size < 0 {
			return fmt.Errorf("layer %d size cannot be negative: %d", i, layer.Size)
		}
	}

	return nil
}

// manifestPath returns the storage path for a manifest digest.
func (mc *manifestCache) manifestPath(digest string) string {
	// Use digest as filename directly since it's already a safe identifier
	// Format: manifests/sha256:abc123...
	return fmt.Sprintf("manifests/%s", digest)
}

// readManifestEntry reads a manifest entry from storage.
func (mc *manifestCache) readManifestEntry(
	ctx context.Context,
	digest string,
) (*manifestCacheEntry, error) {
	path := mc.manifestPath(digest)
	data, err := mc.storage.ReadWithIntegrity(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest entry from storage: %w", err)
	}

	var entry manifestCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest entry: %w", err)
	}

	return &entry, nil
}

// writeManifestEntry writes a manifest entry to storage.
func (mc *manifestCache) writeManifestEntry(
	ctx context.Context,
	digest string,
	entry *manifestCacheEntry,
) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest entry: %w", err)
	}

	path := mc.manifestPath(digest)
	if err := mc.storage.WriteAtomically(ctx, path, data); err != nil {
		return fmt.Errorf("failed to write manifest entry to storage: %w", err)
	}

	return nil
}

// removeManifestEntry removes a manifest entry from storage.
func (mc *manifestCache) removeManifestEntry(ctx context.Context, digest string) error {
	path := mc.manifestPath(digest)
	return mc.storage.Remove(ctx, path)
}

// isExpired checks if a manifest entry has expired based on the cache TTL.
func (mc *manifestCache) isExpired(entry *manifestCacheEntry) bool {
	ttl := mc.manager.Config().DefaultTTL
	if ttl <= 0 {
		return false // No expiration if TTL is disabled
	}

	return time.Since(entry.CreatedAt) > ttl
}

// isValidDigest checks if a digest string has valid format (algorithm:hex).
func isValidDigest(digest string) bool {
	if digest == "" {
		return false
	}

	parts := strings.Split(digest, ":")
	if len(parts) != 2 {
		return false
	}

	algorithm, hash := parts[0], parts[1]
	if algorithm == "" || hash == "" {
		return false
	}

	return isValidAlgorithm(algorithm) && isValidHexHash(algorithm, hash)
}

// isValidAlgorithm checks if the algorithm is supported.
func isValidAlgorithm(algorithm string) bool {
	switch algorithm {
	case "sha256", "sha384", "sha512":
		return true
	default:
		return false
	}
}

// isValidHexHash checks if the hash is valid hex and has the correct length for the algorithm.
func isValidHexHash(algorithm, hash string) bool {
	if !isHexString(hash) {
		return false
	}

	// Check length based on algorithm
	switch algorithm {
	case "sha256":
		return len(hash) == 64
	case "sha384":
		return len(hash) == 96
	case "sha512":
		return len(hash) == 128
	default:
		return false
	}
}

// isHexString checks if a string contains only hexadecimal characters.
func isHexString(s string) bool {
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'A') || (r > 'F' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
