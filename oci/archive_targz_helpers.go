package ocibundle

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmgilman/go/fs/core"

	validatepkg "github.com/jmgilman/go/oci/internal/validate"
)

// collectFileInfos walks the source directory and returns all entries with
// their original path, relative path, and os.FileInfo.
func collectFileInfos(fsys core.FS, sourceDir string) ([]fileInfoEntry, error) {
	var fileInfos []fileInfoEntry
	walkErr := fsys.Walk(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk failed at %s: %w", path, err)
		}

		relPath, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, relErr)
		}

		if relPath == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}

		fileInfos = append(fileInfos, fileInfoEntry{
			path:     path,
			relPath:  relPath,
			info:     info,
			fileSize: info.Size(),
		})

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("failed to collect files from %s: %w", sourceDir, walkErr)
	}
	return fileInfos, nil
}

// processArchiveResults consumes worker results and writes entries to tarWriter.
func processArchiveResults(
	ctx context.Context,
	results <-chan archiveResult,
	tarWriter *tar.Writer,
	copyWithProgress func(dst io.Writer, src io.Reader, progress func(int64)) (int64, error),
	currentSize *int64,
	totalSize int64,
	progress func(current, total int64),
) error {
	for result := range results {
		if err := isDone(ctx, "archiving"); err != nil {
			return err
		}

		if result.err != nil {
			return fmt.Errorf("worker error for %s: %w", result.relPath, result.err)
		}

		if err := writeArchiveEntry(tarWriter, result, copyWithProgress, currentSize, totalSize, progress); err != nil {
			return err
		}
	}
	return nil
}

// writeArchiveEntry writes a single header and optional content to tarWriter.
func writeArchiveEntry(
	tarWriter *tar.Writer,
	result archiveResult,
	copyWithProgress func(dst io.Writer, src io.Reader, progress func(int64)) (int64, error),
	currentSize *int64,
	totalSize int64,
	progress func(current, total int64),
) error {
	if err := tarWriter.WriteHeader(result.header); err != nil {
		return fmt.Errorf("failed to write tar header for %s: %w", result.relPath, err)
	}

	if result.content == nil {
		return nil
	}
	defer result.content.Close()

	if progress != nil {
		if _, err := copyWithProgress(tarWriter, result.content, func(written int64) {
			*currentSize += written
			progress(*currentSize, totalSize)
		}); err != nil {
			return fmt.Errorf("failed to write file content for %s: %w", result.relPath, err)
		}
		return nil
	}

	if _, err := io.Copy(tarWriter, result.content); err != nil {
		return fmt.Errorf("failed to write file content for %s: %w", result.relPath, err)
	}
	return nil
}

// isDone returns a wrapped context cancellation error if ctx is done.
func isDone(ctx context.Context, action string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%s canceled: %w", action, ctx.Err())
	default:
		return nil
	}
}

// safeJoin ensures that the resulting path is within rootAbs.
func safeJoin(rootAbs, targetDir, member string) (string, error) {
	fullPath := filepath.Join(targetDir, member)
	targetAbs, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return "", fmt.Errorf("failed to resolve target path: %w", err)
	}
	if !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) && targetAbs != rootAbs {
		return "", fmt.Errorf("path escapes target directory: %s", member)
	}
	return targetAbs, nil
}

// ensureParentDir creates the parent directory for a path.
func ensureParentDir(fsys core.FS, fullPath string) error {
	if err := fsys.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", fullPath, err)
	}
	return nil
}

// handleHeader validates and dispatches extraction by header type.
func handleHeader(
	ctx context.Context,
	tr *tar.Reader,
	hdr *tar.Header,
	targetDir string,
	rootAbs string,
	opts ExtractOptions,
	validators Validator,
	pv *validatepkg.PathTraversalValidator,
	totalSize *int64,
	fileCount *int,
	fsys core.FS,
) error {
	if err := isDone(ctx, "extraction"); err != nil {
		return err
	}

	*fileCount++

	fullPath, err := normalizeAndResolvePath(pv, hdr.Name, opts.StripPrefix, targetDir, rootAbs)
	if err != nil {
		return err
	}

	if err := validateFileAndArchive(validators, hdr, opts, totalSize, fileCount); err != nil {
		return err
	}

	if err := ensureParentDir(fsys, fullPath); err != nil {
		return err
	}

	return performExtraction(tr, hdr, fullPath, opts, pv, fsys)
}

// normalizeAndResolvePath validates the header path, applies strip prefix, and ensures it stays within root.
func normalizeAndResolvePath(
	pv *validatepkg.PathTraversalValidator,
	headerName string,
	stripPrefix string,
	targetDir string,
	rootAbs string,
) (string, error) {
	if validateErr := pv.ValidatePath(headerName); validateErr != nil {
		return "", NewBundleError("extract", headerName, ErrSecurityViolation)
	}

	filePath := headerName
	if stripPrefix != "" && strings.HasPrefix(filePath, stripPrefix) {
		filePath = strings.TrimPrefix(filePath, stripPrefix)
		filePath = strings.TrimPrefix(filePath, "/")
	}

	fullPath, err := safeJoin(rootAbs, targetDir, filePath)
	if err != nil {
		return "", NewBundleError("extract", headerName, ErrSecurityViolation)
	}
	return fullPath, nil
}

// validateFileAndArchive runs validators for file and archive-level constraints and updates counters.
func validateFileAndArchive(
	validators Validator,
	hdr *tar.Header,
	opts ExtractOptions,
	totalSize *int64,
	fileCount *int,
) error {
	info := FileInfo{
		Name: hdr.Name,
		Size: hdr.Size,
		Mode: uint32(hdr.Mode),
	}
	if err := validators.ValidateFile(info); err != nil {
		return NewBundleError("extract", hdr.Name, err)
	}

	if opts.MaxFiles > 0 && *fileCount > opts.MaxFiles {
		return NewBundleError("extract", hdr.Name, ErrSecurityViolation)
	}

	*totalSize += hdr.Size
	archiveStats := ArchiveStats{
		TotalFiles: *fileCount,
		TotalSize:  *totalSize,
	}
	if err := validators.ValidateArchive(archiveStats); err != nil {
		return NewBundleError("extract", hdr.Name, err)
	}
	return nil
}

// performExtraction dispatches by typeflag and applies perms when needed.
func performExtraction(
	tr *tar.Reader,
	hdr *tar.Header,
	fullPath string,
	opts ExtractOptions,
	pv *validatepkg.PathTraversalValidator,
	fsys core.FS,
) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return extractDir(fsys, fullPath)
	case tar.TypeReg:
		if err := extractRegularFile(fsys, tr, fullPath); err != nil {
			return err
		}
		if !opts.PreservePerms {
			// Ensure mode on create; optionally adjust if FS exposes chmod in future.
			return nil
		}
		return nil
	case tar.TypeSymlink:
		return extractSymlink(fsys, pv, hdr, fullPath)
	default:
		return nil
	}
}

// extractDir creates a directory.
func extractDir(fsys core.FS, fullPath string) error {
	if err := fsys.MkdirAll(fullPath, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
	}
	return nil
}

// extractRegularFile writes out a regular file from a tar reader.
func extractRegularFile(fsys core.FS, tr *tar.Reader, fullPath string) error {
	file, err := fsys.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, tr); err != nil {
		return fmt.Errorf("failed to write file content for %s: %w", fullPath, err)
	}
	return nil
}

// extractSymlink creates a symlink after validator approval.
func extractSymlink(
	fsys core.FS,
	pv *validatepkg.PathTraversalValidator,
	hdr *tar.Header,
	fullPath string,
) error {
	linkTarget := hdr.Linkname
	if err := pv.ValidateSymlink(hdr.Name, linkTarget); err != nil {
		return NewBundleError("extract", hdr.Name, ErrSecurityViolation)
	}

	// Check if filesystem supports symlinks
	if sfs, ok := fsys.(core.SymlinkFS); ok {
		if err := sfs.Symlink(linkTarget, fullPath); err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %w", fullPath, linkTarget, err)
		}
		return nil
	}

	// If symlinks not supported, skip silently or log warning
	return nil
}
