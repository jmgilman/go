package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndex_Get(t *testing.T) {
	index := NewIndex("", 100)

	// Test getting non-existent entry
	entry, exists := index.Get("nonexistent")
	assert.Nil(t, entry)
	assert.False(t, exists)

	// Add an entry
	testEntry := &IndexEntry{
		Key:         "test",
		Size:        100,
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		TTL:         time.Hour,
		AccessCount: 1,
	}
	err := index.Put("test", testEntry)
	require.NoError(t, err)

	// Get the entry
	entry, exists = index.Get("test")
	assert.True(t, exists)
	assert.Equal(t, testEntry.Key, entry.Key)
	assert.Equal(t, testEntry.Size, entry.Size)
	assert.Equal(t, int64(2), entry.AccessCount) // Incremented on Get
}

func TestIndex_Put(t *testing.T) {
	index := NewIndex("", 100)

	entry := &IndexEntry{
		Key:         "test",
		Size:        50,
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		TTL:         time.Hour,
		AccessCount: 1,
	}

	// Put entry
	err := index.Put("test", entry)
	require.NoError(t, err)

	// Verify entry exists
	retrieved, exists := index.Get("test")
	assert.True(t, exists)
	assert.Equal(t, entry.Key, retrieved.Key)

	// Update entry
	updatedEntry := &IndexEntry{
		Key:         "test",
		Size:        100,
		CreatedAt:   entry.CreatedAt,
		AccessedAt:  time.Now(),
		TTL:         2 * time.Hour,
		AccessCount: 2,
	}
	err = index.Put("test", updatedEntry)
	require.NoError(t, err)

	// Verify update
	retrieved, exists = index.Get("test")
	assert.True(t, exists)
	assert.Equal(t, int64(100), retrieved.Size)
	assert.Equal(t, 2*time.Hour, retrieved.TTL)
}

func TestIndex_Delete(t *testing.T) {
	index := NewIndex("", 100)

	// Add entry
	entry := &IndexEntry{Key: "test", Size: 50}
	err := index.Put("test", entry)
	require.NoError(t, err)

	// Verify exists
	_, exists := index.Get("test")
	assert.True(t, exists)

	// Delete entry
	err = index.Delete("test")
	require.NoError(t, err)

	// Verify deleted
	_, exists = index.Get("test")
	assert.False(t, exists)
}

func TestIndex_Size(t *testing.T) {
	index := NewIndex("", 100)

	// Empty index
	assert.Equal(t, 0, index.Size())

	// Add entries
	for i := 0; i < 5; i++ {
		entry := &IndexEntry{Key: fmt.Sprintf("key%d", i), Size: 10}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, index.Size())

	// Delete one entry
	err := index.Delete("key2")
	require.NoError(t, err)

	assert.Equal(t, 4, index.Size())
}

func TestIndex_Keys(t *testing.T) {
	index := NewIndex("", 100)

	// Add entries
	entries := map[string]*IndexEntry{
		"key1": {Key: "key1", Size: 10, TTL: time.Hour},
		"key2": {Key: "key2", Size: 20, TTL: 0}, // No TTL
		"key3": {Key: "key3", Size: 30, TTL: time.Hour},
	}

	for key, entry := range entries {
		err := index.Put(key, entry)
		require.NoError(t, err)
	}

	// Get all keys
	allKeys := index.Keys(nil)
	assert.Len(t, allKeys, 3)
	assert.Contains(t, allKeys, "key1")
	assert.Contains(t, allKeys, "key2")
	assert.Contains(t, allKeys, "key3")

	// Get keys with filter
	noTTLKeys := index.Keys(func(entry *IndexEntry) bool {
		return entry.TTL == 0
	})
	assert.Len(t, noTTLKeys, 1)
	assert.Equal(t, "key2", noTTLKeys[0])
}

func TestIndex_ExpiredKeys(t *testing.T) {
	index := NewIndex("", 100)

	now := time.Now()

	// Add entries with different expiration times
	expiredEntry := &IndexEntry{
		Key:        "expired",
		Size:       10,
		CreatedAt:  now.Add(-2 * time.Hour),
		AccessedAt: now.Add(-2 * time.Hour),
		TTL:        time.Hour, // Expired
	}

	freshEntry := &IndexEntry{
		Key:        "fresh",
		Size:       20,
		CreatedAt:  now,
		AccessedAt: now,
		TTL:        time.Hour,
	}

	noTTL := &IndexEntry{
		Key:        "no_ttl",
		Size:       30,
		CreatedAt:  now.Add(-time.Hour),
		AccessedAt: now.Add(-time.Hour),
		TTL:        0, // No expiration
	}

	err := index.Put("expired", expiredEntry)
	require.NoError(t, err)
	err = index.Put("fresh", freshEntry)
	require.NoError(t, err)
	err = index.Put("no_ttl", noTTL)
	require.NoError(t, err)

	// Get expired keys
	expiredKeys := index.ExpiredKeys()
	assert.Len(t, expiredKeys, 1)
	assert.Equal(t, "expired", expiredKeys[0])
}

func TestIndex_Load(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "index_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "test_index.db")

	// Create and populate index
	originalIndex := NewIndex(indexPath, 100)
	entry := &IndexEntry{
		Key:         "test",
		Size:        100,
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		TTL:         time.Hour,
		AccessCount: 1,
	}
	err = originalIndex.Put("test", entry)
	require.NoError(t, err)

	// Persist index
	err = originalIndex.Persist()
	require.NoError(t, err)

	// Create new index and load
	newIndex := NewIndex(indexPath, 100)
	err = newIndex.Load(context.Background())
	require.NoError(t, err)

	// Verify loaded entry
	loadedEntry, exists := newIndex.Get("test")
	assert.True(t, exists)
	assert.Equal(t, entry.Key, loadedEntry.Key)
	assert.Equal(t, entry.Size, loadedEntry.Size)
}

func TestIndex_LoadCorrupted(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "index_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "corrupted_index.db")

	// Write corrupted data to index file
	err = os.WriteFile(indexPath, []byte("invalid json\n"), 0o644)
	require.NoError(t, err)

	// Try to load corrupted index
	index := NewIndex(indexPath, 100)
	err = index.Load(context.Background())
	// Should not fail, but should handle corruption gracefully
	require.NoError(t, err)

	// Index should be empty or have only valid entries
	assert.Equal(t, 0, index.Size())
}

func TestIndex_Persist(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "index_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "persist_test.db")
	index := NewIndex(indexPath, 100)

	// Add entries
	for i := 0; i < 10; i++ {
		entry := &IndexEntry{
			Key:         fmt.Sprintf("key%d", i),
			Size:        int64(10 * i),
			CreatedAt:   time.Now(),
			AccessedAt:  time.Now(),
			TTL:         time.Hour,
			AccessCount: 1,
		}
		err = index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	// Persist
	err = index.Persist()
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(indexPath)
	assert.NoError(t, err)
}

func TestIndex_Compact(t *testing.T) {
	index := NewIndex("", 100)

	// Add entries with different expiration times
	now := time.Now()
	for i := 0; i < 10; i++ {
		entry := &IndexEntry{
			Key:        fmt.Sprintf("key%d", i),
			Size:       10,
			CreatedAt:  now.Add(time.Duration(-i) * time.Hour),
			AccessedAt: now.Add(time.Duration(-i) * time.Hour),
			TTL:        time.Hour,
		}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	originalSize := index.Size()
	assert.Equal(t, 10, originalSize)

	// Compact (should remove expired entries)
	err := index.Compact()
	require.NoError(t, err)

	// Should have fewer entries (expired ones removed)
	compactedSize := index.Size()
	assert.True(t, compactedSize < originalSize)
}

func TestIndex_Cleanup(t *testing.T) {
	index := NewIndex("", 100)

	// Add mix of expired and fresh entries
	now := time.Now()
	for i := 0; i < 10; i++ {
		ttl := time.Hour
		if i < 5 {
			ttl = time.Minute // Will be expired
		}

		entry := &IndexEntry{
			Key:        fmt.Sprintf("key%d", i),
			Size:       10,
			CreatedAt:  now.Add(-30 * time.Minute), // 30 minutes ago
			AccessedAt: now.Add(-30 * time.Minute),
			TTL:        ttl,
		}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	originalSize := index.Size()
	assert.Equal(t, 10, originalSize)

	// Cleanup
	err := index.Cleanup(context.Background(), false)
	require.NoError(t, err)

	// Should have fewer entries
	cleanedSize := index.Size()
	assert.True(t, cleanedSize < originalSize)
	assert.Equal(t, 5, cleanedSize) // 5 fresh entries should remain
}

func TestIndex_Stats(t *testing.T) {
	index := NewIndex("", 100)

	now := time.Now()

	// Add entries
	for i := 0; i < 5; i++ {
		entry := &IndexEntry{
			Key:         fmt.Sprintf("key%d", i),
			Size:        100,
			CreatedAt:   now,
			AccessedAt:  now,
			TTL:         time.Hour,
			AccessCount: 1,
		}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	stats := index.Stats()

	assert.Equal(t, 5, stats.TotalEntries)
	assert.Equal(t, int64(500), stats.TotalSize) // 5 * 100
	assert.Equal(t, float64(1), stats.AverageAccessCount)
	assert.Equal(t, 0, stats.ExpiredEntries)
}

func TestIndex_Iterator(t *testing.T) {
	index := NewIndex("", 100)

	// Add entries
	for i := 0; i < 5; i++ {
		entry := &IndexEntry{
			Key:  fmt.Sprintf("key%d", i),
			Size: 10,
		}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	// Create iterator
	iter := index.NewIterator(nil)
	defer iter.Close()

	count := 0
	for iter.Next() {
		assert.NotEmpty(t, iter.Key())
		entry := iter.Entry()
		assert.NotNil(t, entry)
		assert.Equal(t, iter.Key(), entry.Key)
		count++
	}

	assert.Equal(t, 5, count)
}

func TestIndex_IteratorWithFilter(t *testing.T) {
	index := NewIndex("", 100)

	// Add entries with different sizes
	for i := 0; i < 10; i++ {
		entry := &IndexEntry{
			Key:  fmt.Sprintf("key%d", i),
			Size: int64(i * 10), // 0, 10, 20, ..., 90
		}
		err := index.Put(fmt.Sprintf("key%d", i), entry)
		require.NoError(t, err)
	}

	// Filter for entries with size >= 50
	iter := index.NewIterator(func(entry *IndexEntry) bool {
		return entry.Size >= 50
	})
	defer iter.Close()

	var keys []string
	for iter.Next() {
		keys = append(keys, iter.Key())
		entry := iter.Entry()
		assert.True(t, entry.Size >= 50)
	}

	// Should have keys 5-9 (sizes 50, 60, 70, 80, 90)
	expectedKeys := []string{"key5", "key6", "key7", "key8", "key9"}
	assert.Equal(t, expectedKeys, keys)
}

func TestIndexConcurrency(t *testing.T) {
	index := NewIndex("", 1000)

	// Test concurrent operations
	done := make(chan bool, 20)

	// Concurrent puts
	for i := 0; i < 10; i++ {
		go func(id int) {
			entry := &IndexEntry{
				Key:  fmt.Sprintf("put_key%d", id),
				Size: 10,
			}
			err := index.Put(fmt.Sprintf("put_key%d", id), entry)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Concurrent gets
	for i := 0; i < 10; i++ {
		go func(id int) {
			// Some gets will be for non-existent keys, which is fine
			_, _ = index.Get(fmt.Sprintf("get_key%d", id))
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	assert.Equal(t, 10, index.Size())
}
