package cache

import (
	"context"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Cache defines the core interface for cache operations.
// Implementations should be safe for concurrent use by multiple goroutines.
type Cache interface {
	// Get retrieves a cache entry by key.
	// Returns ErrCacheExpired if the entry exists but has expired.
	// Returns an error if the key is not found or if retrieval fails.
	Get(ctx context.Context, key string) (*Entry, error)

	// Put stores a cache entry with the given key.
	// Returns ErrCacheFull if the cache cannot accommodate the entry.
	// Overwrites existing entries with the same key.
	Put(ctx context.Context, key string, entry *Entry) error

	// Delete removes a cache entry by key.
	// Returns nil if the key doesn't exist (idempotent operation).
	Delete(ctx context.Context, key string) error

	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error

	// Size returns the total size of all cache entries in bytes.
	Size(ctx context.Context) (int64, error)
}

// ManifestCache defines operations specific to OCI manifest caching.
// Manifests are typically small and have short TTLs for freshness.
type ManifestCache interface {
	// GetManifest retrieves a manifest by its digest.
	// Returns the manifest content or an error if not found.
	GetManifest(ctx context.Context, digest string) (*ocispec.Manifest, error)

	// PutManifest stores a manifest with the given digest.
	// The digest should be validated before storage.
	PutManifest(ctx context.Context, digest string, manifest *ocispec.Manifest) error

	// HasManifest checks if a manifest exists in the cache.
	// This is more efficient than GetManifest when only existence matters.
	HasManifest(ctx context.Context, digest string) (bool, error)

	// ValidateManifest performs validation of a manifest against a reference.
	// This may involve checking the manifest digest against a registry.
	ValidateManifest(ctx context.Context, reference, digest string) (bool, error)
}

// BlobCache defines operations specific to OCI blob caching.
// Blobs can be large and benefit from streaming operations.
type BlobCache interface {
	// GetBlob retrieves a blob by its digest.
	// Returns a ReadCloser that the caller must close.
	GetBlob(ctx context.Context, digest string) (io.ReadCloser, error)

	// PutBlob stores a blob with the given digest.
	// The reader content is consumed and stored.
	PutBlob(ctx context.Context, digest string, reader io.Reader) error

	// HasBlob checks if a blob exists in the cache.
	// This is more efficient than GetBlob when only existence matters.
	HasBlob(ctx context.Context, digest string) (bool, error)

	// DeleteBlob removes a blob from the cache.
	// Returns nil if the blob doesn't exist (idempotent operation).
	DeleteBlob(ctx context.Context, digest string) error
}

// TagCache defines operations for caching tag-to-digest mappings.
// Tags are mutable references that can change frequently, requiring efficient
// resolution with TTL-based caching and history tracking.
type TagCache interface {
	// GetTagMapping retrieves the current digest for a tag reference.
	// Returns the tag mapping or an error if not found or expired.
	GetTagMapping(ctx context.Context, reference string) (*TagMapping, error)

	// PutTagMapping stores or updates a tag-to-digest mapping.
	// Creates history entries when updating existing mappings.
	PutTagMapping(ctx context.Context, reference, digest string) error

	// HasTagMapping checks if a tag mapping exists and is not expired.
	// This is more efficient than GetTagMapping when only existence matters.
	HasTagMapping(ctx context.Context, reference string) (bool, error)

	// DeleteTagMapping removes a tag mapping from the cache.
	// Returns nil if the mapping doesn't exist (idempotent operation).
	DeleteTagMapping(ctx context.Context, reference string) error

	// GetTagHistory retrieves the history of digests for a tag reference.
	// Returns a slice of historical entries in chronological order (oldest first).
	GetTagHistory(ctx context.Context, reference string) ([]TagHistoryEntry, error)
}

// ConfigProvider defines the interface that cache components need from a manager.
type ConfigProvider interface {
	Config() Config
}

// EvictionStrategy defines how entries are selected for eviction when cache size limits are reached.
// Implementations should be deterministic and thread-safe.
type EvictionStrategy interface {
	// SelectForEviction chooses which cache entries should be evicted.
	// Returns a slice of keys to evict, ordered by eviction priority.
	// The entries map provides access to all current cache entries for decision making.
	SelectForEviction(entries map[string]*Entry) []string

	// OnAccess is called when a cache entry is accessed (read).
	// Implementations can update access patterns or priorities.
	OnAccess(entry *Entry)

	// OnAdd is called when a new entry is added to the cache.
	// Implementations can initialize eviction metadata.
	OnAdd(entry *Entry)

	// OnRemove is called when an entry is removed from the cache.
	// Implementations can clean up eviction metadata.
	OnRemove(entry *Entry)
}
