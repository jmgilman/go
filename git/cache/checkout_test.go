package cache

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jmgilman/go/git"
)

func TestGetCheckout_CreatesNewCheckout(t *testing.T) {
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

	// Get checkout with a specific cache key (explicitly specify master branch)
	checkoutPath, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout: %v", err)
	}

	// Verify checkout path was returned
	if checkoutPath == "" {
		t.Fatal("expected non-empty checkout path")
	}

	// Verify checkout exists on disk
	if _, err := fs.Stat(checkoutPath); err != nil {
		t.Errorf("expected checkout to exist at %s: %v", checkoutPath, err)
	}

	// Verify test.txt exists in the checkout
	testFile := filepath.Join(checkoutPath, "test.txt")
	if _, err := fs.Stat(testFile); err != nil {
		t.Errorf("expected test.txt in checkout: %v", err)
	}

	// Verify metadata was created
	compositeKey := makeCompositeKey(sourceRepo, "master", "test-key")
	metadata := cache.index.get(compositeKey)
	if metadata == nil {
		t.Fatal("expected metadata to be created")
	}
	if metadata.URL != sourceRepo {
		t.Errorf("expected URL %s, got %s", sourceRepo, metadata.URL)
	}
	if metadata.Ref != "master" {
		t.Errorf("expected ref master, got %s", metadata.Ref)
	}
	if metadata.CacheKey != "test-key" {
		t.Errorf("expected cache key test-key, got %s", metadata.CacheKey)
	}
}

func TestGetCheckout_ReusesExistingCheckout(t *testing.T) {
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

	// First call creates the checkout
	path1, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout (first call): %v", err)
	}

	// Get initial metadata and save timestamp values
	compositeKey := makeCompositeKey(sourceRepo, "master", "test-key")
	metadata1 := cache.index.get(compositeKey)
	if metadata1 == nil {
		t.Fatal("expected metadata after first checkout")
	}
	createdAt := metadata1.CreatedAt
	lastAccess1 := metadata1.LastAccess

	// Wait a bit to ensure timestamp would change if recreated
	time.Sleep(10 * time.Millisecond)

	// Second call should reuse the same checkout
	path2, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout (second call): %v", err)
	}

	// Paths should be the same
	if path1 != path2 {
		t.Errorf("expected same checkout path, got %s and %s", path1, path2)
	}

	// Metadata should be updated (LastAccess changed)
	metadata2 := cache.index.get(compositeKey)
	if metadata2 == nil {
		t.Fatal("expected metadata after second checkout")
	}

	// CreatedAt should remain the same (not recreated)
	if !metadata2.CreatedAt.Equal(createdAt) {
		t.Error("expected CreatedAt to remain the same when reusing checkout")
	}

	// LastAccess should be updated (compare to saved timestamp, not pointer)
	if !metadata2.LastAccess.After(lastAccess1) {
		t.Errorf("expected LastAccess to be updated: was %v, now %v", lastAccess1, metadata2.LastAccess)
	}
}

func TestGetCheckout_DifferentRefsCreateSeparateCheckouts(t *testing.T) {
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

	// Get checkout for master
	path1, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout for master: %v", err)
	}

	// Get checkout for a commit hash (we'll use HEAD which resolves to the same commit)
	// First, get the commit hash from the source repo
	srcRepo, err := git.Open(sourceRepo, git.WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to open source repo: %v", err)
	}
	head, err := srcRepo.Underlying().Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	commitHash := head.Hash().String()

	// Get checkout for the commit hash (different ref, same content)
	path2, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef(commitHash))
	if err != nil {
		t.Fatalf("failed to get checkout for commit hash: %v", err)
	}

	// Paths should be different
	if path1 == path2 {
		t.Error("expected different checkout paths for different refs")
	}

	// Both checkouts should exist
	if _, err := fs.Stat(path1); err != nil {
		t.Errorf("expected master checkout to exist: %v", err)
	}
	if _, err := fs.Stat(path2); err != nil {
		t.Errorf("expected HEAD checkout to exist: %v", err)
	}

	// Should have two entries in the index
	if len(cache.index.Checkouts) != 2 {
		t.Errorf("expected 2 index entries, got %d", len(cache.index.Checkouts))
	}
}

func TestGetCheckout_DifferentCacheKeysCreateSeparateCheckouts(t *testing.T) {
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

	// Get checkout with first cache key
	path1, err := cache.GetCheckout(ctx, sourceRepo, "key1", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout with key1: %v", err)
	}

	// Get checkout with second cache key
	path2, err := cache.GetCheckout(ctx, sourceRepo, "key2", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout with key2: %v", err)
	}

	// Paths should be different
	if path1 == path2 {
		t.Error("expected different checkout paths for different cache keys")
	}

	// Both checkouts should exist
	if _, err := fs.Stat(path1); err != nil {
		t.Errorf("expected key1 checkout to exist: %v", err)
	}
	if _, err := fs.Stat(path2); err != nil {
		t.Errorf("expected key2 checkout to exist: %v", err)
	}

	// Should have two entries in the index
	if len(cache.index.Checkouts) != 2 {
		t.Errorf("expected 2 index entries, got %d", len(cache.index.Checkouts))
	}
}

func TestGetCheckout_WithTTL(t *testing.T) {
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
	ttl := 1 * time.Hour

	// Get checkout with TTL
	_, err = cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"), WithTTL(ttl))
	if err != nil {
		t.Fatalf("failed to get checkout: %v", err)
	}

	// Verify metadata has TTL set
	compositeKey := makeCompositeKey(sourceRepo, "master", "test-key")
	metadata := cache.index.get(compositeKey)
	if metadata == nil {
		t.Fatal("expected metadata to exist")
	}

	if metadata.TTL == nil {
		t.Fatal("expected TTL to be set")
	}
	if *metadata.TTL != ttl {
		t.Errorf("expected TTL %v, got %v", ttl, *metadata.TTL)
	}

	if metadata.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}

	// ExpiresAt should be approximately CreatedAt + TTL
	expectedExpiration := metadata.CreatedAt.Add(ttl)
	if metadata.ExpiresAt.Sub(expectedExpiration).Abs() > time.Second {
		t.Errorf("expected ExpiresAt to be around %v, got %v", expectedExpiration, *metadata.ExpiresAt)
	}
}

func TestGetCheckout_WithUpdate(t *testing.T) {
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

	// First call creates the checkout
	path1, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout (first call): %v", err)
	}

	// Add a new file to source repo
	newFile := filepath.Join(sourceRepo, "new.txt")
	f, err := fs.Create(newFile)
	if err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}
	if _, err := f.Write([]byte("new content\n")); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close new file: %v", err)
	}

	// Open source repo and commit the new file
	repo, err := git.Open(sourceRepo, git.WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to open source repo: %v", err)
	}
	wt, err := repo.Underlying().Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatalf("failed to add new file: %v", err)
	}
	_, err = wt.Commit("add new file", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Second call with WithUpdate should refresh the checkout
	path2, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"), WithUpdate())
	if err != nil {
		t.Fatalf("failed to get checkout with update: %v", err)
	}

	// Paths should be the same (reused checkout)
	if path1 != path2 {
		t.Errorf("expected same checkout path, got %s and %s", path1, path2)
	}

	// New file should now exist in the checkout
	newFileInCheckout := filepath.Join(path2, "new.txt")
	if _, err := fs.Stat(newFileInCheckout); err != nil {
		t.Errorf("expected new.txt in updated checkout: %v", err)
	}
}

func TestGetCheckout_ConcurrentAccess(t *testing.T) {
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

	// Launch multiple goroutines that try to get the same checkout
	var wg sync.WaitGroup
	numGoroutines := 10
	paths := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
			paths[idx] = path
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All calls should succeed
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}

	// All should return the same path
	firstPath := paths[0]
	for i, path := range paths[1:] {
		if path != firstPath {
			t.Errorf("goroutine %d got different path: %s vs %s", i+1, path, firstPath)
		}
	}

	// Should only have one entry in the index
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 entry in index, got %d", len(cache.index.Checkouts))
	}

	// Checkout should exist
	if _, err := fs.Stat(firstPath); err != nil {
		t.Errorf("expected checkout to exist at %s: %v", firstPath, err)
	}
}

func TestRemoveCheckout(t *testing.T) {
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

	// Create a checkout
	checkoutPath, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout: %v", err)
	}

	// Verify it exists
	if _, err := fs.Stat(checkoutPath); err != nil {
		t.Fatalf("checkout should exist before removal: %v", err)
	}

	// Remove the checkout
	err = cache.RemoveCheckout(sourceRepo, "test-key")
	if err != nil {
		t.Fatalf("failed to remove checkout: %v", err)
	}

	// Verify it no longer exists in the index
	compositeKey := makeCompositeKey(sourceRepo, "master", "test-key")
	metadata := cache.index.get(compositeKey)
	if metadata != nil {
		t.Error("expected metadata to be removed from index")
	}

	// Verify it's removed from in-memory cache
	if _, exists := cache.checkouts[compositeKey]; exists {
		t.Error("expected checkout to be removed from in-memory cache")
	}

	// Verify directory was removed (note: may fail if parent dirs exist)
	if _, err := fs.Stat(checkoutPath); err == nil {
		t.Error("expected checkout directory to be removed")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error checking removed checkout: %v", err)
	}
}

func TestRemoveCheckout_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Try to remove a checkout that doesn't exist
	err = cache.RemoveCheckout("https://example.com/repo", "nonexistent-key")
	if err == nil {
		t.Error("expected error when removing nonexistent checkout")
	}
}

func TestRemoveCheckout_MultipleRefsWithSameKey(t *testing.T) {
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

	// Create checkouts for the same cache key but different refs
	path1, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout for master: %v", err)
	}

	// Get the commit hash for a different ref pointing to same commit
	srcRepo, err := git.Open(sourceRepo, git.WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to open source repo: %v", err)
	}
	head, err := srcRepo.Underlying().Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	commitHash := head.Hash().String()

	path2, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef(commitHash))
	if err != nil {
		t.Fatalf("failed to get checkout for commit hash: %v", err)
	}

	// Both should exist
	if _, err := fs.Stat(path1); err != nil {
		t.Fatalf("master checkout should exist: %v", err)
	}
	if _, err := fs.Stat(path2); err != nil {
		t.Fatalf("HEAD checkout should exist: %v", err)
	}

	// Should have two entries in index
	if len(cache.index.Checkouts) != 2 {
		t.Fatalf("expected 2 index entries, got %d", len(cache.index.Checkouts))
	}

	// Remove all checkouts with this cache key
	err = cache.RemoveCheckout(sourceRepo, "test-key")
	if err != nil {
		t.Fatalf("failed to remove checkouts: %v", err)
	}

	// Both should be removed from index
	if len(cache.index.Checkouts) != 0 {
		t.Errorf("expected 0 index entries after removal, got %d", len(cache.index.Checkouts))
	}
}

func TestGetCheckout_WithExplicitRef(t *testing.T) {
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

	// Get checkout with explicit ref
	checkoutPath, err := cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"))
	if err != nil {
		t.Fatalf("failed to get checkout: %v", err)
	}

	// Verify checkout exists
	if _, err := fs.Stat(checkoutPath); err != nil {
		t.Errorf("expected checkout to exist: %v", err)
	}

	// Verify metadata uses the specified ref
	compositeKey := makeCompositeKey(sourceRepo, "master", "test-key")
	metadata := cache.index.get(compositeKey)
	if metadata == nil {
		t.Fatal("expected metadata to exist")
	}
	if metadata.Ref != "master" {
		t.Errorf("expected ref to be master, got %s", metadata.Ref)
	}
}
