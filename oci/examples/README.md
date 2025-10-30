# OCI Bundle Examples

This directory contains runnable example programs demonstrating how to use the OCI Bundle Distribution Module.

## Examples

### Basic Usage

- **[basic/main.go](./basic/main.go)** - Simple push/pull operations with default settings
- **[basic_with_progress/main.go](./basic_with_progress/main.go)** - Basic operations with progress reporting

### Advanced Usage

- **[advanced/main.go](./advanced/main.go)** - Advanced usage with custom options and error handling
- **[custom_auth/main.go](./custom_auth/main.go)** - Custom authentication examples
- **[custom_archiver/main.go](./custom_archiver/main.go)** - Implementing custom archiver formats

### Caching Examples

- **[basic_caching/main.go](./basic_caching/main.go)** - Basic caching functionality and performance benefits
- **[advanced_caching/main.go](./advanced_caching/main.go)** - Advanced cache configuration with size limits and policies
- **[cache_management/main.go](./cache_management/main.go)** - Cache monitoring, statistics, and management operations

### Security Examples

- **[security/main.go](./security/main.go)** - Security best practices and validation
- **[large_files/main.go](./large_files/main.go)** - Handling large files and archives

## Running Examples

Each example is a complete, runnable Go program. To run an example:

```bash
cd examples/basic
go run main.go
```

## Prerequisites

Most examples require:
- Access to an OCI registry (Docker Hub, GitHub Container Registry, etc.)
- Appropriate authentication configured (see main README for details)
- A directory with files to bundle

## Test Registry

For testing without affecting production registries, you can use a local test registry:

```bash
# Start a local registry
docker run -d -p 5000:5000 --name registry registry:2

# Use in examples with:
client, err := ocibundle.NewWithOptions(
    ocibundle.WithAllowHTTP(),
    ocibundle.WithStaticAuth("localhost:5000", "testuser", "testpass"),
)
```

## Example Structure

Each example follows this structure:
- `main.go` - Main example code
- Comments explaining key concepts
- Error handling demonstrations
- Best practices implementation
