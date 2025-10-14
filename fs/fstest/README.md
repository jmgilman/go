# fstest - Filesystem Conformance Test Suite

[![Go Reference](https://pkg.go.dev/badge/github.com/jmgilman/go/fs/fstest.svg)](https://pkg.go.dev/github.com/jmgilman/go/fs/fstest)

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
  - [Basic Integration](#basic-integration)
  - [Testing Specific Providers](#testing-specific-providers)
- [Usage Patterns](#usage-patterns)
  - [Selective Testing](#selective-testing)
  - [OpenFile Flags Configuration](#openfile-flags-configuration)
  - [Testing Optional Interfaces](#testing-optional-interfaces)
- [Test Functions Reference](#test-functions-reference)
  - [Core Test Functions](#core-test-functions)
  - [Optional Interface Tests](#optional-interface-tests)
  - [File Capability Tests](#file-capability-tests)
- [Testing Philosophy](#testing-philosophy)
  - [What We Test](#what-we-test)
  - [Example: Permission Handling](#example-permission-handling)
  - [Example: Optional Interface Detection](#example-optional-interface-detection)
- [Provider Requirements](#provider-requirements)
- [Expected Errors](#expected-errors)
- [Architecture](#architecture)
- [Coverage Goals](#coverage-goals)
- [Security Testing](#security-testing)
  - [Chroot Boundary Enforcement](#chroot-boundary-enforcement)
  - [Path Handling](#path-handling)
- [Related Packages](#related-packages)
- [Design Documentation](#design-documentation)
- [Contributing](#contributing)
- [License](#license)
- [References](#references)

## Overview

The `fstest` package provides a conformance test suite for validating filesystem provider implementations against the `core.FS` interface contracts. This package helps ensure that different filesystem backends (local, memory, S3, etc.) correctly implement the unified filesystem abstraction defined by the `core.FS` interface.

**Key Features:**
- Comprehensive test coverage for all `core.FS` interface methods
- Support for optional interfaces (MetadataFS, SymlinkFS, TempFS)
- File capability testing (Seeker, ReaderAt, WriterAt, etc.)
- Configurable flag support testing
- Security testing (chroot boundary enforcement)
- Simple integration with just one function call

**Design Philosophy:**

The test suite validates **interface contracts**, not backend-specific behavior. Different providers have different capabilities (e.g., S3 ignores permission parameters, some providers don't support symlinks), and the tests verify that all providers honor the interface contract while gracefully handling documented differences.

## Installation

```bash
go get github.com/jmgilman/go/fs/fstest
```

## Quick Start

### Basic Integration

The simplest way to test your filesystem implementation is to use `TestSuite`, which runs all applicable conformance tests:

```go
package myprovider_test

import (
    "testing"
    "github.com/jmgilman/go/fs/core"
    "github.com/jmgilman/go/fs/fstest"
    "github.com/jmgilman/go/fs/myprovider"
)

func TestMyProvider(t *testing.T) {
    fstest.TestSuite(t, func() core.FS {
        return myprovider.New()
    })
}
```

The `newFS` function should return a fresh, empty filesystem instance for each test. Tests will create and modify files, so each invocation should start with a clean state.

### Testing Specific Providers

**Local Filesystem Example:**
```go
func TestAferoLocalFS(t *testing.T) {
    fstest.TestSuite(t, func() core.FS {
        return afero.NewLocal()
    })
}
```

**Memory Filesystem Example:**
```go
func TestAferoMemoryFS(t *testing.T) {
    fstest.TestSuite(t, func() core.FS {
        return afero.NewMemory()
    })
}
```

## Usage Patterns

### Selective Testing

If you only want to test specific functionality, you can call individual test functions:

```go
func TestMyProviderReadOnly(t *testing.T) {
    fs := myprovider.NewReadOnly()

    // Only test read operations
    fstest.TestReadFS(t, fs)
}

func TestMyProviderWalkDirectory(t *testing.T) {
    fs := myprovider.New()

    // Only test directory traversal
    fstest.TestWalkFS(t, fs)
}
```

### OpenFile Flags Configuration

Different providers support different OpenFile flags. Use `TestOpenFileFlags` to test provider-specific flag support:

```go
func TestAferoOpenFileFlags(t *testing.T) {
    fs := afero.NewMemory()

    // Specify which flags your provider supports
    supportedFlags := []int{
        os.O_RDONLY, os.O_WRONLY, os.O_RDWR,
        os.O_CREATE, os.O_TRUNC, os.O_APPEND, os.O_EXCL,
    }

    fstest.TestOpenFileFlags(t, fs, supportedFlags)
}
```

The test will verify that:
- All supported flags work correctly
- Unsupported flags return `core.ErrUnsupported`

### Testing Optional Interfaces

Some tests automatically detect and test optional interfaces:

```go
func TestMyProviderOptionalFeatures(t *testing.T) {
    fs := myprovider.New()

    // These tests will skip gracefully if the interface is not implemented
    fstest.TestMetadataFS(t, fs)  // Tests Chmod, Chtimes, Lstat
    fstest.TestSymlinkFS(t, fs)   // Tests Symlink, Readlink
    fstest.TestTempFS(t, fs)      // Tests TempFile, TempDir
}
```

## Test Functions Reference

### Core Test Functions

These test the mandatory `core.FS` interface methods:

#### `TestSuite(t *testing.T, newFS func() core.FS)`

Runs all applicable conformance tests against a filesystem. This is the recommended entry point for most providers.

**What it tests:**
- All read, write, and management operations
- Directory traversal (Walk)
- Security boundaries (Chroot)
- Optional interfaces (MetadataFS, SymlinkFS, TempFS)
- File capabilities (Seeker, ReaderAt, etc.)

**Note:** `TestOpenFileFlags` is not included in `TestSuite` because it requires provider-specific flag configuration. Call it separately with your supported flags.

#### `TestReadFS(t *testing.T, fs core.FS)`

Tests read-only operations:
- `Open` - Opening existing files
- `Stat` - Getting file/directory information
- `ReadDir` - Listing directory contents
- `ReadFile` - Reading entire file contents

**Tests:**
- Successful read operations
- Error handling for non-existent files (`fs.ErrNotExist`)
- Directory and file stat operations

#### `TestWriteFS(t *testing.T, fs core.FS)`

Tests write operations:
- `Create` - Creating new files
- `OpenFile` - Opening files with various flags
- `WriteFile` - Writing complete file contents
- `Mkdir` - Creating single directories
- `MkdirAll` - Creating nested directory structures

**Tests:**
- File and directory creation
- Writing and verifying data
- Error handling for invalid operations

#### `TestManageFS(t *testing.T, fs core.FS)`

Tests file management operations:
- `Remove` - Deleting files and empty directories
- `RemoveAll` - Recursive deletion
- `Rename` - Moving/renaming files and directories

**Tests:**
- Single file/directory removal
- Recursive removal of directory trees
- Renaming operations
- Error handling for non-existent files

#### `TestWalkFS(t *testing.T, fs core.FS)`

Tests directory tree traversal:
- `Walk` - Recursive directory traversal

**Tests:**
- Correct ordering of traversal
- Path handling (absolute and relative)
- Walking empty directories
- Walking nested directory structures

#### `TestChrootFS(t *testing.T, fs core.FS)`

Tests scoped filesystem views and security:
- `Chroot` - Creating restricted filesystem views

**Tests:**
- Operations stay within chroot boundary
- Path traversal attack prevention (`../` escapes)
- Nested chroot operations
- Security boundary enforcement

#### `TestOpenFileFlags(t *testing.T, fs core.FS, supportedFlags []int)`

Tests OpenFile flag support with provider-specific configuration.

**Parameters:**
- `fs` - The filesystem to test
- `supportedFlags` - List of flags your provider supports (e.g., `os.O_RDONLY`, `os.O_CREATE`)

**Tests:**
- Each supported flag works correctly
- Unsupported flags return `core.ErrUnsupported`
- Common flag combinations (e.g., `O_CREATE | O_TRUNC`)

**Example supported flags:**
```go
// Full-featured provider (local, memory)
supportedFlags := []int{
    os.O_RDONLY, os.O_WRONLY, os.O_RDWR,
    os.O_CREATE, os.O_TRUNC, os.O_APPEND, os.O_EXCL,
}

// Limited provider (S3)
supportedFlags := []int{
    os.O_RDONLY,  // Read-only
    os.O_WRONLY,  // Write-only
    os.O_CREATE,  // Create new files
    os.O_TRUNC,   // Truncate on open
}
```

### Optional Interface Tests

These test optional FS-level interfaces. They automatically skip if the interface is not implemented:

#### `TestMetadataFS(t *testing.T, fs core.FS)`

Tests metadata operations (optional `core.MetadataFS` interface):
- `Chmod` - Changing file permissions
- `Chtimes` - Changing file timestamps
- `Lstat` - Getting file info without following symlinks

**Skips if:** Filesystem doesn't implement `core.MetadataFS`

**Typical support:**
- ✅ Local filesystems
- ✅ Memory filesystems
- ❌ S3 backends (no POSIX permissions)

#### `TestSymlinkFS(t *testing.T, fs core.FS)`

Tests symbolic link operations (optional `core.SymlinkFS` interface):
- `Symlink` - Creating symbolic links
- `Readlink` - Reading symlink targets

**Skips if:** Filesystem doesn't implement `core.SymlinkFS`

**Typical support:**
- ✅ Local filesystems
- ❌ Memory filesystems (usually)
- ❌ S3 backends

#### `TestTempFS(t *testing.T, fs core.FS)`

Tests temporary file operations (optional `core.TempFS` interface):
- `TempFile` - Creating temporary files with unique names
- `TempDir` - Creating temporary directories with unique names

**Skips if:** Filesystem doesn't implement `core.TempFS`

**Typical support:**
- ✅ Local filesystems
- ✅ Memory filesystems
- ❌ S3 backends (usually)

### File Capability Tests

#### `TestFileCapabilities(t *testing.T, fs core.FS)`

Tests optional File-level capabilities using type assertions.

**Capabilities tested:**
- **`io.Seeker`** - Seeking to different file positions
- **`io.ReaderAt`** - Reading from specific offsets
- **`io.WriterAt`** - Writing to specific offsets
- **`core.Truncater`** - Truncating file size
- **`core.Syncer`** - Syncing writes to storage
- **`fs.ReadDirFile`** - Reading directory entries from directory handle

**How it works:**
- Opens test files and directories
- Attempts type assertion for each capability
- Tests the capability if supported
- Skips gracefully if not supported

**Typical support matrix:**

| Capability  | Local | Memory | S3 Read | S3 Write |
| ----------- | ----- | ------ | ------- | -------- |
| Seeker      | ✅    | ✅     | ✅      | ❌       |
| ReaderAt    | ✅    | ✅     | ✅      | ❌       |
| WriterAt    | ✅    | ✅     | ❌      | ❌       |
| Truncater   | ✅    | ✅     | ❌      | ❌       |
| Syncer      | ✅    | ✅     | ✅      | ✅       |
| ReadDirFile | ✅    | ✅     | Varies  | N/A      |

## Testing Philosophy

### What We Test

✅ **Interface contracts:**
- Method signatures are correct
- Basic functionality works (write data, read it back)
- Error handling is appropriate
- Core operations complete successfully

❌ **What we don't test:**
- Backend-specific behavior
- Performance characteristics
- Implementation details (buffering, caching, etc.)

### Example: Permission Handling

The `WriteFile` method accepts a `perm` parameter:

```go
err := fs.WriteFile("test.txt", []byte("data"), 0644)
```

**Test behavior:**
- ✅ Verifies the method succeeds and file is created
- ❌ Does NOT verify the file has mode 0644 on disk

**Rationale:**
- Local/Memory providers apply permissions
- S3 providers ignore permissions (uses IAM policies)
- The test validates the interface contract, not backend implementation

### Example: Optional Interface Detection

```go
// TestMetadataFS automatically detects support
if mfs, ok := fs.(core.MetadataFS); ok {
    // Provider supports it, run tests
    mfs.Chmod("file.txt", 0600)
} else {
    // Provider doesn't support it, skip gracefully
    t.Skip("MetadataFS not supported")
}
```

## Provider Requirements

To use the fstest suite, your provider must:

1. **Implement `core.FS` interface** - All mandatory methods
2. **Provide fresh instances** - Each call to `newFS()` should return an isolated filesystem
3. **Start empty** - Tests create their own test data
4. **Handle cleanup** - Tests may create files; fresh instances prevent state leakage

**Example provider setup:**

```go
func TestMyProvider(t *testing.T) {
    fstest.TestSuite(t, func() core.FS {
        // Return a fresh, empty filesystem instance
        // Each test gets its own isolated instance
        return myprovider.New()
    })
}
```

## Expected Errors

The test suite verifies that your provider returns appropriate errors:

- **`fs.ErrNotExist`** - File or directory not found
- **`fs.ErrExist`** - File or directory already exists
- **`fs.ErrInvalid`** - Invalid operation or parameter
- **`fs.ErrPermission`** - Permission denied (if applicable)
- **`fs.ErrClosed`** - Operation on closed file
- **`core.ErrUnsupported`** - Operation not supported by provider

## Architecture

The fstest package follows a simple, single-file design:

```
fstest/
├── go.mod          # Module definition
├── suite.go        # All test functions (~3000 LOC)
└── README.md       # This file
```

**Design rationale:**
- No external dependencies (stdlib + core only)
- Simple integration (import and call)
- Clear test organization (one function per capability)
- Self-contained test suite

## Coverage Goals

The test suite aims to:
- Cover all `core.FS` interface methods
- Test both success and error cases
- Verify edge cases (empty files, nested directories, special characters)
- Test security boundaries (path traversal prevention)
- Validate optional interface support

**Target coverage:** 80%+ of provider code when using the full suite

## Security Testing

The test suite includes security-focused tests:

### Chroot Boundary Enforcement

```go
fstest.TestChrootFS(t, fs)
```

**Tests:**
- Path traversal attack prevention
- `../` escape attempts fail securely
- Operations stay within chroot boundary
- Nested chroot operations work correctly

### Path Handling

- Special characters in filenames
- Path normalization
- Absolute vs relative paths
- Empty and invalid paths

## Related Packages

- **`fs/core`** - Core filesystem interfaces and types
- **`fs/afero`** - Afero-based filesystem providers (local, memory)
- **`fs/billy`** - Billy-based filesystem providers
- **`fs/awsv2`** - AWS S3 filesystem provider (AWS SDK v2)
- **`fs/minio`** - MinIO/S3-compatible filesystem provider

## Contributing

When adding new tests:
1. Follow existing test patterns
2. Use `t.Run()` for subtests
3. Provide clear error messages with context
4. Document expected behavior
5. Handle optional interfaces gracefully with type assertions

## License

See the repository root for license information.

## References

- [Go testing package](https://pkg.go.dev/testing) - Standard testing framework
- [Go io/fs package](https://pkg.go.dev/io/fs) - Filesystem interfaces
- [testing/fstest](https://pkg.go.dev/testing/fstest) - Stdlib filesystem testing utilities
- [core.FS interface](https://pkg.go.dev/github.com/jmgilman/go/fs/core) - The interface being tested
