package cache

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestLRUEviction_SelectForEviction(t *testing.T) {
	tests := []struct {
		name     string
		entries  map[string]*Entry
		expected []string
	}{
		{
			name:     "empty cache",
			entries:  map[string]*Entry{},
			expected: nil,
		},
		{
			name: "single entry",
			entries: map[string]*Entry{
				"key1": {
					Key:        "key1",
					AccessedAt: time.Now().Add(-time.Hour),
				},
			},
			expected: []string{"key1"},
		},
		{
			name: "multiple entries ordered by access time",
			entries: map[string]*Entry{
				"key1": {
					Key:        "key1",
					AccessedAt: time.Now().Add(-3 * time.Hour),
				},
				"key2": {
					Key:        "key2",
					AccessedAt: time.Now().Add(-1 * time.Hour),
				},
				"key3": {
					Key:        "key3",
					AccessedAt: time.Now().Add(-2 * time.Hour),
				},
			},
			expected: []string{"key1", "key3", "key2"}, // Oldest first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lru := NewLRUEviction()

			// Add entries in order of their access time (oldest first)
			// to ensure consistent LRU ordering
			var entriesToAdd []*Entry
			for _, entry := range tt.entries {
				entriesToAdd = append(entriesToAdd, entry)
			}
			// Sort by access time (oldest first)
			sort.Slice(entriesToAdd, func(i, j int) bool {
				return entriesToAdd[i].AccessedAt.Before(entriesToAdd[j].AccessedAt)
			})

			for _, entry := range entriesToAdd {
				lru.OnAdd(entry)
			}

			result := lru.SelectForEviction(tt.entries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLRUEviction_OnAccess(t *testing.T) {
	lru := NewLRUEviction()
	entry := &Entry{
		Key:        "test",
		AccessedAt: time.Now().Add(-time.Hour),
	}

	// Add entry first so it can be accessed
	lru.OnAdd(entry)

	// Initial access
	lru.OnAccess(entry)
	assert.True(t, entry.AccessedAt.After(time.Now().Add(-time.Minute)))
	assert.Equal(t, int64(1), entry.AccessCount)

	// Subsequent access
	oldAccessTime := entry.AccessedAt
	time.Sleep(time.Millisecond) // Ensure time difference
	lru.OnAccess(entry)
	assert.True(t, entry.AccessedAt.After(oldAccessTime))
	assert.Equal(t, int64(2), entry.AccessCount)
}

func TestLRUEviction_OnAdd(t *testing.T) {
	lru := NewLRUEviction()

	// Add new entry
	entry1 := &Entry{Key: "key1", AccessedAt: time.Now()}
	lru.OnAdd(entry1)

	// Add another entry
	entry2 := &Entry{Key: "key2", AccessedAt: time.Now()}
	lru.OnAdd(entry2)

	// Access first entry (should move to front)
	lru.OnAccess(entry1)

	// Check eviction order
	entries := map[string]*Entry{
		"key1": entry1,
		"key2": entry2,
	}
	toEvict := lru.SelectForEviction(entries)

	// key2 should be evicted first (least recently used)
	assert.Equal(t, []string{"key2", "key1"}, toEvict)
}

func TestLRUEviction_OnRemove(t *testing.T) {
	lru := NewLRUEviction()
	entry := &Entry{Key: "test"}

	// Add and then remove
	lru.OnAdd(entry)
	lru.OnRemove(entry)

	// Should not be in eviction candidates
	entries := map[string]*Entry{"test": entry}
	toEvict := lru.SelectForEviction(entries)
	assert.Empty(t, toEvict)
}

func TestSizeEviction_SelectForEviction(t *testing.T) {
	tests := []struct {
		name     string
		maxSize  int64
		entries  map[string]*Entry
		expected []string
	}{
		{
			name:    "under size limit",
			maxSize: 200,
			entries: map[string]*Entry{
				"key1": {Key: "key1", Data: make([]byte, 50)},
			},
			expected: nil,
		},
		{
			name:    "over size limit with expired entry",
			maxSize: 100,
			entries: map[string]*Entry{
				"expired": {
					Key:       "expired",
					Data:      make([]byte, 80),
					CreatedAt: time.Now().Add(-time.Hour),
					TTL:       time.Minute, // Expired
				},
				"fresh": {
					Key:       "fresh",
					Data:      make([]byte, 30),
					CreatedAt: time.Now(),
					TTL:       time.Hour,
				},
			},
			expected: []string{"expired"}, // Expired first
		},
		{
			name:    "over size limit by size",
			maxSize: 100,
			entries: map[string]*Entry{
				"small": {
					Key:       "small",
					Data:      make([]byte, 30),
					CreatedAt: time.Now(),
					TTL:       time.Hour,
				},
				"large": {
					Key:       "large",
					Data:      make([]byte, 80),
					CreatedAt: time.Now(),
					TTL:       time.Hour,
				},
			},
			expected: []string{"large"}, // Largest first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeEviction := NewSizeEviction(tt.maxSize)
			result := sizeEviction.SelectForEviction(tt.entries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTTLEviction_SelectForEviction(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		entries  map[string]*Entry
		expected []string
	}{
		{
			name: "no expired entries",
			entries: map[string]*Entry{
				"key1": {
					Key:       "key1",
					CreatedAt: now,
					TTL:       time.Hour,
				},
			},
			expected: nil,
		},
		{
			name: "mixed expired and fresh entries",
			entries: map[string]*Entry{
				"expired1": {
					Key:       "expired1",
					CreatedAt: now.Add(-2 * time.Hour),
					TTL:       time.Hour,
				},
				"expired2": {
					Key:       "expired2",
					CreatedAt: now.Add(-3 * time.Hour),
					TTL:       time.Hour,
				},
				"fresh": {
					Key:       "fresh",
					CreatedAt: now,
					TTL:       time.Hour,
				},
			},
			expected: []string{"expired2", "expired1"}, // Oldest expired first
		},
		{
			name: "no TTL entries",
			entries: map[string]*Entry{
				"key1": {
					Key:       "key1",
					CreatedAt: now.Add(-time.Hour),
					TTL:       0, // No expiration
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttlEviction := NewTTLEviction()
			result := ttlEviction.SelectForEviction(tt.entries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompositeEviction_SelectForEviction(t *testing.T) {
	now := time.Now()

	// Create entries with different characteristics
	entries := map[string]*Entry{
		"expired_large": {
			Key:        "expired_large",
			Data:       make([]byte, 100),
			CreatedAt:  now.Add(-2 * time.Hour),
			AccessedAt: now.Add(-2 * time.Hour),
			TTL:        time.Hour, // Expired
		},
		"fresh_large": {
			Key:        "fresh_large",
			Data:       make([]byte, 80),
			CreatedAt:  now,
			AccessedAt: now,
			TTL:        time.Hour,
		},
		"expired_small": {
			Key:        "expired_small",
			Data:       make([]byte, 20),
			CreatedAt:  now.Add(-2 * time.Hour),
			AccessedAt: now.Add(-2 * time.Hour),
			TTL:        time.Hour, // Expired
		},
	}

	// Create composite eviction with TTL priority 1, LRU priority 2
	ttlEviction := NewTTLEviction()
	lruEviction := NewLRUEviction()
	composite := NewCompositeEviction(
		[]EvictionStrategy{ttlEviction, lruEviction},
		[]int{1, 2}, // TTL has higher priority (lower number)
	)

	result := composite.SelectForEviction(entries)

	// Should prioritize expired entries, then by LRU within same priority
	assert.Contains(t, result, "expired_large")
	assert.Contains(t, result, "expired_small")
	// The order of expired entries should be determined by TTL strategy
}

func TestCompositeEviction_OnAccess(t *testing.T) {
	ttlEviction := NewTTLEviction()
	lruEviction := NewLRUEviction()
	composite := NewCompositeEviction(
		[]EvictionStrategy{ttlEviction, lruEviction},
		[]int{1, 2},
	)

	entry := &Entry{
		Key:        "test",
		AccessedAt: time.Now().Add(-time.Hour),
	}

	// OnAccess should be called on all strategies
	composite.OnAccess(entry)

	// Both strategies should have been notified
	assert.True(t, entry.AccessedAt.After(time.Now().Add(-time.Minute)))
	assert.Equal(t, int64(1), entry.AccessCount)
}

func TestEvictionStrategyConcurrency(t *testing.T) {
	lru := NewLRUEviction()

	// Test concurrent access to LRU eviction
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			entry := &Entry{
				Key:        fmt.Sprintf("key%d", id),
				AccessedAt: time.Now(),
			}
			lru.OnAdd(entry)
			lru.OnAccess(entry)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 10 entries
	entries := make(map[string]*Entry)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		entries[key] = &Entry{Key: key}
	}

	result := lru.SelectForEviction(entries)
	assert.Len(t, result, 10)
}

func TestEvictionUnderPressure(t *testing.T) {
	// Test eviction behavior under memory pressure simulation
	lru := NewLRUEviction()

	// Add many entries
	entries := make(map[string]*Entry)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key%d", i)
		entry := &Entry{
			Key:        key,
			AccessedAt: time.Now().Add(time.Duration(i) * time.Minute), // Different access times
		}
		entries[key] = entry
		lru.OnAdd(entry)
	}

	// Access some entries to change their LRU order
	for i := 50; i < 70; i++ {
		key := fmt.Sprintf("key%d", i)
		if entry, exists := entries[key]; exists {
			lru.OnAccess(entry)
		}
	}

	// Get eviction candidates
	toEvict := lru.SelectForEviction(entries)
	require.True(t, len(toEvict) > 0)

	// Debug: print the eviction list
	t.Logf("Eviction list: %v", toEvict)
	t.Logf("First few to evict: %v", toEvict[:min(5, len(toEvict))])
	t.Logf("Last few to evict: %v", toEvict[max(0, len(toEvict)-5):])

	// Most recently accessed entries (50-69) should be at the end of the eviction list
	// (meaning they should be evicted last, not first)
	if len(toEvict) >= 20 {
		// Check that recently accessed entries are near the end
		lastFew := toEvict[len(toEvict)-10:] // Last 10 entries
		assert.Contains(t, lastFew, "key69", "key69 should be evicted last")
		assert.Contains(t, lastFew, "key68", "key68 should be evicted last")
		assert.Contains(t, lastFew, "key67", "key67 should be evicted last")
	}
}
