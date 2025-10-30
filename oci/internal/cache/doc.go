// Package cache provides a comprehensive caching system for OCI (Open Container Initiative) artifacts.
//
// This package implements a multi-tier caching strategy optimized for OCI registries,
// with separate handling for manifests and blobs. The cache system is designed to:
//
//   - Reduce network requests to OCI registries
//   - Provide configurable TTL (Time-To-Live) for cache entries
//   - Store already-compressed OCI artifacts efficiently (no double compression)
//   - Handle concurrent access safely
//   - Provide detailed metrics and observability
//   - Support pluggable eviction strategies
//
// # Architecture Overview
//
// The cache system consists of several key components:
//
//   - CacheManager: Central coordinator that manages cache configuration and metrics
//   - Cache: Core interface for basic cache operations (get, put, delete, clear)
//   - ManifestCache: Specialized interface for OCI manifest caching with validation
//   - BlobCache: Specialized interface for OCI blob caching with streaming support
//   - EvictionStrategy: Pluggable strategies for cache size management
//
// # Cache Entry Lifecycle
//
// 1. **Creation**: Entries are created with a key, data, and optional TTL
// 2. **Storage**: Entries are stored directly with metadata (OCI artifacts are pre-compressed)
// 3. **Access**: Entries track access patterns for eviction decisions
// 4. **Expiration**: Entries can expire based on TTL or be explicitly invalidated
// 5. **Eviction**: Entries may be evicted when cache size limits are reached
//
// # Error Handling
//
// The package defines specific errors for different cache failure scenarios:
//   - ErrCacheExpired: Entry exists but has exceeded its TTL
//   - ErrCacheCorrupted: Entry data is corrupted or unreadable
//   - ErrCacheFull: Cache cannot accept new entries due to size limits
//   - ErrCacheInvalidated: Entry has been explicitly invalidated
//
// All errors support proper wrapping with context using fmt.Errorf and %w.
//
// # Configuration
//
// Cache behavior is controlled through CacheConfig:
//   - MaxSizeBytes: Maximum cache size in bytes
//   - DefaultTTL: Default time-to-live for entries
//
// # Metrics and Observability
//
// The CacheMetrics struct tracks:
//   - Hit/miss ratios
//   - Eviction counts
//   - Error rates
//   - Storage utilization
//
// # Thread Safety
//
// All cache implementations must be safe for concurrent use by multiple goroutines.
// The package uses appropriate synchronization primitives to ensure data consistency.
//
// # Implementation Notes
//
// This package follows Go best practices:
//   - Interfaces are defined at the point of use
//   - Errors are wrapped with context
//   - Configuration is validated on creation
//   - Tests achieve high coverage with race detection
//   - Code follows the Go style guide from docs/guides/go/style.md
package cache
