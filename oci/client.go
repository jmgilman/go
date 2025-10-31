package ocibundle

import (
	"bytes"
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
	orasint "github.com/jmgilman/go/oci/internal/oras"
)

const (
	// maxConcurrentWorkers is the maximum number of concurrent workers for archiving operations.
	maxConcurrentWorkers = 8
	// footerSizeBytes is the size of the eStargz footer in bytes (sufficient for TOC offset).
	footerSizeBytes = 100
)

// validatePullInputs validates inputs for pull operations.
func validatePullInputs(reference, targetDir string) error {
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}
	if targetDir == "" {
		return fmt.Errorf("target directory cannot be empty")
	}
	return nil
}

// Client provides OCI bundle operations using ORAS for registry communication.
// The client is safe for concurrent use and isolates ORAS dependencies in internal packages.
type Client struct {
	// options contains the client configuration
	options *ClientOptions

	// orasClient provides ORAS operations (injected for testability)
	orasClient orasint.Client

	// cache provides caching functionality for OCI operations
	cache cache.Cache

	// cacheOnce ensures cache is initialized only once
	cacheOnce sync.Once

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
		orasClient = &orasint.DefaultORASClient{}
	}

	// Convert public HTTPConfig to internal AuthOptions format
	if options.HTTPConfig != nil {
		if options.Auth == nil {
			options.Auth = &orasint.AuthOptions{}
		}
		options.Auth.HTTPConfig = &orasint.HTTPConfig{
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
func (c *Client) createRepository(ctx context.Context, reference string) (*remote.Repository, error) {
	// Note: mutex is already held by caller, so we don't need to lock here
	repo, err := orasint.NewRepository(ctx, reference, c.options.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository for %s: %w", reference, err)
	}
	return repo, nil
}

// retryOperation retries a function with exponential backoff for network-related errors
func retryOperation(ctx context.Context, maxRetries int, delay time.Duration, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := isDone(ctx, "retry operation"); err != nil {
			return err
		}

		if attempt > 0 {
			// Exponential backoff: delay * 2^(attempt-1)
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
	// Don't retry if context was canceled
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Retry on deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for network errors with Temporary() method
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Temporary() {
		return true
	}

	// String matching as fallback for errors that don't expose proper types
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "service unavailable") ||
		strings.Contains(errStr, "internal server error")
}

// Push uploads a directory as an OCI artifact to the specified reference.
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
	tempDir, tmpErr := c.createTempDir("ocibundle-push-")
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
		desc := &orasint.PushDescriptor{
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
// Supports selective extraction using glob patterns and enforces security validation.
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

	// Create authenticated repository (needed for HTTP Range requests and authentication)
	repo, repoErr := c.createRepository(ctx, reference)
	if repoErr != nil {
		return repoErr
	}

	// Pull the artifact with retry logic
	var descriptor *orasint.PullDescriptor
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

	// Create extract options from pull options
	extractOpts := ExtractOptions{
		MaxFiles:         pullOpts.MaxFiles,
		MaxSize:          pullOpts.MaxSize,
		MaxFileSize:      pullOpts.MaxFileSize,
		AllowHiddenFiles: pullOpts.AllowHiddenFiles,
		StripPrefix:      pullOpts.StripPrefix,
		PreservePerms:    pullOpts.PreservePermissions,
		FilesToExtract:   pullOpts.FilesToExtract,
	}

	// Decision tree for extraction strategy
	if len(pullOpts.FilesToExtract) > 0 {
		// Selective extraction requested - try HTTP Range for bandwidth optimization
		var readerAt io.ReaderAt
		var blobSize int64

		// Try HTTP Range approach for bandwidth savings
		blobURL, httpClient, urlErr := getBlobURLFromRepository(repo, descriptor.Digest)
		if urlErr == nil && testBlobRangeSupport(ctx, httpClient, blobURL) {
			// Registry supports Range requests - use HTTP Range seeker
			// This downloads only the chunks needed for selected files
			descriptor.Data.Close() // Close the full download stream

			rangeSeeker := newHTTPRangeSeeker(httpClient, blobURL)
			defer rangeSeeker.Close()

			readerAt = newReaderAtFromSeeker(rangeSeeker, descriptor.Size)
			blobSize = descriptor.Size
		} else {
			// Fallback: Read the full blob into memory
			// This happens when:
			// - Registry doesn't support HTTP Range requests
			// - Unable to get blob URL or HTTP client
			blobData, err := io.ReadAll(descriptor.Data)
			if err != nil {
				return fmt.Errorf("failed to read blob data: %w", err)
			}

			// Create a ReaderAt from the blob data
			readerAt = bytes.NewReader(blobData)
			blobSize = int64(len(blobData))
		}

		// Try selective extraction with estargz
		tempDir, tmpErr := c.createTempDir("ocibundle-selective-")
		if tmpErr != nil {
			return fmt.Errorf("failed to create temporary directory: %w", tmpErr)
		}
		defer func() { _ = c.removeAllFS(tempDir) }()

		// Extract selectively to temp directory
		if err := extractSelectiveFromStargz(
			ctx,
			readerAt,
			blobSize,
			tempDir,
			pullOpts.FilesToExtract,
			extractOpts,
			c.options.FS,
		); err != nil {
			return fmt.Errorf("failed to extract selectively: %w", err)
		}

		// Ensure target directory exists
		if err := c.options.FS.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}

		// Move extracted files to target directory
		if err := c.moveFiles(tempDir, targetDir); err != nil {
			// Clean up any partially moved files
			_ = c.removeAllFS(targetDir)
			return fmt.Errorf("failed to move extracted files: %w", err)
		}

	} else {
		// Full extraction (no selective patterns)
		// Use existing extraction logic
		archiver := NewTarGzArchiverWithFS(c.options.FS)

		if err := c.extractAtomically(ctx, archiver, descriptor.Data, targetDir, extractOpts); err != nil {
			return fmt.Errorf("failed to extract archive: %w", err)
		}
	}

	return nil
}

// PullWithCache downloads and extracts an OCI artifact with caching support.
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
	if err := validatePullInputs(reference, targetDir); err != nil {
		return err
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

	// Step 1: Resolve tag to digest (with caching)
	digest, err := c.resolveTagWithCache(ctx, reference)
	if err != nil {
		// Failed to resolve - fall back to regular pull
		// This shouldn't fail in normal operation, but gracefully degrade
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	// Step 2: Generate cache key from immutable digest
	cacheKey := c.generateCacheKey(digest)

	// Step 3: Try to get from cache first
	if err := c.getFromCache(ctx, cacheKey, targetDir); err == nil {
		// Successfully served from cache - no network access needed!
		return nil
	}
	// Cache miss or error - proceed with network pull

	// Step 4: Perform network pull (will cache blob during download in future)
	// TODO: Implement blob caching during download using tee pattern (Phase 3)
	if err := c.Pull(ctx, reference, targetDir, opts...); err != nil {
		return err
	}

	// Note: Blob caching will be done during Pull() in Phase 3
	// For now, blob is not cached (but tag→digest mapping is cached)

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

// ensureCacheInitialized ensures the cache is properly initialized using sync.Once
// for thread-safe initialization.
func (c *Client) ensureCacheInitialized(ctx context.Context) error {
	var initErr error

	c.cacheOnce.Do(func() {
		if c.options.CacheConfig == nil || c.options.CacheConfig.Coordinator == nil {
			initErr = fmt.Errorf("cache not configured")
			return
		}

		// Initialize cache with the configured coordinator
		c.cache = c.options.CacheConfig.Coordinator
	})

	return initErr
}

// generateCacheKey creates a unique cache key from a content digest.
// Uses digest-based keys for immutability (tags are mutable, digests are not).
func (c *Client) generateCacheKey(digest string) string {
	return fmt.Sprintf("blob:%s", digest)
}

// resolveTagWithCache resolves a tag or reference to its digest, using cache when possible.
func (c *Client) resolveTagWithCache(ctx context.Context, reference string) (string, error) {
	// If reference is already a digest, return it directly
	if strings.Contains(reference, "@sha256:") || strings.Contains(reference, "@sha512:") {
		// Extract digest from reference like "repo@sha256:abc"
		parts := strings.SplitN(reference, "@", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}

	// Cast cache to Coordinator to access tag mapping methods
	coordinator, ok := c.cache.(*cache.Coordinator)
	if !ok {
		// Cache is not a Coordinator - can't use tag caching
		// Fall back to direct resolution
		return c.resolveTagDirect(ctx, reference)
	}

	// Check tag cache first
	mapping, err := coordinator.GetTagMapping(ctx, reference)
	if err == nil && mapping != nil {
		// Cache hit - return cached digest
		return mapping.Digest, nil
	}

	// Cache miss - need to query registry to resolve tag
	digest, err := c.resolveTagDirect(ctx, reference)
	if err != nil {
		return "", err
	}

	// Cache the tag→digest mapping for future use
	if cacheErr := coordinator.PutTagMapping(ctx, reference, digest); cacheErr != nil {
		// Don't fail if caching fails - resolution succeeded
		// Just log the error (in production, would use proper logging)
	}

	return digest, nil
}

// resolveTagDirect queries the registry to resolve a tag to a digest without caching.
func (c *Client) resolveTagDirect(ctx context.Context, reference string) (string, error) {
	// Use the internal ORAS client to pull and get the digest
	// This is a lightweight operation that just fetches the manifest
	descriptor, err := c.orasClient.Pull(ctx, reference, c.options.Auth)
	if err != nil {
		return "", fmt.Errorf("failed to resolve tag: %w", err)
	}
	defer descriptor.Data.Close()

	// The descriptor includes the digest
	return descriptor.Digest, nil
}

// getFromCache attempts to retrieve and extract from cache.
// Returns nil on success, error on cache miss or extraction failure.
func (c *Client) getFromCache(ctx context.Context, cacheKey, targetDir string) error {
	if c.cache == nil {
		return fmt.Errorf("cache not configured")
	}

	coordinator, ok := c.cache.(*cache.Coordinator)
	if !ok {
		return fmt.Errorf("cache is not a coordinator")
	}

	// Extract digest from cache key (format: "blob:sha256:abc...")
	digest := strings.TrimPrefix(cacheKey, "blob:")

	// Try to get cached blob
	blobReader, err := coordinator.GetBlob(ctx, digest)
	if err != nil {
		return fmt.Errorf("cache miss: %w", err)
	}
	defer blobReader.Close()

	// Create archiver for extraction
	archiver := NewTarGzArchiverWithFS(c.options.FS)

	// Extract cached blob to target directory with default options
	extractOpts := ExtractOptions{
		PreservePerms: true,
	}
	if err := c.extractAtomically(ctx, archiver, blobReader, targetDir, extractOpts); err != nil {
		return fmt.Errorf("failed to extract cached blob: %w", err)
	}

	return nil
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
	tempDir, tmpErr := c.createTempDir("ocibundle-pull-")
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
// This is a best-effort cleanup operation that ignores errors from individual
// file/directory removals, as some may fail due to permissions or concurrent access.
func (c *Client) removeAllFS(root string) error {
	// Check if filesystem supports RemoveAll directly
	if remover, ok := c.options.FS.(interface {
		RemoveAll(path string) error
	}); ok {
		return remover.RemoveAll(root)
	}

	// Fallback: Simple recursive delete using Walk in reverse order: files before dirs.
	var toDelete []string
	_ = c.options.FS.Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Ignore individual walk errors
		}
		toDelete = append(toDelete, path)
		return nil
	})
	// Delete deepest paths first
	for i := len(toDelete) - 1; i >= 0; i-- {
		_ = c.options.FS.Remove(toDelete[i]) // Ignore individual removal errors (best-effort)
	}
	return nil
}

// createTempDir creates a unique temporary directory using TempFS interface if available,
// otherwise uses os.MkdirTemp for automatic unique naming.
func (c *Client) createTempDir(pattern string) (string, error) {
	if tfs, ok := c.options.FS.(core.TempFS); ok {
		return tfs.TempDir("", pattern)
	}
	// Fallback: use os.MkdirTemp which handles uniqueness automatically
	return os.MkdirTemp("", pattern)
}
