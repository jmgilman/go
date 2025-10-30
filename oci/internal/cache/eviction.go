package cache

import (
	"container/list"
	"sort"
	"sync"
	"time"
)

// LRUEviction implements an LRU (Least Recently Used) eviction strategy.
// It maintains access order using a doubly-linked list and provides O(1) access tracking.
type LRUEviction struct {
	mu         sync.RWMutex
	entries    map[string]*list.Element
	accessList *list.List
}

// lruEntry wraps a cache entry with list element metadata for LRU tracking.
type lruEntry struct {
	key   string
	entry *Entry
}

// NewLRUEviction creates a new LRU eviction strategy.
func NewLRUEviction() *LRUEviction {
	return &LRUEviction{
		entries:    make(map[string]*list.Element),
		accessList: list.New(),
	}
}

// SelectForEviction chooses which cache entries should be evicted based on LRU policy.
// Returns keys ordered by eviction priority (oldest accessed first).
func (l *LRUEviction) SelectForEviction(entries map[string]*Entry) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Build eviction list in LRU order (oldest first)
	var candidates []string

	// Iterate through the access list from back to front (oldest to newest)
	for elem := l.accessList.Back(); elem != nil; elem = elem.Prev() {
		lruEntry := elem.Value.(*lruEntry)
		key := lruEntry.key

		// Only include entries that are still in the current entries map
		if _, exists := entries[key]; exists {
			candidates = append(candidates, key)
		}
	}

	return candidates
}

// OnAccess is called when a cache entry is accessed (read).
// Moves the entry to the front of the access list (most recently used).
func (l *LRUEviction) OnAccess(entry *Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := entry.Key
	if elem, exists := l.entries[key]; exists {
		// Move to front (most recently used)
		l.accessList.MoveToFront(elem)
		// Update the entry's access time
		if lru := elem.Value.(*lruEntry); lru.entry != nil {
			lru.entry.AccessedAt = time.Now()
			lru.entry.AccessCount++
		}
	}
}

// OnAdd is called when a new entry is added to the cache.
// Adds the entry to the front of the access list.
func (l *LRUEviction) OnAdd(entry *Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := entry.Key
	if _, exists := l.entries[key]; exists {
		// Entry already exists, just move to front
		l.OnAccess(entry)
		return
	}

	// Create new LRU entry
	lru := &lruEntry{
		key:   key,
		entry: entry,
	}

	// Add to front of access list
	elem := l.accessList.PushFront(lru)
	l.entries[key] = elem
}

// OnRemove is called when an entry is removed from the cache.
// Removes the entry from the access tracking.
func (l *LRUEviction) OnRemove(entry *Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := entry.Key
	if elem, exists := l.entries[key]; exists {
		l.accessList.Remove(elem)
		delete(l.entries, key)
	}
}

// SizeEviction implements size-based eviction when cache exceeds configured limits.
// It prioritizes evicting larger entries and expired entries.
type SizeEviction struct {
	maxSizeBytes int64
}

// NewSizeEviction creates a new size-based eviction strategy.
func NewSizeEviction(maxSizeBytes int64) *SizeEviction {
	return &SizeEviction{maxSizeBytes: maxSizeBytes}
}

// SelectForEviction chooses entries to evict when cache size exceeds limits.
// Prioritizes expired entries, then largest entries.
func (s *SizeEviction) SelectForEviction(entries map[string]*Entry) []string {
	var candidates []string
	var totalSize int64

	// Calculate total size and identify candidates
	for key, entry := range entries {
		totalSize += entry.Size()
		candidates = append(candidates, key)
	}

	// If under size limit, no eviction needed
	if totalSize <= s.maxSizeBytes {
		return nil
	}

	// Sort by eviction priority: expired first, then by size (largest first)
	sort.Slice(candidates, func(i, j int) bool {
		entryI := entries[candidates[i]]
		entryJ := entries[candidates[j]]

		// Prioritize expired entries
		expiredI := entryI.IsExpired()
		expiredJ := entryJ.IsExpired()

		if expiredI != expiredJ {
			return expiredI // Expired entries first
		}

		// Then by size (largest first)
		return entryI.Size() > entryJ.Size()
	})

	// Calculate how much space we need to free
	bytesToFree := totalSize - s.maxSizeBytes
	var selected []string
	var freedBytes int64

	for _, key := range candidates {
		if freedBytes >= bytesToFree {
			break
		}
		selected = append(selected, key)
		freedBytes += entries[key].Size()
	}

	return selected
}

// OnAccess is called when a cache entry is accessed.
// For size-based eviction, this is a no-op since we don't track access patterns.
func (s *SizeEviction) OnAccess(entry *Entry) {
	// No-op for size-based eviction
}

// OnAdd is called when a new entry is added to the cache.
// For size-based eviction, this is a no-op since we don't maintain state.
func (s *SizeEviction) OnAdd(entry *Entry) {
	// No-op for size-based eviction
}

// OnRemove is called when an entry is removed from the cache.
// For size-based eviction, this is a no-op since we don't maintain state.
func (s *SizeEviction) OnRemove(entry *Entry) {
	// No-op for size-based eviction
}

// TTLEviction implements TTL-based cleanup for expired entries.
// It identifies and removes entries that have exceeded their time-to-live.
type TTLEviction struct{}

// NewTTLEviction creates a new TTL-based eviction strategy.
func NewTTLEviction() *TTLEviction {
	return &TTLEviction{}
}

// SelectForEviction identifies all expired entries for cleanup.
// Returns expired entries ordered by expiration time (oldest first).
func (t *TTLEviction) SelectForEviction(entries map[string]*Entry) []string {
	var expired []string

	for key, entry := range entries {
		if entry.IsExpired() {
			expired = append(expired, key)
		}
	}

	// Sort by expiration time (oldest expired first)
	sort.Slice(expired, func(i, j int) bool {
		entryI := entries[expired[i]]
		entryJ := entries[expired[j]]

		// Calculate expiration times
		expiryI := entryI.CreatedAt.Add(entryI.TTL)
		expiryJ := entryJ.CreatedAt.Add(entryJ.TTL)

		return expiryI.Before(expiryJ)
	})

	return expired
}

// OnAccess is called when a cache entry is accessed.
// For TTL eviction, updates the access time but doesn't affect eviction priority.
func (t *TTLEviction) OnAccess(entry *Entry) {
	entry.AccessedAt = time.Now()
	entry.AccessCount++
}

// OnAdd is called when a new entry is added to the cache.
// For TTL eviction, this is a no-op since we check expiration on demand.
func (t *TTLEviction) OnAdd(entry *Entry) {
	// No-op for TTL-based eviction
}

// OnRemove is called when an entry is removed from the cache.
// For TTL eviction, this is a no-op since we don't maintain state.
func (t *TTLEviction) OnRemove(entry *Entry) {
	// No-op for TTL-based eviction
}

// CompositeEviction combines multiple eviction strategies with configurable priorities.
// It allows different strategies to work together for optimal cache management.
type CompositeEviction struct {
	strategies []EvictionStrategy
	priorities []int // Lower priority number = higher priority
}

// NewCompositeEviction creates a new composite eviction strategy.
func NewCompositeEviction(strategies []EvictionStrategy, priorities []int) *CompositeEviction {
	if len(strategies) != len(priorities) {
		panic("strategies and priorities must have the same length")
	}

	return &CompositeEviction{
		strategies: strategies,
		priorities: priorities,
	}
}

// SelectForEviction combines results from all strategies based on priority.
// Higher priority strategies (lower priority numbers) are processed first.
func (c *CompositeEviction) SelectForEviction(entries map[string]*Entry) []string {
	// Create strategy-priority pairs and sort by priority
	type strategyWithPriority struct {
		strategy EvictionStrategy
		priority int
	}

	var pairs []strategyWithPriority
	for i, strategy := range c.strategies {
		pairs = append(pairs, strategyWithPriority{
			strategy: strategy,
			priority: c.priorities[i],
		})
	}

	// Sort by priority (ascending)
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].priority < pairs[j].priority
	})

	// Collect all candidates, avoiding duplicates
	selected := make(map[string]bool)
	var result []string

	for _, pair := range pairs {
		candidates := pair.strategy.SelectForEviction(entries)
		for _, key := range candidates {
			if !selected[key] {
				selected[key] = true
				result = append(result, key)
			}
		}
	}

	return result
}

// OnAccess notifies all strategies of an access event.
func (c *CompositeEviction) OnAccess(entry *Entry) {
	for _, strategy := range c.strategies {
		strategy.OnAccess(entry)
	}
}

// OnAdd notifies all strategies of an add event.
func (c *CompositeEviction) OnAdd(entry *Entry) {
	for _, strategy := range c.strategies {
		strategy.OnAdd(entry)
	}
}

// OnRemove notifies all strategies of a remove event.
func (c *CompositeEviction) OnRemove(entry *Entry) {
	for _, strategy := range c.strategies {
		strategy.OnRemove(entry)
	}
}
