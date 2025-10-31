// Package ocibundle provides OCI bundle distribution functionality.
// This file defines the interface for signature verification.
package ocibundle

import (
	"context"

	orasint "github.com/jmgilman/go/oci/internal/oras"
)

// SignatureVerifier validates OCI artifact signatures before extraction.
// This interface allows pluggable signature verification implementations
// without requiring the main oci package to depend on signature libraries.
//
// Implementations are provided by the oci/signature submodule, which includes
// support for Sigstore/Cosign signature verification with both public key
// and keyless (OIDC) verification modes.
//
// The verifier is called during Pull operations after the manifest is fetched
// but before the blob is extracted. This ensures that only cryptographically
// verified artifacts are written to disk.
//
// Example usage with public key verification:
//
//	import (
//	    "github.com/jmgilman/go/oci"
//	    "github.com/jmgilman/go/oci/signature"
//	)
//
//	// Load public key
//	pubKey, err := signature.LoadPublicKey("cosign.pub")
//	if err != nil {
//	    return err
//	}
//
//	// Create signature verifier instance
//	verifier := signature.NewPublicKeyVerifier(pubKey)
//
//	// Create client with verifier
//	client, err := oci.NewWithOptions(
//	    oci.WithSignatureVerifier(verifier),
//	)
//	if err != nil {
//	    return err
//	}
//
//	// Pull will verify signature before extraction
//	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
//	if err != nil {
//	    var bundleErr *oci.BundleError
//	    if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
//	        log.Printf("Signature verification failed: %s", bundleErr.SignatureInfo.Reason)
//	    }
//	    return err
//	}
//
// Example usage with keyless verification:
//
//	// Create keyless verifier with identity restrictions
//	verifier := signature.NewKeylessVerifier(
//	    signature.WithAllowedIdentities("*@example.com"),
//	    signature.WithRekor(true), // Require transparency log
//	)
//
//	client, err := oci.NewWithOptions(
//	    oci.WithSignatureVerifier(verifier),
//	)
//
//	// Pull will verify signature using Sigstore keyless signing
//	err = client.Pull(ctx, "ghcr.io/org/app:v1.0", "./app")
type SignatureVerifier interface {
	// Verify validates the signature for the given OCI artifact.
	//
	// This method is called after the manifest is fetched but before the blob
	// is extracted. The verifier should:
	// 1. Fetch the signature artifact from the registry
	// 2. Verify the cryptographic signature matches the artifact digest
	// 3. Validate any policy requirements (identity, annotations, etc.)
	// 4. Optionally verify transparency log inclusion (Rekor)
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - reference: Full OCI reference (e.g., "ghcr.io/org/repo:tag")
	//   - descriptor: Pull descriptor containing digest, size, and data stream
	//
	// The descriptor provides:
	//   - Digest: Content digest (SHA256) for signature lookup
	//   - MediaType: Content type of the artifact
	//   - Size: Total blob size
	//   - Data: Blob stream (should NOT be consumed by verifier)
	//
	// Returns:
	//   - nil if signature is valid and all policies pass
	//   - BundleError with SignatureErrorInfo if verification fails
	//   - Error for any other failures (network, parsing, etc.)
	//
	// Error types returned by implementations:
	//   - ErrSignatureNotFound: No signature found for artifact
	//   - ErrSignatureInvalid: Signature exists but cryptographic verification failed
	//   - ErrUntrustedSigner: Signature valid but signer not in allowed list
	//   - ErrRekorVerificationFailed: Transparency log verification failed
	//   - ErrCertificateExpired: Certificate used for signing has expired
	//   - ErrInvalidAnnotations: Required annotations missing or incorrect
	//
	// The verifier should NOT:
	//   - Consume the descriptor.Data stream (this is used for extraction)
	//   - Retry on verification failures (client handles retries for network errors)
	//   - Cache results internally (caching is handled by the client)
	Verify(ctx context.Context, reference string, descriptor *orasint.PullDescriptor) error
}
