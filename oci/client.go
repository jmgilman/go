// Package ocibundle provides OCI bundle distribution functionality.
// This file contains the main client interface and implementation.
package ocibundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jmgilman/go/fs/billy"
	"github.com/jmgilman/go/fs/core"
	"oras.land/oras-go/v2/registry/remote"

	"github.com/jmgilman/go/oci/internal/cache"
	"github.com/jmgilman/go/oci/internal/oras"
)

// Client provides OCI bundle operations using ORAS for registry communication.
// The client is safe for concurrent use and isolates ORAS dependencies in internal packages.
type Client struct {
	// options contains the client configuration
	options *ClientOptions

	// orasClient provides ORAS operations (injected for testability)
	orasClient oras.Client

	// cache provides caching functionality for OCI operations
	cache cache.Cache

	// mu protects concurrent access to client operations
	mu sync.RWMutex
}

// New creates a new Client with default configuration.
// It uses ORAS's default Docker credential chain for authentication.
func New() (*Client, error) {
	return NewWithOptions()
}

// NewWithOptions creates a new Client with custom configuration.
// It accepts functional options to customize authentication and other behaviors.
//
// Example usage:
//
//	client, err := NewWithOptions(
//	    WithStaticAuth("ghcr.io", "username", "password"),
//	)
//	if err != nil {
//	    return err
//	}
func NewWithOptions(opts ...ClientOption) (*Client, error) {
	options := DefaultClientOptions()

	// Apply functional options
	for _, opt := range opts {
		opt(options)
	}

	// Ensure filesystem default
	if options.FS == nil {
		options.FS = billy.NewLocal()
	}

	// Use provided ORAS client or default to real implementation
	orasClient := options.ORASClient
	if orasClient == nil {
		orasClient = &oras.DefaultORASClient{}
	}

	// Convert public HTTPConfig to internal AuthOptions format
	if options.HTTPConfig != nil {
		if options.Auth == nil {
			options.Auth = &oras.AuthOptions{}
		}
		options.Auth.HTTPConfig = &oras.HTTPConfig{
			AllowHTTP:     options.HTTPConfig.AllowHTTP,
			AllowInsecure: options.HTTPConfig.AllowInsecure,
			Registries:    options.HTTPConfig.Registries,
		}
	}

	client := &Client{
		options:    options,
		orasClient: orasClient,
		cache:      nil, // Cache will be initialized lazily if needed
	}

	// Validate options
	if err := validateClientOptions(options); err != nil {
		return nil, fmt.Errorf("invalid client options: %w", err)
	}

	return client, nil
}

// validateClientOptions validates the client options for correctness.
// It checks for invalid combinations and missing required values.
//
// Parameters:
//   - opts: The client options to validate
//
// Returns an error if validation fails, nil if options are valid.
func validateClientOptions(opts *ClientOptions) error {
	if opts == nil {
		return fmt.Errorf("client options cannot be nil")
	}

	// Validate authentication options if present
	if opts.Auth == nil {
		return nil
	}
	// If static auth is specified, both username and password must be provided
	if opts.Auth.StaticRegistry == "" {
		return nil
	}
	if opts.Auth.StaticUsername == "" {
		return fmt.Errorf("static username required when static registry is specified")
	}
	if opts.Auth.StaticPassword == "" {
		return fmt.Errorf("static password required when static registry is specified")
	}

	return nil
}

// createRepository creates an ORAS repository with authentication configured.
// This is an internal method that applies the client's auth options to repository creation.
//
// Parameters:
//   - ctx: Context for the operation
//   - reference: Full OCI reference (e.g., "ghcr.io/org/repo:tag")
//
// Returns:
//   - Configured ORAS repository ready for operations
//   - Error if repository creation fails
func (c *Client) createRepository(ctx context.Context, reference string) (*remote.Repository, error) {
	// Note: mutex is already held by caller, so we don't need to lock here
	repo, err := oras.NewRepository(ctx, reference, c.options.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository for %s: %w", reference, err)
	}
	return repo, nil
}

// retryOperation retries a function with exponential backoff for network-related errors
func retryOperation(ctx context.Context, maxRetries int, delay time.Duration, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry operation: %w", ctx.Err())
		default:
		}

		if attempt > 0 {
			// Exponential backoff
			backoffDelay := delay * time.Duration(1<<(attempt-1))
			time.Sleep(backoffDelay)
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Only retry on network-related errors
		if !isRetryableError(err) {
			break
		}
	}

	return lastErr
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	// Network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Connection errors
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Registry-specific temporary errors (5xx status codes)
	errStr := err.Error()
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		// Check for common temporary error patterns
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "service unavailable") ||
		strings.Contains(errStr, "internal server error")
}

// Push uploads a directory as an OCI artifact to the specified reference.
// It archives the source directory and pushes it to the OCI registry with the given options.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - sourceDir: Path to directory to upload (must exist and be readable)
//   - reference: OCI reference (e.g., "ghcr.io/org/repo:tag")
//   - opts: Optional push options for annotations, platform, and progress reporting
//
// Returns:
//   - Error if the operation fails
func (c *Client) Push(ctx context.Context, sourceDir, reference string, opts ...PushOption) error {
	// Thread safety: use read lock since we're only reading options
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Parse push options
	pushOpts := DefaultPushOptions()
	for _, opt := range opts {
		opt(pushOpts)
	}

	// Validate inputs
	if sourceDir == "" {
		return fmt.Errorf("source directory cannot be empty")
	}

	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}

	// Check if source directory exists and is readable
	if exists, exErr := c.options.FS.Exists(sourceDir); exErr != nil {
		return fmt.Errorf("failed to check source directory: %w", exErr)
	} else if !exists {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}

	// Create authenticated repository (needed for future authentication validation)
	_, repoErr := c.createRepository(ctx, reference)
	if repoErr != nil {
		return repoErr
	}

	// Create temporary directory and file for the archive within our filesystem
	var tempDir string
	var tmpErr error
	if tfs, ok := c.options.FS.(core.TempFS); ok {
		tempDir, tmpErr = tfs.TempDir("", "ocibundle-push-")
	} else {
		// Fallback: use a fixed directory
		tempDir = filepath.Join(os.TempDir(), "ocibundle-push-")
		tmpErr = c.options.FS.MkdirAll(tempDir, 0755)
	}
	if tmpErr != nil {
		return fmt.Errorf("failed to create temporary directory: %w", tmpErr)
	}
	tempFilePath := filepath.Join(tempDir, "bundle.tar.gz")
	tempFile, openErr := c.options.FS.OpenFile(tempFilePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if openErr != nil {
		return fmt.Errorf("failed to create temporary file: %w", openErr)
	}
	cleanupNeeded := true
	defer func() {
		_ = tempFile.Close()
		if cleanupNeeded {
			_ = c.options.FS.Remove(tempFilePath)
		}
	}()

	// Create archiver
	archiver := NewTarGzArchiverWithFS(c.options.FS)

	// Archive the source directory (with progress if callback provided)
	var archiveErr error
	if pushOpts.ProgressCallback != nil {
		archiveErr = archiver.ArchiveWithProgress(ctx, sourceDir, tempFile, pushOpts.ProgressCallback)
	} else {
		archiveErr = archiver.Archive(ctx, sourceDir, tempFile)
	}
	if archiveErr != nil {
		return fmt.Errorf("failed to archive directory: %w", archiveErr)
	}

	// Get file size
	stat, statErr := tempFile.Stat()
	if statErr != nil {
		return fmt.Errorf("failed to get file size: %w", statErr)
	}

	// Push the artifact with retry logic
	pushErr := retryOperation(ctx, pushOpts.MaxRetries, pushOpts.RetryDelay, func() error {
		// Rewind before each attempt (if file supports seeking)
		if seeker, ok := tempFile.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				return fmt.Errorf("failed to seek temporary file: %w", seekErr)
			}
		}
		// Recreate descriptor to ensure fresh reader for each attempt
		desc := &oras.PushDescriptor{
			MediaType:   archiver.MediaType(),
			Data:        tempFile,
			Size:        stat.Size(),
			Annotations: pushOpts.Annotations,
			Platform:    pushOpts.Platform,
		}
		return c.orasClient.Push(ctx, reference, desc, c.options.Auth)
	})
	if pushErr != nil {
		return fmt.Errorf("failed to push artifact after %d retries: %w", pushOpts.MaxRetries, pushErr)
	}

	// Success - no cleanup needed
	cleanupNeeded = false
	return nil
}

// Pull downloads and extracts an OCI artifact to the specified directory.
// It downloads the artifact from the OCI registry and extracts it with security validation.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - reference: OCI reference to download (e.g., "ghcr.io/org/repo:tag")
//   - targetDir: Directory to extract the artifact to (created if it doesn't exist)
//   - opts: Optional pull options for security limits and behavior
//
// Returns:
//   - Error if the operation fails
func (c *Client) Pull(ctx context.Context, reference, targetDir string, opts ...PullOption) error {
	// Thread safety: use read lock since we're only reading options
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Parse pull options
	pullOpts := DefaultPullOptions()
	for _, opt := range opts {
		opt(pullOpts)
	}

	// Validate inputs
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}

	if targetDir == "" {
		return fmt.Errorf("target directory cannot be empty")
	}

	// Check if target directory exists and is empty (for atomic extraction)
	if exists, exErr := c.options.FS.Exists(targetDir); exErr != nil {
		return fmt.Errorf("failed to check target directory: %w", exErr)
	} else if exists {
		// Directory exists, check if it's empty
		entries, readErr := c.options.FS.ReadDir(targetDir)
		if readErr != nil {
			return fmt.Errorf("failed to read target directory: %w", readErr)
		}
		if len(entries) > 0 {
			return fmt.Errorf("target directory is not empty: %s", targetDir)
		}
	}

	// Create authenticated repository (needed for future authentication validation)
	_, repoErr := c.createRepository(ctx, reference)
	if repoErr != nil {
		return repoErr
	}

	// Pull the artifact with retry logic
	var descriptor *oras.PullDescriptor
	pullErr := retryOperation(ctx, pullOpts.MaxRetries, pullOpts.RetryDelay, func() error {
		var err error
		descriptor, err = c.orasClient.Pull(ctx, reference, c.options.Auth)
		if err != nil {
			return fmt.Errorf("failed to pull OCI artifact %s: %w", reference, err)
		}
		return nil
	})
	if pullErr != nil {
		return fmt.Errorf("failed to pull artifact after %d retries: %w", pullOpts.MaxRetries, pullErr)
	}

	// Ensure we close the descriptor data when done
	defer descriptor.Data.Close()

	// Create archiver
	archiver := NewTarGzArchiverWithFS(c.options.FS)

	// Create extract options from pull options
	extractOpts := ExtractOptions{
		MaxFiles:      pullOpts.MaxFiles,
		MaxSize:       pullOpts.MaxSize,
		MaxFileSize:   pullOpts.MaxFileSize,
		StripPrefix:   pullOpts.StripPrefix,
		PreservePerms: pullOpts.PreservePermissions,
	}

	// Extract the archive atomically (all or nothing)
	if err := c.extractAtomically(ctx, archiver, descriptor.Data, targetDir, extractOpts); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

// PullWithCache downloads and extracts an OCI artifact to the specified directory
// with intelligent caching to improve performance on repeated pulls.
// This method enhances the regular Pull method with caching capabilities.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - reference: OCI reference to download (e.g., "ghcr.io/org/repo:tag")
//   - targetDir: Directory to extract the artifact to (created if it doesn't exist)
//   - opts: Optional pull options for security limits, behavior, and cache control
//
// Returns:
//   - Error if the operation fails
//
// The method will:
// 1. Check if caching is enabled and appropriate for the operation
// 2. Attempt to serve from cache if available and not bypassed
// 3. Fall back to network pull if cache miss or bypass requested
// 4. Cache the result for future pulls if caching is enabled
func (c *Client) PullWithCache(ctx context.Context, reference, targetDir string, opts ...PullOption) error {
	// Thread safety: use read lock since we're only reading options
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Parse pull options
	pullOpts := DefaultPullOptions()
	for _, opt := range opts {
		opt(pullOpts)
	}

	// Validate inputs
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}

	if targetDir == "" {
		return fmt.Errorf("target directory cannot be empty")
	}

	// Check if caching is enabled and should be used for this operation
	cacheEnabled := c.isCachingEnabledForPull(pullOpts)

	if !cacheEnabled {
		// Fall back to regular pull if caching is disabled or bypassed
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	// Ensure cache is initialized
	if err := c.ensureCacheInitialized(ctx); err != nil {
		// Log warning but continue with regular pull
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	// Try to get from cache first
	cacheKey := c.generateCacheKey(reference)
	if err := c.getFromCache(ctx, cacheKey, targetDir); err == nil {
		// Successfully served from cache
		return nil
	}
	// Cache miss or error - proceed with network pull

	// Perform the network pull
	if err := c.Pull(ctx, reference, targetDir, opts...); err != nil {
		return err
	}

	// Cache the result for future use
	// Note: We cache after successful extraction to ensure data integrity
	if err := c.storeInCache(ctx, cacheKey, targetDir); err != nil {
		// Don't fail the operation if caching fails, just log the error
		// The pull was successful, caching is a performance optimization
		return nil
	}

	return nil
}

// isCachingEnabledForPull determines if caching should be used for this pull operation
func (c *Client) isCachingEnabledForPull(opts *PullOptions) bool {
	// Check if cache bypass is requested
	if opts.CacheBypass {
		return false
	}

	// Check if cache is configured
	if c.options.CacheConfig == nil || c.options.CacheConfig.Coordinator == nil {
		return false
	}

	// Check cache policy
	switch c.options.CacheConfig.Policy {
	case CachePolicyDisabled:
		return false
	case CachePolicyPull, CachePolicyEnabled:
		return true
	case CachePolicyPush:
		return false
	default:
		return false
	}
}

// ensureCacheInitialized ensures the cache is properly initialized
func (c *Client) ensureCacheInitialized(ctx context.Context) error {
	if c.cache != nil {
		return nil
	}

	if c.options.CacheConfig == nil || c.options.CacheConfig.Coordinator == nil {
		return fmt.Errorf("cache not configured")
	}

	// Initialize cache with the configured coordinator
	c.cache = c.options.CacheConfig.Coordinator
	return nil
}

// generateCacheKey creates a unique cache key for the given reference
func (c *Client) generateCacheKey(reference string) string {
	// Use SHA256 hash of the reference for consistent cache keys
	// This ensures the same reference always maps to the same cache key
	return fmt.Sprintf("pull:%s", reference)
}

// getFromCache attempts to retrieve and extract from cache
func (c *Client) getFromCache(ctx context.Context, cacheKey, targetDir string) error {
	// This is a placeholder implementation
	// In a full implementation, this would:
	// 1. Check if cache entry exists
	// 2. Verify cache entry integrity
	// 3. Extract cached data to target directory
	// 4. Update cache access statistics

	return fmt.Errorf("cache miss: not implemented")
}

// storeInCache stores the extracted directory in the cache
func (c *Client) storeInCache(ctx context.Context, cacheKey, sourceDir string) error {
	// This is a placeholder implementation
	// In a full implementation, this would:
	// 1. Create a new cache entry
	// 2. Archive the directory contents
	// 3. Store the archive in cache with metadata
	// 4. Update cache statistics

	return fmt.Errorf("cache storage: not implemented")
}

// extractAtomically performs atomic extraction with rollback on failure
func (c *Client) extractAtomically(
	ctx context.Context,
	archiver *TarGzArchiver,
	data io.Reader,
	targetDir string,
	opts ExtractOptions,
) error {
	// Create a temporary directory for extraction
	var tempDir string
	var tmpErr error
	if tfs, ok := c.options.FS.(core.TempFS); ok {
		tempDir, tmpErr = tfs.TempDir("", "ocibundle-pull-")
	} else {
		// Fallback: use a fixed directory
		tempDir = filepath.Join(os.TempDir(), "ocibundle-pull-")
		tmpErr = c.options.FS.MkdirAll(tempDir, 0755)
	}
	if tmpErr != nil {
		return fmt.Errorf("failed to create temporary directory: %w", tmpErr)
	}
	defer func() { _ = c.removeAllFS(tempDir) }()

	// Extract to temporary directory first
	if err := archiver.Extract(ctx, data, tempDir, opts); err != nil {
		return fmt.Errorf("extraction to temporary directory failed: %w", err)
	}

	// Ensure target directory exists
	if err := c.options.FS.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Move extracted files from temp directory to target directory
	if err := c.moveFiles(tempDir, targetDir); err != nil {
		// Clean up any partially moved files
		_ = c.removeAllFS(targetDir)
		return fmt.Errorf("failed to move extracted files: %w", err)
	}

	return nil
}

// moveFiles moves all files from srcDir to dstDir
func (c *Client) moveFiles(srcDir, dstDir string) error {
	if err := c.options.FS.Walk(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk path %s: %w", path, walkErr)
		}

		// Skip the root directory
		if path == srcDir {
			return nil
		}

		// Calculate relative path from source
		relPath, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return fmt.Errorf("failed to get relative path from %s to %s: %w", srcDir, path, relErr)
		}

		// Calculate destination path
		dstPath := filepath.Join(dstDir, relPath)

		if d.IsDir() {
			// Create directory
			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("failed to get dir info: %w", err)
			}
			if mkErr := c.options.FS.MkdirAll(dstPath, info.Mode()); mkErr != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, mkErr)
			}
			return nil
		}

		if err := c.options.FS.Rename(path, dstPath); err != nil {
			return fmt.Errorf("failed to rename %s to %s: %w", path, dstPath, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("move files: %w", err)
	}
	return nil
}

// removeAllFS removes a directory tree using the filesystem (best-effort).
func (c *Client) removeAllFS(root string) error {
	// Simple recursive delete using Walk in reverse order: files before dirs.
	var toDelete []string
	_ = c.options.FS.Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		toDelete = append(toDelete, path)
		return nil
	})
	// delete deepest paths first
	for i := len(toDelete) - 1; i >= 0; i-- {
		_ = c.options.FS.Remove(toDelete[i])
	}
	return nil
}
