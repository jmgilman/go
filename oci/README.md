# OCI Bundle Distribution Module

A secure, extensible Go library for distributing file bundles as OCI artifacts using ORAS. This module provides a simple API for pushing directories to and pulling archives from OCI registries.

## Features

- **eStargz Format**: Modern seekable tar.gz format with 100% backward compatibility
- **Selective Extraction**: Extract only needed files using glob patterns to save disk I/O and CPU
- **Secure by Default**: Prevents common vulnerabilities like path traversal and zip bombs
- **Extensible**: Support for multiple archive formats via interfaces
- **Flexible Authentication**: Multiple auth mechanisms (Docker config, static, custom functions)
- **Streaming**: Handles large files without memory exhaustion
- **ORAS Integration**: Uses ORAS v2 for OCI artifact operations
- **Progress Reporting**: Built-in progress callbacks for long operations
- **Retry Logic**: Automatic retry with exponential backoff
- **Thread Safe**: Safe for concurrent use

## Installation

```bash
go get github.com/jmgilman/go/oci
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/jmgilman/go/oci"
)

func main() {
    ctx := context.Background()

    // Create a client with default settings
    client, err := ocibundle.New()
    if err != nil {
        log.Fatal(err)
    }

    // Push a directory to an OCI registry
    err = client.Push(ctx, "./my-files", "ghcr.io/myorg/bundle:v1.0.0")
    if err != nil {
        log.Fatal(err)
    }

    // Pull and extract an OCI artifact
    err = client.Pull(ctx, "ghcr.io/myorg/bundle:v1.0.0", "./output")
    if err != nil {
        log.Fatal(err)
    }
}
```

## eStargz Format & Selective Extraction

### What is eStargz?

All archives created by this module use the **eStargz (extended stargz)** format, which provides:
- **100% backward compatibility** with standard tar.gz (can be extracted with `tar -xzf`)
- **Table of Contents (TOC)** for efficient file lookup without full decompression
- **Random access** capability for HTTP Range request optimizations
- **No additional dependencies** for basic tar.gz compatibility

### Selective File Extraction

Extract only the files you need instead of the entire archive:

```go
// Extract only JSON configuration files
err := client.Pull(ctx, "ghcr.io/myorg/bundle:v1.0", "./config",
    ocibundle.WithFilesToExtract("**/*.json"),
)

// Extract multiple file types
err := client.Pull(ctx, "ghcr.io/myorg/bundle:v1.0", "./app",
    ocibundle.WithFilesToExtract(
        "config.json",      // Specific file
        "data/*.json",      // All JSON in data/ directory
        "**/*.yaml",        // All YAML files recursively
        "bin/app",          // Specific binary
    ),
)

// Extract source code only
err := client.Pull(ctx, "ghcr.io/myorg/source:v1.0", "./src",
    ocibundle.WithFilesToExtract("**/*.go", "**/*.mod", "**/*.sum"),
)
```

### Pattern Syntax

Supported glob patterns for selective extraction:

| Pattern | Description | Example Match |
|---------|-------------|---------------|
| `*.json` | Files in root with .json extension | `config.json` |
| `config/*` | All files directly in config/ | `config/app.yaml` |
| `**/*.txt` | All .txt files recursively | `data/logs/app.txt` |
| `data/**/*.json` | All .json under data/ | `data/users/1.json` |
| `bin/app` | Exact file path | `bin/app` |

### Benefits of Selective Extraction

- **Faster extraction**: Only processes needed files
- **Saves disk space**: Unwanted files never written to disk
- **Reduces I/O**: Non-matching files skipped entirely
- **Lower CPU usage**: Less decompression work
- **Security maintained**: All validators still enforced on matched files

### Example Use Cases

**Configuration deployment:**
```go
// Extract only runtime configuration
err := client.Pull(ctx, ref, "./runtime",
    ocibundle.WithFilesToExtract("config/*.yaml", "secrets/*.json"),
)
```

**Development environment:**
```go
// Get only source code, skip binaries
err := client.Pull(ctx, ref, "./workspace",
    ocibundle.WithFilesToExtract("**/*.go", "**/*.proto", "**/*.md"),
)
```

**Multi-stage builds:**
```go
// Extract different artifacts in stages
// Stage 1: Get build artifacts
err := client.Pull(ctx, ref, "./build",
    ocibundle.WithFilesToExtract("dist/**/*"),
)

// Stage 2: Get documentation
err := client.Pull(ctx, ref, "./docs",
    ocibundle.WithFilesToExtract("**/*.md", "**/*.html"),
)
```

## Examples

The [`examples/`](./examples/) directory contains runnable examples:

- **[Basic usage](./examples/basic/)** - Simple push/pull operations
- **[With progress](./examples/basic_with_progress/)** - Progress reporting
- **[Advanced usage](./examples/advanced/)** - Custom options and error handling
- **[Custom auth](./examples/custom_auth/)** - Different authentication methods
- **[Custom archiver](./examples/custom_archiver/)** - Implementing custom archive formats

## Advanced Usage

### Custom Configuration

```go
// Create client with custom configuration
client, err := ocibundle.NewWithOptions(
    // Security limits
    ocibundle.WithMaxFiles(10000),
    ocibundle.WithMaxSize(1*1024*1024*1024), // 1GB
    ocibundle.WithMaxFileSize(100*1024*1024), // 100MB per file

    // Authentication
    ocibundle.WithStaticAuth("registry.example.com", "user", "token"),

    // HTTP configuration
    ocibundle.WithHTTP(true, false, []string{"localhost:5000"}),
)
```

### Push with Metadata

```go
// Push with annotations and platform information
annotations := map[string]string{
    "org.opencontainers.image.title":       "My Application Bundle",
    "org.opencontainers.image.description": "Production application files",
    "org.opencontainers.image.version":     "2.1.0",
    "org.opencontainers.image.vendor":      "My Company",
    "com.example.build-id":                 "build-12345",
    "com.example.git-commit":               "abc123def456",
}

err := client.Push(ctx, "./dist", "ghcr.io/myorg/app:v2.1.0",
    ocibundle.WithAnnotations(annotations),
    ocibundle.WithPlatform("linux/amd64"),
    ocibundle.WithProgressCallback(func(current, total int64) {
        percentage := float64(current) / float64(total) * 100
        fmt.Printf("\rUpload progress: %.1f%%", percentage)
    }),
)
```

### Pull with Security Options

```go
// Pull with security constraints
err := client.Pull(ctx, "ghcr.io/myorg/bundle:v1.0.0", "./app",
    // Security limits
    ocibundle.WithMaxFiles(5000),
    ocibundle.WithMaxSize(500*1024*1024), // 500MB
    ocibundle.WithMaxFileSize(50*1024*1024), // 50MB per file

    // Extraction options
    ocibundle.WithPullPreservePermissions(false), // Sanitize permissions
    ocibundle.WithPullStripPrefix("bundle-root/"), // Remove prefix
    ocibundle.WithPullAllowHiddenFiles(false), // Reject hidden files

    // Retry configuration
    ocibundle.WithPullMaxRetries(5),
    ocibundle.WithPullRetryDelay(3*time.Second),
)
```

## Authentication

The module uses ORAS's native authentication system, providing robust support for Docker's standard authentication mechanisms.

### Default Authentication (Recommended)

By default, the client uses ORAS's built-in Docker credential chain:

```go
// Uses ~/.docker/config.json and credential helpers automatically
client, err := ocibundle.New()
```

This automatically supports:
- **Docker config files** (`~/.docker/config.json`)
- **Credential helpers** (`osxkeychain`, `pass`, `desktop`, `wincred`, etc.)
- **Multiple registries** with different authentication methods
- **Token refresh** and OAuth2 flows where supported

Example `~/.docker/config.json`:
```json
{
  "auths": {
    "ghcr.io": {
      "auth": "dXNlcjpwYXNzd29yZA=="
    },
    "docker.io": {
      "auth": "dXNlcjpwYXNzd29yZA=="
    }
  },
  "credHelpers": {
    "registry.example.com": "desktop"
  },
  "credsStore": "osxkeychain"
}
```

### Static Credentials Override

For specific registries, override the default chain:

```go
client, err := ocibundle.NewWithOptions(
    ocibundle.WithStaticAuth("ghcr.io", "username", "personal-access-token"),
)
```

### Custom Credential Function

For advanced scenarios requiring custom credential logic:

```go
import "oras.land/oras-go/v2/registry/remote/auth"

customCreds := func(ctx context.Context, registry string) (auth.Credential, error) {
    switch registry {
    case "ghcr.io":
        return auth.Credential{Username: "user", Password: "token"}, nil
    case "registry.company.com":
        return getCompanyCredentials(ctx, registry)
    default:
        return auth.Credential{}, nil // Anonymous access
    }
}

client, err := ocibundle.NewWithOptions(
    ocibundle.WithCredentialFunc(customCreds),
)
```

### HTTP and Insecure Registries

```go
// HTTP-only registry (local development)
client, err := ocibundle.NewWithOptions(
    ocibundle.WithAllowHTTP(),
)

// Insecure registry (testing only)
client, err := ocibundle.NewWithOptions(
    ocibundle.WithInsecureHTTP(),
)
```

## Security

- **Threats Addressed**:
  - Path traversal and absolute path injection
  - Symlink-based directory escape
  - Zip/decompression bombs (file count and total size)
  - Oversized individual files
  - Dangerous permission bits (setuid/setgid)

- **Validators and Enforcement**:
  - `internal/validate.PathTraversalValidator` rejects absolute paths, `..`, encoded traversal variants, and validates symlink targets against the extraction root.
  - `SizeValidator` enforces per-file and total uncompressed size limits.
  - `FileCountValidator` enforces file-count limits to prevent resource exhaustion.
  - `PermissionSanitizer` rejects files with setuid/setgid bits and sanitizes permissions when writing.
  - `ValidatorChain` composes validators and fails fast on the first violation.

- **Safe Defaults**:
  - `DefaultExtractOptions`: 10,000 files, 1GB total, 100MB per file, permissions sanitized, hidden files rejected.

- **Testing & Verification**:
  - Unit tests for each validator and extraction behavior.
  - Fuzz tests for path validation and size validator to ensure robustness against arbitrary inputs.
  - Malicious archive generators (OWASP inspired) validate that extraction blocks path traversal and symlink attacks.

- **Credentials Handling**:
  - Authentication is delegated to ORAS; the library never logs usernames, passwords, or tokens.
  - Error messages avoid echoing sensitive values; only generic messages are returned (e.g., "static password required").


## Error Handling

The module provides detailed error information through the `BundleError` type:

```go
err := client.Push(ctx, sourceDir, reference)
if err != nil {
    var bundleErr *ocibundle.BundleError
    if errors.As(err, &bundleErr) {
        // Handle specific error types
        if bundleErr.IsAuthError() {
            log.Printf("Authentication failed for %s", bundleErr.Reference)
        } else if bundleErr.IsSecurityError() {
            log.Printf("Security violation in %s", bundleErr.Reference)
        }
    }
    return fmt.Errorf("push failed: %w", err)
}
```
## API Reference

- [`Client`](https://pkg.go.dev/github.com/jmgilman/go/oci#Client) - Main client for operations
- [`ClientOptions`](https://pkg.go.dev/github.com/jmgilman/go/oci#ClientOptions) - Configuration options
- [`BundleError`](https://pkg.go.dev/github.com/jmgilman/go/oci#BundleError) - Detailed error information
- [`Archiver`](https://pkg.go.dev/github.com/jmgilman/go/oci#Archiver) - Archive format interface
- [`Validator`](https://pkg.go.dev/github.com/jmgilman/go/oci#Validator) - Security validation interface

## Testing

The module includes comprehensive tests:

```bash
# Run all tests
go test ./lib/oci/...

# Run with coverage
go test -cover ./lib/oci/...

# Run integration tests (requires registry)
go test -tags=integration ./lib/oci/...

# Run specific test
go test -run TestClient_Push ./lib/oci/
```

### Test Infrastructure

- **Unit Tests**: Comprehensive coverage of all components
- **Integration Tests**: End-to-end testing with test registries
- **Security Tests**: Malicious archive testing
- **Benchmark Tests**: Performance validation

## Filesystem Injection and In-Memory Testing

The client and archiver operate over an abstract filesystem interface so you can swap the backing store.

- Default: OS filesystem (rooted at "/").
- Custom: Inject any implementation (e.g., in-memory) for tests.

```go
import (
    billyfs "github.com/jmgilman/go/fs/billy"
    ocibundle "github.com/jmgilman/go/oci"
)

// In-memory filesystem for fast, isolated tests
mem := billyfs.NewInMemoryFS()

client, err := ocibundle.NewWithOptions(
    ocibundle.WithFilesystem(mem),
)
if err != nil { /* handle */ }

// Use the same FS with the archiver
archiver := ocibundle.NewTarGzArchiverWithFS(mem)

// Build fixture
_ = mem.MkdirAll("/src", 0o755)
_ = mem.WriteFile("/src/hello.txt", []byte("hi"), 0o644)

// Archive and extract entirely in-memory
var buf bytes.Buffer
_ = archiver.Archive(context.Background(), "/src", &buf)
_ = archiver.Extract(context.Background(), &buf, "/dst", ocibundle.DefaultExtractOptions)

b, _ := mem.ReadFile("/dst/hello.txt")
```

### Unit Tests: Avoiding Network Dependencies

Unit tests should not perform real network calls. Inject a mocked ORAS client and disable retries to keep tests fast and deterministic:

```go
mock := &mocks.ClientMock{
    PushFunc: func(ctx context.Context, ref string, d *oras.PushDescriptor, a *oras.AuthOptions) error { return fmt.Errorf("simulated push error") },
    PullFunc: func(ctx context.Context, ref string, a *oras.AuthOptions) (*oras.PullDescriptor, error) { /* return small tar.gz */ return desc, nil },
}
client, _ := ocibundle.NewWithOptions(
    ocibundle.WithORASClient(mock),
    ocibundle.WithFilesystem(billyfs.NewInMemoryFS()),
)
// Disable retries in tests that expect error paths
_ = client.Push(ctx, "/src", "example/repo:tag", ocibundle.WithMaxRetries(0), ocibundle.WithRetryDelay(0))
```

## Registry Compatibility

### OCI Compliance

- ✅ OCI Distribution Specification v1.1
- ✅ OCI Image Specification
- ✅ ORAS artifact types
- ✅ Standard media types

## Performance

### Benchmarks

```bash
# Run performance benchmarks
go test -bench=. ./lib/oci/

# Memory profiling
go test -bench=. -memprofile=mem.out ./lib/oci/
go tool pprof mem.out

# CPU profiling
go test -bench=. -cpuprofile=cpu.out ./lib/oci/
go tool pprof cpu.out
```

### Performance Characteristics

- **Memory Usage**: Constant memory for any file size (streaming)
- **Large Files**: Handles files > 10GB without memory exhaustion
- **Concurrent Operations**: Thread-safe for multiple simultaneous operations
- **Network Efficiency**: Automatic retry and connection reuse
- **Registry Optimization**: Request batching and connection pooling

## Contributing

This module follows Go best practices and uses Test-Driven Development (TDD).

### Development Setup

```bash
# Clone the repository
git clone https://github.com/jmgilman/go.git
cd go/oci

# Install dependencies
go mod download

# Run tests
go test ./...

# Generate mocks (if needed)
go generate ./...
```

### Code Quality

```bash
# Run linters
golangci-lint run

# Format code
gofmt -w .

# Check for security issues
gosec ./...

# Run static analysis
staticcheck ./...
```

## Architecture

The module follows clean architecture principles:

### Core Components

- **Client**: Main entry point with push/pull operations
- **Archiver**: Interface for different compression formats (default: tar.gz)
- **Validator**: Interface for security validation with chain pattern
- **Options**: Functional options pattern for configuration

### Key Interfaces

```go
// Archiver handles compression/decompression
type Archiver interface {
    Archive(ctx context.Context, sourceDir string, output io.Writer) error
    ArchiveWithProgress(ctx context.Context, sourceDir string, output io.Writer, progress func(current, total int64)) error
    Extract(ctx context.Context, input io.Reader, targetDir string, opts ExtractOptions) error
    MediaType() string
}

// Validator checks for security issues
type Validator interface {
    ValidatePath(path string) error
    ValidateFile(info FileInfo) error
    ValidateArchive(stats ArchiveStats) error
}
```

## Related Projects

- [ORAS](https://oras.land/) - OCI Registry as Storage
- [OCI Distribution Spec](https://github.com/opencontainers/distribution-spec)
- [go-containerregistry](https://github.com/google/go-containerregistry)
- [Docker Registry](https://docs.docker.com/registry/)

## License

See [LICENSE](../../LICENSE) for license information.
