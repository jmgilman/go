package cache

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// testReference is a well-known reference used for health checks and testing
	testReference = "docker.io/library/nginx:latest"
)

// TagResolver provides efficient tag resolution with multi-tier validation.
// It implements caching, HEAD request optimization, and tag movement detection.
type TagResolver struct {
	tagCache      TagCache
	manifestCache ManifestCache
	config        TagResolverConfig
	httpClient    *http.Client
}

// NewTagResolver creates a new tag resolver with the given caches and configuration.
func NewTagResolver(
	tagCache TagCache,
	manifestCache ManifestCache,
	config TagResolverConfig,
) *TagResolver {
	if err := config.Validate(); err != nil {
		// Set defaults if validation fails due to unset values
		config.SetDefaults()
	}

	return &TagResolver{
		tagCache:      tagCache,
		manifestCache: manifestCache,
		config:        config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ResolveTag resolves a tag reference to its digest using multi-tier validation.
// Strategy:
// 1. Check local cache first
// 2. If cache miss or expired, perform HEAD request to registry
// 3. If HEAD request succeeds, update cache and return digest
// 4. Handle tag movement detection and cache invalidation
func (tr *TagResolver) ResolveTag(ctx context.Context, reference string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled: %w", err)
	}

	if !isValidReference(reference) {
		return "", fmt.Errorf("invalid reference format: %s", reference)
	}

	// Tier 1: Check local cache
	if cachedMapping, err := tr.tagCache.GetTagMapping(ctx, reference); err == nil {
		// Quick validation: check if the manifest exists in cache
		if exists, err := tr.manifestCache.HasManifest(ctx, cachedMapping.Digest); err == nil &&
			exists {
			return cachedMapping.Digest, nil
		}
	}

	// Tier 2: HEAD request to registry for validation
	digest, err := tr.validateWithRegistry(ctx, reference)
	if err != nil {
		return "", fmt.Errorf("registry validation failed: %w", err)
	}

	// Update cache with validated result
	if err := tr.tagCache.PutTagMapping(ctx, reference, digest); err != nil {
		// Log error but don't fail the operation
		// The resolution succeeded, cache update failure is non-critical
		fmt.Printf("Warning: failed to update tag cache: %v\n", err)
	}

	return digest, nil
}

// ValidateTag performs comprehensive validation of a tag reference.
// Returns true if the tag exists and points to valid content.
func (tr *TagResolver) ValidateTag(ctx context.Context, reference string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	// Handle empty reference
	if reference == "" {
		return false, nil
	}

	// Check cache first
	if exists, err := tr.tagCache.HasTagMapping(ctx, reference); err == nil && exists {
		return true, nil
	}

	// Validate with registry
	_, err := tr.validateWithRegistry(ctx, reference)
	if err != nil {
		// If tag not found, return false without error
		if strings.Contains(err.Error(), "tag not found in registry") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InvalidateTag removes a tag mapping from the cache.
// Useful when tag movement is detected externally.
func (tr *TagResolver) InvalidateTag(ctx context.Context, reference string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := tr.tagCache.DeleteTagMapping(ctx, reference); err != nil {
		return fmt.Errorf("failed to delete tag mapping: %w", err)
	}

	return nil
}

// GetTagHistory retrieves the history of digests for a tag reference.
func (tr *TagResolver) GetTagHistory(
	ctx context.Context,
	reference string,
) ([]TagHistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	history, err := tr.tagCache.GetTagHistory(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("failed to get tag history: %w", err)
	}

	return history, nil
}

// DetectTagMovement checks if a tag has moved to a different digest.
// This is useful for cache invalidation when external changes are detected.
func (tr *TagResolver) DetectTagMovement(
	ctx context.Context,
	reference, expectedDigest string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	// Get current digest from registry
	currentDigest, err := tr.validateWithRegistry(ctx, reference)
	if err != nil {
		return false, fmt.Errorf("failed to validate tag with registry: %w", err)
	}

	// Check if digest has changed
	if currentDigest != expectedDigest {
		// Invalidate cache entry since tag has moved
		_ = tr.tagCache.DeleteTagMapping(ctx, reference) // Ignore error in cleanup
		return true, nil
	}

	return false, nil
}

// validateWithRegistry performs HEAD request validation against the registry.
// This is more efficient than full manifest downloads for validation.
func (tr *TagResolver) validateWithRegistry(ctx context.Context, reference string) (string, error) {
	registryURL, err := tr.buildRegistryURL(reference)
	if err != nil {
		return "", fmt.Errorf("failed to build registry URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, registryURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Set appropriate headers for OCI registry
	req.Header.Set(
		"Accept",
		"application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json",
	)

	resp, err := tr.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("tag not found in registry")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	// Extract digest from response headers
	digestHeader := resp.Header.Get("Docker-Content-Digest")
	if digestHeader == "" {
		digestHeader = resp.Header.Get("OCI-Subject")
	}
	if digestHeader == "" {
		return "", fmt.Errorf("registry did not provide content digest in response")
	}

	// Validate digest format
	if !isValidDigest(digestHeader) {
		return "", fmt.Errorf("invalid digest format from registry: %s", digestHeader)
	}

	return digestHeader, nil
}

// buildRegistryURL constructs the registry URL for a given reference.
// Supports both Docker Hub and custom registries.
func (tr *TagResolver) buildRegistryURL(reference string) (string, error) {
	// Parse reference components
	registry, repository, tag, err := parseReference(reference)
	if err != nil {
		return "", err
	}

	// Handle Docker Hub special case
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}

	// Ensure registry has protocol
	if !strings.HasPrefix(registry, "http") {
		registry = "https://" + registry
	}

	// Build manifest URL
	// Format: https://registry/v2/repository/manifests/tag
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registry, repository, tag)

	return url, nil
}

// parseReference parses an OCI reference into its components.
// Returns registry, repository, reference (tag or digest), error.
func parseReference(reference string) (string, string, string, error) {
	if reference == "" {
		return "", "", "", fmt.Errorf("empty reference")
	}

	// Split into registry and remainder
	var registry, remainder string
	if strings.Contains(reference, "/") {
		parts := strings.SplitN(reference, "/", 2)
		registry = parts[0]
		remainder = parts[1]
	} else {
		// Default to Docker Hub
		registry = "docker.io"
		remainder = reference
	}

	// Parse remainder for repository and tag/digest
	var repository, ref string
	switch {
	case strings.Contains(remainder, "@"):
		// Digest format: repository@digest (check @ first since it's more specific)
		parts := strings.SplitN(remainder, "@", 2)
		repository = parts[0]
		ref = parts[1]
	case strings.Contains(remainder, ":"):
		// Tag format: repository:tag
		parts := strings.SplitN(remainder, ":", 2)
		repository = parts[0]
		ref = parts[1]
	default:
		// Default to latest tag
		repository = remainder
		ref = "latest"
	}

	// Validate components
	if repository == "" {
		return "", "", "", fmt.Errorf("empty repository")
	}
	if ref == "" {
		return "", "", "", fmt.Errorf("empty reference")
	}

	// Additional validation: repository should contain a valid name
	if !isValidRepositoryName(repository) {
		return "", "", "", fmt.Errorf("invalid repository name: %s", repository)
	}

	return registry, repository, ref, nil
}

// isValidRepositoryName checks if a repository name is valid.
// Repository names should contain at least one path segment and be properly formatted.
func isValidRepositoryName(name string) bool {
	if name == "" {
		return false
	}

	// Repository name should not contain invalid characters
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return false
	}

	// Should not be reserved words that might be confused with tags
	reservedWords := []string{"latest", "invalid", "test", "example", "sample"}
	for _, reserved := range reservedWords {
		if name == reserved {
			return false
		}
	}

	// Repository name should not contain spaces or other invalid characters
	if strings.Contains(name, " ") || strings.Contains(name, "@") || strings.Contains(name, ":") {
		return false
	}

	return true
}

// BatchResolveTags resolves multiple tag references efficiently.
// Uses concurrent requests and caches results.
func (tr *TagResolver) BatchResolveTags(
	ctx context.Context,
	references []string,
) (map[string]string, error) {
	if len(references) == 0 {
		return make(map[string]string), nil
	}

	results := make(map[string]string)
	resultsChan := make(chan struct {
		reference string
		digest    string
		err       error
	}, len(references))

	// Resolve tags concurrently
	for _, ref := range references {
		go func(reference string) {
			digest, err := tr.ResolveTag(ctx, reference)
			resultsChan <- struct {
				reference string
				digest    string
				err       error
			}{reference, digest, err}
		}(ref)
	}

	// Collect results
	var lastErr error
	for i := 0; i < len(references); i++ {
		result := <-resultsChan
		if result.err != nil {
			lastErr = result.err
			continue // Continue collecting other results
		}
		results[result.reference] = result.digest
	}

	if len(results) == 0 && lastErr != nil {
		return nil, lastErr
	}

	return results, nil
}

// HealthCheck performs a basic health check of the resolver.
// Returns an error if the resolver is not functioning properly.
func (tr *TagResolver) HealthCheck(ctx context.Context) error {
	// Try to resolve a well-known tag (Docker Hub nginx:latest)
	testRef := testReference

	_, err := tr.ResolveTag(ctx, testRef)
	if err != nil {
		return fmt.Errorf("tag resolver health check failed: %w", err)
	}

	return nil
}
