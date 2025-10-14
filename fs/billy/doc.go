// Package billy provides a go-billy-backed filesystem implementation
// of the core.FS interface, enabling go-git compatibility.
//
// This package wraps go-billy's osfs (local) and memfs (in-memory)
// implementations, providing a thin adapter layer that implements
// core.FS while maintaining the ability to access the underlying
// billy.Filesystem for go-git integration.
//
// Usage:
//
//	// Create local filesystem
//	fs := billy.NewLocal()
//
//	// Use with core.FS interface
//	data, err := fs.ReadFile("config.json")
//
//	// Unwrap for go-git integration
//	billyFS := fs.Unwrap()
//	repo, err := git.Clone(storage, billyFS, &git.CloneOptions{...})
//
// # Memory Filesystem
//
// For testing or temporary storage, use the in-memory filesystem:
//
//	fs := billy.NewMemory()
//	err := fs.WriteFile("temp.txt", []byte("data"), 0644)
//
// # Thread Safety
//
// FS instances (LocalFS, MemoryFS) are safe for concurrent use by
// multiple goroutines. File handles are not safe for concurrent use.
package billy
