package cache

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jmgilman/go/git"
)

// createTestRepo creates a test repository with a single commit for testing purposes.
// Returns the path to the repository.
func createTestRepo(t *testing.T, fs billy.Filesystem, repoPath string) string {
	t.Helper()

	// Initialize repository
	repo, err := git.Init(repoPath, git.WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to init test repo: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(repoPath, "test.txt")
	f, err := fs.Create(testFile)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if _, err := f.Write([]byte("test content\n")); err != nil {
		f.Close()
		t.Fatalf("failed to write test file: %v", err)
	}
	f.Close()

	// Add and commit the file
	wt, err := repo.Underlying().Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	if _, err := wt.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	commit, err := wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Ensure the master branch reference exists and HEAD points to it
	underlying := repo.Underlying()
	masterRef := plumbing.NewHashReference("refs/heads/master", commit)
	if err := underlying.Storer.SetReference(masterRef); err != nil {
		t.Fatalf("failed to set master ref: %v", err)
	}

	headRef := plumbing.NewSymbolicReference("HEAD", "refs/heads/master")
	if err := underlying.Storer.SetReference(headRef); err != nil {
		t.Fatalf("failed to set HEAD: %v", err)
	}

	return repoPath
}

func TestGetOrCreateBareRepo_CreatesNewRepo(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository to clone from
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	// First call should create the bare repo
	repo, err := cache.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to get or create bare repo: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	// Verify it's in the bare cache
	normalized := normalizeURL(sourceRepo)
	if _, exists := cache.bare[normalized]; !exists {
		t.Error("expected repo to be in bare cache")
	}

	// Verify the bare repo exists on disk
	barePath := filepath.Join(cache.bareDir, normalized+".git")
	if _, err := fs.Stat(barePath); err != nil {
		t.Errorf("expected bare repo to exist at %s: %v", barePath, err)
	}
}

func TestGetOrCreateBareRepo_UsesCachedRepo(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	// First call creates the repo
	repo1, err := cache.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to get or create bare repo (first call): %v", err)
	}

	// Second call should return the same cached repo
	repo2, err := cache.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to get or create bare repo (second call): %v", err)
	}

	// They should be the exact same instance (pointer equality)
	if repo1 != repo2 {
		t.Error("expected same repository instance from cache")
	}
}

func TestGetOrCreateBareRepo_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	// Launch multiple goroutines that try to get the same repo
	var wg sync.WaitGroup
	numGoroutines := 10
	repos := make([]*git.Repository, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			repo, err := cache.getOrCreateBareRepo(ctx, sourceRepo, opts)
			repos[idx] = repo
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

	// All should return the same repository instance
	firstRepo := repos[0]
	for i, repo := range repos[1:] {
		if repo != firstRepo {
			t.Errorf("goroutine %d got different repo instance", i+1)
		}
	}

	// Should only have one entry in the cache
	if len(cache.bare) != 1 {
		t.Errorf("expected 1 entry in bare cache, got %d", len(cache.bare))
	}
}

func TestGetOrCreateBareRepo_OpensExistingDiskRepo(t *testing.T) {
	// Use osfs for this test since we need persistence across cache instances
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create first cache instance and create a bare repo
	cache1, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create first cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	_, err = cache1.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to create bare repo with first cache: %v", err)
	}

	// Create second cache instance (simulating restart)
	cache2, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create second cache: %v", err)
	}

	// Second cache should open the existing bare repo from disk
	repo, err := cache2.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to open bare repo with second cache: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	// Verify it's in the cache
	normalized := normalizeURL(sourceRepo)
	if _, exists := cache2.bare[normalized]; !exists {
		t.Error("expected repo to be in bare cache")
	}
}

func TestCloneBareRepo_CreatesRepository(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}
	barePath := filepath.Join(tempDir, "cache", "bare", "test.git")

	// Clone the bare repo
	repo, err := cache.cloneBareRepo(ctx, sourceRepo, barePath, opts)
	if err != nil {
		t.Fatalf("failed to clone bare repo: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	// Verify the repository was created at the path
	if _, err := fs.Stat(barePath); err != nil {
		t.Errorf("expected bare repo at %s: %v", barePath, err)
	}

	// Verify it has a remote named "origin"
	remotes, err := repo.ListRemotes()
	if err != nil {
		t.Fatalf("failed to list remotes: %v", err)
	}
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Errorf("expected remote named 'origin', got '%s'", remotes[0].Name)
	}
	if len(remotes[0].URLs) == 0 || remotes[0].URLs[0] != sourceRepo {
		t.Errorf("expected remote URL '%s', got '%v'", sourceRepo, remotes[0].URLs)
	}

	// Verify it fetched refs (should have at least HEAD and refs/heads/master or main)
	refs, err := repo.Underlying().References()
	if err != nil {
		t.Fatalf("failed to get references: %v", err)
	}

	refCount := 0
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		refCount++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate references: %v", err)
	}

	if refCount == 0 {
		t.Error("expected bare repo to have references after clone")
	}
}

func TestUpdateBareRepo_FetchesChanges(t *testing.T) {
	// This test is more complex as it requires creating changes in the source repo
	// and verifying they're fetched. For now, we'll test that updateBareRepo
	// doesn't fail on an existing repo.

	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create a source repository
	sourceRepo := createTestRepo(t, fs, filepath.Join(tempDir, "source"))

	// Create cache and bare repo
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	repo, err := cache.getOrCreateBareRepo(ctx, sourceRepo, opts)
	if err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	// Update should succeed (even though there are no new changes)
	err = cache.updateBareRepo(ctx, repo, opts)
	if err != nil {
		t.Errorf("updateBareRepo failed: %v", err)
	}
}

func TestGetOrCreateBareRepo_DifferentURLs(t *testing.T) {
	tempDir := t.TempDir()
	fs := osfs.New("/") // Root at system root so absolute paths work

	// Create two source repositories
	sourceRepo1 := createTestRepo(t, fs, filepath.Join(tempDir, "source1"))
	sourceRepo2 := createTestRepo(t, fs, filepath.Join(tempDir, "source2"))

	// Create cache
	cache, err := NewRepositoryCache(filepath.Join(tempDir, "cache"), WithFilesystem(fs))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctx := context.Background()
	opts := &cacheOptions{}

	// Get repos for both URLs
	repo1, err := cache.getOrCreateBareRepo(ctx, sourceRepo1, opts)
	if err != nil {
		t.Fatalf("failed to get repo1: %v", err)
	}

	repo2, err := cache.getOrCreateBareRepo(ctx, sourceRepo2, opts)
	if err != nil {
		t.Fatalf("failed to get repo2: %v", err)
	}

	// They should be different instances
	if repo1 == repo2 {
		t.Error("expected different repository instances for different URLs")
	}

	// Should have two entries in the cache
	if len(cache.bare) != 2 {
		t.Errorf("expected 2 entries in bare cache, got %d", len(cache.bare))
	}
}
