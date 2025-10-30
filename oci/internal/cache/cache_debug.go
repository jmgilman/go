package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DebugInfo provides comprehensive debugging information about the cache state.
type DebugInfo struct {
	Timestamp   time.Time        `json:"timestamp"`
	Config      Config           `json:"config"`
	Metrics     MetricsSnapshot  `json:"metrics"`
	IndexStats  IndexStats       `json:"index_stats"`
	CacheStats  Stats            `json:"cache_stats"`
	TopEntries  []EntryInfo      `json:"top_entries"`
	Integrity   IntegrityReport  `json:"integrity"`
	StorageInfo StorageDebugInfo `json:"storage_info"`
}

// EntryInfo provides detailed information about a cache entry.
type EntryInfo struct {
	Key         string        `json:"key"`
	Size        int64         `json:"size"`
	CreatedAt   time.Time     `json:"created_at"`
	AccessedAt  time.Time     `json:"accessed_at"`
	TTL         time.Duration `json:"ttl"`
	AccessCount int64         `json:"access_count"`
	FilePath    string        `json:"file_path"`
	IsExpired   bool          `json:"is_expired"`
	Type        string        `json:"type"` // "manifest" or "blob"
}

// IntegrityReport provides information about cache integrity.
type IntegrityReport struct {
	TotalEntries     int           `json:"total_entries"`
	CorruptedEntries []string      `json:"corrupted_entries"`
	MissingFiles     []string      `json:"missing_files"`
	InvalidDigests   []string      `json:"invalid_digests"`
	IsHealthy        bool          `json:"is_healthy"`
	CheckDuration    time.Duration `json:"check_duration"`
}

// StorageDebugInfo provides information about storage usage.
type StorageDebugInfo struct {
	TotalFiles     int       `json:"total_files"`
	TotalSize      int64     `json:"total_size"`
	CachePath      string    `json:"cache_path"`
	Subdirectories []DirInfo `json:"subdirectories"`
}

// DirInfo provides information about a subdirectory.
type DirInfo struct {
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

// DebugTools provides debugging and maintenance utilities for the cache.
type DebugTools struct {
	coordinator *Coordinator
}

// NewDebugTools creates a new debug tools instance for the given coordinator.
func NewDebugTools(coordinator *Coordinator) *DebugTools {
	return &DebugTools{
		coordinator: coordinator,
	}
}

// CollectDebugInfo gathers comprehensive debugging information about the cache.
func (dt *DebugTools) CollectDebugInfo(ctx context.Context) (*DebugInfo, error) {
	dt.coordinator.mu.RLock()
	defer dt.coordinator.mu.RUnlock()

	info := &DebugInfo{
		Timestamp:  time.Now(),
		Config:     dt.coordinator.config,
		Metrics:    dt.coordinator.metrics.GetSnapshot(),
		IndexStats: dt.coordinator.index.Stats(),
		CacheStats: dt.coordinator.GetStats(),
	}

	// Collect top entries by access count
	if err := dt.collectTopEntries(ctx, info); err != nil {
		return nil, fmt.Errorf("failed to collect top entries: %w", err)
	}

	// Run integrity check
	integrity, err := dt.checkIntegrity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check integrity: %w", err)
	}
	info.Integrity = *integrity

	// Collect storage information
	storageInfo, err := dt.collectStorageInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect storage info: %w", err)
	}
	info.StorageInfo = *storageInfo

	return info, nil
}

// collectTopEntries collects information about the most frequently accessed entries.
func (dt *DebugTools) collectTopEntries(ctx context.Context, info *DebugInfo) error {
	allKeys := dt.coordinator.index.Keys(nil)
	entries := make([]EntryInfo, 0, len(allKeys))

	for _, key := range allKeys {
		indexEntry, exists := dt.coordinator.index.Get(key)
		if !exists {
			continue
		}

		entryType := "unknown"
		if strings.HasPrefix(indexEntry.FilePath, "manifests/") {
			entryType = "manifest"
		} else if strings.HasPrefix(indexEntry.FilePath, "blobs/") {
			entryType = "blob"
		}

		entry := EntryInfo{
			Key:         key,
			Size:        indexEntry.Size,
			CreatedAt:   indexEntry.CreatedAt,
			AccessedAt:  indexEntry.AccessedAt,
			TTL:         indexEntry.TTL,
			AccessCount: indexEntry.AccessCount,
			FilePath:    indexEntry.FilePath,
			IsExpired:   indexEntry.IsExpired(),
			Type:        entryType,
		}
		entries = append(entries, entry)
	}

	// Sort by access count (descending)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AccessCount > entries[j].AccessCount
	})

	// Take top 20 entries
	if len(entries) > 20 {
		info.TopEntries = entries[:20]
	} else {
		info.TopEntries = entries
	}

	return nil
}

// checkIntegrity performs a comprehensive integrity check of the cache.
func (dt *DebugTools) checkIntegrity(ctx context.Context) (*IntegrityReport, error) {
	start := time.Now()
	report := &IntegrityReport{
		CorruptedEntries: make([]string, 0),
		MissingFiles:     make([]string, 0),
		InvalidDigests:   make([]string, 0),
	}

	allKeys := dt.coordinator.index.Keys(nil)
	report.TotalEntries = len(allKeys)

	for _, key := range allKeys {
		indexEntry, exists := dt.coordinator.index.Get(key)
		if !exists {
			continue
		}

		// Check if file exists
		exists, err := dt.coordinator.storage.Exists(ctx, indexEntry.FilePath)
		if err != nil {
			report.CorruptedEntries = append(report.CorruptedEntries, key)
			continue
		}
		if !exists {
			report.MissingFiles = append(report.MissingFiles, key)
			continue
		}

		// Validate digest if we can read the file
		if err := dt.validateEntryDigest(ctx, key, indexEntry.FilePath); err != nil {
			report.InvalidDigests = append(report.InvalidDigests, key)
		}
	}

	report.IsHealthy = len(report.CorruptedEntries) == 0 &&
		len(report.MissingFiles) == 0 &&
		len(report.InvalidDigests) == 0
	report.CheckDuration = time.Since(start)

	return report, nil
}

// validateEntryDigest validates that the stored file matches its digest.
func (dt *DebugTools) validateEntryDigest(ctx context.Context, expectedDigest, filePath string) error {
	data, err := dt.coordinator.storage.ReadWithIntegrity(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(data)
	actualDigest := fmt.Sprintf("sha256:%x", hasher.Sum(nil))

	if actualDigest != expectedDigest {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expectedDigest, actualDigest)
	}

	return nil
}

// collectStorageInfo gathers information about storage usage.
func (dt *DebugTools) collectStorageInfo(ctx context.Context) (*StorageDebugInfo, error) {
	info := &StorageDebugInfo{
		CachePath:      dt.coordinator.storage.rootPath,
		Subdirectories: make([]DirInfo, 0),
	}

	// Get total size from storage
	totalSize, err := dt.coordinator.storage.Size(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage size: %w", err)
	}
	info.TotalSize = totalSize

	// Count files in known directories
	dirs := []string{"", "manifests", "blobs", "tags"}
	for _, dir := range dirs {
		files, err := dt.coordinator.storage.ListFiles(ctx, dir)
		if err != nil {
			// Skip directories that don't exist or can't be read
			continue
		}

		dirInfo := DirInfo{
			Path:      dir,
			FileCount: len(files),
		}

		// Estimate size for this directory (rough approximation)
		for _, file := range files {
			if indexEntry, exists := dt.coordinator.index.Get(file); exists {
				dirInfo.TotalSize += indexEntry.Size
			}
		}

		info.Subdirectories = append(info.Subdirectories, dirInfo)
		info.TotalFiles += len(files)
	}

	return info, nil
}

// TriggerCleanup manually triggers cache cleanup operations.
func (dt *DebugTools) TriggerCleanup(ctx context.Context) error {
	dt.coordinator.logger.Info(ctx, "manual cleanup triggered")

	if err := dt.coordinator.performCleanup(ctx); err != nil {
		dt.coordinator.logger.Error(ctx, "manual cleanup failed", "error", err)
		return fmt.Errorf("cleanup failed: %w", err)
	}

	dt.coordinator.logger.Info(ctx, "manual cleanup completed")
	return nil
}

// ClearExpiredEntries removes all expired entries from the cache.
func (dt *DebugTools) ClearExpiredEntries(ctx context.Context) (int, error) {
	dt.coordinator.mu.Lock()
	defer dt.coordinator.mu.Unlock()

	expiredKeys := dt.coordinator.index.ExpiredKeys()
	removedCount := 0

	for _, key := range expiredKeys {
		if err := dt.coordinator.deleteEntry(ctx, key); err != nil {
			dt.coordinator.logger.Warn(ctx, "failed to delete expired entry", "key", key, "error", err)
			continue
		}

		if indexEntry, exists := dt.coordinator.index.Get(key); exists {
			dt.coordinator.metrics.RecordEviction(indexEntry.Size)
			LogEviction(ctx, dt.coordinator.logger, key, indexEntry.Size, "manual_cleanup")
		}
		removedCount++
	}

	dt.coordinator.logger.Info(ctx, "cleared expired entries", "count", removedCount)
	return removedCount, nil
}

// RepairIntegrity attempts to repair integrity issues found during checks.
func (dt *DebugTools) RepairIntegrity(ctx context.Context) (*IntegrityReport, error) {
	dt.coordinator.logger.Info(ctx, "starting integrity repair")

	// First, check current integrity
	report, err := dt.checkIntegrity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check integrity: %w", err)
	}

	if report.IsHealthy {
		dt.coordinator.logger.Info(ctx, "cache integrity is already healthy")
		return report, nil
	}

	dt.coordinator.mu.Lock()
	defer dt.coordinator.mu.Unlock()

	// Remove entries with missing files
	for _, key := range report.MissingFiles {
		if err := dt.coordinator.deleteEntry(ctx, key); err != nil {
			dt.coordinator.logger.Warn(ctx, "failed to remove entry with missing file", "key", key, "error", err)
		} else {
			dt.coordinator.logger.Info(ctx, "removed entry with missing file", "key", key)
		}
	}

	// Remove entries with invalid digests
	for _, key := range report.InvalidDigests {
		if err := dt.coordinator.deleteEntry(ctx, key); err != nil {
			dt.coordinator.logger.Warn(ctx, "failed to remove entry with invalid digest", "key", key, "error", err)
		} else {
			dt.coordinator.logger.Info(ctx, "removed entry with invalid digest", "key", key)
		}
	}

	// Note: Corrupted entries (I/O errors) are not automatically removed as they might be transient

	dt.coordinator.logger.Info(
		ctx,
		"integrity repair completed",
		"removed_missing",
		len(report.MissingFiles),
		"removed_invalid",
		len(report.InvalidDigests),
	)

	// Return updated integrity report
	return dt.checkIntegrity(ctx)
}

// ExportDebugInfo exports debug information to a JSON file.
func (dt *DebugTools) ExportDebugInfo(ctx context.Context, outputPath string) error {
	info, err := dt.CollectDebugInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect debug info: %w", err)
	}

	// Create output directory if it doesn't exist
	if mkdirErr := os.MkdirAll(filepath.Dir(outputPath), 0o755); mkdirErr != nil {
		return fmt.Errorf("failed to create output directory: %w", mkdirErr)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(info); err != nil {
		return fmt.Errorf("failed to encode debug info: %w", err)
	}

	dt.coordinator.logger.Info(ctx, "debug info exported", "path", outputPath)
	return nil
}

// GetCacheContents returns a list of all cache entries with their metadata.
func (dt *DebugTools) GetCacheContents(ctx context.Context) ([]EntryInfo, error) {
	dt.coordinator.mu.RLock()
	defer dt.coordinator.mu.RUnlock()

	allKeys := dt.coordinator.index.Keys(nil)
	entries := make([]EntryInfo, 0, len(allKeys))

	for _, key := range allKeys {
		indexEntry, exists := dt.coordinator.index.Get(key)
		if !exists {
			continue
		}

		entryType := "unknown"
		if strings.HasPrefix(indexEntry.FilePath, "manifests/") {
			entryType = "manifest"
		} else if strings.HasPrefix(indexEntry.FilePath, "blobs/") {
			entryType = "blob"
		}

		entry := EntryInfo{
			Key:         key,
			Size:        indexEntry.Size,
			CreatedAt:   indexEntry.CreatedAt,
			AccessedAt:  indexEntry.AccessedAt,
			TTL:         indexEntry.TTL,
			AccessCount: indexEntry.AccessCount,
			FilePath:    indexEntry.FilePath,
			IsExpired:   indexEntry.IsExpired(),
			Type:        entryType,
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ValidateAllDigests performs digest validation on all cache entries.
func (dt *DebugTools) ValidateAllDigests(ctx context.Context) (int, []string, error) {
	dt.coordinator.mu.RLock()
	defer dt.coordinator.mu.RUnlock()

	allKeys := dt.coordinator.index.Keys(nil)
	invalidDigests := make([]string, 0)
	validCount := 0

	for _, key := range allKeys {
		indexEntry, exists := dt.coordinator.index.Get(key)
		if !exists {
			continue
		}

		if err := dt.validateEntryDigest(ctx, key, indexEntry.FilePath); err != nil {
			invalidDigests = append(invalidDigests, key)
			dt.coordinator.logger.Warn(ctx, "invalid digest found", "key", key, "error", err)
		} else {
			validCount++
		}
	}

	return validCount, invalidDigests, nil
}
