package core_test

import (
	"fmt"
	"io/fs"

	"github.com/jmgilman/go/fs/core"
)

// ExampleFS demonstrates the FS interface structure.
// This example shows the interface composition pattern used in fs/core.
func ExampleFS() {
	// The FS interface is composed of multiple sub-interfaces:
	// - fs.FS (stdlib compatibility)
	// - ReadFS (read operations)
	// - WriteFS (write operations)
	// - ManageFS (management operations)
	// - WalkFS (directory traversal)
	// - ChrootFS (scoped views)

	// In practice, you would get an actual implementation like:
	// filesystem := local.New("/path/to/root")
	// or
	// filesystem := memory.New()

	fmt.Println("FS interface composes multiple focused interfaces")
	// Output: FS interface composes multiple focused interfaces
}

// Example_interfaceComposition demonstrates how the interfaces compose.
func Example_interfaceComposition() {
	// The design uses interface composition to allow flexible implementations.
	// A type that implements FS must implement all sub-interfaces.

	fmt.Println("Interface composition enables modular design")
	// Output: Interface composition enables modular design
}

// Example_readOperationsSignature demonstrates the ReadFS interface signature.
func Example_readOperationsSignature() {
	// ReadFS defines five read-only operations:
	// - Open(name string) (fs.File, error)
	// - Stat(name string) (fs.FileInfo, error)
	// - ReadDir(name string) ([]fs.DirEntry, error)
	// - ReadFile(name string) ([]byte, error)
	// - Exists(name string) (bool, error)

	fmt.Println("ReadFS provides read-only operations")
	// Output: ReadFS provides read-only operations
}

// Example_writeOperationsSignature demonstrates the WriteFS interface signature.
func Example_writeOperationsSignature() {
	// WriteFS defines five write operations:
	// - Create(name string) (File, error)
	// - OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	// - WriteFile(name string, data []byte, perm fs.FileMode) error
	// - Mkdir(name string, perm fs.FileMode) error
	// - MkdirAll(path string, perm fs.FileMode) error

	fmt.Println("WriteFS provides write operations")
	// Output: WriteFS provides write operations
}

// Example_manageOperationsSignature demonstrates the ManageFS interface signature.
func Example_manageOperationsSignature() {
	// ManageFS defines three management operations:
	// - Remove(name string) error
	// - RemoveAll(path string) error
	// - Rename(oldpath, newpath string) error

	fmt.Println("ManageFS provides file management operations")
	// Output: ManageFS provides file management operations
}

// Example_walkOperationsSignature demonstrates the WalkFS interface signature.
func Example_walkOperationsSignature() {
	// WalkFS defines directory traversal:
	// - Walk(root string, walkFn fs.WalkDirFunc) error

	fmt.Println("WalkFS provides directory tree traversal")
	// Output: WalkFS provides directory tree traversal
}

// Example_chrootOperationsSignature demonstrates the ChrootFS interface signature.
func Example_chrootOperationsSignature() {
	// ChrootFS defines scoped filesystem views:
	// - Chroot(dir string) (FS, error)

	fmt.Println("ChrootFS provides scoped filesystem views")
	// Output: ChrootFS provides scoped filesystem views
}

// Example_fileInterface demonstrates the File interface.
func Example_fileInterface() {
	// File embeds fs.File and adds:
	// - io.Writer (Write method)
	// - Name() string

	// Optional capabilities can be checked via type assertions:
	// - io.Seeker
	// - io.ReaderAt
	// - io.WriterAt
	// - Truncater
	// - Syncer
	// - fs.ReadDirFile

	fmt.Println("File extends fs.File with write capabilities")
	// Output: File extends fs.File with write capabilities
}

// Example_optionalInterfaces demonstrates optional filesystem capabilities.
func Example_optionalInterfaces() {
	// Optional interfaces allow provider-specific features:
	//
	// MetadataFS - metadata operations (local/memory only)
	// - Lstat(name string) (fs.FileInfo, error)
	// - Chmod(name string, mode fs.FileMode) error
	// - Chtimes(name string, atime, mtime time.Time) error
	//
	// SymlinkFS - symlink operations (local only)
	// - Symlink(oldname, newname string) error
	// - Readlink(name string) (string, error)
	//
	// TempFS - temporary files (local/memory only)
	// - TempFile(dir, pattern string) (File, error)
	// - TempDir(dir, pattern string) (string, error)

	fmt.Println("Optional interfaces enable provider-specific features")
	// Output: Optional interfaces enable provider-specific features
}

// Example_capabilityChecking demonstrates how to check for optional capabilities.
func Example_capabilityChecking() {
	// Type assertion pattern for checking capabilities:
	//
	// if mfs, ok := filesystem.(core.MetadataFS); ok {
	//     // Filesystem supports metadata operations
	//     _ = mfs.Chmod("file.txt", 0600)
	// }
	//
	// if sfs, ok := filesystem.(core.SymlinkFS); ok {
	//     // Filesystem supports symlinks
	//     _ = sfs.Symlink("target", "link")
	// }

	fmt.Println("Use type assertions to check optional capabilities")
	// Output: Use type assertions to check optional capabilities
}

// Example_stdlibCompatibility demonstrates stdlib compatibility.
func Example_stdlibCompatibility() {
	// FS embeds fs.FS, making it compatible with stdlib functions:
	//
	// - fs.WalkDir(filesystem, root, walkFn)
	// - fs.ReadFile(filesystem, name)
	// - fs.Stat(filesystem, name)
	// - fs.ReadDir(filesystem, name)
	//
	// This allows using core.FS with any stdlib function that accepts fs.FS

	fmt.Println("FS is compatible with stdlib fs package")
	// Output: FS is compatible with stdlib fs package
}

// Example_polymorphicUsage demonstrates writing filesystem-agnostic code.
func Example_polymorphicUsage() {
	// Functions can accept core.FS and work with any implementation:
	//
	// func ProcessFiles(filesystem core.FS) error {
	//     data, err := filesystem.ReadFile("input.txt")
	//     if err != nil {
	//         return err
	//     }
	//     return filesystem.WriteFile("output.txt", processed, 0644)
	// }
	//
	// This works with local, memory, or S3 implementations

	fmt.Println("Write once, use with any filesystem implementation")
	// Output: Write once, use with any filesystem implementation
}

// Example_errorHandling demonstrates the error types available.
func Example_errorHandling() {
	// Standard error types from stdlib (re-exported):
	// - core.ErrNotExist   (file/directory not found)
	// - core.ErrExist      (already exists)
	// - core.ErrPermission (permission denied)
	// - core.ErrClosed     (operation on closed file)
	//
	// Custom error:
	// - core.ErrUnsupported (operation not available for this provider)
	//
	// Use errors.Is() to check error types:
	// if errors.Is(err, core.ErrNotExist) { ... }

	fmt.Println("Use standard error types for consistent error handling")
	// Output: Use standard error types for consistent error handling
}

// Example_existsUsage demonstrates how to use the Exists method.
func Example_existsUsage() {
	// The Exists method checks if a file or directory exists:
	//
	// exists, err := filesystem.Exists("config.yaml")
	// if err != nil {
	//     // Error occurred while checking (e.g., permission denied)
	//     return err
	// }
	// if !exists {
	//     // File definitely doesn't exist
	//     return fmt.Errorf("config file not found")
	// }
	//
	// Important: Always check the error! A false result with an error
	// means the existence could not be determined, not that the file
	// doesn't exist.
	//
	// Common pattern for handling missing files:
	// exists, err := filesystem.Exists("optional.txt")
	// if err != nil {
	//     return err
	// }
	// if exists {
	//     // File exists, read it
	//     data, _ := filesystem.ReadFile("optional.txt")
	//     _ = data
	// } else {
	//     // File doesn't exist, use defaults
	// }

	fmt.Println("Exists checks if a file or directory exists")
	// Output: Exists checks if a file or directory exists
}

// mockFS is a minimal implementation for testing interface satisfaction.
type mockFS struct{}

func (m *mockFS) Open(_ string) (fs.File, error)          { return nil, nil }
func (m *mockFS) Stat(_ string) (fs.FileInfo, error)      { return nil, nil }
func (m *mockFS) ReadDir(_ string) ([]fs.DirEntry, error) { return nil, nil }
func (m *mockFS) ReadFile(_ string) ([]byte, error)       { return nil, nil }
func (m *mockFS) Exists(_ string) (bool, error)           { return false, nil }
func (m *mockFS) Create(_ string) (core.File, error)      { return nil, nil }
func (m *mockFS) OpenFile(_ string, _ int, _ fs.FileMode) (core.File, error) {
	return nil, nil
}
func (m *mockFS) WriteFile(_ string, _ []byte, _ fs.FileMode) error { return nil }
func (m *mockFS) Mkdir(_ string, _ fs.FileMode) error               { return nil }
func (m *mockFS) MkdirAll(_ string, _ fs.FileMode) error            { return nil }
func (m *mockFS) Remove(_ string) error                             { return nil }
func (m *mockFS) RemoveAll(_ string) error                          { return nil }
func (m *mockFS) Rename(_, _ string) error                          { return nil }
func (m *mockFS) Walk(_ string, _ fs.WalkDirFunc) error             { return nil }
func (m *mockFS) Chroot(_ string) (core.FS, error)                  { return nil, nil }
func (m *mockFS) Type() core.FSType                                 { return core.FSTypeUnknown }

// Example_implementingFS demonstrates that a type can implement the FS interface.
func Example_implementingFS() {
	// This demonstrates that mockFS satisfies the core.FS interface
	var filesystem core.FS = &mockFS{}

	// The filesystem can now be used polymorphically
	_ = filesystem

	fmt.Println("Custom types can implement the FS interface")
	// Output: Custom types can implement the FS interface
}

// Example_interfaceSegregation demonstrates interface segregation principle.
func Example_interfaceSegregation() {
	// Code can depend on just the capabilities it needs:
	//
	// func ReadConfig(rfs core.ReadFS) error {
	//     // Only needs read operations
	//     return nil
	// }
	//
	// func WriteOutput(wfs core.WriteFS) error {
	//     // Only needs write operations
	//     return nil
	// }
	//
	// Both functions can accept a full core.FS or just the specific interface

	fmt.Println("Depend on the smallest interface needed")
	// Output: Depend on the smallest interface needed
}
