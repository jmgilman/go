package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
)

func TestStartGC_RunsPeriodically(t *testing.T) {
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

	// Create checkout with very short TTL
	_, err = cache.GetCheckout(ctx, sourceRepo, "short-lived", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	// Verify checkout exists
	if len(cache.index.Checkouts) != 1 {
		t.Fatalf("expected 1 checkout, got %d", len(cache.index.Checkouts))
	}

	// Start GC with short interval
	stop := cache.StartGC(100*time.Millisecond, PruneExpired())
	defer stop()

	// Wait for TTL to expire and GC to run at least once
	time.Sleep(200 * time.Millisecond)

	// Checkout should have been pruned by GC
	if len(cache.index.Checkouts) != 0 {
		t.Errorf("expected 0 checkouts after GC, got %d", len(cache.index.Checkouts))
	}
}

func TestStartGC_StopFunction(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Start GC
	stop := cache.StartGC(100*time.Millisecond, PruneExpired())

	// Stop should return quickly and not block
	done := make(chan bool)
	go func() {
		stop()
		done <- true
	}()

	select {
	case <-done:
		// Success - stop returned
	case <-time.After(1 * time.Second):
		t.Fatal("stop function did not return within 1 second")
	}
}

func TestStartGC_StopCanBeCalledMultipleTimes(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Start GC
	stop := cache.StartGC(100*time.Millisecond, PruneExpired())

	// Call stop multiple times - should be safe
	stop()
	stop()
	stop()

	// Should not panic or block
}

func TestStartGC_AppliesMultipleStrategies(t *testing.T) {
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

	// Create checkout that will expire
	_, err = cache.GetCheckout(ctx, sourceRepo, "will-expire", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	// Create checkout that's old but won't expire
	_, err = cache.GetCheckout(ctx, sourceRepo, "old-checkout", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Make it old
	compositeKeyOld := makeCompositeKey(sourceRepo, "master", "old-checkout")
	oldMetadata := cache.index.get(compositeKeyOld)
	oldMetadata.LastAccess = time.Now().Add(-2 * time.Hour)

	// Create recent checkout
	_, err = cache.GetCheckout(ctx, sourceRepo, "recent", WithRef("master"), WithTTL(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to create checkout 3: %v", err)
	}

	// Verify we have 3 checkouts
	if len(cache.index.Checkouts) != 3 {
		t.Fatalf("expected 3 checkouts, got %d", len(cache.index.Checkouts))
	}

	// Start GC with multiple strategies
	stop := cache.StartGC(100*time.Millisecond, PruneExpired(), PruneOlderThan(1*time.Hour))
	defer stop()

	// Wait for TTL to expire and GC to run
	time.Sleep(200 * time.Millisecond)

	// Should have removed both expired and old checkouts, kept recent
	if len(cache.index.Checkouts) != 1 {
		t.Errorf("expected 1 checkout after GC, got %d", len(cache.index.Checkouts))
	}

	// Verify recent checkout still exists
	compositeKeyRecent := makeCompositeKey(sourceRepo, "master", "recent")
	if cache.index.get(compositeKeyRecent) == nil {
		t.Error("expected recent checkout to remain after GC")
	}
}

func TestStartGC_DoesNotBlockOnStart(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/")

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// StartGC should return immediately
	done := make(chan bool)
	go func() {
		stop := cache.StartGC(1*time.Second, PruneExpired())
		defer stop()
		done <- true
	}()

	select {
	case <-done:
		// Success - StartGC returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("StartGC did not return within 100ms")
	}
}

func TestStartGC_ContinuesRunningAfterPruneErrors(t *testing.T) {
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

	// Create first checkout
	_, err = cache.GetCheckout(ctx, sourceRepo, "checkout-1", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout 1: %v", err)
	}

	// Start GC
	stop := cache.StartGC(100*time.Millisecond, PruneExpired())
	defer stop()

	// Wait for first GC run and expiration
	time.Sleep(200 * time.Millisecond)

	// Create second checkout
	_, err = cache.GetCheckout(ctx, sourceRepo, "checkout-2", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout 2: %v", err)
	}

	// Wait for second GC run
	time.Sleep(200 * time.Millisecond)

	// Both checkouts should have been pruned (GC continued running)
	if len(cache.index.Checkouts) != 0 {
		t.Errorf("expected 0 checkouts after GC runs, got %d (GC may have stopped prematurely)", len(cache.index.Checkouts))
	}
}

func TestStartGC_WithNoStrategies(t *testing.T) {
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
	_, err = cache.GetCheckout(ctx, sourceRepo, "test-key", WithRef("master"), WithTTL(50*time.Millisecond))
	if err != nil {
		t.Fatalf("failed to create checkout: %v", err)
	}

	// Start GC with no strategies (should default to PruneExpired)
	stop := cache.StartGC(100 * time.Millisecond)
	defer stop()

	// Wait for expiration and GC run
	time.Sleep(200 * time.Millisecond)

	// Checkout should have been pruned (default strategy applied)
	if len(cache.index.Checkouts) != 0 {
		t.Errorf("expected 0 checkouts after GC with default strategy, got %d", len(cache.index.Checkouts))
	}
}
