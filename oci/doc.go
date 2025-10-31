// Package ocibundle provides OCI bundle distribution functionality with support
// for eStargz (seekable tar.gz) format and selective file extraction.
//
// # Overview
//
// This package enables pushing and pulling filesystem bundles to/from OCI-compliant
// registries. It uses the eStargz (extended stargz) format, which provides:
//   - 100% backward compatibility with standard tar.gz archives
//   - Table of Contents (TOC) for efficient random access
//   - Support for selective file extraction using glob patterns
//   - Future support for HTTP Range requests to minimize bandwidth usage
//
// # Basic Usage
//
//	client, err := ocibundle.New("localhost:5000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Push a directory as an OCI artifact
//	err = client.Push(ctx, "/path/to/bundle", "localhost:5000/myrepo:latest")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Pull the artifact to a directory
//	err = client.Pull(ctx, "localhost:5000/myrepo:latest", "/path/to/target")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Selective File Extraction
//
// The package supports extracting only specific files using glob patterns,
// which saves disk I/O and CPU when you only need a subset of files:
//
//	// Extract only JSON files
//	err = client.Pull(ctx, reference, targetDir,
//	    ocibundle.WithFilesToExtract("**/*.json"),
//	)
//
//	// Extract multiple patterns
//	err = client.Pull(ctx, reference, targetDir,
//	    ocibundle.WithFilesToExtract("config.json", "data/*.json", "**/*.yaml"),
//	)
//
// Supported glob patterns:
//   - "*.json" - matches all .json files in the root directory
//   - "config/*" - matches all files in the config directory
//   - "**/*.txt" - matches all .txt files recursively
//   - "data/**/*.json" - matches all .json files under data/ and subdirectories
//
// # eStargz Format
//
// All archives are created in eStargz format, which is fully compatible with
// standard tar.gz but includes additional metadata:
//   - Table of Contents (TOC) stored at the end of the archive
//   - Enables efficient random access without decompressing the entire archive
//   - Future optimization: HTTP Range requests to download only needed chunks
//
// Archives created with this package can be extracted using standard tar tools:
//
//	tar -xzf archive.tar.gz
//
// # Security Features
//
// The package includes comprehensive security validation:
//   - Path traversal prevention (prevents ../../../etc/passwd attacks)
//   - Configurable size limits (per-file and total archive size)
//   - File count limits to prevent resource exhaustion
//   - Permission sanitization (removes setuid/setgid bits)
//   - All security features work with selective extraction
//
// Configure security limits:
//
//	err = client.Pull(ctx, reference, targetDir,
//	    ocibundle.WithMaxSize(100*1024*1024),      // 100MB total
//	    ocibundle.WithMaxFileSize(10*1024*1024),   // 10MB per file
//	    ocibundle.WithMaxFiles(1000),              // Max 1000 files
//	)
//
// # Caching
//
// The package supports optional caching to avoid redundant network requests:
//
//	cacheDir := "/tmp/oci-cache"
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithCache(cacheDir),
//	)
//
//	// Use cached pull when available
//	err = client.PullWithCache(ctx, reference, targetDir)
//
// # HTTP Configuration
//
// Configure registry connection settings:
//
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithHTTP(
//	        true,                    // allowHTTP - allow insecure HTTP registries
//	        true,                    // allowInsecureTLS - allow self-signed certificates
//	        []string{"localhost"},   // insecureRegistries - specific registries to allow HTTP
//	    ),
//	    ocibundle.WithTimeout(30*time.Second),
//	)
//
// # Filesystem Abstraction
//
// The package supports custom filesystems via the billy interface, enabling
// in-memory operations or custom storage backends:
//
//	import "github.com/jmgilman/go/fs/billy"
//
//	memFS := billy.NewMemory()
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithFS(memFS),
//	)
//
// # Error Handling
//
// All operations return detailed errors with context. Common error scenarios:
//   - Network errors when connecting to registry
//   - Authentication failures
//   - Security constraint violations (path traversal, size limits)
//   - Archive format errors
//   - Filesystem errors during extraction
//
// Example error handling:
//
//	err := client.Pull(ctx, reference, targetDir)
//	if err != nil {
//	    if strings.Contains(err.Error(), "security constraint") {
//	        log.Printf("Security validation failed: %v", err)
//	    } else if strings.Contains(err.Error(), "authentication") {
//	        log.Printf("Authentication required: %v", err)
//	    } else {
//	        log.Printf("Pull failed: %v", err)
//	    }
//	}
//
// # Backward Compatibility
//
// The package maintains 100% backward compatibility:
//   - Archives created with eStargz can be extracted with standard tar tools
//   - Legacy tar.gz archives can be extracted by this package
//   - No breaking changes to existing API
//   - All existing security features continue to work
package ocibundle
