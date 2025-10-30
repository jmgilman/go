// Package testutil provides testing utilities for the OCI bundle library.
// This file contains archive generation utilities for testing.
package testutil

import (
	"archive/tar"
	"compress/gzip"
	"context"
	crand "crypto/rand"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArchiveGenerator creates test archives of various sizes and types for testing.
// It can generate archives with specific file counts, sizes, and content patterns.
type ArchiveGenerator struct {
	tempDir string
}

// NewArchiveGenerator creates a new archive generator with a temporary directory.
func NewArchiveGenerator() (*ArchiveGenerator, error) {
	tempDir, err := os.MkdirTemp("", "archive-gen-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &ArchiveGenerator{tempDir: tempDir}, nil
}

// NewArchiveGeneratorWithFSTemp creates a new archive generator using a provided filesystem root path.
// Tests can pass an in-memory root path when using a memfs implementation.
func NewArchiveGeneratorWithFSTemp(root string) *ArchiveGenerator {
	return &ArchiveGenerator{tempDir: root}
}

// Close cleans up the temporary directory used by the generator.
func (g *ArchiveGenerator) Close() error {
	if g.tempDir != "" {
		if err := os.RemoveAll(g.tempDir); err != nil {
			return fmt.Errorf("failed to remove temp directory %s: %w", g.tempDir, err)
		}
	}
	return nil
}

// GenerateTestArchive creates a test tar.gz archive with the specified parameters.
// It generates files with predictable or random content based on the options.
//
// Parameters:
//   - size: Approximate total size of the uncompressed archive in bytes
//   - fileCount: Number of files to generate
//   - pattern: Content pattern - "zeros", "random", "text", or "mixed"
//   - outputPath: Path where the tar.gz file will be created
//
// Returns the actual size of the generated archive and any error.
func (g *ArchiveGenerator) GenerateTestArchive(
	ctx context.Context,
	size int64,
	fileCount int,
	pattern, outputPath string,
) (int64, error) {
	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	var totalSize int64
	bytesPerFile := size / int64(fileCount)
	if bytesPerFile < 1 {
		bytesPerFile = 1
	}

	// Generate files
	for i := 0; i < fileCount; i++ {
		select {
		case <-ctx.Done():
			return totalSize, fmt.Errorf("context cancelled during archive generation: %w", ctx.Err())
		default:
		}

		fileName := fmt.Sprintf("test-file-%04d.txt", i)
		fileSize := bytesPerFile

		// Last file gets remaining size to hit target
		if i == fileCount-1 {
			fileSize = size - totalSize
		}

		// Generate file content
		content, err := g.generateContent(pattern, fileSize)
		if err != nil {
			return totalSize, fmt.Errorf("failed to generate content for file %s: %w", fileName, err)
		}

		// Add file to tar
		header := &tar.Header{
			Name:    fileName,
			Size:    int64(len(content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return totalSize, fmt.Errorf("failed to write tar header for %s: %w", fileName, err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			return totalSize, fmt.Errorf("failed to write tar content for %s: %w", fileName, err)
		}

		totalSize += int64(len(content))
	}

	return totalSize, nil
}

// generateContent generates content based on the specified pattern.
func (g *ArchiveGenerator) generateContent(pattern string, size int64) ([]byte, error) {
	switch pattern {
	case "zeros":
		return make([]byte, size), nil
	case "random":
		content := make([]byte, size)
		if _, err := crand.Read(content); err != nil {
			return nil, fmt.Errorf("failed to generate random content: %w", err)
		}
		return content, nil
	case "text":
		return g.generateTextContent(size)
	case "mixed":
		return g.generateMixedContent(size)
	default:
		return g.generateTextContent(size)
	}
}

// generateTextContent generates readable text content.
func (g *ArchiveGenerator) generateTextContent(size int64) ([]byte, error) {
	words := []string{
		"lorem", "ipsum", "dolor", "sit", "amet", "consectetur",
		"adipiscing", "elit", "sed", "do", "eiusmod", "tempor",
		"incididunt", "ut", "labore", "et", "dolore", "magna", "aliqua",
	}

	var content strings.Builder
	for content.Len() < int(size) {
		for _, word := range words {
			if content.Len()+len(word)+1 >= int(size) {
				break
			}
			content.WriteString(word)
			content.WriteString(" ")
		}
		content.WriteString("\n")
	}

	result := content.String()
	if int64(len(result)) > size {
		result = result[:size]
	}

	return []byte(result), nil
}

// generateMixedContent generates a mix of text and binary data.
func (g *ArchiveGenerator) generateMixedContent(size int64) ([]byte, error) {
	textSize := size / 2
	binarySize := size - textSize

	textContent, err := g.generateTextContent(textSize)
	if err != nil {
		return nil, err
	}

	binaryContent := make([]byte, binarySize)
	if _, err := crand.Read(binaryContent); err != nil {
		return nil, fmt.Errorf("failed to generate random binary content: %w", err)
	}

	return append(textContent, binaryContent...), nil
}

// GenerateTestDirectory creates a directory structure with test files for archiving.
// This is useful for testing the archiving process with real directory structures.
//
// Parameters:
//   - baseDir: Base directory to create the structure in
//   - depth: Directory depth (1 = flat, 2 = one level of subdirs, etc.)
//   - filesPerDir: Number of files per directory
//   - avgFileSize: Average file size in bytes
//
// Returns the total size of all created files.
func (g *ArchiveGenerator) GenerateTestDirectory(
	baseDir string,
	depth, filesPerDir int,
	avgFileSize int64,
) (int64, error) {
	return g.generateDirectoryRecursive(baseDir, depth, filesPerDir, avgFileSize, 0)
}

// generateDirectoryRecursive recursively generates directory structure.
func (g *ArchiveGenerator) generateDirectoryRecursive(
	baseDir string,
	maxDepth, filesPerDir int,
	avgFileSize int64,
	currentDepth int,
) (int64, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create directory %s: %w", baseDir, err)
	}

	var totalSize int64

	// Create files in current directory
	for i := 0; i < filesPerDir; i++ {
		fileName := filepath.Join(baseDir, fmt.Sprintf("file-%03d.txt", i))

		// Vary file size around average (±25%)
		sizeVariation := (mrand.Int63n(avgFileSize/2) - avgFileSize/4)
		fileSize := avgFileSize + sizeVariation
		if fileSize < 1 {
			fileSize = 1
		}

		content, err := g.generateContent("text", fileSize)
		if err != nil {
			return totalSize, fmt.Errorf("failed to generate content for %s: %w", fileName, err)
		}

		if err := os.WriteFile(fileName, content, 0o644); err != nil {
			return totalSize, fmt.Errorf("failed to write file %s: %w", fileName, err)
		}

		totalSize += int64(len(content))
	}

	// Create subdirectories if not at max depth
	if currentDepth < maxDepth-1 {
		subdirs := []string{"subdir-a", "subdir-b", "subdir-c"}
		for _, subdir := range subdirs {
			subdirPath := filepath.Join(baseDir, subdir)
			size, err := g.generateDirectoryRecursive(subdirPath, maxDepth, filesPerDir, avgFileSize, currentDepth+1)
			if err != nil {
				return totalSize, err
			}
			totalSize += size
		}
	}

	return totalSize, nil
}

// GenerateLargeArchive creates an archive with a single large file.
// This is useful for testing streaming and memory usage.
//
// Parameters:
//   - size: Size of the large file in bytes
//   - outputPath: Path where the tar.gz file will be created
//
// Returns the actual size of the generated archive.
func (g *ArchiveGenerator) GenerateLargeArchive(ctx context.Context, size int64, outputPath string) (int64, error) {
	return g.GenerateTestArchive(ctx, size, 1, "random", outputPath)
}

// GenerateEmptyArchive creates an empty tar.gz archive.
// This is useful for testing edge cases.
func (g *ArchiveGenerator) GenerateEmptyArchive(outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Empty tar archive - just close the writers
	return nil
}
