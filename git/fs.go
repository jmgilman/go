package git

import (
	"fmt"
	"strings"

	"github.com/go-git/go-billy/v5"
)

// isMemoryFilesystem checks if the given filesystem is memory-based.
// Memory-based filesystems (like memfs) cannot be used with git CLI operations
// since the CLI operates on the real filesystem.
//
// Returns true if the filesystem is memory-based, false otherwise.
func isMemoryFilesystem(fs billy.Filesystem) bool {
	// Check the type name - if it contains "mem", it's likely a memory filesystem
	typeName := fmt.Sprintf("%T", fs)
	return strings.Contains(strings.ToLower(typeName), "mem")
}
