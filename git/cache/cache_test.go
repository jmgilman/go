package cache

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
)

func TestNewRepositoryCache(t *testing.T) {
	t.Run("creates cache directory structure", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		basePath := "git-cache"

		cache, err := NewRepositoryCache(basePath, WithFilesystem(fs))
		if err != nil {
			t.Fatalf("NewRepositoryCache() error = %v", err)
		}

		// Verify directories were created
		if _, err := fs.Stat(cache.bareDir); err != nil {
			t.Errorf("bare directory was not created: %v", err)
		}

		if _, err := fs.Stat(cache.checkoutDir); err != nil {
			t.Errorf("checkouts directory was not created: %v", err)
		}

		// Verify directory paths
		expectedBareDir := filepath.Join(basePath, "bare")
		if cache.bareDir != expectedBareDir {
			t.Errorf("cache.bareDir = %v, want %v", cache.bareDir, expectedBareDir)
		}

		expectedCheckoutDir := filepath.Join(basePath, "checkouts")
		if cache.checkoutDir != expectedCheckoutDir {
			t.Errorf("cache.checkoutDir = %v, want %v", cache.checkoutDir, expectedCheckoutDir)
		}

		// Verify index was initialized
		if cache.index == nil {
			t.Error("cache.index is nil")
		}

		if cache.index.Version != indexVersion {
			t.Errorf("cache.index.Version = %v, want %v", cache.index.Version, indexVersion)
		}
	})

	t.Run("loads existing index", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		basePath := "git-cache"

		// Create cache once
		cache1, err := NewRepositoryCache(basePath, WithFilesystem(fs))
		if err != nil {
			t.Fatalf("NewRepositoryCache() error = %v", err)
		}

		// Add some metadata
		cache1.index.set("test-key", &CheckoutMetadata{
			URL:      "https://github.com/my/repo",
			Ref:      "main",
			CacheKey: "test",
		})

		// Save index
		if err := cache1.index.save(fs, cache1.indexPath); err != nil {
			t.Fatalf("save() error = %v", err)
		}

		// Create cache again (should load existing index)
		cache2, err := NewRepositoryCache(basePath, WithFilesystem(fs))
		if err != nil {
			t.Fatalf("NewRepositoryCache() error = %v", err)
		}

		// Verify metadata was loaded
		metadata := cache2.index.get("test-key")
		if metadata == nil {
			t.Fatal("metadata was not loaded from existing index")
		}

		if metadata.URL != "https://github.com/my/repo" {
			t.Errorf("metadata.URL = %v, want %v", metadata.URL, "https://github.com/my/repo")
		}
	})

	t.Run("uses memfs for testing", func(t *testing.T) {
		fs := memfs.New()
		basePath := "/cache/git"

		cache, err := NewRepositoryCache(basePath, WithFilesystem(fs))
		if err != nil {
			t.Fatalf("NewRepositoryCache() error = %v", err)
		}

		// Verify directories were created in memfs
		if _, err := fs.Stat(cache.bareDir); err != nil {
			t.Errorf("bare directory was not created in memfs: %v", err)
		}

		if _, err := fs.Stat(cache.checkoutDir); err != nil {
			t.Errorf("checkouts directory was not created in memfs: %v", err)
		}
	})

	t.Run("initializes empty maps", func(t *testing.T) {
		fs := memfs.New()
		basePath := "/cache"

		cache, err := NewRepositoryCache(basePath, WithFilesystem(fs))
		if err != nil {
			t.Fatalf("NewRepositoryCache() error = %v", err)
		}

		if cache.bare == nil {
			t.Error("cache.bare is nil")
		}

		if cache.checkouts == nil {
			t.Error("cache.checkouts is nil")
		}

		if len(cache.bare) != 0 {
			t.Errorf("len(cache.bare) = %v, want 0", len(cache.bare))
		}

		if len(cache.checkouts) != 0 {
			t.Errorf("len(cache.checkouts) = %v, want 0", len(cache.checkouts))
		}
	})
}
