// Package ocibundle provides OCI bundle distribution functionality.
// This file contains the tar.gz implementation of the Archiver interface.
package ocibundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/jmgilman/go/fs/billy"
	"github.com/jmgilman/go/fs/core"

	validatepkg "github.com/jmgilman/go/oci/internal/validate"
)

// TarGzArchiver implements the Archiver interface using tar.gz format.
// It provides secure, streaming archive and extraction operations with
// comprehensive validation and progress reporting capabilities.
// Uses concurrent processing for improved performance on multi-core systems.
type TarGzArchiver struct {
	fs core.FS
}

// NewTarGzArchiver creates a new TarGzArchiver instance.
// The archiver uses standard tar.gz format compatible with common tools
// and implements security validation during extraction.
func NewTarGzArchiver() *TarGzArchiver {
	return &TarGzArchiver{fs: billy.NewLocal()}
}

// Archive creates a tar.gz archive from the specified source directory.
// The archive is written to the provided output writer in a streaming fashion
// to minimize memory usage even with large directories.
//
// Parameters:
//   - ctx: Context for cancellation
//   - sourceDir: Directory to archive (must exist and be readable)
//   - output: Writer to receive the compressed archive data
//
// Returns an error if the source directory doesn't exist, is not readable,
// or if writing to the output fails.
func (a *TarGzArchiver) Archive(ctx context.Context, sourceDir string, output io.Writer) error {
	return a.ArchiveWithProgress(ctx, sourceDir, output, nil)
}

// ArchiveWithProgress creates a tar.gz archive from the specified source directory with progress reporting.
// The archive is written to the provided output writer in a streaming fashion
// to minimize memory usage even with large directories.
// Uses concurrent file processing for improved performance.
//
// Parameters:
//   - ctx: Context for cancellation
//   - sourceDir: Directory to archive (must exist and be readable)
//   - output: Writer to receive the compressed archive data
//   - progress: Optional callback for progress reporting (current, total bytes)
//
// Returns an error if the source directory doesn't exist, is not readable,
// or if writing to the output fails.
func (a *TarGzArchiver) ArchiveWithProgress(
	ctx context.Context,
	sourceDir string,
	output io.Writer,
	progress func(current, total int64),
) error {
	if sourceDir == "" {
		return fmt.Errorf("source directory cannot be empty")
	}

	if output == nil {
		return fmt.Errorf("output writer cannot be nil")
	}

	if _, err := a.fs.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}

	// If progress callback is provided, calculate total size first
	var totalSize int64
	if progress != nil {
		er := a.fs.Walk(sourceDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				if info.Mode().IsRegular() {
					totalSize += info.Size()
				}
			}
			return nil
		})
		if er != nil {
			return fmt.Errorf("failed to calculate total size: %w", er)
		}
	}

	gzipWriter := gzip.NewWriter(output)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	var currentSize int64

	// Use concurrent processing for better performance
	return a.archiveWithConcurrency(ctx, sourceDir, tarWriter, &currentSize, totalSize, progress)
}

// archiveWithConcurrency implements concurrent file processing for archiving.
// It uses a worker pool to process multiple files concurrently while maintaining
// tar archive order through coordination.
func (a *TarGzArchiver) archiveWithConcurrency(
	ctx context.Context,
	sourceDir string,
	tarWriter *tar.Writer,
	currentSize *int64,
	totalSize int64,
	progress func(current, total int64),
) error {
	// Collect all file paths first
	fileInfos, err := collectFileInfos(a.fs, sourceDir)
	if err != nil {
		return err
	}

	// Determine optimal number of workers (based on CPU cores, but limit to reasonable number)
	numWorkers := min(len(fileInfos), 8) // Use up to 8 workers
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Create channels for coordination
	jobs := make(chan fileInfoEntry, len(fileInfos))
	results := make(chan archiveResult, len(fileInfos))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.worker(ctx, jobs, results)
		}()
	}

	// Send jobs
	for _, fileInfo := range fileInfos {
		jobs <- fileInfo
	}
	close(jobs)

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results and write entries
	return processArchiveResults(ctx, results, tarWriter, a.copyWithProgress, currentSize, totalSize, progress)
}

// fileInfoEntry holds information about a file to be archived
type fileInfoEntry struct {
	path     string
	relPath  string
	info     os.FileInfo
	fileSize int64
}

// archiveResult holds the result of processing a file for archiving
type archiveResult struct {
	relPath string
	header  *tar.Header
	content io.ReadCloser
	err     error
}

// worker processes files concurrently for archiving
func (a *TarGzArchiver) worker(ctx context.Context, jobs <-chan fileInfoEntry, results chan<- archiveResult) {
	for job := range jobs {
		select {
		case <-ctx.Done():
			results <- archiveResult{err: ctx.Err()}
			return
		default:
		}

		header, err := tar.FileInfoHeader(job.info, "")
		if err != nil {
			results <- archiveResult{relPath: job.relPath, err: fmt.Errorf("failed to create tar header for %s: %w", job.path, err)}
			continue
		}

		header.Name = job.relPath

		var content io.ReadCloser
		if job.info.Mode().IsRegular() {
			file, err := a.fs.Open(job.path)
			if err != nil {
				results <- archiveResult{relPath: job.relPath, err: fmt.Errorf("failed to open file %s: %w", job.path, err)}
				continue
			}
			content = file
		}

		results <- archiveResult{
			relPath: job.relPath,
			header:  header,
			content: content,
			err:     nil,
		}
	}
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// copyWithProgress copies data from src to dst while reporting progress
func (a *TarGzArchiver) copyWithProgress(dst io.Writer, src io.Reader, progress func(int64)) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return total, fmt.Errorf("copyWithProgress write: %w", writeErr)
			}
			total += int64(n)
			if progress != nil {
				progress(int64(n))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, fmt.Errorf("copyWithProgress read: %w", err)
		}
	}
	return total, nil
}

// Extract expands a tar.gz archive to the specified target directory.
// The extraction process includes security validation to prevent common
// archive-based attacks such as path traversal and resource exhaustion.
//
// Parameters:
//   - ctx: Context for cancellation
//   - input: Reader providing the compressed archive data
//   - targetDir: Directory to extract files to (created if it doesn't exist)
//   - opts: Extraction options controlling security limits and behavior
//
// Returns an error if the archive is corrupted, contains security violations,
// exceeds configured limits, or if file operations fail.
func (a *TarGzArchiver) Extract(ctx context.Context, input io.Reader, targetDir string, opts ExtractOptions) error {
	if input == nil {
		return fmt.Errorf("input reader cannot be nil")
	}

	if targetDir == "" {
		return fmt.Errorf("target directory cannot be empty")
	}

	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	if mkErr := a.fs.MkdirAll(targetDir, 0o755); mkErr != nil {
		return fmt.Errorf("failed to create target directory: %w", mkErr)
	}

	validators := NewValidatorChain(
		NewSizeValidator(opts.MaxFileSize, opts.MaxSize),
		NewFileCountValidator(opts.MaxFiles),
		NewPermissionSanitizer(),
	)

	// Path traversal and symlink validation (internal validator)
	pv := validatepkg.NewPathTraversalValidator()
	pv.AllowHiddenFiles = false
	pv.RootPath = targetDir

	totalSize := int64(0)
	fileCount := 0

	rootAbs, absErr := filepath.Abs(targetDir)
	if absErr != nil {
		return fmt.Errorf("failed to resolve target directory: %w", absErr)
	}

	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break // End of archive
		}
		if nextErr != nil {
			return fmt.Errorf("failed to read tar header: %w", nextErr)
		}

		if err := handleHeader(ctx, tarReader, header, targetDir, rootAbs, opts, validators, pv, &totalSize, &fileCount, a.fs); err != nil {
			return err
		}
	}

	return nil
}

// MediaType returns the OCI media type for tar.gz archives.
// This is used when pushing bundles to OCI registries to identify
// the archive format to registry clients.
func (a *TarGzArchiver) MediaType() string {
	return "application/vnd.oci.image.layer.v1.tar+gzip"
}
