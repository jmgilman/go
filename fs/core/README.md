# fs/core

[![Go Reference](https://pkg.go.dev/badge/github.com/jmgilman/go/fs/core.svg)](https://pkg.go.dev/github.com/jmgilman/go/fs/core)
[![Go Report Card](https://goreportcard.com/badge/github.com/jmgilman/go/fs/core)](https://goreportcard.com/report/github.com/jmgilman/go/fs/core)

**Foundational interfaces and types for multi-provider filesystem abstraction.**

The `fs/core` package defines the contracts that filesystem providers must implement, enabling applications to write filesystem-agnostic code that works seamlessly with local filesystems, in-memory filesystems, and cloud storage (S3) through a unified interface.

## Features

- **Zero Dependencies** - Uses only the Go standard library
- **Stdlib Compatible** - Extends `fs.FS` and `fs.File` rather than replacing them
- **Interface Composition** - Small, focused interfaces compose into rich contracts
- **Optional Capabilities** - Use type assertions for provider-specific features
- **Pure Interfaces** - No implementation logic, minimal overhead

## Installation

```bash
go get github.com/jmgilman/go/fs/core
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/jmgilman/go/fs/core"
)

// Write filesystem-agnostic code
func ProcessFiles(filesystem core.FS) error {
    // Read a file
    data, err := filesystem.ReadFile("config.json")
    if err != nil {
        return err
    }

    // Process data...
    result := process(data)

    // Write results
    return filesystem.WriteFile("output.json", result, 0644)
}
```

## Interface Hierarchy

The main `FS` interface is composed of five sub-interfaces:

```
FS (main interface)
├── fs.FS              (stdlib compatibility)
├── ReadFS             (Open, Stat, ReadDir, ReadFile)
├── WriteFS            (Create, OpenFile, WriteFile, Mkdir, MkdirAll)
├── ManageFS           (Remove, RemoveAll, Rename)
├── WalkFS             (Walk)
└── ChrootFS           (Chroot)
```

### Optional Interfaces

Providers may implement these interfaces for additional capabilities:

- **MetadataFS** - Metadata operations (Lstat, Chmod, Chtimes)
- **SymlinkFS** - Symbolic link operations (Symlink, Readlink)
- **TempFS** - Temporary file operations (TempFile, TempDir)

### File Interface

The `File` interface extends `fs.File` with write capabilities:

```go
type File interface {
    fs.File    // Read, Close, Stat
    io.Writer  // Write
    Name() string
}
```

## Usage Examples

### Basic Operations

```go
func Example(fs core.FS) error {
    // Create a directory
    if err := fs.MkdirAll("data/logs", 0755); err != nil {
        return err
    }

    // Write a file
    content := []byte("Hello, World!")
    if err := fs.WriteFile("data/hello.txt", content, 0644); err != nil {
        return err
    }

    // Read it back
    data, err := fs.ReadFile("data/hello.txt")
    if err != nil {
        return err
    }

    // Walk directory tree
    return fs.Walk("data", func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        fmt.Println(path)
        return nil
    })
}
```

### Checking Optional Capabilities

```go
func UseMetadata(filesystem core.FS) error {
    // Check if filesystem supports metadata operations
    if mfs, ok := filesystem.(core.MetadataFS); ok {
        return mfs.Chmod("file.txt", 0600)
    }
    return core.ErrUnsupported
}

func UseSymlinks(filesystem core.FS) error {
    // Check if filesystem supports symlinks
    if sfs, ok := filesystem.(core.SymlinkFS); ok {
        return sfs.Symlink("target.txt", "link.txt")
    }
    return core.ErrUnsupported
}
```

### Stdlib Compatibility

The `FS` interface embeds `fs.FS`, making it compatible with standard library functions:

```go
import (
    "io/fs"
    "github.com/jmgilman/go/fs/core"
)

func UseStdlib(filesystem core.FS) error {
    // Works with stdlib fs.WalkDir
    return fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        fmt.Println(path)
        return nil
    })
}
```

### Error Handling

```go
import (
    "errors"
    "github.com/jmgilman/go/fs/core"
)

func HandleErrors(filesystem core.FS) {
    data, err := filesystem.ReadFile("missing.txt")
    if err != nil {
        if errors.Is(err, core.ErrNotExist) {
            // File doesn't exist
        } else if errors.Is(err, core.ErrPermission) {
            // Permission denied
        } else if errors.Is(err, core.ErrUnsupported) {
            // Operation not supported by this provider
        }
    }
}
```

## Error Types

The package provides these error variables:

```go
var (
    ErrNotExist   = fs.ErrNotExist   // File/directory not found
    ErrExist      = fs.ErrExist      // Already exists
    ErrPermission = fs.ErrPermission // Permission denied
    ErrClosed     = fs.ErrClosed     // Operation on closed file
    ErrUnsupported                   // Operation not supported
)
```

## Design Philosophy

1. **"Accept Interfaces, Return Structs"** - This module defines interfaces; providers return concrete types
2. **Stdlib Compatibility** - Embeds `fs.FS` and `fs.File` for seamless integration with existing code
3. **Interface Composition** - Small, focused interfaces combine to form larger contracts
4. **Zero Implementation** - Pure interface definitions with no business logic
5. **Dependency-Free** - Will remain free of external dependencies forever

## Provider Implementations

This package contains only interface definitions. Concrete implementations are provided by:

- [`fs/afero`](../afero) - Afero-backed providers (local, memory)
- [`fs/billy`](../billy) - go-billy-backed providers
- [`fs/awsv2`](../awsv2) - AWS SDK v2 S3 provider
- [`fs/minio`](../minio) - MinIO S3 provider

## Testing

The module includes example tests that serve dual purposes:

1. **Validation** - Ensure interfaces compile and can be implemented
2. **Documentation** - Provide clear usage examples in generated godoc

Run tests:

```bash
go test ./...
```

View examples:

```bash
go test -v -run Example
```

## Documentation

Full API documentation is available at:

- [pkg.go.dev](https://pkg.go.dev/github.com/jmgilman/go/fs/core)

Or view locally:

```bash
go doc -all
```
