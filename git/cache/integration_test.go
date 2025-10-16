package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/jmgilman/go/git"
)

func TestClear_RemovesAllDataForURL(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create two source repositories
	sourceRepo1 := createTestRepo(t, fs, filepath.Join(tempDir, "source1"))
	sourceRepo2 := createTestRepo(t, fs, filepath.Join(tempDir, "source2"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create checkouts for both repos
	path1, err := cache.GetCheckout(ctx, sourceRepo1, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout for repo1: %v", err)
	}

	_, err = cache.GetCheckout(ctx, sourceRepo1, "key2", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout for repo1 key2: %v", err)
	}

	path2, err := cache.GetCheckout(ctx, sourceRepo2, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout for repo2: %v", err)
	}

	// Verify checkouts exist
	if len(cache.index.Checkouts) != 3 {
		t.Fatalf("expected 3 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Clear repo1
	err = cache.Clear(sourceRepo1)
	if err != nil {
		t.Fatalf("failed to clear repo1: %v", err)
	}

	// Should have removed repo1 checkouts, kept repo2
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after clear, got %d", len(cache.index.Checkouts))
	}

	// Verify repo1 checkouts removed from disk
	if _, err := fs.Stat(path1); !os.IsNotExist(err) {
		t.Error("expected repo1 checkout to be removed from disk")
	}

	// Verify repo2 checkout still exists
	if _, err := fs.Stat(path2); err != nil {
		t.Errorf("expected repo2 checkout to exist: %v", err)
	}

	// Verify bare repo for repo1 is removed
	normalized1 := normalizeURL(sourceRepo1)
	barePath1 := filepath.Join(cache.bareDir, normalized1+".git")
	if _, err := fs.Stat(barePath1); !os.IsNotExist(err) {
		t.Error("expected bare repo1 to be removed from disk")
	}

	// Verify bare repo for repo2 still exists
	normalized2 := normalizeURL(sourceRepo2)
	barePath2 := filepath.Join(cache.bareDir, normalized2+".git")
	if _, err := fs.Stat(barePath2); err != nil {
		t.Errorf("expected bare repo2 to exist: %v", err)
	}
}

func TestClearAll_RemovesAllData(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create source repositories
	sourceRepo1 := createTestRepo(t, fs, filepath.Join(tempDir, "source1"))
	sourceRepo2 := createTestRepo(t, fs, filepath.Join(tempDir, "source2"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Create multiple checkouts
	path1, err := cache.GetCheckout(ctx, sourceRepo1, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	path2, err := cache.GetCheckout(ctx, sourceRepo2, "key2", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Verify data exists
	if len(cache.index.Checkouts) != 2 {
		t.Fatalf("expected 2 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Clear all
	err = cache.ClearAll()
	if err != nil {
		t.Fatalf("failed to clear all: %v", err)
	}

	// Verify everything is removed
	if len(cache.index.Checkouts) != 0 {
		t.Errorf("expected 0 checkouts after clear all, got %d", len(cache.index.Checkouts))
	}

	if len(cache.bare) != 0 {
		t.Errorf("expected 0 bare repos after clear all, got %d", len(cache.bare))
	}

	if len(cache.checkouts) != 0 {
		t.Errorf("expected 0 in-memory checkouts after clear all, got %d", len(cache.checkouts))
	}

	// Verify removed from disk
	if _, err := fs.Stat(path1); !os.IsNotExist(err) {
		t.Error("expected checkout 1 to be removed from disk")
	}

	if _, err := fs.Stat(path2); !os.IsNotExist(err) {
		t.Error("expected checkout 2 to be removed from disk")
	}

	// Verify directories still exist (should be recreated)
	if _, err := fs.Stat(cache.bareDir); err != nil {
		t.Errorf("expected bare directory to exist: %v", err)
	}

	if _, err := fs.Stat(cache.checkoutDir); err != nil {
		t.Errorf("expected checkout directory to exist: %v", err)
	}
}

func TestStats_ReturnsCorrectStatistics(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Initial stats should be zero
	stats, err := cache.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.BareRepos != 0 {
		t.Errorf("expected 0 bare repos, got %d", stats.BareRepos)
	}

	if stats.Checkouts != 0 {
		t.Errorf("expected 0 checkouts, got %d", stats.Checkouts)
	}

	// Create first checkout
	time1 := time.Now()
	_, err = cache.GetCheckout(ctx, sourceRepo, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Create second checkout
	time2 := time.Now()
	_, err = cache.GetCheckout(ctx, sourceRepo, "key2", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Get stats
	stats, err = cache.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	// Verify counts
	if stats.BareRepos != 1 {
		t.Errorf("expected 1 bare repo, got %d", stats.BareRepos)
	}

	if stats.Checkouts != 2 {
		t.Errorf("expected 2 checkouts, got %d", stats.Checkouts)
	}

	// Verify sizes are non-zero
	if stats.TotalSize == 0 {
		t.Error("expected non-zero total size")
	}

	if stats.BareSize == 0 {
		t.Error("expected non-zero bare size")
	}

	if stats.CheckoutsSize == 0 {
		t.Error("expected non-zero checkouts size")
	}

	if stats.TotalSize != stats.BareSize+stats.CheckoutsSize {
		t.Errorf("total size (%d) != bare size (%d) + checkouts size (%d)",
			stats.TotalSize, stats.BareSize, stats.CheckoutsSize)
	}

	// Verify oldest/newest checkouts
	if stats.OldestCheckout == nil {
		t.Fatal("expected oldest checkout to be set")
	}

	if stats.NewestCheckout == nil {
		t.Fatal("expected newest checkout to be set")
	}

	// Oldest should be around time1, newest around time2
	if stats.OldestCheckout.Before(time1.Add(-1*time.Second)) || stats.OldestCheckout.After(time1.Add(1*time.Second)) {
		t.Errorf("oldest checkout time unexpected: got %v, expected around %v", *stats.OldestCheckout, time1)
	}

	if stats.NewestCheckout.Before(time2.Add(-1*time.Second)) || stats.NewestCheckout.After(time2.Add(1*time.Second)) {
		t.Errorf("newest checkout time unexpected: got %v, expected around %v", *stats.NewestCheckout, time2)
	}
}

func TestIntegration_CompleteWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// 1. Create persistent checkout
	persistentPath, err := cache.GetCheckout(ctx, sourceRepo, "persistent", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create persistent checkout: %v", err)
	}

	// 2. Create ephemeral checkout with TTL
	_, err = cache.GetCheckout(ctx, sourceRepo, "ephemeral", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create ephemeral checkout: %v", err)
	}

	// 3. Verify both exist
	stats, err := cache.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.Checkouts != 2 {
		t.Errorf("expected 2 checkouts, got %d", stats.Checkouts)
	}

	// 4. Start GC
	stop := cache.StartGC(100*time.Millisecond, PruneExpired())
	defer stop()

	// 5. Wait for ephemeral to expire and be pruned
	time.Sleep(200 * time.Millisecond)

	// 6. Verify only persistent remains
	stats, err = cache.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.Checkouts != 1 {
		t.Errorf("expected 1 checkout after GC, got %d", stats.Checkouts)
	}

	// 7. Verify persistent checkout still works
	if _, err := fs.Stat(persistentPath); err != nil {
		t.Errorf("persistent checkout should still exist: %v", err)
	}

	// 8. Remove persistent checkout
	err = cache.RemoveCheckout(sourceRepo, "persistent")
	if err != nil {
		t.Fatalf("failed to remove checkout: %v", err)
	}

	// 9. Verify everything is cleaned up
	stats, err = cache.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.Checkouts != 0 {
		t.Errorf("expected 0 checkouts, got %d", stats.Checkouts)
	}

	// Stop GC
	stop()
}

func TestIntegration_CacheReuse(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create first cache instance
	cache1, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache1: %v", err)
	}

	ctx := context.Background()

	// Create checkout with first cache
	path1, err := cache1.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout with cache1: %v", err)
	}

	// Verify it exists
	if _, err := fs.Stat(path1); err != nil {
		t.Fatalf("checkout should exist: %v", err)
	}

	// Create second cache instance (simulating restart)
	cache2, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache2: %v", err)
	}

	// Second cache should load existing index
	if len(cache2.index.Checkouts) != 1 {
		t.Errorf("expected cache2 to load 1 checkout from index, got %d", len(cache2.index.Checkouts))
	}

	// Get the same checkout with second cache - should reuse
	path2, err := cache2.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout with cache2: %v", err)
	}

	// Paths should be the same
	if path1 != path2 {
		t.Errorf("expected same path, got %s and %s", path1, path2)
	}

	// Should still only have 1 checkout
	if len(cache2.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout, got %d", len(cache2.index.Checkouts))
	}
}

func TestIntegration_MultipleRefsAndKeys(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()

	// Get commit hash for testing different refs
	repo, err := git.Open(sourceRepo, git.WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to open source repo: %v", err)
	}

	head, err := repo.Underlying().Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	commitHash := head.Hash().String()

	// Create checkouts with different combinations
	paths := make(map[string]string)

	// Same URL, different refs, same key
	p1, err := cache.GetCheckout(ctx, sourceRepo, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}
	paths["master-key1"] = p1

	p2, err := cache.GetCheckout(ctx, sourceRepo, "key1", WithRef(commitHash))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}
	paths["hash-key1"] = p2

	// Same URL, same ref, different keys
	p3, err := cache.GetCheckout(ctx, sourceRepo, "key2", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to create checkout 3: %v", err)
	}
	paths["master-key2"] = p3

	// Should have 3 different checkouts
	if len(cache.index.Checkouts) != 3 {
		t.Errorf("expected 3 checkouts, got %d", len(cache.index.Checkouts))
	}

	// All paths should be different
	seen := make(map[string]bool)
	for name, path := range paths {
		if seen[path] {
			t.Errorf("duplicate path for %s: %s", name, path)
		}
		seen[path] = true
	}

	// RemoveCheckout with key1 should remove 2 checkouts (master and hash)
	err = cache.RemoveCheckout(sourceRepo, "key1")
	if err != nil {
		t.Fatalf("failed to remove key1 checkouts: %v", err)
	}

	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after removing key1, got %d", len(cache.index.Checkouts))
	}

	// Only master-key2 should remain
	compositeKey := makeCompositeKey(sourceRepo, "master", "key2")
	if cache.index.get(compositeKey) == nil {
		t.Error("expected master-key2 to remain")
	}
}
