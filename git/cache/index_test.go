package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
)

func TestLoadOrCreateIndex(t *testing.T) {
	t.Run("creates new index if file doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		indexPath := "index.json"

		index, err := loadOrCreateIndex(fs, indexPath)
		if err != nil {
			t.Fatalf("loadOrCreateIndex() error = %v", err)
		}

		if index.Version != indexVersion {
			t.Errorf("index.Version = %v, want %v", index.Version, indexVersion)
		}

		if index.Checkouts == nil {
			t.Error("index.Checkouts is nil, want empty map")
		}

		if len(index.Checkouts) != 0 {
			t.Errorf("len(index.Checkouts) = %v, want 0", len(index.Checkouts))
		}
	})

	t.Run("loads existing index from disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		indexPath := "index.json"

		// Create an index and save it
		original := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"github.com/my/repo/main/test": {
					URL:        "https://github.com/my/repo",
					Ref:        "main",
					CacheKey:   "test",
					CreatedAt:  time.Now(),
					LastAccess: time.Now(),
				},
			},
		}

		if err := original.save(fs, indexPath); err != nil {
			t.Fatalf("save() error = %v", err)
		}

		// Load it back
		loaded, err := loadOrCreateIndex(fs, indexPath)
		if err != nil {
			t.Fatalf("loadOrCreateIndex() error = %v", err)
		}

		if loaded.Version != indexVersion {
			t.Errorf("loaded.Version = %v, want %v", loaded.Version, indexVersion)
		}

		if len(loaded.Checkouts) != 1 {
			t.Errorf("len(loaded.Checkouts) = %v, want 1", len(loaded.Checkouts))
		}

		metadata := loaded.Checkouts["github.com/my/repo/main/test"]
		if metadata == nil {
			t.Fatal("metadata is nil")
		}

		if metadata.URL != "https://github.com/my/repo" {
			t.Errorf("metadata.URL = %v, want %v", metadata.URL, "https://github.com/my/repo")
		}
	})

	t.Run("returns error for corrupted index", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		indexPath := "index.json"

		// Write corrupted JSON
		if err := os.WriteFile(filepath.Join(tmpDir, indexPath), []byte("not valid json"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		_, err := loadOrCreateIndex(fs, indexPath)
		if err == nil {
			t.Error("loadOrCreateIndex() expected error for corrupted JSON, got nil")
		}
	})
}

func TestIndexOperations(t *testing.T) {
	t.Run("get returns nil for non-existent key", func(t *testing.T) {
		index := &cacheIndex{
			Version:   indexVersion,
			Checkouts: make(map[string]*CheckoutMetadata),
		}

		result := index.get("non-existent")
		if result != nil {
			t.Errorf("get() = %v, want nil", result)
		}
	})

	t.Run("set and get work correctly", func(t *testing.T) {
		index := &cacheIndex{
			Version:   indexVersion,
			Checkouts: make(map[string]*CheckoutMetadata),
		}

		metadata := &CheckoutMetadata{
			URL:        "https://github.com/my/repo",
			Ref:        "main",
			CacheKey:   "test",
			CreatedAt:  time.Now(),
			LastAccess: time.Now(),
		}

		key := "github.com/my/repo/main/test"
		index.set(key, metadata)

		result := index.get(key)
		if result == nil {
			t.Fatal("get() returned nil")
		}

		if result.URL != metadata.URL {
			t.Errorf("result.URL = %v, want %v", result.URL, metadata.URL)
		}
	})

	t.Run("delete removes metadata", func(t *testing.T) {
		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {URL: "https://github.com/my/repo"},
			},
		}

		index.delete("key1")

		if index.get("key1") != nil {
			t.Error("get() after delete returned non-nil")
		}
	})

	t.Run("updateLastAccess updates timestamp", func(t *testing.T) {
		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {
					URL:        "https://github.com/my/repo",
					Ref:        "main",
					CacheKey:   "test",
					CreatedAt:  time.Now().Add(-1 * time.Hour),
					LastAccess: time.Now().Add(-1 * time.Hour),
				},
			},
		}

		oldAccess := index.get("key1").LastAccess
		time.Sleep(10 * time.Millisecond)

		index.updateLastAccess("key1")

		newAccess := index.get("key1").LastAccess
		if !newAccess.After(oldAccess) {
			t.Error("LastAccess was not updated")
		}
	})

	t.Run("updateLastAccess with TTL updates expiration", func(t *testing.T) {
		ttl := 1 * time.Hour
		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {
					URL:        "https://github.com/my/repo",
					Ref:        "main",
					CacheKey:   "test",
					CreatedAt:  time.Now(),
					LastAccess: time.Now(),
					TTL:        &ttl,
				},
			},
		}

		index.updateLastAccess("key1")

		metadata := index.get("key1")
		if metadata.ExpiresAt == nil {
			t.Fatal("ExpiresAt is nil after updateLastAccess with TTL")
		}

		expectedExpiration := metadata.LastAccess.Add(ttl)
		if metadata.ExpiresAt.Sub(expectedExpiration).Abs() > time.Second {
			t.Errorf("ExpiresAt = %v, want approximately %v", metadata.ExpiresAt, expectedExpiration)
		}
	})

	t.Run("list returns all metadata", func(t *testing.T) {
		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {URL: "https://github.com/repo1"},
				"key2": {URL: "https://github.com/repo2"},
			},
		}

		result := index.list()
		if len(result) != 2 {
			t.Errorf("len(list()) = %v, want 2", len(result))
		}
	})

	t.Run("filterByURL returns only matching URLs", func(t *testing.T) {
		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {URL: "https://github.com/my/repo"},
				"key2": {URL: "https://github.com/my/repo"},
				"key3": {URL: "https://github.com/other/repo"},
			},
		}

		result := index.filterByURL("https://github.com/my/repo.git")
		if len(result) != 2 {
			t.Errorf("len(filterByURL()) = %v, want 2", len(result))
		}
	})
}

func TestIndexSave(t *testing.T) {
	t.Run("saves index atomically", func(t *testing.T) {
		tmpDir := t.TempDir()
		fs := osfs.New(tmpDir)
		indexPath := "index.json"

		index := &cacheIndex{
			Version: indexVersion,
			Checkouts: map[string]*CheckoutMetadata{
				"key1": {
					URL:        "https://github.com/my/repo",
					Ref:        "main",
					CacheKey:   "test",
					CreatedAt:  time.Now(),
					LastAccess: time.Now(),
				},
			},
		}

		if err := index.save(fs, indexPath); err != nil {
			t.Fatalf("save() error = %v", err)
		}

		// Verify file exists
		if _, err := fs.Stat(indexPath); os.IsNotExist(err) {
			t.Error("index file was not created")
		}

		// Verify temp file was cleaned up
		tmpPath := indexPath + ".tmp"
		if _, err := fs.Stat(tmpPath); !os.IsNotExist(err) {
			t.Error("temporary file was not cleaned up")
		}

		// Verify content can be loaded
		loaded, err := loadOrCreateIndex(fs, indexPath)
		if err != nil {
			t.Fatalf("loadOrCreateIndex() error = %v", err)
		}

		if len(loaded.Checkouts) != 1 {
			t.Errorf("len(loaded.Checkouts) = %v, want 1", len(loaded.Checkouts))
		}
	})
}
