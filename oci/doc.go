// Package ocibundle provides OCI bundle distribution with eStargz format support.
//
// This package enables pushing and pulling filesystem bundles to OCI-compliant
// registries using the eStargz format. Key features:
//   - eStargz archives (100% backward compatible with tar.gz)
//   - Selective file extraction using glob patterns
//   - HTTP Range requests for bandwidth optimization
//   - Comprehensive security validation (path traversal, size limits, permissions)
//   - Optional signature verification for supply chain security
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
// Signature Verification:
//
// Optional signature verification ensures artifacts are cryptographically verified
// before extraction using Sigstore/Cosign. The signature submodule
// (github.com/jmgilman/go/oci/signature) keeps verification dependencies separate.
//
//	import "github.com/jmgilman/go/oci/signature"
//
//	// Public key verification
//	pubKey, _ := signature.LoadPublicKey("cosign.pub")
//	verifier := signature.NewPublicKeyVerifier(pubKey)
//
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(verifier),
//	)
//
//	// Pull will verify signature before extraction
//	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
//
//	// Keyless verification with OIDC
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true),
//	)
//
//	client, err := ocibundle.NewWithOptions(
//	    ocibundle.WithSignatureVerifier(verifier),
//	)
//
// See the README and signature/README.md for detailed documentation and examples.
package ocibundle
