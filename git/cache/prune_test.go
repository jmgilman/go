package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
)

func TestPrune_DefaultStrategy(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create checkouts: one expired, one not expired
	ttl := 10 * time.Millisecond

	// Create checkout with TTL
	_, err = cache.GetCheckout(ctx, sourceRepo, "expired-key", WithRef("master"), WithTTL(ttl))
	if err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	// Create checkout without TTL (persistent)
	_, err = cache.GetCheckout(ctx, sourceRepo, "persistent-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	// Verify we have 2 checkouts
	if len(cache.index.Checkouts) != 2 {
		t.Fatalf("expected 2 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Wait for the TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Run prune with default strategy (PruneExpired)
	err = cache.Prune()
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Should have removed expired checkout, kept persistent
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after prune, got %d", len(cache.index.Checkouts))
	}

	// Verify the persistent checkout still exists
	compositeKey := makeCompositeKey(sourceRepo, "master", "persistent-key")
	if cache.index.get(compositeKey) == nil {
		t.Error("expected persistent checkout to remain")
	}
}

func TestPrune_ExpiredStrategy(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	ttl := 50 * time.Millisecond

	// Create multiple checkouts with different expiration times
	_, err = cache.GetCheckout(ctx, sourceRepo, "expired-1", WithRef("master"), WithTTL(ttl))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	_, err = cache.GetCheckout(ctx, sourceRepo, "not-expired", WithRef("master"), WithTTL(100*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Wait for first checkout to expire
	time.Sleep(30 * time.Millisecond)

	// Run prune with explicit PruneExpired strategy
	err = cache.Prune(PruneExpired())
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Should have removed only the expired checkout
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after prune, got %d", len(cache.index.Checkouts))
	}

	// Verify the non-expired checkout still exists
	compositeKey := makeCompositeKey(sourceRepo, "master", "not-expired")
	if cache.index.get(compositeKey) == nil {
		t.Error("expected non-expired checkout to remain")
	}
}

func TestPrune_OlderThanStrategy(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create old checkout
	_, err = cache.GetCheckout(ctx, sourceRepo, "old-key", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create old checkout: %v", err)
	}

	// Manually adjust LastAccess to make it old
	compositeKeyOld := makeCompositeKey(sourceRepo, "master", "old-key")
	oldMetadata := cache.index.get(compositeKeyOld)
	oldMetadata.LastAccess = time.Now().Add(-2 * time.Hour)

	// Wait a bit then create a recent checkout
	time.Sleep(10 * time.Millisecond)

	_, err = cache.GetCheckout(ctx, sourceRepo, "recent-key", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create recent checkout: %v", err)
	}

	// Verify we have 2 checkouts
	if len(cache.index.Checkouts) != 2 {
		t.Fatalf("expected 2 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Prune checkouts older than 1 hour
	err = cache.Prune(PruneOlderThan(1 * time.Hour))
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Should have removed old checkout, kept recent
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after prune, got %d", len(cache.index.Checkouts))
	}

	// Verify the recent checkout still exists
	compositeKeyRecent := makeCompositeKey(sourceRepo, "master", "recent-key")
	if cache.index.get(compositeKeyRecent) == nil {
		t.Error("expected recent checkout to remain")
	}
}

func TestPrune_MultipleStrategies(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create checkout that expires soon
	_, err = cache.GetCheckout(ctx, sourceRepo, "expires-soon", WithRef("master"), WithTTL(10*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	// Create checkout that's old but not expired
	_, err = cache.GetCheckout(ctx, sourceRepo, "old-but-valid", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Manually adjust LastAccess to make it old
	compositeKeyOld := makeCompositeKey(sourceRepo, "master", "old-but-valid")
	oldMetadata := cache.index.get(compositeKeyOld)
	oldMetadata.LastAccess = time.Now().Add(-2 * time.Hour)

	// Create checkout that's recent
	_, err = cache.GetCheckout(ctx, sourceRepo, "recent", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 3: %v", err)
	}

	// Verify we have 3 checkouts
	if len(cache.index.Checkouts) != 3 {
		t.Fatalf("expected 3 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Apply multiple strategies (OR logic - should remove if ANY strategy matches)
	err = cache.Prune(PruneExpired(), PruneOlderThan(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Should have removed both the expired and the old checkout
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after prune, got %d", len(cache.index.Checkouts))
	}

	// Verify the recent checkout still exists
	compositeKeyRecent := makeCompositeKey(sourceRepo, "master", "recent")
	if cache.index.get(compositeKeyRecent) == nil {
		t.Error("expected recent checkout to remain")
	}
}

func TestPrune_ToSizeStrategy(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create multiple checkouts with TTL (ephemeral)
	_, err = cache.GetCheckout(ctx, sourceRepo, "ephemeral-1", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	// Make the first one accessed longer ago
	compositeKey1 := makeCompositeKey(sourceRepo, "master", "ephemeral-1")
	metadata1 := cache.index.get(compositeKey1)
	metadata1.LastAccess = time.Now().Add(-2 * time.Hour)

	time.Sleep(10 * time.Millisecond)

	_, err = cache.GetCheckout(ctx, sourceRepo, "ephemeral-2", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Create persistent checkout (no TTL) - should never be removed by size strategy
	_, err = cache.GetCheckout(ctx, sourceRepo, "persistent", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create persistent checkout: %v", err)
	}

	// Verify we have 3 checkouts
	if len(cache.index.Checkouts) != 3 {
		t.Fatalf("expected 3 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Prune to a very small size - should remove oldest ephemeral checkout
	err = cache.Prune(PruneToSize(1)) // 1 byte - essentially remove LRU ephemeral
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Should have removed at least one ephemeral checkout
	// Persistent checkout should remain
	remaining := len(cache.index.Checkouts)
	if remaining < 1 || remaining > 2 {
		t.Errorf("expected 1-2 checkouts after prune, got %d", remaining)
	}

	// Verify persistent checkout still exists
	compositeKeyPersistent := makeCompositeKey(sourceRepo, "master", "persistent")
	if cache.index.get(compositeKeyPersistent) == nil {
		t.Error("expected persistent checkout to remain (size strategy should not remove persistent checkouts)")
	}
}

func TestPrune_EmptyCache(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Prune empty cache should not error
	err = cache.Prune()
	if err != nil {
		t.Errorf("prune on empty cache should not error: %v", err)
	}
}

func TestPrune_RemovesFromDisk(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create checkout with TTL
	checkoutPath, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"), WithTTL(10*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	// Verify checkout exists on disk
	if _, err := fs.Stat(checkoutPath); err != nil {
		t.Fatalf("checkout should exist on disk: %v", err)
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Prune
	err = cache.Prune(PruneExpired())
	if err != nil {
		t.Fatalf("failed to prune: %v", err)
	}

	// Verify checkout was removed from disk
	if _, err := fs.Stat(checkoutPath); !os.IsNotExist(err) {
		t.Error("expected checkout to be removed from disk")
	}
}
