// Package core provides the foundational interfaces and types for a
// multi-provider filesystem abstraction.
//
// This package defines contracts that filesystem providers must implement,
// enabling applications to write filesystem-agnostic code that works with
// local filesystems, in-memory filesystems, and cloud storage (S3) through
// a unified interface.
//
// # Design Philosophy
//
// The core package follows these principles:
//
//   - Zero dependencies: Only uses Go standard library
//   - Interface composition: Small focused interfaces compose into larger contracts
//   - Stdlib compatibility: Extends fs.FS and fs.File rather than replacing them
//   - Optional capabilities: Use type assertions for provider-specific features
//
// # Interface Hierarchy
//
// The main FS interface is composed of five sub-interfaces:
//
//   - ReadFS: Read-only operations (Open, Stat, ReadDir, ReadFile)
//   - WriteFS: Write operations (Create, OpenFile, WriteFile, Mkdir)
//   - ManageFS: File management (Remove, RemoveAll, Rename)
//   - WalkFS: Directory traversal (Walk)
//   - ChrootFS: Scoped filesystem views (Chroot)
//
// Optional interfaces for provider-specific capabilities:
//
//   - MetadataFS: Metadata operations (Lstat, Chmod, Chtimes)
//   - SymlinkFS: Symbolic link operations (Symlink, Readlink)
//   - TempFS: Temporary file operations (TempFile, TempDir)
//
// # Usage Example
//
//	import "github.com/jmgilman/go/fs/core"
//
//	func ProcessFiles(filesystem core.FS) error {
//	    data, err := filesystem.ReadFile("config.json")
//	    if err != nil {
//	        return err
//	    }
//	    // Process data...
//	    return filesystem.WriteFile("output.json", result, 0644)
//	}
//
// # Checking Optional Capabilities
//
//	if mfs, ok := filesystem.(core.MetadataFS); ok {
//	    mfs.Chmod("file.txt", 0600)
//	}
//
// # Stdlib Compatibility
//
// The FS interface embeds fs.FS, making it compatible with standard library
// functions like fs.WalkDir, fs.ReadFile, etc.
//
//	import "io/fs"
//
//	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
//	    fmt.Println(path)
//	    return nil
//	})
//
// # Provider Implementations
//
// This package contains only interface definitions. Concrete implementations
// are provided by separate provider modules:
//
//   - github.com/jmgilman/go/fs/afero - Afero-backed providers
//   - github.com/jmgilman/go/fs/billy - go-billy-backed providers
//   - github.com/jmgilman/go/fs/awsv2 - AWS SDK v2 S3 provider
//   - github.com/jmgilman/go/fs/minio - MinIO S3 provider
package core
