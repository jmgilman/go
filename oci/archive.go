// Package ocibundle provides OCI bundle distribution functionality.
// This file contains archive interface and implementations.
package ocibundle

import (
	"context"
	"io"

	"github.com/jmgilman/go/fs/billy"
	"github.com/jmgilman/go/fs/core"
)

// Archiver handles compression/decompression of file bundles.
// Implementations of this interface provide different archive formats
// such as tar.gz, zip, etc. for bundling files for OCI distribution.
type Archiver interface {
	// Archive creates an archive from a directory.
	// The sourceDir parameter specifies the directory to archive.
	// The output parameter is where the archive data is written.
	// Returns an error if the archiving process fails.
	Archive(ctx context.Context, sourceDir string, output io.Writer) error

	// ArchiveWithProgress creates an archive from a directory with progress reporting.
	// The sourceDir parameter specifies the directory to archive.
	// The output parameter is where the archive data is written.
	// The progress callback is called periodically during archiving.
	// Returns an error if the archiving process fails.
	ArchiveWithProgress(
		ctx context.Context,
		sourceDir string,
		output io.Writer,
		progress func(current, total int64),
	) error

	// Extract expands an archive to a directory.
	// The input parameter provides the archive data to read.
	// The targetDir parameter specifies where to extract the files.
	// The opts parameter controls extraction behavior and security constraints.
	// Returns an error if the extraction process fails.
	Extract(ctx context.Context, input io.Reader, targetDir string, opts ExtractOptions) error

	// MediaType returns the OCI media type for this archive format.
	// This is used as the artifact type when pushing to OCI registries.
	// Examples: "application/vnd.oci.image.layer.v1.tar+gzip"
	MediaType() string
}

// NewTarGzArchiverWithFS returns a tar.gz archiver bound to the provided filesystem.
// Phase 0 placeholder: the filesystem will be used internally starting in Phase 1.
func NewTarGzArchiverWithFS(fsys core.FS) *TarGzArchiver {
	if fsys == nil {
		fsys = billy.NewLocal()
	}
	return &TarGzArchiver{fs: fsys}
}

// ExtractOptions controls extraction behavior and security constraints.
// These options provide safety limits to prevent various attack vectors
// such as zip bombs, path traversal attacks, and resource exhaustion.
type ExtractOptions struct {
	// MaxFiles is the maximum number of files allowed in the archive.
	// Set to 0 for unlimited (not recommended for security).
	MaxFiles int

	// MaxSize is the maximum total uncompressed size of all files combined.
	MaxSize int64

	// MaxFileSize is the maximum size allowed for any individual file.
	MaxFileSize int64

	// StripPrefix removes this prefix from all file paths during extraction.
	// Useful for removing leading directory names from archived paths.
	StripPrefix string

	// PreservePerms determines whether to preserve original file permissions.
	// When false, permissions are sanitized for security.
	PreservePerms bool

	// FilesToExtract specifies glob patterns for selective file extraction.
	// When non-empty, only files matching at least one pattern will be extracted.
	// Supports standard glob patterns:
	//   - *.json: matches all .json files in root
	//   - config/*: matches all files in config directory
	//   - data/**/*.txt: matches all .txt files in data and subdirectories
	// When empty, all files are extracted (default behavior).
	FilesToExtract []string
}

// DefaultExtractOptions provides safe defaults for archive extraction.
// These defaults enforce security constraints to prevent common attacks:
// - MaxFiles: 10000 (prevents file count attacks)
// - MaxSize: 1GB (prevents resource exhaustion)
// - MaxFileSize: 100MB (prevents large individual files)
// - PreservePerms: false (sanitizes permissions)
// - FilesToExtract: empty (extracts all files)
var DefaultExtractOptions = ExtractOptions{
	MaxFiles:       10000,
	MaxSize:        1 * 1024 * 1024 * 1024, // 1GB
	MaxFileSize:    100 * 1024 * 1024,      // 100MB
	StripPrefix:    "",
	PreservePerms:  false,
	FilesToExtract: nil, // Extract all files by default
}

// DefaultArchiver returns an archiver initialized with the default OS-backed filesystem.
func DefaultArchiver() *TarGzArchiver {
	_ = billy.NewLocal() // reserved for future internal use
	return NewTarGzArchiver()
}

// TODO: Implement archive implementations
