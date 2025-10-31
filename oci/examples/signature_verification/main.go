package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/signature"
)

func main() {
	// Parse command-line flags
	mode := flag.String("mode", "public-key", "Verification mode: public-key or keyless")
	pubKeyPath := flag.String("key", "cosign.pub", "Path to public key file (for public-key mode)")
	identity := flag.String("identity", "*@example.com", "Allowed identity pattern (for keyless mode)")
	rekor := flag.Bool("rekor", true, "Enable Rekor transparency log verification (for keyless mode)")
	reference := flag.String("ref", "", "OCI reference to pull (required)")
	targetDir := flag.String("target", "./output", "Target directory for extraction")

	flag.Parse()

	if *reference == "" {
		fmt.Println("Error: -ref flag is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Create appropriate verifier based on mode
	var verifier *signature.CosignVerifier
	var err error

	switch *mode {
	case "public-key":
		verifier, err = createPublicKeyVerifier(*pubKeyPath)
		if err != nil {
			log.Fatalf("Failed to create public key verifier: %v", err)
		}
		fmt.Printf("Using public key verification with key: %s\n", *pubKeyPath)

	case "keyless":
		verifier = createKeylessVerifier(*identity, *rekor)
		fmt.Printf("Using keyless verification with identity: %s\n", *identity)
		if *rekor {
			fmt.Println("Rekor transparency log verification enabled")
		}

	default:
		log.Fatalf("Unknown mode: %s (use 'public-key' or 'keyless')", *mode)
	}

	// Create OCI client with signature verification
	client, err := ocibundle.NewWithOptions(
		ocibundle.WithSignatureVerifier(verifier),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fmt.Printf("\nPulling and verifying: %s\n", *reference)
	fmt.Printf("Target directory: %s\n\n", *targetDir)

	// Pull artifact - signature will be verified before extraction
	err = client.Pull(ctx, *reference, *targetDir)
	if err != nil {
		handleVerificationError(err)
		os.Exit(1)
	}

	fmt.Println("\n✓ Signature verified successfully!")
	fmt.Printf("✓ Artifact extracted to: %s\n", *targetDir)
}

// createPublicKeyVerifier creates a verifier for public key mode
func createPublicKeyVerifier(keyPath string) (*signature.CosignVerifier, error) {
	// Load public key from file
	pubKey, err := signature.LoadPublicKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	// Create verifier
	verifier := signature.NewPublicKeyVerifier(pubKey)
	return verifier, nil
}

// createKeylessVerifier creates a verifier for keyless mode
func createKeylessVerifier(identityPattern string, enableRekor bool) *signature.CosignVerifier {
	// Build options based on flags
	opts := []signature.VerifierOption{
		signature.WithAllowedIdentities(identityPattern),
	}

	if enableRekor {
		opts = append(opts, signature.WithRekor(true))
	}

	// Create keyless verifier
	verifier := signature.NewKeylessVerifier(opts...)
	return verifier
}

// handleVerificationError provides detailed error information
func handleVerificationError(err error) {
	fmt.Println("\n✗ Verification failed!")

	var bundleErr *ocibundle.BundleError
	if errors.As(err, &bundleErr) && bundleErr.IsSignatureError() {
		fmt.Println("\nSignature Verification Details:")
		fmt.Printf("  Operation: %s\n", bundleErr.Op)
		fmt.Printf("  Reference: %s\n", bundleErr.Reference)
		if bundleErr.SignatureInfo != nil {
			fmt.Printf("  Digest: %s\n", bundleErr.SignatureInfo.Digest)
			fmt.Printf("  Reason: %s\n", bundleErr.SignatureInfo.Reason)
			fmt.Printf("  Failure Stage: %s\n", bundleErr.SignatureInfo.FailureStage)
			if bundleErr.SignatureInfo.Signer != "" {
				fmt.Printf("  Signer: %s\n", bundleErr.SignatureInfo.Signer)
			}
		}

		fmt.Println("\nError Type:")
		switch {
		case errors.Is(bundleErr.Err, ocibundle.ErrSignatureNotFound):
			fmt.Println("  No signature found for artifact")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. Ensure the artifact was signed with: cosign sign <image>")
			fmt.Println("  2. Check that you have read access to the signature artifact")
			fmt.Println("  3. Verify the signature reference format: repo:sha256-<digest>.sig")

		case errors.Is(bundleErr.Err, ocibundle.ErrSignatureInvalid):
			fmt.Println("  Signature verification failed")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. The artifact may have been tampered with")
			fmt.Println("  2. Ensure you're using the correct public key")
			fmt.Println("  3. Try re-signing the artifact")

		case errors.Is(bundleErr.Err, ocibundle.ErrUntrustedSigner):
			fmt.Println("  Signature valid but signer not trusted")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. Check the allowed identity patterns match the actual signer")
			fmt.Println("  2. Verify the certificate issuer matches your policy")
			fmt.Println("  3. Update allowed identities to trust this signer")

		case errors.Is(bundleErr.Err, ocibundle.ErrRekorVerificationFailed):
			fmt.Println("  Transparency log verification failed")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. Ensure network access to Rekor server (rekor.sigstore.dev)")
			fmt.Println("  2. Check if the signature was uploaded to Rekor")
			fmt.Println("  3. Try with -rekor=false for testing")

		case errors.Is(bundleErr.Err, ocibundle.ErrCertificateExpired):
			fmt.Println("  Certificate expired")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. For keyless signing, this may indicate an old signature")
			fmt.Println("  2. Re-sign the artifact with a fresh certificate")

		case errors.Is(bundleErr.Err, ocibundle.ErrInvalidAnnotations):
			fmt.Println("  Required annotations not found")
			fmt.Println("\nTroubleshooting:")
			fmt.Println("  1. Check signature annotations with: cosign verify <image>")
			fmt.Println("  2. Ensure annotations were added during signing")
		}
	} else {
		// Non-signature error
		fmt.Printf("\nError: %v\n", err)
	}
}
