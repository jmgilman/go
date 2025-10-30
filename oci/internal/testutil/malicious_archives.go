// Package testutil provides testing utilities for the OCI bundle library.
// This file contains malicious archive generators for security testing.
package testutil

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"time"
)

// MaliciousArchiveGenerator creates archives that exploit common vulnerabilities
// in archive processing, based on OWASP Top 10 for file processing.
type MaliciousArchiveGenerator struct {
	tempDir string
}

// NewMaliciousArchiveGenerator creates a new malicious archive generator.
func NewMaliciousArchiveGenerator() (*MaliciousArchiveGenerator, error) {
	tempDir, err := os.MkdirTemp("", "malicious-archives-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &MaliciousArchiveGenerator{tempDir: tempDir}, nil
}

// Close cleans up the temporary directory.
func (g *MaliciousArchiveGenerator) Close() error {
	if g.tempDir != "" {
		if err := os.RemoveAll(g.tempDir); err != nil {
			return fmt.Errorf("failed to remove temp directory %s: %w", g.tempDir, err)
		}
	}
	return nil
}

// GeneratePathTraversalArchive creates an archive with path traversal attempts.
// This tests for "../" and absolute path vulnerabilities.
//
// OWASP Risk: Path Traversal
// Common patterns: ../../../etc/passwd, /etc/passwd, ..\\..\\windows\\system32
func (g *MaliciousArchiveGenerator) GeneratePathTraversalArchive(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	maliciousPaths := []struct {
		name    string
		content string
	}{
		{"../../../etc/passwd", "malicious content"},
		{"../../../../etc/shadow", "secret data"},
		{"..\\..\\..\\windows\\system32\\config\\sam", "windows secrets"},
		{"/etc/passwd", "absolute path attack"},
		{"//etc/passwd", "double slash attack"},
		{"subdir/../../../root.txt", "nested traversal"},
		{"normal-file.txt", "legitimate content"},
	}

	for _, entry := range maliciousPaths {
		header := &tar.Header{
			Name:    entry.name,
			Size:    int64(len(entry.content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", entry.name, err)
		}

		if _, err := tarWriter.Write([]byte(entry.content)); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", entry.name, err)
		}
	}

	return nil
}

// GenerateZipBomb creates a zip bomb - a small archive that expands to a huge size.
// This tests for decompression bombs and resource exhaustion.
//
// OWASP Risk: Decompression Bomb / Resource Exhaustion
// Strategy: Create many small files that expand to consume memory/disk
func (g *MaliciousArchiveGenerator) GenerateZipBomb(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	// Create 100 files, each claiming to be 1MB but containing minimal data
	// This is a mild zip bomb for testing - real ones are much more aggressive
	for i := 0; i < 100; i++ {
		fileName := fmt.Sprintf("file-%03d.txt", i)
		writer, err := zipWriter.Create(fileName)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", fileName, err)
		}

		// Write minimal content that could be expanded
		content := fmt.Sprintf("This is file %d with some padding content to make it larger.\n", i)
		content += strings.Repeat("padding ", 100) + "\n"

		if _, err := writer.Write([]byte(content)); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", fileName, err)
		}
	}

	return nil
}

// GenerateFileCountBomb creates an archive with too many files.
// This tests for file count limits and resource exhaustion.
//
// OWASP Risk: Resource Exhaustion via File Count
func (g *MaliciousArchiveGenerator) GenerateFileCountBomb(outputPath string, fileCount int) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file-%06d.txt", i)
		content := fmt.Sprintf("Content of file %d\n", i)

		header := &tar.Header{
			Name:    fileName,
			Size:    int64(len(content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", fileName, err)
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", fileName, err)
		}
	}

	return nil
}

// GenerateSymlinkBomb creates an archive with dangerous symlinks.
// This tests for symlink-based path traversal and directory traversal.
//
// OWASP Risk: Symlink Traversal / Directory Traversal
func (g *MaliciousArchiveGenerator) GenerateSymlinkBomb(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Create files first
	files := []struct {
		name    string
		content string
	}{
		{"legitimate.txt", "This is a normal file"},
		{"data.txt", "Some data"},
	}

	for _, f := range files {
		header := &tar.Header{
			Name:    f.name,
			Size:    int64(len(f.content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", f.name, err)
		}

		if _, err := tarWriter.Write([]byte(f.content)); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", f.name, err)
		}
	}

	// Create dangerous symlinks
	symlinks := []struct {
		name string
		link string
	}{
		{"evil-link", "../../../etc/passwd"},
		{"another-link", "../../../../root/.ssh/id_rsa"},
		{"absolute-link", "/etc/shadow"},
		{"relative-link", "../../secret.txt"},
		{"self-link", "./legitimate.txt"}, // This should be safe
	}

	for _, link := range symlinks {
		header := &tar.Header{
			Name:     link.name,
			Linkname: link.link,
			Typeflag: tar.TypeSymlink,
			Mode:     0o777,
			ModTime:  time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write symlink header for %s: %w", link.name, err)
		}
	}

	return nil
}

// GenerateMalformedArchive creates an archive with structural issues.
// This tests for robustness against corrupted or malformed archives.
//
// OWASP Risk: Input Validation / Malformed Input
func (g *MaliciousArchiveGenerator) GenerateMalformedArchive(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Write some valid tar.gz data first
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	// Add a valid file
	content := []byte("valid content")
	header := &tar.Header{
		Name:    "valid.txt",
		Size:    int64(len(content)),
		Mode:    0o644,
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for valid.txt: %w", err)
	}

	if _, err := tarWriter.Write(content); err != nil {
		return fmt.Errorf("failed to write tar content for valid.txt: %w", err)
	}

	// Close writers properly
	tarWriter.Close()
	gzipWriter.Close()

	// Now append some corrupted data to make it malformed
	corruptedData := []byte("this is not valid tar data and should cause parsing errors")
	if _, err := file.Write(corruptedData); err != nil {
		return fmt.Errorf("failed to append corrupted data: %w", err)
	}

	return nil
}

// GenerateNestedArchive creates deeply nested directory structures.
// This tests for path length limits and deep recursion.
//
// OWASP Risk: Resource Exhaustion / Deep Recursion
func (g *MaliciousArchiveGenerator) GenerateNestedArchive(outputPath string, depth int) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Generate deeply nested paths
	for i := 0; i < 10; i++ {
		currentPath := ""
		for d := 0; d < depth; d++ {
			if currentPath != "" {
				currentPath += "/"
			}
			currentPath += fmt.Sprintf("level%d", d)
		}
		currentPath += fmt.Sprintf("/file-%d.txt", i)

		content := fmt.Sprintf("Content of deeply nested file %d at depth %d\n", i, depth)

		header := &tar.Header{
			Name:    currentPath,
			Size:    int64(len(content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", currentPath, err)
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", currentPath, err)
		}
	}

	return nil
}

// GenerateLargeFileArchive creates an archive with a single very large file.
// This tests for memory exhaustion with individual large files.
//
// OWASP Risk: Resource Exhaustion / Memory Exhaustion
func (g *MaliciousArchiveGenerator) GenerateLargeFileArchive(outputPath string, size int64) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Create a large file entry (but don't actually write the content to avoid disk space issues)
	header := &tar.Header{
		Name:    "large-file.bin",
		Size:    size, // Claim it's very large
		Mode:    0o644,
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write header for large file: %w", err)
	}

	// Write minimal content instead of the claimed size
	content := []byte("This file claims to be very large but isn't really")
	if _, err := tarWriter.Write(content); err != nil {
		return fmt.Errorf("failed to write content for large file: %w", err)
	}

	return nil
}
