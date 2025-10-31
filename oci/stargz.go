// Package ocibundle provides OCI bundle distribution functionality.
// This file contains stargz-specific functionality for HTTP Range requests
// and selective file extraction from eStargz archives.
package ocibundle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/jmgilman/go/fs/core"
)

// testBlobRangeSupport checks if a registry blob URL supports HTTP Range requests.
// It sends a minimal Range request and checks for a 206 Partial Content response.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - httpClient: HTTP client to use for the request
//   - blobURL: Full URL to the blob (e.g., http://registry/v2/repo/blobs/sha256:...)
//
// Returns true if the registry supports Range requests, false otherwise.
// Errors are treated as "not supported" and return false.
func testBlobRangeSupport(ctx context.Context, httpClient *http.Client, blobURL string) bool {
	// Create request with Range header for first byte
	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")

	// Add timeout to prevent hanging
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req = req.WithContext(reqCtx)

	// Make request
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Check for 206 Partial Content response
	return resp.StatusCode == http.StatusPartialContent
}

// newHTTPRangeSeeker creates an HTTP Range request seeker for a blob URL.
// This allows random access to blob content via HTTP Range requests.
//
// Parameters:
//   - httpClient: HTTP client to use for requests
//   - blobURL: Full URL to the blob
//
// Returns an io.ReadSeekCloser that fetches data on-demand via Range requests.
func newHTTPRangeSeeker(httpClient *http.Client, blobURL string) io.ReadSeekCloser {
	return transport.NewHTTPReadSeeker(httpClient, blobURL, nil)
}

// buildBlobURL constructs the OCI blob URL for a given registry, repository, and digest.
// Format: {registryURL}/v2/{repository}/blobs/{digest}
//
// Parameters:
//   - registryURL: Base registry URL (e.g., "http://localhost:5000")
//   - repository: Repository name (e.g., "myorg/myrepo")
//   - digest: Content digest (e.g., "sha256:abc123...")
//
// Returns the complete blob URL.
func buildBlobURL(registryURL, repository, digest string) string {
	// Ensure registryURL doesn't end with /
	registryURL = strings.TrimRight(registryURL, "/")

	// Ensure repository doesn't start with /
	repository = strings.TrimLeft(repository, "/")

	// Ensure digest doesn't start with /
	digest = strings.TrimLeft(digest, "/")

	return fmt.Sprintf("%s/v2/%s/blobs/%s", registryURL, repository, digest)
}

// parseReference splits an OCI reference into registry URL and repository path.
// Handles formats like:
//   - localhost:5000/repo:tag → ("http://localhost:5000", "repo")
//   - ghcr.io/org/repo:tag → ("https://ghcr.io", "org/repo")
//   - registry.example.com/path/to/repo@sha256:abc → ("https://registry.example.com", "path/to/repo")
//
// Parameters:
//   - reference: Full OCI reference
//   - allowHTTP: Whether to use HTTP for localhost registries
//
// Returns registry URL and repository path.
func parseReference(reference string, allowHTTP bool) (registryURL, repository string) {
	// Remove tag or digest from reference
	refWithoutTag := reference
	if idx := strings.LastIndex(reference, ":"); idx != -1 {
		// Check if this is a port or a tag
		after := reference[idx+1:]
		if !strings.Contains(after, "/") {
			// It's a tag or digest, remove it
			refWithoutTag = reference[:idx]
		}
	}
	if idx := strings.LastIndex(refWithoutTag, "@"); idx != -1 {
		refWithoutTag = refWithoutTag[:idx]
	}

	// Split into registry and repository
	parts := strings.SplitN(refWithoutTag, "/", 2)
	if len(parts) == 1 {
		// No slash, assume Docker Hub (but we shouldn't get here in practice)
		return "https://index.docker.io", parts[0]
	}

	registryHost := parts[0]
	repository = parts[1]

	// Determine protocol
	protocol := "https"
	if allowHTTP || strings.HasPrefix(registryHost, "localhost") || strings.Contains(registryHost, "localhost:") {
		protocol = "http"
	}

	registryURL = fmt.Sprintf("%s://%s", protocol, registryHost)
	return registryURL, repository
}

// readerAtFromSeeker adapts an io.ReadSeeker to io.ReaderAt interface.
// This is thread-safe and serializes ReadAt calls with a mutex since
// each ReadAt performs a Seek followed by a Read which could conflict
// if called concurrently.
type readerAtFromSeeker struct {
	mu     sync.Mutex
	seeker io.ReadSeeker
	size   int64
}

// newReaderAtFromSeeker creates a new ReaderAt adapter from a ReadSeeker.
//
// Parameters:
//   - seeker: The ReadSeeker to adapt (e.g., HTTP Range seeker)
//   - size: The total size of the content
//
// Returns a thread-safe ReaderAt implementation.
func newReaderAtFromSeeker(seeker io.ReadSeeker, size int64) *readerAtFromSeeker {
	return &readerAtFromSeeker{
		seeker: seeker,
		size:   size,
	}
}

// ReadAt implements io.ReaderAt by seeking to the offset and reading.
// This method is thread-safe via mutex serialization.
func (r *readerAtFromSeeker) ReadAt(p []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.seeker.Seek(offset, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("seek to offset %d failed: %w", offset, err)
	}

	return io.ReadFull(r.seeker, p)
}

// Size returns the total size of the content.
func (r *readerAtFromSeeker) Size() int64 {
	return r.size
}

// extractSelectiveFromStargz extracts files matching patterns from an eStargz archive
// using an io.ReaderAt (typically from HTTP Range requests for bandwidth savings).
//
// Parameters:
//   - ctx: Context for cancellation
//   - readerAt: ReaderAt for the archive (e.g., HTTP Range seeker)
//   - size: Total size of the archive
//   - targetDir: Directory to extract files to
//   - patterns: Glob patterns to match files (empty = extract all)
//   - opts: Extraction options including security validators
//   - fsys: Filesystem to write to
//
// Returns error if extraction fails.
func extractSelectiveFromStargz(
	ctx context.Context,
	readerAt io.ReaderAt,
	size int64,
	targetDir string,
	patterns []string,
	opts ExtractOptions,
	fsys core.FS,
) error {
	// Create a SectionReader from the ReaderAt (required by estargz)
	sectionReader := io.NewSectionReader(readerAt, 0, size)

	// Open the estargz archive
	stargzReader, err := estargz.Open(sectionReader)
	if err != nil {
		return fmt.Errorf("failed to open estargz archive: %w", err)
	}

	// Create validators
	validators := NewValidatorChain(
		NewSizeValidator(opts.MaxFileSize, opts.MaxSize),
		NewFileCountValidator(opts.MaxFiles),
		NewPermissionSanitizer(),
	)

	// Track statistics
	var totalSize int64
	var fileCount int

	// Collect all matching files first (estargz stores entries with full paths)
	var filesToExtract []string

	// Get root entry to walk the TOC
	rootEntry, ok := stargzReader.Lookup("")
	if !ok {
		return fmt.Errorf("failed to lookup root entry in stargz TOC")
	}

	// Walk all entries in the TOC
	var collectEntries func(entry *estargz.TOCEntry) error
	collectEntries = func(entry *estargz.TOCEntry) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("extraction canceled: %w", ctx.Err())
		default:
		}

		entryName := entry.Name

		// Skip root entry itself
		if entryName == "" {
			// Process children
			entry.ForeachChild(func(_ string, childEntry *estargz.TOCEntry) bool {
				if err := collectEntries(childEntry); err != nil {
					return false
				}
				return true
			})
			return nil
		}

		// Skip if it doesn't match patterns (but always include directories for structure)
		if entry.Type != "dir" && len(patterns) > 0 {
			if !matchesAnyPattern(entryName, patterns) {
				// Still need to process children in case they match
				entry.ForeachChild(func(_ string, childEntry *estargz.TOCEntry) bool {
					if err := collectEntries(childEntry); err != nil {
						return false
					}
					return true
				})
				return nil
			}
		}

		// Add to extraction list
		filesToExtract = append(filesToExtract, entryName)

		// Process children
		entry.ForeachChild(func(_ string, childEntry *estargz.TOCEntry) bool {
			if err := collectEntries(childEntry); err != nil {
				return false
			}
			return true
		})

		return nil
	}

	// Collect all entries
	if err := collectEntries(rootEntry); err != nil {
		return err
	}

	// Now extract the collected files
	for _, entryName := range filesToExtract {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("extraction canceled: %w", ctx.Err())
		default:
		}

		// Lookup the entry
		entry, ok := stargzReader.Lookup(entryName)
		if !ok {
			continue // Entry not found, skip
		}

		// Increment file count
		fileCount++

		// Validate against security constraints
		fileInfo := FileInfo{
			Name: entryName,
			Size: entry.Size,
			Mode: uint32(entry.Mode),
		}

		if err := validators.ValidateFile(fileInfo); err != nil {
			return fmt.Errorf("validation failed for %s: %w", entryName, err)
		}

		totalSize += entry.Size
		archiveStats := ArchiveStats{
			TotalFiles: fileCount,
			TotalSize:  totalSize,
		}
		if err := validators.ValidateArchive(archiveStats); err != nil {
			return fmt.Errorf("archive validation failed: %w", err)
		}

		// Create target path
		targetPath := filepath.Join(targetDir, entryName)

		// Handle based on entry type
		switch entry.Type {
		case "dir":
			// Create directory
			if err := fsys.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case "reg":
			// Extract regular file
			if err := fsys.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", targetPath, err)
			}

			// Open the file from stargz (uses Range requests)
			sr, err := stargzReader.OpenFile(entryName)
			if err != nil {
				return fmt.Errorf("failed to open file %s from stargz: %w", entryName, err)
			}

			// Create target file
			targetFile, err := fsys.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}
			defer targetFile.Close()

			// Copy content (Range requests happen here)
			if _, err := io.Copy(targetFile, sr); err != nil {
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}

		case "symlink":
			// Handle symlinks if filesystem supports them
			if sfs, ok := fsys.(core.SymlinkFS); ok {
				if err := sfs.Symlink(entry.LinkName, targetPath); err != nil {
					return fmt.Errorf("failed to create symlink %s: %w", targetPath, err)
				}
			}
			// If symlinks not supported, skip silently
		}
	}

	return nil
}
