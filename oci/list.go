// Package ocibundle provides OCI bundle distribution functionality.
// This file contains functionality for listing files in OCI artifacts
// without downloading the full archive, using eStargz TOC.
package ocibundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"

	"github.com/jmgilman/go/oci/internal/cache"
	orasint "github.com/jmgilman/go/oci/internal/oras"
)

// FileMetadata represents metadata about a file in an OCI artifact.
// This information is extracted from the eStargz Table of Contents (TOC)
// without downloading the actual file data.
type FileMetadata struct {
	// Name is the full path of the file within the archive
	Name string

	// Size is the logical size of the file in bytes (0 for directories)
	Size int64

	// Mode is the file permission bits (e.g., 0644, 0755)
	Mode os.FileMode

	// ModTime is the file modification time
	ModTime time.Time

	// IsDir indicates whether this is a directory
	IsDir bool

	// LinkTarget is the target path for symbolic links (empty for regular files)
	LinkTarget string

	// Type is the entry type: "reg", "dir", "symlink", "hardlink", etc.
	Type string

	// Digest is the OCI digest for regular files (e.g., "sha256:abc123...")
	Digest string
}

// ListFilesResult contains the results of listing files in an OCI artifact.
type ListFilesResult struct {
	// Files is the list of all files in the artifact
	Files []FileMetadata

	// TotalSize is the total uncompressed size of all files
	TotalSize int64

	// FileCount is the total number of files (excluding directories)
	FileCount int

	// DirCount is the total number of directories
	DirCount int
}

// ListFiles lists all files in an OCI artifact without downloading the full archive.
// Uses HTTP Range requests to download only the TOC, saving significant bandwidth.
func (c *Client) ListFiles(ctx context.Context, reference string) (*ListFilesResult, error) {
	// Thread safety: use read lock since we're only reading
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Validate input
	if reference == "" {
		return nil, fmt.Errorf("reference cannot be empty")
	}

	// Create authenticated repository
	repo, err := orasint.NewRepository(ctx, reference, c.options.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Extract tag or digest from reference
	_, refPart, _ := splitReference(reference)
	if refPart == "" {
		return nil, fmt.Errorf("reference must include a tag or digest")
	}

	// Fetch the manifest
	manifestDesc, reader, err := oras.Fetch(ctx, repo, refPart, oras.DefaultFetchOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer reader.Close()

	// Handle manifest to get the layer descriptor (which contains the actual blob)
	var layerDesc ocispec.Descriptor
	if manifestDesc.MediaType == ocispec.MediaTypeImageManifest {
		// Read and parse the manifest
		manifestBytes, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read manifest: %w", err)
		}

		var manifest ocispec.Manifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}

		if len(manifest.Layers) == 0 {
			return nil, fmt.Errorf("no layers in manifest")
		}

		// Use the first layer (our blob)
		layerDesc = manifest.Layers[0]
	} else {
		// Direct blob reference
		layerDesc = manifestDesc
	}

	// Check TOC cache first for instant results (Phase 2 optimization)
	// This avoids all network operations when TOC is cached
	var tocEntries []*estargz.TOCEntry
	digest := layerDesc.Digest.String()
	cachedTOC, tocErr := c.getTOCFromCache(ctx, digest)
	if tocErr == nil && cachedTOC != nil {
		// Cache hit! Deserialize TOC and return immediately (0 network bytes)
		if err := json.Unmarshal(cachedTOC.TOCData, &tocEntries); err == nil {
			result := convertTOCToListResult(tocEntries)
			return result, nil
		}
		// Deserialization failed - continue to fetch fresh TOC
	}

	// Try HTTP Range approach first for bandwidth optimization
	// This downloads only ~100KB (footer + TOC) instead of the full blob
	blobURL, httpClient, urlErr := getBlobURLFromRepository(repo, layerDesc.Digest.String())
	if urlErr == nil {
		// Try extracting TOC via HTTP Range requests
		tocEntries, err = extractTOCFromBlob(ctx, httpClient, blobURL, layerDesc.Size)
		if err == nil {
			// Success! We got the TOC with minimal bandwidth
			// Cache the TOC for next time (Phase 2 optimization)
			c.cacheTOC(ctx, digest, tocEntries)

			// Convert TOC entries to file metadata and return
			result := convertTOCToListResult(tocEntries)
			return result, nil
		}
		// HTTP Range extraction failed - will fall through to full download
	}

	// Fallback: Download the full blob to parse the TOC
	// This happens when:
	// - Registry doesn't support HTTP Range requests
	// - HTTP Range extraction failed for any reason
	// - Unable to get blob URL or HTTP client
	var layerReader io.ReadCloser
	layerReader, err = repo.Blobs().Fetch(ctx, layerDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch layer: %w", err)
	}
	defer layerReader.Close()

	// Read the full blob into memory
	blobData, err := io.ReadAll(layerReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}

	// Parse the TOC from the blob using the existing function
	tocEntries, err = parseTOCFromBytes(blobData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TOC: %w", err)
	}

	// Cache the TOC for next time (Phase 2 optimization)
	c.cacheTOC(ctx, digest, tocEntries)

	// Convert TOC entries to file metadata
	result := convertTOCToListResult(tocEntries)

	return result, nil
}

// getTOCFromCache retrieves a cached TOC if available.
// Returns nil if cache is not configured or TOC is not cached.
func (c *Client) getTOCFromCache(ctx context.Context, digest string) (*cache.TOCCacheEntry, error) {
	if c.cache == nil {
		return nil, fmt.Errorf("cache not configured")
	}

	coordinator, ok := c.cache.(*cache.Coordinator)
	if !ok {
		return nil, fmt.Errorf("cache is not a coordinator")
	}

	return coordinator.GetTOC(ctx, digest)
}

// cacheTOC stores a TOC in the cache for future ListFiles operations.
// Silently fails if cache is not configured.
func (c *Client) cacheTOC(ctx context.Context, digest string, tocEntries []*estargz.TOCEntry) {
	if c.cache == nil {
		return // Cache not configured
	}

	coordinator, ok := c.cache.(*cache.Coordinator)
	if !ok {
		return // Not a coordinator
	}

	// Serialize TOC entries to JSON
	tocData, err := json.Marshal(tocEntries)
	if err != nil {
		return // Serialization failed, silently skip caching
	}

	// Calculate statistics
	var fileCount int
	var totalSize int64
	for _, entry := range tocEntries {
		if entry.Type != "dir" && entry.Type != "chunk" {
			fileCount++
			totalSize += entry.Size
		}
	}

	// Create TOC cache entry
	tocEntry := &cache.TOCCacheEntry{
		Digest:     digest,
		TOCData:    tocData,
		FileCount:  fileCount,
		TotalSize:  totalSize,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
		TTL:        24 * time.Hour, // TOCs don't change, can cache for 24 hours
	}

	// Store in cache (errors ignored - caching is best-effort)
	coordinator.PutTOC(ctx, tocEntry)
}

// parseTOCFromBytes parses the eStargz TOC from raw bytes.
// The TOC is stored as a compressed tar entry containing JSON.
func parseTOCFromBytes(tocBytes []byte) ([]*estargz.TOCEntry, error) {
	// Create a SectionReader from the bytes
	reader := bytes.NewReader(tocBytes)
	sectionReader := io.NewSectionReader(reader, 0, int64(len(tocBytes)))

	// Open the eStargz reader
	stargzReader, err := estargz.Open(sectionReader)
	if err != nil {
		return nil, fmt.Errorf("failed to open estargz reader: %w", err)
	}

	// Get the root entry and collect all entries
	rootEntry, ok := stargzReader.Lookup("")
	if !ok {
		return nil, fmt.Errorf("failed to lookup root entry in TOC")
	}

	// Collect all TOC entries
	var entries []*estargz.TOCEntry
	collectTOCEntries(rootEntry, &entries)

	return entries, nil
}

// collectTOCEntries recursively collects all TOC entries from the tree structure.
func collectTOCEntries(entry *estargz.TOCEntry, entries *[]*estargz.TOCEntry) {
	if entry.Name != "" {
		*entries = append(*entries, entry)
	}

	entry.ForeachChild(func(_ string, child *estargz.TOCEntry) bool {
		collectTOCEntries(child, entries)
		return true
	})
}

// convertTOCToListResult converts eStargz TOC entries to FileMetadata and calculates statistics.
func convertTOCToListResult(tocEntries []*estargz.TOCEntry) *ListFilesResult {
	result := &ListFilesResult{
		Files: make([]FileMetadata, 0, len(tocEntries)),
	}

	for _, entry := range tocEntries {
		// Skip chunk entries (they're part of large files, not separate files)
		if entry.Type == "chunk" {
			continue
		}

		// Skip eStargz metadata files
		if entry.Name == ".no.prefetch.landmark" || entry.Name == "stargz.index.json" {
			continue
		}

		isDir := entry.Type == "dir"

		metadata := FileMetadata{
			Name:       entry.Name,
			Size:       entry.Size,
			Mode:       os.FileMode(entry.Mode),
			ModTime:    entry.ModTime(),
			IsDir:      isDir,
			LinkTarget: entry.LinkName,
			Type:       entry.Type,
			Digest:     entry.Digest,
		}

		result.Files = append(result.Files, metadata)

		// Update statistics
		if isDir {
			result.DirCount++
		} else {
			result.FileCount++
			result.TotalSize += entry.Size
		}
	}

	return result
}

// splitReference splits a full OCI reference into repository path and reference part (tag or digest).
// Examples:
//   - localhost:5000/myrepo:latest -> ("localhost:5000/myrepo", "latest", false)
//   - ghcr.io/org/name@sha256:abcd -> ("ghcr.io/org/name", "sha256:abcd", true)
func splitReference(full string) (repoPath, refPart string, isDigest bool) {
	if full == "" {
		return "", "", false
	}
	// Find last slash to isolate the repo name tail
	lastSlash := strings.LastIndex(full, "/")
	if lastSlash == -1 {
		return full, "", false
	}
	head := full[:lastSlash]
	tail := full[lastSlash+1:]

	// Check for digest form (name@digest)
	if at := strings.Index(tail, "@"); at != -1 {
		return head + "/" + tail[:at], tail[at+1:], true
	}

	// Check for tag form (name:tag) - look for colon in the tail only
	if colon := strings.Index(tail, ":"); colon != -1 {
		return head + "/" + tail[:colon], tail[colon+1:], false
	}

	// No tag/digest found
	return full, "", false
}

// ListFilesWithFilter lists files matching the specified glob patterns.
func (c *Client) ListFilesWithFilter(ctx context.Context, reference string, patterns ...string) (*ListFilesResult, error) {
	// Get all files
	allFiles, err := c.ListFiles(ctx, reference)
	if err != nil {
		return nil, err
	}

	// If no patterns specified, return all files
	if len(patterns) == 0 {
		return allFiles, nil
	}

	// Filter files by patterns
	filtered := &ListFilesResult{
		Files: make([]FileMetadata, 0),
	}

	for _, file := range allFiles.Files {
		// Skip directories - only filter regular files
		if file.IsDir {
			continue
		}

		// Check if file matches any pattern
		if matchesAnyPattern(file.Name, patterns) {
			filtered.Files = append(filtered.Files, file)
			filtered.FileCount++
			filtered.TotalSize += file.Size
		}
	}

	return filtered, nil
}
