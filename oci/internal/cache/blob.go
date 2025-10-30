package cache

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// blobCacheImpl implements the BlobCache interface with deduplication and TTL support.
// It uses the underlying Storage layer for atomic filesystem operations and maintains
// reference counting to enable safe deduplication of blob data.
type blobCacheImpl struct {
	storage       *Storage
	blobDir       string
	refsDir       string
	defaultTTL    time.Duration
	refCountMutex sync.RWMutex
}

// NewBlobCache creates a new blob cache with the given storage backend.
// The defaultTTL specifies how long blobs should be cached (24 hours default).
//
//nolint:ireturn // returning interface allows for better testability and dependency injection
func NewBlobCache(storage *Storage, defaultTTL time.Duration) (BlobCache, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour // Default to 24 hours
	}

	blobDir := "blobs"
	refsDir := "refs"

	// Ensure directories exist
	if err := storage.fs.MkdirAll(filepath.Join(storage.rootPath, blobDir), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create blob directory: %w", err)
	}
	if err := storage.fs.MkdirAll(filepath.Join(storage.rootPath, refsDir), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create refs directory: %w", err)
	}

	return &blobCacheImpl{
		storage:    storage,
		blobDir:    blobDir,
		refsDir:    refsDir,
		defaultTTL: defaultTTL,
	}, nil
}

// GetBlob retrieves a blob by its digest.
// Returns a ReadCloser that the caller must close.
// Implements streaming reads to handle large blobs efficiently.
func (bc *blobCacheImpl) GetBlob(ctx context.Context, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if digest == "" {
		return nil, fmt.Errorf("digest cannot be empty")
	}

	// Validate digest format (should be sha256:<hash>)
	if !isValidDigest(digest) {
		return nil, fmt.Errorf("invalid digest format: %s", digest)
	}

	// Check if blob exists and is not expired
	exists, err := bc.HasBlob(ctx, digest)
	if err != nil {
		return nil, fmt.Errorf("failed to check blob existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("blob not found: %s", digest)
	}

	// Get the blob path
	blobPath, err := bc.getBlobPath(digest)
	if err != nil {
		return nil, fmt.Errorf("failed to get blob path: %w", err)
	}

	// Read the blob data with integrity checking
	blobData, err := bc.storage.ReadWithIntegrity(ctx, blobPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob with integrity: %w", err)
	}

	// Create a reader from the verified data
	return &verifiedBlobReader{
		data:   blobData,
		offset: 0,
	}, nil
}

// PutBlob stores a blob with the given digest.
// The reader content is consumed and stored with deduplication.
// Uses streaming operations to handle large blobs efficiently.
func (bc *blobCacheImpl) PutBlob(ctx context.Context, digest string, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if digest == "" {
		return fmt.Errorf("digest cannot be empty")
	}

	if reader == nil {
		return fmt.Errorf("reader cannot be nil")
	}

	// Validate digest format
	if !isValidDigest(digest) {
		return fmt.Errorf("invalid digest format: %s", digest)
	}

	// Check if blob already exists
	exists, err := bc.HasBlob(ctx, digest)
	if err != nil {
		return fmt.Errorf("failed to check blob existence: %w", err)
	}
	if exists {
		// Blob already exists, just update the reference
		return bc.addReference(ctx, digest)
	}

	// Create blob path
	blobPath, err := bc.getBlobPath(digest)
	if err != nil {
		return fmt.Errorf("failed to get blob path: %w", err)
	}

	// Use StreamWriter for atomic writes with built-in integrity checking
	streamWriter, err := bc.storage.NewStreamWriter(ctx, blobPath)
	if err != nil {
		return fmt.Errorf("failed to create stream writer: %w", err)
	}

	// Copy data to the stream writer
	_, err = io.Copy(streamWriter, reader)
	if err != nil {
		streamWriter.Close() // Cleanup on error
		return fmt.Errorf("failed to write blob data: %w", err)
	}

	// Close the stream writer (this performs the atomic rename and integrity verification)
	if err := streamWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize blob write: %w", err)
	}

	// Add reference for this digest
	return bc.addReference(ctx, digest)
}

// HasBlob checks if a blob exists in the cache and is not expired.
func (bc *blobCacheImpl) HasBlob(ctx context.Context, digest string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	if digest == "" {
		return false, fmt.Errorf("digest cannot be empty")
	}

	// Check if reference exists and is not expired
	refPath, err := bc.getRefPath(digest)
	if err != nil {
		return false, fmt.Errorf("failed to get ref path: %w", err)
	}

	exists, err := bc.storage.Exists(ctx, refPath)
	if err != nil {
		return false, fmt.Errorf("failed to check reference existence: %w", err)
	}
	if !exists {
		return false, nil
	}

	// Read the reference to check TTL
	refData, err := bc.storage.ReadWithIntegrity(ctx, refPath)
	if err != nil {
		return false, fmt.Errorf("failed to read reference: %w", err)
	}

	ref, err := parseBlobRef(refData)
	if err != nil {
		return false, fmt.Errorf("failed to parse reference: %w", err)
	}

	// Check if reference has expired
	if time.Since(ref.CreatedAt) > ref.TTL {
		// Reference expired, clean it up
		if removeErr := bc.storage.Remove(ctx, refPath); removeErr != nil {
			return false, fmt.Errorf("failed to remove expired reference: %w", removeErr)
		}
		return false, nil
	}

	// Check if the actual blob file exists
	blobPath, err := bc.getBlobPath(digest)
	if err != nil {
		return false, fmt.Errorf("failed to get blob path: %w", err)
	}

	return bc.storage.Exists(ctx, blobPath)
}

// DeleteBlob removes a blob from the cache.
// Uses reference counting to only delete the blob data when no references remain.
func (bc *blobCacheImpl) DeleteBlob(ctx context.Context, digest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if digest == "" {
		return fmt.Errorf("digest cannot be empty")
	}

	// Remove reference
	return bc.removeReference(ctx, digest)
}

// getBlobPath returns the filesystem path for a blob digest.
// Uses a sharded directory structure to avoid too many files in a single directory.
func (bc *blobCacheImpl) getBlobPath(digest string) (string, error) {
	hash := extractHashFromDigest(digest)
	if len(hash) < 4 {
		return "", fmt.Errorf("hash too short: %s", hash)
	}

	// Create sharded directory structure (first 2 chars as directory)
	shardDir := hash[:2]
	return filepath.Join(bc.blobDir, shardDir, hash), nil
}

// getRefPath returns the filesystem path for a blob reference.
func (bc *blobCacheImpl) getRefPath(digest string) (string, error) {
	hash := extractHashFromDigest(digest)
	if len(hash) < 4 {
		return "", fmt.Errorf("hash too short: %s", hash)
	}

	// Create sharded directory structure for references too
	shardDir := hash[:2]
	return filepath.Join(bc.refsDir, shardDir, hash), nil
}

// addReference adds a reference to a blob.
// Creates or updates the reference file with current timestamp and TTL.
func (bc *blobCacheImpl) addReference(ctx context.Context, digest string) error {
	bc.refCountMutex.Lock()
	defer bc.refCountMutex.Unlock()

	refPath, err := bc.getRefPath(digest)
	if err != nil {
		return fmt.Errorf("failed to get ref path: %w", err)
	}

	// Create reference data
	ref := blobRef{
		Digest:    digest,
		CreatedAt: time.Now(),
		TTL:       bc.defaultTTL,
		RefCount:  1, // Will be updated if reference already exists
	}

	// Check if reference already exists
	exists, err := bc.storage.Exists(ctx, refPath)
	if err != nil {
		return fmt.Errorf("failed to check reference existence: %w", err)
	}

	if exists {
		// Read existing reference and increment count
		existingRefData, readErr := bc.storage.ReadWithIntegrity(ctx, refPath)
		if readErr != nil {
			return fmt.Errorf("failed to read existing reference: %w", readErr)
		}

		existingRef, parseErr := parseBlobRef(existingRefData)
		if parseErr != nil {
			return fmt.Errorf("failed to parse existing reference: %w", parseErr)
		}

		ref.RefCount = existingRef.RefCount + 1
		ref.CreatedAt = existingRef.CreatedAt // Preserve original creation time
		ref.TTL = existingRef.TTL             // Preserve original TTL
	}

	// Write reference atomically
	refData, err := ref.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal reference: %w", err)
	}

	return bc.storage.WriteAtomically(ctx, refPath, refData)
}

// removeReference removes a reference to a blob.
// Only deletes the blob data when the reference count reaches zero.
func (bc *blobCacheImpl) removeReference(ctx context.Context, digest string) error {
	bc.refCountMutex.Lock()
	defer bc.refCountMutex.Unlock()

	refPath, err := bc.getRefPath(digest)
	if err != nil {
		return fmt.Errorf("failed to get ref path: %w", err)
	}

	// Check if reference exists
	exists, err := bc.storage.Exists(ctx, refPath)
	if err != nil {
		return fmt.Errorf("failed to check reference existence: %w", err)
	}
	if !exists {
		return nil // Reference doesn't exist, nothing to do
	}

	// Read existing reference
	refData, err := bc.storage.ReadWithIntegrity(ctx, refPath)
	if err != nil {
		return fmt.Errorf("failed to read reference: %w", err)
	}

	ref, err := parseBlobRef(refData)
	if err != nil {
		return fmt.Errorf("failed to parse reference: %w", err)
	}

	// Decrement reference count
	ref.RefCount--

	// No more references, remove both reference and blob
	if ref.RefCount <= 0 {
		if removeErr := bc.storage.Remove(ctx, refPath); removeErr != nil {
			return fmt.Errorf("failed to remove reference: %w", removeErr)
		}

		blobPath, blobErr := bc.getBlobPath(digest)
		if blobErr != nil {
			return fmt.Errorf("failed to get blob path: %w", blobErr)
		}

		if blobRemoveErr := bc.storage.Remove(ctx, blobPath); blobRemoveErr != nil {
			return fmt.Errorf("failed to remove blob: %w", blobRemoveErr)
		}
		return nil
	}

	// Update reference with decremented count
	updatedData, marshalErr := ref.MarshalText()
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal updated reference: %w", marshalErr)
	}

	if writeErr := bc.storage.WriteAtomically(ctx, refPath, updatedData); writeErr != nil {
		return fmt.Errorf("failed to update reference: %w", writeErr)
	}

	return nil
}

// blobRef represents a reference to a cached blob.
type blobRef struct {
	Digest    string        `json:"digest"`
	CreatedAt time.Time     `json:"created_at"`
	TTL       time.Duration `json:"ttl"`
	RefCount  int           `json:"ref_count"`
}

// MarshalText converts a blobRef to text format for storage.
func (br *blobRef) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s\n%d\n%d\n%d",
		br.Digest,
		br.CreatedAt.UnixNano(),
		int64(br.TTL),
		br.RefCount)), nil
}

// parseBlobRef parses a blobRef from text format.
func parseBlobRef(data []byte) (*blobRef, error) {
	lines := make([]string, 0, 4)
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	// Add the last line if there's no trailing newline
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}

	if len(lines) != 4 {
		return nil, fmt.Errorf("invalid reference format: expected 4 lines, got %d", len(lines))
	}

	createdAtNanos, err := parseInt64(lines[1])
	if err != nil {
		return nil, fmt.Errorf("invalid created_at: %w", err)
	}

	ttlNanos, err := parseInt64(lines[2])
	if err != nil {
		return nil, fmt.Errorf("invalid ttl: %w", err)
	}

	refCount, err := parseInt(lines[3])
	if err != nil {
		return nil, fmt.Errorf("invalid ref_count: %w", err)
	}

	return &blobRef{
		Digest:    lines[0],
		CreatedAt: time.Unix(0, createdAtNanos),
		TTL:       time.Duration(ttlNanos),
		RefCount:  refCount,
	}, nil
}

// parseInt64 parses a string to int64.
func parseInt64(s string) (int64, error) {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse int64: %w", err)
	}
	return val, nil
}

// parseInt parses a string to int.
func parseInt(s string) (int, error) {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("failed to parse int: %w", err)
	}
	return val, nil
}

// verifiedBlobReader wraps verified blob data with a reader interface.
type verifiedBlobReader struct {
	data   []byte
	offset int
	closed bool
}

// Read reads data from the verified blob.
func (vbr *verifiedBlobReader) Read(p []byte) (int, error) {
	if vbr.closed {
		return 0, os.ErrClosed
	}

	if vbr.offset >= len(vbr.data) {
		return 0, io.EOF
	}

	n := copy(p, vbr.data[vbr.offset:])
	vbr.offset += n

	return n, nil
}

// Close closes the blob reader.
func (vbr *verifiedBlobReader) Close() error {
	if vbr.closed {
		return nil
	}
	vbr.closed = true
	return nil
}

// Helper functions

// extractHashFromDigest extracts the hash portion from a digest string.
func extractHashFromDigest(digest string) string {
	const sha256Prefix = "sha256:"
	if strings.HasPrefix(digest, sha256Prefix) {
		return digest[len(sha256Prefix):]
	}
	return digest
}
