package cache

import (
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/jmgilman/go/git"
)

// RepositoryCache manages a two-tier cache of Git repositories.
//
// Tier 1 (Bare Cache): Stores bare repositories (Git database only) for efficient
// storage and network operations.
//
// Tier 2 (Checkout Cache): Stores working trees for actual file access. Each
// checkout is identified by a composite key (URL + ref + cacheKey).
//
// The cache maintains a metadata index for lifecycle management, supporting
// TTL-based expiration and automatic garbage collection.
type RepositoryCache struct {
	basePath    string // Base cache directory (e.g., ~/.cache/git)
	bareDir     string // bare/ subdirectory
	checkoutDir string // checkouts/ subdirectory
	indexPath   string // index.json path

	fs        billy.Filesystem           // Filesystem abstraction for all I/O
	index     *cacheIndex                // Metadata index
	bare      map[string]*git.Repository // URL → bare repo (in-memory)
	barePaths map[string]string          // normalized URL → bare repo filesystem path
	checkouts map[string]*git.Repository // composite key → checkout repo (in-memory)

	mu sync.RWMutex
}

// CheckoutMetadata tracks metadata for a single checkout.
type CheckoutMetadata struct {
	URL        string         // Original repository URL
	Ref        string         // Git reference (branch/tag/commit)
	CacheKey   string         // User-provided cache key
	CreatedAt  time.Time      // When checkout was created
	LastAccess time.Time      // Last time checkout was accessed
	TTL        *time.Duration // Time-to-live (nil = persistent)
	ExpiresAt  *time.Time     // Computed expiration time
}

// CacheStats provides statistics about the cache.
type CacheStats struct {
	BareRepos      int   // Number of bare repositories
	Checkouts      int   // Number of checkouts
	TotalSize      int64 // Total disk usage in bytes
	BareSize       int64 // Disk usage of bare repositories
	CheckoutsSize  int64 // Disk usage of checkouts
	OldestCheckout *time.Time
	NewestCheckout *time.Time
}

// CacheOption configures cache operations.
type CacheOption func(*cacheOptions)

type cacheOptions struct {
	auth   git.Auth
	update bool           // Check remote and update if needed
	depth  int            // For shallow clones
	ref    string         // Git reference to checkout
	ttl    *time.Duration // Time-to-live for automatic cleanup
}

// RepositoryCacheOption configures RepositoryCache creation.
type RepositoryCacheOption func(*repositoryCacheOptions)

type repositoryCacheOptions struct {
	fs billy.Filesystem // Filesystem to use for all I/O operations
}

// PruneStrategy determines which checkouts should be removed during pruning.
type PruneStrategy interface {
	ShouldPrune(metadata *CheckoutMetadata) bool
}

// pruneExpired implements PruneStrategy for TTL-based expiration.
type pruneExpired struct{}

func (p *pruneExpired) ShouldPrune(metadata *CheckoutMetadata) bool {
	if metadata.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*metadata.ExpiresAt)
}

// pruneOlderThan implements PruneStrategy for last-access-based expiration.
type pruneOlderThan struct {
	maxAge time.Duration
}

func (p *pruneOlderThan) ShouldPrune(metadata *CheckoutMetadata) bool {
	return time.Since(metadata.LastAccess) > p.maxAge
}

// pruneToSize implements PruneStrategy for size-based pruning.
type pruneToSize struct {
	maxBytes int64
}

func (p *pruneToSize) ShouldPrune(metadata *CheckoutMetadata) bool {
	// This strategy requires special handling in the Prune method
	// to calculate total size and remove oldest checkouts first
	return false
}
