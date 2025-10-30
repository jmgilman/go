package cache

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// IndexEntry represents a single entry in the cache index.
type IndexEntry struct {
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	CreatedAt   time.Time         `json:"created_at"`
	AccessedAt  time.Time         `json:"accessed_at"`
	TTL         time.Duration     `json:"ttl"`
	AccessCount int64             `json:"access_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	FilePath    string            `json:"file_path"` // Relative path to the data file
}

// IsExpired returns true if the index entry has expired.
func (ie *IndexEntry) IsExpired() bool {
	if ie.TTL <= 0 {
		return false
	}
	return time.Since(ie.CreatedAt) > ie.TTL
}

// Index manages in-memory cache index with persistence and recovery capabilities.
type Index struct {
	mu                  sync.RWMutex
	entries             map[string]*IndexEntry
	indexPath           string
	maxEntries          int
	compactionThreshold float64
	lastCompaction      time.Time
}

// NewIndex creates a new cache index with the specified persistence path.
func NewIndex(indexPath string, maxEntries int) *Index {
	return &Index{
		entries:             make(map[string]*IndexEntry),
		indexPath:           indexPath,
		maxEntries:          maxEntries,
		compactionThreshold: 0.75, // Compact when 75% of entries are expired/deleted
		lastCompaction:      time.Now(),
	}
}

// Get retrieves an index entry by key.
func (idx *Index) Get(key string) (*IndexEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, exists := idx.entries[key]
	if !exists {
		return nil, false
	}

	// Update access time
	entry.AccessedAt = time.Now()
	entry.AccessCount++

	return entry, true
}

// Put stores or updates an index entry.
func (idx *Index) Put(key string, entry *IndexEntry) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries[key] = entry

	// Check if we need to persist or compact
	if len(idx.entries) >= idx.maxEntries {
		return idx.persist()
	}

	return nil
}

// Delete removes an index entry by key.
func (idx *Index) Delete(key string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.entries, key)

	// Mark for compaction if we have too many deleted entries
	if float64(len(idx.entries)) < float64(idx.maxEntries)*idx.compactionThreshold {
		return idx.compact()
	}

	return nil
}

// Size returns the current number of entries in the index.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// Keys returns all keys in the index, optionally filtered by a predicate.
func (idx *Index) Keys(filter func(*IndexEntry) bool) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var keys []string
	for key, entry := range idx.entries {
		if filter == nil || filter(entry) {
			keys = append(keys, key)
		}
	}
	return keys
}

// ExpiredKeys returns keys of all expired entries.
func (idx *Index) ExpiredKeys() []string {
	return idx.Keys(func(entry *IndexEntry) bool {
		return entry.IsExpired()
	})
}

// Load reads the index from persistent storage.
func (idx *Index) Load(ctx context.Context) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, err := os.Stat(idx.indexPath); os.IsNotExist(err) {
		// Index file doesn't exist, start with empty index
		return nil
	}

	file, err := os.Open(idx.indexPath)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer file.Close()

	// Read and validate the index
	scanner := bufio.NewScanner(file)
	lineNum := 0
	validEntries := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry IndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Log corruption but continue loading other entries
			continue
		}

		// Validate entry
		if entry.Key == "" {
			continue
		}

		idx.entries[entry.Key] = &entry
		validEntries++

		// Check for context cancellation periodically
		if lineNum%1000 == 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during index load: %w", ctx.Err())
			default:
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading index file: %w", err)
	}

	return nil
}

// Persist writes the current index to persistent storage.
func (idx *Index) Persist() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.persist()
}

// persist is the internal persistence method (assumes lock is held).
func (idx *Index) persist() error {
	// Skip persistence if indexPath is empty (for testing)
	if idx.indexPath == "" {
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(idx.indexPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	// Write to temporary file first
	tempPath := idx.indexPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp index file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// Sort keys for consistent output
	keys := make([]string, 0, len(idx.entries))
	for key := range idx.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Write each entry as a JSON line
	for _, key := range keys {
		entry := idx.entries[key]
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal index entry: %w", err)
		}

		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("failed to write index entry: %w", err)
		}
		if _, err := writer.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush index file: %w", err)
	}

	// Sync to ensure data is written
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync index file: %w", err)
	}

	// Close file before renaming
	file.Close()

	// Atomic rename
	if err := os.Rename(tempPath, idx.indexPath); err != nil {
		return fmt.Errorf("failed to rename temp index file: %w", err)
	}

	return nil
}

// Compact removes expired entries and optimizes the index structure.
func (idx *Index) Compact() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.compact()
}

// compact is the internal compaction method (assumes lock is held).
func (idx *Index) compact() error {
	// Remove expired entries
	for key, entry := range idx.entries {
		if entry.IsExpired() {
			delete(idx.entries, key)
		}
	}

	// Update compaction timestamp
	idx.lastCompaction = time.Now()

	// Persist the compacted index
	return idx.persist()
}

// IndexStats returns statistics about the index.
type IndexStats struct {
	TotalEntries       int
	ExpiredEntries     int
	TotalSize          int64
	LastCompaction     time.Time
	AverageAccessCount float64
}

// Stats returns current index statistics.
func (idx *Index) Stats() IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	stats := IndexStats{
		TotalEntries:   len(idx.entries),
		LastCompaction: idx.lastCompaction,
	}

	var totalAccessCount int64
	for _, entry := range idx.entries {
		stats.TotalSize += entry.Size
		totalAccessCount += entry.AccessCount

		if entry.IsExpired() {
			stats.ExpiredEntries++
		}
	}

	if stats.TotalEntries > 0 {
		stats.AverageAccessCount = float64(totalAccessCount) / float64(stats.TotalEntries)
	}

	return stats
}

// Cleanup removes expired entries and optionally compacts the index.
func (idx *Index) Cleanup(ctx context.Context, forceCompact bool) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove expired entries
	expiredKeys := make([]string, 0)
	for key, entry := range idx.entries {
		if entry.IsExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(idx.entries, key)
	}

	// Check if compaction is needed
	shouldCompact := forceCompact ||
		float64(len(idx.entries)) < float64(idx.maxEntries)*idx.compactionThreshold ||
		time.Since(idx.lastCompaction) > 24*time.Hour // Auto-compact daily

	if shouldCompact {
		return idx.compact()
	}

	return nil
}

// Iterator allows iteration over index entries with optional filtering.
type Iterator struct {
	idx    *Index
	keys   []string
	index  int
	filter func(*IndexEntry) bool
}

// NewIterator creates a new iterator over the index entries.
func (idx *Index) NewIterator(filter func(*IndexEntry) bool) *Iterator {
	idx.mu.RLock()
	keys := make([]string, 0, len(idx.entries))
	for key := range idx.entries {
		keys = append(keys, key)
	}
	idx.mu.RUnlock()

	sort.Strings(keys)

	return &Iterator{
		idx:    idx,
		keys:   keys,
		index:  -1,
		filter: filter,
	}
}

// Next advances the iterator to the next entry.
func (it *Iterator) Next() bool {
	it.index++
	for it.index < len(it.keys) {
		key := it.keys[it.index]
		entry, exists := it.idx.Get(key)
		if exists && (it.filter == nil || it.filter(entry)) {
			return true
		}
		it.index++
	}
	return false
}

// Entry returns the current entry.
func (it *Iterator) Entry() *IndexEntry {
	if it.index < 0 || it.index >= len(it.keys) {
		return nil
	}
	if entry, exists := it.idx.Get(it.keys[it.index]); exists {
		return entry
	}
	return nil
}

// Key returns the current key.
func (it *Iterator) Key() string {
	if it.index < 0 || it.index >= len(it.keys) {
		return ""
	}
	return it.keys[it.index]
}

// Close releases any resources held by the iterator.
func (it *Iterator) Close() {
	// No-op for in-memory iterator
}
