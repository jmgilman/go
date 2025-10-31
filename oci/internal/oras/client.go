// Package oras provides ORAS wrapper functionality.
// This isolates the ORAS dependency in an internal package.
package oras

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// DefaultORASClient implements Client using the real ORAS library.
type DefaultORASClient struct{}

// Ensure DefaultORASClient implements Client.
// var _ Client = (*DefaultORASClient)(nil) // Verified in interfaces.go

// Push pushes an artifact to an OCI registry using the real ORAS library.
func (c *DefaultORASClient) Push(
	ctx context.Context,
	reference string,
	descriptor *PushDescriptor,
	opts *AuthOptions,
) error {
	return Push(ctx, reference, descriptor, opts)
}

// Pull pulls an artifact from an OCI registry using the real ORAS library.
func (c *DefaultORASClient) Pull(ctx context.Context, reference string, opts *AuthOptions) (*PullDescriptor, error) {
	return Pull(ctx, reference, opts)
}

// AuthConfig represents authentication configuration for ORAS operations.
// This matches the public AuthConfig struct for consistency.
type AuthConfig struct {
	Username string
	Password string
}

// CredentialFunc is an alias for ORAS's credential function type.
// It provides credentials for a given registry (host:port).
type CredentialFunc = auth.CredentialFunc

// HTTPConfig contains configuration for HTTP transport settings.
type HTTPConfig struct {
	// AllowHTTP enables HTTP instead of HTTPS for registry connections.
	AllowHTTP bool

	// AllowInsecure allows connections with self-signed certificates.
	AllowInsecure bool

	// Registries specifies which registries this applies to.
	// If empty, applies to all registries.
	Registries []string
}

// AuthOptions configures authentication and HTTP settings for ORAS operations.
type AuthOptions struct {
	// StaticAuth provides static credentials for a specific registry.
	// If set, this overrides the default Docker credential chain for that registry.
	StaticRegistry string
	StaticUsername string
	StaticPassword string

	// CredentialFunc provides a custom credential callback.
	// If set, this completely overrides the default credential chain.
	CredentialFunc CredentialFunc

	// HTTPConfig controls HTTP vs HTTPS and certificate validation.
	HTTPConfig *HTTPConfig

	// Transport provides a custom HTTP transport with connection pooling.
	// If nil, a default transport with connection pooling is used.
	Transport http.RoundTripper
}

// NewRepository creates a new ORAS repository with authentication configured.
// It sets up the default Docker credential chain and applies any auth overrides.
// Uses connection pooling for improved performance across multiple operations.
//
// Parameters:
//   - ctx: Context for the operation
//   - reference: Full OCI reference (e.g., "ghcr.io/org/repo:tag")
//   - opts: Authentication options (can be nil for default behavior)
//
// Returns:
//   - Configured remote repository ready for ORAS operations
//   - Error if repository creation fails
//
// Authentication behavior:
//  1. If CredentialFunc is provided, it takes complete precedence
//  2. If StaticAuth is provided, it overrides credentials for that specific registry
//  3. Otherwise, uses ORAS's default Docker credential chain (config + helpers)
//
// This isolates ORAS authentication logic and provides clean injection points
// for testing and programmatic credential management.
func NewRepository(ctx context.Context, reference string, opts *AuthOptions) (*remote.Repository, error) {
	// Parse the reference to obtain only the repository path (without tag/digest)
	repoPath, _, _ := splitReference(reference)
	if repoPath == "" {
		return nil, fmt.Errorf("invalid reference: %s", reference)
	}

	repo, err := remote.NewRepository(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Default: use ORAS's default Docker credential chain (config + helpers)
	authClient := auth.DefaultClient

	// Use optimized transport with connection pooling
	transport := newDefaultTransport(opts)

	// Apply HTTP configuration (scheme and TLS settings)
	if opts != nil && opts.HTTPConfig != nil && shouldApplyHTTPConfig(reference, opts.HTTPConfig) {
		// Use HTTP scheme if explicitly requested
		if opts.HTTPConfig.AllowHTTP {
			repo.PlainHTTP = true
		}
		// Note: TLS settings are already handled in newDefaultTransport
	}

	// Set the optimized transport
	if authClient.Client == nil {
		authClient.Client = &http.Client{Transport: transport}
	} else {
		authClient.Client.Transport = transport
	}

	// Apply auth overrides with caching if provided
	if opts != nil {
		switch {
		case opts.CredentialFunc != nil:
			// Custom credential function takes complete precedence
			// Wrap with caching for performance
			authClient.Credential = newCachedCredentialFunc(opts.CredentialFunc)
		case opts.StaticRegistry != "" && opts.StaticUsername != "":
			// Static auth override for specific registry with caching
			staticCred := auth.Credential{
				Username: opts.StaticUsername,
				Password: opts.StaticPassword,
			}
			authClient.Credential = newCachedCredentialFunc(
				auth.StaticCredential(opts.StaticRegistry, staticCred))
		default:
			// Use cached version of default credential chain
			authClient.Credential = newCachedCredentialFunc(authClient.Credential)
		}
	} else {
		// No auth options provided, still use caching for default credentials
		authClient.Credential = newCachedCredentialFunc(authClient.Credential)
	}

	repo.Client = authClient

	return repo, nil
}

func shouldApplyHTTPConfig(reference string, config *HTTPConfig) bool {
	// If no specific registries are configured, apply to all
	if len(config.Registries) == 0 {
		return true
	}

	// Parse the registry hostname from the reference (format: registry/path:tag)
	parts := strings.Split(reference, "/")
	if len(parts) == 0 {
		return false
	}

	registry := parts[0]

	// Check if the registry matches any configured registry
	for _, configuredRegistry := range config.Registries {
		if registry == configuredRegistry {
			return true
		}
		// Also check if it matches as a hostname (without port)
		if strings.Contains(configuredRegistry, ":") {
			// configuredRegistry has port, check exact match
			if registry == configuredRegistry {
				return true
			}
		} else {
			// configuredRegistry is hostname only, check hostname match
			if strings.HasPrefix(registry, configuredRegistry+":") {
				return true
			}
		}
	}

	return false
}

// PushDescriptor describes the content to be pushed to an OCI registry.
// It contains the media type and the data stream for the artifact.
type PushDescriptor struct {
	MediaType   string
	Data        io.Reader
	Size        int64
	Annotations map[string]string
	Platform    string
}

// pushStreamIfPossible attempts to stream-push the data when it is seekable.
// Returns handled=true when streaming was performed (with success or error to propagate).
// Returns handled=false to indicate the caller should fall back to buffered logic.
func pushStreamIfPossible(
	ctx context.Context,
	repo *remote.Repository,
	reference, refPart string,
	descriptor *PushDescriptor,
) (handled bool, err error) {
	rs, ok := descriptor.Data.(io.ReadSeeker)
	if !ok {
		return false, nil
	}

	// Determine size by seeking to end and back
	sz, szErr := rs.Seek(0, io.SeekEnd)
	if szErr != nil || sz < 0 {
		return false, nil
	}
	if _, back := rs.Seek(0, io.SeekStart); back != nil {
		return false, nil
	}

	// Compute digest by reading the stream (then rewind)
	d, dErr := digest.FromReader(rs)
	if dErr != nil {
		return false, nil
	}
	if _, back2 := rs.Seek(0, io.SeekStart); back2 != nil {
		return false, nil
	}

	expected := ocispec.Descriptor{
		MediaType: descriptor.MediaType,
		Digest:    d,
		Size:      sz,
	}

	// Push blob streaming from file. On push error, fall back to buffered.
	if pErr := repo.Blobs().Push(ctx, expected, io.LimitReader(rs, sz)); pErr != nil {
		return false, nil
	}

	// Pack manifest and tag
	packOpts := oras.PackManifestOptions{Layers: []ocispec.Descriptor{expected}}
	artifactType := "application/vnd.catalyst.bundle.v1"
	manDesc, mErr := oras.PackManifest(ctx, repo, oras.PackManifestVersion1_1, artifactType, packOpts)
	if mErr != nil {
		return true, mapORASError("push", reference, fmt.Errorf("pack manifest v1.1: %w", mErr))
	}
	if _, tErr := oras.Tag(ctx, repo, manDesc.Digest.String(), refPart); tErr != nil {
		return true, mapORASError("push", reference, fmt.Errorf("tag manifest: %w", tErr))
	}
	return true, nil
}

// Push pushes an artifact to an OCI registry using ORAS.
// Streaming from io.ReadSeeker when possible; falls back to buffered.
func Push(ctx context.Context, reference string, descriptor *PushDescriptor, opts *AuthOptions) error {
	if descriptor == nil {
		return fmt.Errorf("descriptor cannot be nil")
	}

	// Create the repository with authentication
	repo, err := NewRepository(ctx, reference, opts)
	if err != nil {
		return mapORASError("push", reference, fmt.Errorf("failed to create repository: %w", err))
	}

	// Extract tag or digest from reference
	_, refPart, _ := splitReference(reference)
	if refPart == "" {
		return mapORASError("push", reference, fmt.Errorf("reference must include a tag or digest"))
	}

	// Try streaming path first
	if handled, sErr := pushStreamIfPossible(ctx, repo, reference, refPart, descriptor); handled {
		return sErr
	}

	// Buffered fallback: read entire data into memory
	data, rErr := io.ReadAll(descriptor.Data)
	if rErr != nil {
		return mapORASError("push", reference, fmt.Errorf("failed to read data: %w", rErr))
	}
	if len(data) == 0 {
		return mapORASError("push", reference, fmt.Errorf("no data to push"))
	}

	// 1) Push the content blob
	blobDesc, bErr := oras.PushBytes(ctx, repo, descriptor.MediaType, data)
	if bErr != nil {
		return mapORASError("push", reference, fmt.Errorf("push blob: %w", bErr))
	}

	// 2) Pack an OCI 1.1 manifest with artifactType and empty config
	packOpts := oras.PackManifestOptions{Layers: []ocispec.Descriptor{blobDesc}}
	artifactType := "application/vnd.catalyst.bundle.v1"
	manDesc, pErr := oras.PackManifest(ctx, repo, oras.PackManifestVersion1_1, artifactType, packOpts)
	if pErr != nil {
		return mapORASError("push", reference, fmt.Errorf("pack manifest v1.1: %w", pErr))
	}

	// 3) Tag the manifest with the requested ref
	if _, tErr := oras.Tag(ctx, repo, manDesc.Digest.String(), refPart); tErr != nil {
		return mapORASError("push", reference, fmt.Errorf("tag manifest: %w", tErr))
	}

	return nil
}

// PullDescriptor describes the content pulled from an OCI registry.
// It contains metadata about the pulled artifact.
type PullDescriptor struct {
	MediaType string
	Data      io.ReadCloser
	Size      int64
	Digest    string // OCI digest of the blob (e.g., "sha256:abc123...")
}

// Pull pulls an artifact from an OCI registry using ORAS.
// It retrieves the content and returns it as a descriptor with a reader.
//
// Parameters:
//   - ctx: Context for the operation
//   - reference: Full OCI reference (e.g., "ghcr.io/org/repo:tag")
//   - opts: Authentication options (can be nil for default behavior)
//
// Returns the pulled descriptor and an error if the pull operation fails.
func Pull(ctx context.Context, reference string, opts *AuthOptions) (*PullDescriptor, error) {
	// Create the repository with authentication
	repo, err := NewRepository(ctx, reference, opts)
	if err != nil {
		return nil, mapORASError("pull", reference, fmt.Errorf("failed to create repository: %w", err))
	}

	// Extract tag or digest from reference
	_, refPart, _ := splitReference(reference)
	if refPart == "" {
		return nil, mapORASError("pull", reference, fmt.Errorf("reference must include a tag or digest"))
	}

	// Pull the target using ORAS Fetch function
	desc, reader, err := oras.Fetch(ctx, repo, refPart, oras.DefaultFetchOptions)
	if err != nil {
		return nil, mapORASError("pull", reference, err)
	}

	// If not a manifest, the fetched target is the content itself
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		return &PullDescriptor{
			MediaType: desc.MediaType,
			Data:      reader,
			Size:      desc.Size,
			Digest:    desc.Digest.String(),
		}, nil
	}

	// Handle image manifest by fetching first layer/blob
	manifestBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, mapORASError("pull", reference, fmt.Errorf("read manifest: %w", err))
	}
	reader.Close()

	var imgMan ocispec.Manifest
	if unmarshalErr := json.Unmarshal(manifestBytes, &imgMan); unmarshalErr != nil {
		return nil, mapORASError("pull", reference, fmt.Errorf("unrecognized manifest format"))
	}
	if len(imgMan.Layers) == 0 && imgMan.Config.MediaType == "" {
		return nil, mapORASError("pull", reference, fmt.Errorf("unrecognized manifest format"))
	}
	if len(imgMan.Layers) == 0 {
		return nil, mapORASError("pull", reference, fmt.Errorf("no layers in image manifest"))
	}
	layerDesc := imgMan.Layers[0]
	layerReader, err := repo.Blobs().Fetch(ctx, layerDesc)
	if err != nil {
		return nil, mapORASError("pull", reference, fmt.Errorf("fetch layer: %w", err))
	}
	return &PullDescriptor{
		MediaType: layerDesc.MediaType,
		Data:      layerReader,
		Size:      layerDesc.Size,
		Digest:    layerDesc.Digest.String(),
	}, nil
}

// splitReference splits a full OCI reference into repository path and reference part (tag or digest).
// Examples:
//
//	localhost:5000/myrepo:latest -> ("localhost:5000/myrepo", "latest", false)
//	ghcr.io/org/name@sha256:abcd -> ("ghcr.io/org/name", "sha256:abcd", true)
func splitReference(full string) (repoPath, refPart string, isDigest bool) {
	if full == "" {
		return "", "", false
	}
	// Find last slash to isolate the repo name tail
	lastSlash := strings.LastIndex(full, "/")
	if lastSlash == -1 {
		// No slash; cannot reliably parse; return as-is
		return full, "", false
	}
	head := full[:lastSlash]
	tail := full[lastSlash+1:]

	if at := strings.LastIndex(tail, "@"); at != -1 {
		// Digest form name@digest
		return head + "/" + tail[:at], tail[at+1:], true
	}
	if colon := strings.LastIndex(tail, ":"); colon != -1 {
		// Tag form name:tag (safe because we looked only in tail, avoiding port)
		return head + "/" + tail[:colon], tail[colon+1:], false
	}
	// No tag/digest found
	return full, "", false
}

// mapORASError maps ORAS errors to domain-specific errors.
// It analyzes the error type and returns appropriate domain errors.
//
// Parameters:
//   - op: The operation that failed ("push" or "pull")
//   - ref: The OCI reference being processed
//   - err: The original ORAS error
//
// Returns a domain error with proper context.
func mapORASError(op, ref string, err error) error {
	if err == nil {
		return nil
	}

	// Check for authentication errors
	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Check for network/registry unreachable errors
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return fmt.Errorf("registry unreachable: %w", err)
	}

	// For other errors, create a detailed error message
	return fmt.Errorf("%s %s: %w", op, ref, err)
}
