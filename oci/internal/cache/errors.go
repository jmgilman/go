package cache

import "errors"

// ErrCacheExpired is returned when attempting to access an expired cache entry.
var ErrCacheExpired = errors.New("cache entry has expired")

// ErrCacheCorrupted is returned when a cache entry is found to be corrupted.
var ErrCacheCorrupted = errors.New("cache entry is corrupted")

// ErrCacheFull is returned when the cache cannot store additional entries due to size limits.
var ErrCacheFull = errors.New("cache is full")

// ErrCacheInvalidated is returned when a cache entry has been invalidated and can no longer be used.
var ErrCacheInvalidated = errors.New("cache entry has been invalidated")
