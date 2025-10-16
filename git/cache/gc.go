package cache

import (
	"context"
	"sync"
	"time"
)

// StartGC starts a background garbage collector that periodically prunes the cache.
// The GC runs at the specified interval using the provided pruning strategies.
//
// Returns a function to stop the garbage collector. This function is safe to call
// multiple times and will block until the GC goroutine has fully stopped.
//
// The stop function should be called on shutdown or deferred to ensure clean shutdown.
//
// Examples:
//
//	// Start GC that runs every 5 minutes, removing expired checkouts
//	stop := cache.StartGC(5*time.Minute, PruneExpired())
//	defer stop()
//
//	// Worker service with multiple strategies
//	stop := cache.StartGC(10*time.Minute,
//	    PruneExpired(),
//	    PruneOlderThan(24*time.Hour))
//	defer stop()
func (c *RepositoryCache) StartGC(interval time.Duration, strategies ...PruneStrategy) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Run prune with the provided strategies
				_ = c.Prune(strategies...)
				// Note: Errors are silently ignored as this runs in background
				// In production, you might want to log these
			}
		}
	}()

	// Return stop function that cancels context and waits for goroutine to finish
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			wg.Wait()
		})
	}
}
