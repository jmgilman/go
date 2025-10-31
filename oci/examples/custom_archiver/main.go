// Package main demonstrates how to implement and use custom archivers.
// This example shows creating a simple ZIP archiver as an alternative to tar.gz.
package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	ocibundle "github.com/jmgilman/go/oci"
)

// ZipArchiver implements the Archiver interface using ZIP format.
// This demonstrates how to create custom archiver implementations.
type ZipArchiver struct{}

// NewZipArchiver creates a new ZIP archiver instance.
func NewZipArchiver() *ZipArchiver {
	return &ZipArchiver{}
}

// Archive creates a ZIP archive from the specified source directory.
func (a *ZipArchiver) Archive(ctx context.Context, sourceDir string, output io.Writer) error {
	return a.ArchiveWithProgress(ctx, sourceDir, output, nil)
}

// ArchiveWithProgress creates a ZIP archive with progress reporting.
func (a *ZipArchiver) ArchiveWithProgress(
	ctx context.Context,
	sourceDir string,
	output io.Writer,
	progress func(current, total int64),
) error {
	if sourceDir == "" {
		return fmt.Errorf("source directory cannot be empty")
	}

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}

	zipWriter := zip.NewWriter(output)
	defer zipWriter.Close()

	var totalBytes int64
	if progress != nil {
		// Calculate total size for progress reporting
		err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				totalBytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to calculate total size: %w", err)
		}
	}

	var currentBytes int64
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		// Create ZIP entry
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to create ZIP header for %s: %w", path, err)
		}
		header.Name = relPath
		header.Method = zip.Deflate // Use compression

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("failed to create ZIP entry for %s: %w", path, err)
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			// Copy with progress reporting
			if progress != nil {
				written, err := a.copyWithProgress(writer, file, func(written int64) {
					currentBytes += written
					progress(currentBytes, totalBytes)
				})
				if err != nil {
					return fmt.Errorf("failed to write file content for %s: %w", path, err)
				}
				_ = written
			} else {
				if _, err := io.Copy(writer, file); err != nil {
					return fmt.Errorf("failed to write file content for %s: %w", path, err)
				}
			}
		}

		return nil
	})
}

// copyWithProgress copies data with progress reporting
func (a *ZipArchiver) copyWithProgress(dst io.Writer, src io.Reader, progress func(int64)) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return total, writeErr
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
			return total, err
		}
	}
	return total, nil
}

// Extract extracts a ZIP archive to the specified directory.
// Note: This is a simplified implementation. Production code should include
// the same security validations as TarGzArchiver.
func (a *ZipArchiver) Extract(
	ctx context.Context,
	input io.Reader,
	targetDir string,
	opts ocibundle.ExtractOptions,
) error {
	size, err := input.(io.Seeker).Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek to end: %w", err)
	}

	if _, err := input.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start: %w", err)
	}

	zipReader, err := zip.NewReader(input.(io.ReaderAt), size)
	if err != nil {
		return fmt.Errorf("failed to create ZIP reader: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	for _, file := range zipReader.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Security check: prevent path traversal
		if strings.Contains(file.Name, "..") {
			return fmt.Errorf("path traversal detected: %s", file.Name)
		}

		filePath := filepath.Join(targetDir, file.Name)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", filePath, err)
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", filePath, err)
		}

		// Extract file
		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open ZIP file %s: %w", file.Name, err)
		}

		dst, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			src.Close()
			return fmt.Errorf("failed to create file %s: %w", filePath, err)
		}

		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}

		src.Close()
		dst.Close()
	}

	return nil
}

// MediaType returns the OCI media type for ZIP archives.
func (a *ZipArchiver) MediaType() string {
	return "application/vnd.oci.image.layer.v1.zip"
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get current working directory for absolute paths
	// Note: billy.NewLocal() creates a filesystem rooted at "/", so we need absolute paths
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create sample files
	if err := createSampleFiles(); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	fmt.Println("🗜️  Demonstrating custom ZIP archiver...")

	// Create custom ZIP archiver
	customArchiver := NewZipArchiver()

	// For this example, we'll show how to create a client with a custom archiver
	// Note: The current API doesn't directly support custom archivers yet,
	// but this shows the pattern for implementation

	fmt.Printf("Custom ZIP Archiver Media Type: %s\n", customArchiver.MediaType())

	// Demonstrate archiving (without pushing to registry for this example)
	sourceDir := filepath.Join(cwd, "sample-files")
	tempFile, err := os.CreateTemp("", "custom-archiver-*.zip")
	if err != nil {
		log.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	fmt.Println("📦 Creating ZIP archive...")
	progress := func(current, total int64) {
		if total > 0 {
			percentage := float64(current) / float64(total) * 100
			fmt.Printf("\r📊 Archive progress: %.1f%%", percentage)
		}
	}

	if err := customArchiver.ArchiveWithProgress(ctx, sourceDir, tempFile, progress); err != nil {
		log.Fatalf("Failed to create archive: %v", err)
	}
	fmt.Println("\n✅ Archive created successfully!")

	// Demonstrate extraction
	targetDir := filepath.Join(cwd, "extracted-zip")
	fmt.Println("📤 Extracting ZIP archive...")

	// Reset file pointer for reading
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		log.Fatalf("Failed to seek to beginning: %v", err)
	}

	extractOpts := ocibundle.ExtractOptions{
		MaxFiles:    1000,
		MaxSize:     100 * 1024 * 1024, // 100MB
		MaxFileSize: 10 * 1024 * 1024,  // 10MB
	}

	if err := customArchiver.Extract(ctx, tempFile, targetDir, extractOpts); err != nil {
		log.Fatalf("Failed to extract archive: %v", err)
	}
	fmt.Println("✅ Archive extracted successfully!")

	fmt.Println("\n🎉 Custom archiver example completed!")
	fmt.Printf("Original files: %s\n", sourceDir)
	fmt.Printf("Archive file: %s\n", tempFile.Name())
	fmt.Printf("Extracted files: %s\n", targetDir)
	fmt.Println("\n📝 Custom Archiver Implementation Notes:")
	fmt.Println("   • Implement the ocibundle.Archiver interface")
	fmt.Println("   • Handle security validation in Extract method")
	fmt.Println("   • Support progress reporting for better UX")
	fmt.Println("   • Return appropriate OCI media type")
	fmt.Println("   • Include comprehensive error handling")
}

// createSampleFiles creates sample files for the custom archiver demonstration
func createSampleFiles() error {
	dir := "./sample-files"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	files := []struct {
		name    string
		content string
	}{
		{"README.md", "# Custom Archiver Example\n\nDemonstrates implementing custom archive formats."},
		{"config.json", `{"archiver": "zip", "compression": "deflate", "custom": true}`},
		{"script.sh", "#!/bin/bash\necho 'Hello from custom ZIP archiver!'"},
	}

	for _, file := range files {
		path := filepath.Join(dir, file.name)
		if err := os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", file.name, err)
		}
	}

	return nil
}
