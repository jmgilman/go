package cache

import (
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/jmgilman/go/git"
)

// WithUpdate checks the remote and updates the cache if needed.
// Without this option, cached data is returned as-is (may be stale).
//
// Example:
//
//	path, _ := cache.GetCheckout(ctx, url, key, WithUpdate())
func WithUpdate() CacheOption {
	return func(opts *cacheOptions) {
		opts.update = true
	}
}

// WithAuth provides authentication for network operations.
//
// Example:
//
//	auth, _ := git.SSHKeyFile("git", "~/.ssh/id_rsa")
//	path, _ := cache.GetCheckout(ctx, url, key, WithAuth(auth))
func WithAuth(auth git.Auth) CacheOption {
	return func(opts *cacheOptions) {
		opts.auth = auth
	}
}

// WithDepth sets shallow clone depth (0 = full clone).
//
// Example:
//
//	path, _ := cache.GetCheckout(ctx, url, key, WithDepth(1))
func WithDepth(depth int) CacheOption {
	return func(opts *cacheOptions) {
		opts.depth = depth
	}
}

// WithRef specifies which Git reference to checkout (branch, tag, or commit).
// If not specified, uses the repository's default branch.
// The ref becomes part of the composite cache key.
//
// Example:
//
//	path, _ := cache.GetCheckout(ctx, url, key, WithRef("v1.0.0"))
func WithRef(ref string) CacheOption {
	return func(opts *cacheOptions) {
		opts.ref = ref
	}
}

// WithTTL sets time-to-live for automatic cleanup via Prune().
// Checkouts with expired TTL are removed during pruning.
// Use for ephemeral checkouts (e.g., CI/CD builds).
//
// Example:
//
//	// Auto-cleanup after 1 hour
//	path, _ := cache.GetCheckout(ctx, url, uuid.New().String(),
//	    WithTTL(1*time.Hour))
func WithTTL(ttl time.Duration) CacheOption {
	return func(opts *cacheOptions) {
		opts.ttl = &ttl
	}
}

// PruneExpired removes checkouts with expired TTL.
// This is the default strategy if no strategies are provided.
func PruneExpired() PruneStrategy {
	return &pruneExpired{}
}

// PruneOlderThan removes checkouts not accessed within the specified duration.
//
// Example:
//
//	cache.Prune(PruneOlderThan(7*24*time.Hour)) // Remove if unused for 7 days
func PruneOlderThan(maxAge time.Duration) PruneStrategy {
	return &pruneOlderThan{maxAge: maxAge}
}

// PruneToSize removes oldest checkouts until total cache size is under limit.
// Removes least-recently-accessed checkouts first.
// Never removes checkouts without TTL (persistent checkouts).
//
// Example:
//
//	cache.Prune(PruneToSize(10*1024*1024*1024)) // Keep under 10GB
func PruneToSize(maxBytes int64) PruneStrategy {
	return &pruneToSize{maxBytes: maxBytes}
}

// WithFilesystem sets the billy filesystem to use for cache operations.
// If not provided, defaults to osfs.New(basePath) rooted at the cache base path.
//
// This option is primarily useful for testing, allowing use of memfs or other
// virtual filesystems.
//
// Example:
//
//	cache, err := cache.NewRepositoryCache("/cache/path",
//	    cache.WithFilesystem(memfs.New()))
func WithFilesystem(fs billy.Filesystem) RepositoryCacheOption {
	return func(opts *repositoryCacheOptions) {
		opts.fs = fs
	}
}
