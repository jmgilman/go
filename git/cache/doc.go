// Package cache provides efficient caching of Git repositories with two-tier storage.
//
// # Overview
//
// The repository cache avoids repeated network operations by maintaining:
//
//  1. Bare repositories (Tier 1): Single source of truth for Git objects
//  2. Working tree checkouts (Tier 2): Multiple isolated checkouts per repository
//  3. Metadata index: Tracks checkout lifecycle with TTL-based expiration
//
// # Architecture
//
// The cache uses a two-tier structure:
//
//	~/.cache/git/
//	├── index.json               # Metadata index
//	├── bare/                    # Tier 1: Bare repositories
//	│   └── github.com/
//	│       └── my/
//	│           └── repo.git/
//	└── checkouts/               # Tier 2: Working trees
//	    └── github.com/my/repo/
//	        ├── main/
//	        │   ├── team-docs/       # Persistent checkout
//	        │   └── build-abc123/    # Ephemeral checkout
//	        └── v1.0.0/
//	            └── prod-ref/
//
// # Usage
//
// Create a cache and get a checkout:
//
//	cache, err := cache.NewRepositoryCache("~/.cache/git")
//	if err != nil {
//	    return err
//	}
//
//	// Persistent checkout with stable key
//	path, err := cache.GetCheckout(ctx, "https://github.com/my/repo", "team-docs",
//	    cache.WithRef("main"))
//
//	// Ephemeral checkout with unique key and TTL
//	cacheKey := uuid.New().String()
//	path, err = cache.GetCheckout(ctx, "https://github.com/my/repo", cacheKey,
//	    cache.WithRef("main"),
//	    cache.WithTTL(1*time.Hour))
//
// Start background garbage collection:
//
//	stop := cache.StartGC(5*time.Minute, cache.PruneExpired())
//	defer stop()
//
// # Cache Keys
//
// Checkouts are identified by a composite key: (URL + ref + cacheKey)
//
//   - Same composite key = reuses existing checkout
//   - Different cache key = creates isolated checkout
//   - Different ref = creates separate checkout for that ref
//
// Use stable keys (e.g., "team-docs") for persistent checkouts that should be
// reused across calls. Use unique keys (e.g., UUID) for ephemeral checkouts
// that should be isolated and cleaned up after use.
package cache
