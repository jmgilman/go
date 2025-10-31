// Package ocibundle provides OCI bundle distribution with eStargz format support.
//
// This package enables pushing and pulling filesystem bundles to OCI-compliant
// registries using the eStargz format. Key features:
//   - eStargz archives (100% backward compatible with tar.gz)
//   - Selective file extraction using glob patterns
//   - HTTP Range requests for bandwidth optimization
//   - Comprehensive security validation (path traversal, size limits, permissions)
//   - Optional caching for repeated operations
//   - Filesystem abstraction for testing and custom storage
//
// Basic usage:
//
//	client, err := ocibundle.New()
//	if err != nil {
//	    return err
//	}
//
//	// Push a directory
//	err = client.Push(ctx, "/path/to/bundle", "ghcr.io/myrepo:latest")
//
//	// Pull to a directory
//	err = client.Pull(ctx, "ghcr.io/myrepo:latest", "/path/to/target")
//
//	// Selective extraction with patterns
//	err = client.Pull(ctx, reference, targetDir,
//	    ocibundle.WithFilesToExtract("**/*.json", "config/*.yaml"),
//	)
//
// See the README for detailed documentation and examples.
package ocibundle
