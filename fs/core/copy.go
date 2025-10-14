package core

import (
	"io/fs"
	"path"
	"strings"
)

// CopyFromEmbedFS copies all files from a read-only filesystem (typically embed.FS)
// into a writable core.FS, preserving the directory structure.
//
// The srcRoot parameter specifies the root directory in the source filesystem to copy from.
// Use "." to copy the entire source filesystem.
//
// This function:
//   - Creates all necessary parent directories using MkdirAll
//   - Preserves file permissions from the source filesystem
//   - Uses ReadFile/WriteFile for optimal performance with embedded content
//   - Skips directory entries (only files are copied)
//
// Example:
//
//	//go:embed templates/*
//	var templatesFS embed.FS
//
//	memFS := billy.NewMemory()
//	err := core.CopyFromEmbedFS(templatesFS, memFS, "templates")
func CopyFromEmbedFS(src fs.FS, dst FS, srcRoot string) error {
	return fs.WalkDir(src, srcRoot, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories - they'll be created automatically by MkdirAll
		if d.IsDir() {
			return nil
		}

		// Read file contents from source
		data, err := fs.ReadFile(src, filePath)
		if err != nil {
			return err
		}

		// Get file info for permissions
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Calculate destination path relative to srcRoot
		dstPath := filePath
		if srcRoot != "." && srcRoot != "" {
			// Strip srcRoot prefix to get relative path
			dstPath = strings.TrimPrefix(filePath, srcRoot)
			dstPath = strings.TrimPrefix(dstPath, "/") // Remove leading slash if present
		}

		// Ensure parent directory exists
		if dir := path.Dir(dstPath); dir != "." && dir != "" {
			if err := dst.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}

		// Write file to destination
		return dst.WriteFile(dstPath, data, info.Mode().Perm())
	})
}
