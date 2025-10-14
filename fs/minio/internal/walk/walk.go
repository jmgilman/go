// Package walk provides helper functions for directory tree walking.
package walk

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/jmgilman/go/fs/minio/internal/pathutil"
)

// DirFunc is a function type that can recursively walk a directory.
// This allows the walk helpers to call back into the main filesystem's walkDir method.
type DirFunc func(name, key string, walkFn fs.WalkDirFunc) error

// ProcessEntry processes a single entry during directory walking.
// It determines whether the entry is a file or directory and calls the appropriate handler.
func ProcessEntry(
	parentName, parentKey string,
	entry fs.DirEntry,
	walkFn fs.WalkDirFunc,
	walkDir DirFunc,
) error {
	entryPath := filepath.Join(parentName, entry.Name())
	entryKey := pathutil.BuildEntryKey(parentKey, entry.Name())

	if entry.IsDir() {
		return ProcessDirectory(entryPath, entryKey, walkFn, walkDir)
	}

	return ProcessFile(entryPath, entry, walkFn)
}

// ProcessDirectory handles walking into a subdirectory.
func ProcessDirectory(entryPath, entryKey string, walkFn fs.WalkDirFunc, walkDir DirFunc) error {
	err := walkDir(entryPath, entryKey, walkFn)
	if errors.Is(err, fs.SkipDir) {
		return nil // Skip this directory, continue with siblings
	}
	return err
}

// ProcessFile handles calling walkFn for a file entry.
func ProcessFile(entryPath string, entry fs.DirEntry, walkFn fs.WalkDirFunc) error {
	err := walkFn(entryPath, entry, nil)
	if errors.Is(err, fs.SkipDir) {
		return nil // SkipDir on a file means stop walking
	}
	return err
}
