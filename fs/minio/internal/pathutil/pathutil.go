// Package pathutil provides path normalization and manipulation utilities
// for MinIO/S3 object keys.
package pathutil

import (
	"path/filepath"
	"strings"
)

// Normalize cleans a path and ensures forward slashes.
// It applies: ToSlash → Clean → Trim slashes
// Returns "." for empty paths.
func Normalize(path string) string {
	if path == "" {
		return "."
	}

	// First convert backslashes to forward slashes (for Windows-style paths)
	path = strings.ReplaceAll(path, "\\", "/")

	// Clean the path (resolves . and ..)
	path = filepath.Clean(path)

	// Convert to forward slashes again (filepath.Clean may use OS separator)
	path = filepath.ToSlash(path)

	// Trim leading and trailing slashes
	path = strings.Trim(path, "/")

	// Return "." if path is now empty
	if path == "" {
		return "."
	}

	return path
}

// NormalizePrefix normalizes the prefix path:
// - Converts backslashes to forward slashes
// - Removes leading and trailing slashes
// - Returns empty string if prefix is "." or empty.
func NormalizePrefix(prefix string) string {
	if prefix == "" || prefix == "." {
		return ""
	}

	// First convert backslashes to forward slashes (for Windows-style paths)
	prefix = strings.ReplaceAll(prefix, "\\", "/")

	// Clean the path (resolves . and ..)
	prefix = filepath.Clean(prefix)

	// Convert to forward slashes again (filepath.Clean may use OS separator)
	prefix = filepath.ToSlash(prefix)

	// Trim leading and trailing slashes
	prefix = strings.Trim(prefix, "/")

	return prefix
}

// JoinPath joins a prefix with a name to create a full S3 key.
// It handles empty prefix correctly and uses forward slashes.
func JoinPath(prefix, name string) string {
	name = Normalize(name)

	// Handle special case where normalized name is "."
	if name == "." {
		if prefix == "" {
			return ""
		}
		return prefix
	}

	if prefix == "" {
		return name
	}

	return prefix + "/" + name
}

// BuildEntryKey constructs the S3 key for an entry given its parent key and name.
func BuildEntryKey(parentKey, entryName string) string {
	if parentKey != "" {
		return parentKey + "/" + entryName
	}
	return entryName
}
