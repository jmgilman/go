package ocibundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// applyPushOptions applies functional options to default push options.
func applyPushOptions(opts []PushOption) *PushOptions {
	pushOpts := DefaultPushOptions()
	for _, opt := range opts {
		opt(pushOpts)
	}
	return pushOpts
}

// applyPullOptions applies functional options to default pull options.
func applyPullOptions(opts []PullOption) *PullOptions {
	pullOpts := DefaultPullOptions()
	for _, opt := range opts {
		opt(pullOpts)
	}
	return pullOpts
}

// validatePushInputs validates inputs for push operations.
func validatePushInputs(fsys core.FS, sourceDir, reference string) error {
	if sourceDir == "" {
		return fmt.Errorf("source directory cannot be empty")
	}
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}
	if exists, err := fsys.Exists(sourceDir); err != nil {
		return fmt.Errorf("failed to check source directory: %w", err)
	} else if !exists {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}
	return nil
}

// validatePullInputs validates inputs for pull operations.
func validatePullInputs(fsys core.FS, reference, targetDir string) error {
	if reference == "" {
		return fmt.Errorf("reference cannot be empty")
	}
	if targetDir == "" {
		return fmt.Errorf("target directory cannot be empty")
	}
	if exists, err := fsys.Exists(targetDir); err != nil {
		return fmt.Errorf("failed to check target directory: %w", err)
	} else if exists {
		entries, readErr := fsys.ReadDir(targetDir)
		if readErr != nil {
			return fmt.Errorf("failed to read target directory: %w", readErr)
		}
		if len(entries) > 0 {
			return fmt.Errorf("target directory is not empty: %s", targetDir)
		}
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
			backoffDelay := delay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoffDelay):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryableError(err) {
			break
		}
	}

	return lastErr
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for other retryable network errors by examining the error string
	// (netErr.Temporary() is deprecated since Go 1.18)

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

	pushOpts := applyPushOptions(opts)

	if err := validatePushInputs(c.options.FS, sourceDir, reference); err != nil {
		return err
	}

	_, repoErr := c.createRepository(ctx, reference)
	if repoErr != nil {
		return repoErr
	}

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

	archiver := NewTarGzArchiverWithFS(c.options.FS)

	var archiveErr error
	if pushOpts.ProgressCallback != nil {
		archiveErr = archiver.ArchiveWithProgress(ctx, sourceDir, tempFile, pushOpts.ProgressCallback)
	} else {
		archiveErr = archiver.Archive(ctx, sourceDir, tempFile)
	}
	if archiveErr != nil {
		return fmt.Errorf("failed to archive directory: %w", archiveErr)
	}

	stat, statErr := tempFile.Stat()
	if statErr != nil {
		return fmt.Errorf("failed to get file size: %w", statErr)
	}

	pushErr := retryOperation(ctx, pushOpts.MaxRetries, pushOpts.RetryDelay, func() error {
		if seeker, ok := tempFile.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				return fmt.Errorf("failed to seek temporary file: %w", seekErr)
			}
		}
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

	cleanupNeeded = false
	return nil
}

// Pull downloads and extracts an OCI artifact to the specified directory.
// Supports selective extraction using glob patterns and enforces security validation.
func (c *Client) Pull(ctx context.Context, reference, targetDir string, opts ...PullOption) error {
	// Thread safety: use read lock since we're only reading options
	c.mu.RLock()
	defer c.mu.RUnlock()

	pullOpts := applyPullOptions(opts)

	if err := validatePullInputs(c.options.FS, reference, targetDir); err != nil {
		return err
	}

	repo, repoErr := c.createRepository(ctx, reference)
	if repoErr != nil {
		return repoErr
	}

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

	defer descriptor.Data.Close()

	extractOpts := ExtractOptions{
		MaxFiles:         pullOpts.MaxFiles,
		MaxSize:          pullOpts.MaxSize,
		MaxFileSize:      pullOpts.MaxFileSize,
		AllowHiddenFiles: pullOpts.AllowHiddenFiles,
		StripPrefix:      pullOpts.StripPrefix,
		PreservePerms:    pullOpts.PreservePermissions,
		FilesToExtract:   pullOpts.FilesToExtract,
	}

	if len(pullOpts.FilesToExtract) > 0 {
		return c.extractSelective(ctx, repo, descriptor, targetDir, pullOpts, extractOpts)
	}

	archiver := NewTarGzArchiverWithFS(c.options.FS)
	if err := c.extractAtomically(ctx, archiver, descriptor.Data, targetDir, extractOpts); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}
	return nil
}

// extractSelective handles selective file extraction from OCI artifacts.
func (c *Client) extractSelective(ctx context.Context, repo *remote.Repository, descriptor *orasint.PullDescriptor, targetDir string, pullOpts *PullOptions, extractOpts ExtractOptions) error {
	readerAt, blobSize, err := getBlobReaderAt(ctx, repo, descriptor.Digest, descriptor.Data, descriptor.Size)
	if err != nil {
		return err
	}

	tempDir, tmpErr := c.createTempDir("ocibundle-selective-")
	if tmpErr != nil {
		return fmt.Errorf("failed to create temporary directory: %w", tmpErr)
	}
	defer func() { _ = c.removeAllFS(tempDir) }()

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

	if err := c.options.FS.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	if err := c.moveFiles(tempDir, targetDir); err != nil {
		_ = c.removeAllFS(targetDir)
		return fmt.Errorf("failed to move extracted files: %w", err)
	}

	return nil
}

// PullWithCache downloads and extracts an OCI artifact with caching support.
func (c *Client) PullWithCache(ctx context.Context, reference, targetDir string, opts ...PullOption) error {
	// Thread safety: use read lock since we're only reading options
	c.mu.RLock()
	defer c.mu.RUnlock()

	pullOpts := applyPullOptions(opts)

	if err := validatePullInputs(c.options.FS, reference, targetDir); err != nil {
		return err
	}

	cacheEnabled := c.isCachingEnabledForPull(pullOpts)

	if !cacheEnabled {
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	if err := c.ensureCacheInitialized(ctx); err != nil {
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	digest, err := c.resolveTagWithCache(ctx, reference)
	if err != nil {
		return c.Pull(ctx, reference, targetDir, opts...)
	}

	cacheKey := c.generateCacheKey(digest)

	if err := c.getFromCache(ctx, cacheKey, targetDir); err == nil {
		return nil
	}

	if err := c.Pull(ctx, reference, targetDir, opts...); err != nil {
		return err
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
	_ = coordinator.PutTagMapping(ctx, reference, digest) // Ignore caching errors - resolution succeeded

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
	defer func() { _ = descriptor.Data.Close() }()

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
	defer func() { _ = blobReader.Close() }()

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
		if err := remover.RemoveAll(root); err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", root, err)
		}
		return nil
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
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	return dir, nil
}
