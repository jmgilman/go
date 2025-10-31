// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/cosign/v2/pkg/oci"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/sigstore/sigstore/pkg/tuf"

	ocibundle "github.com/jmgilman/go/oci"
)

// verifyKeylessSignature verifies a signature using keyless (OIDC) verification.
// This uses Sigstore's Fulcio certificate authority and optionally Rekor transparency log.
func (v *CosignVerifier) verifyKeylessSignature(ctx context.Context, digestStr string, sig oci.Signature) error {
	// Get the certificate from the signature
	cert, err := sig.Cert()
	if err != nil || cert == nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrCertificateExpired,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "No certificate found in signature (keyless mode requires certificate)",
				FailureStage: "certificate",
			},
		}
	}

	// Verify certificate is not expired
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrCertificateExpired,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Certificate expired (valid from %v to %v)", cert.NotBefore, cert.NotAfter),
				FailureStage: "certificate",
			},
		}
	}

	// Extract identity from certificate
	identity, err := extractIdentityFromCert(cert)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: fmt.Errorf("failed to extract identity: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to extract identity from certificate: %s", err.Error()),
				FailureStage: "identity",
			},
		}
	}

	// Check if identity matches allowed patterns
	if !v.policy.MatchesIdentity(identity) {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrUntrustedSigner,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Identity %q does not match allowed patterns", identity),
				FailureStage: "identity",
				Signer:       identity,
			},
		}
	}

	// Check issuer if required
	if v.policy.RequiredIssuer != "" {
		issuer, err := extractIssuerFromCert(cert)
		if err != nil {
			return &ocibundle.BundleError{
				Op:  "verify",
				Err: fmt.Errorf("failed to extract issuer: %w", err),
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       digestStr,
					Reason:       fmt.Sprintf("Failed to extract issuer from certificate: %s", err.Error()),
					FailureStage: "certificate",
					Signer:       identity,
				},
			}
		}
		if issuer != v.policy.RequiredIssuer {
			return &ocibundle.BundleError{
				Op:  "verify",
				Err: ocibundle.ErrUntrustedSigner,
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       digestStr,
					Reason:       fmt.Sprintf("Issuer %q does not match required issuer %q", issuer, v.policy.RequiredIssuer),
					FailureStage: "identity",
					Signer:       identity,
				},
			}
		}
	}

	// Verify certificate chain against Fulcio roots
	if err := v.verifyCertificateChain(cert); err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: fmt.Errorf("certificate chain verification failed: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Certificate chain verification failed: %s", err.Error()),
				FailureStage: "certificate",
				Signer:       identity,
			},
		}
	}

	// Get the signature payload
	payload, err := sig.Payload()
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: fmt.Errorf("failed to get payload: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to get signature payload: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Parse the payload to verify it contains the expected digest
	// Structure follows the Cosign Simple Signing format
	var payloadData struct {
		Critical struct {
			Image struct {
				DockerManifestDigest string `json:"docker-manifest-digest"`
			} `json:"image"`
			Type     string `json:"type"`
			Identity struct {
				DockerReference string `json:"docker-reference"`
			} `json:"identity"`
		} `json:"critical"`
	}

	if err := json.Unmarshal(payload, &payloadData); err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to parse signature payload: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Validate required fields in payload
	if payloadData.Critical.Image.DockerManifestDigest == "" {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "Payload missing required field: docker-manifest-digest",
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Validate signature type (Cosign Simple Signing format)
	// Expected: "atomic container signature" or "cosign container image signature"
	if payloadData.Critical.Type != "" {
		validTypes := []string{
			"atomic container signature",
			"cosign container image signature",
		}
		validType := false
		for _, vt := range validTypes {
			if payloadData.Critical.Type == vt {
				validType = true
				break
			}
		}
		if !validType {
			return &ocibundle.BundleError{
				Op:  "verify",
				Err: ocibundle.ErrSignatureInvalid,
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       digestStr,
					Reason:       fmt.Sprintf("Invalid signature type: %q (expected Cosign format)", payloadData.Critical.Type),
					FailureStage: "cryptographic",
					Signer:       identity,
				},
			}
		}
	}

	// Parse and normalize both digests using OCI digest library
	artifactDigest, err := digest.Parse(digestStr)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Invalid artifact digest: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	payloadDigest, err := digest.Parse(payloadData.Critical.Image.DockerManifestDigest)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Invalid payload digest: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Verify algorithm is SHA256 (required by OCI spec)
	if artifactDigest.Algorithm() != digest.SHA256 {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Unsupported digest algorithm: %s (required: sha256)", artifactDigest.Algorithm()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Compare normalized digests (handles case differences and format variations)
	if artifactDigest != payloadDigest {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Payload digest %q does not match artifact digest %q", payloadDigest, artifactDigest),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Get signature bytes
	sigBytes, err := sig.Base64Signature()
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: fmt.Errorf("failed to get signature: %w", err),
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to get signature bytes: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Decode base64 signature
	decodedSig, err := base64.StdEncoding.DecodeString(sigBytes)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to decode signature: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Extract public key from certificate
	pubKey := cert.PublicKey

	// Create a verifier from the certificate's public key
	verifier, err := signature.LoadVerifier(pubKey, crypto.SHA256)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Failed to create verifier from certificate: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Verify the signature using the certificate's public key
	if err := verifier.VerifySignature(bytes.NewReader(decodedSig), bytes.NewReader(payload)); err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrSignatureInvalid,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Signature cryptographic verification failed: %s", err.Error()),
				FailureStage: "cryptographic",
				Signer:       identity,
			},
		}
	}

	// Check annotations if required
	if len(v.policy.RequiredAnnotations) > 0 {
		annotations, err := sig.Annotations()
		if err != nil {
			return &ocibundle.BundleError{
				Op:  "verify",
				Err: ocibundle.ErrInvalidAnnotations,
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       digestStr,
					Reason:       fmt.Sprintf("Failed to get annotations: %s", err.Error()),
					FailureStage: "policy",
					Signer:       identity,
				},
			}
		}
		if !v.checkAnnotations(annotations) {
			return &ocibundle.BundleError{
				Op:  "verify",
				Err: ocibundle.ErrInvalidAnnotations,
				SignatureInfo: &ocibundle.SignatureErrorInfo{
					Digest:       digestStr,
					Reason:       "Required annotations not satisfied",
					FailureStage: "policy",
					Signer:       identity,
				},
			}
		}
	}

	// Verify Rekor transparency log if enabled
	if v.policy.RekorEnabled {
		if err := v.verifyRekorEntry(ctx, digestStr, sig); err != nil {
			// Error already wrapped as BundleError by verifyRekorEntry
			return err
		}
	}

	// All checks passed - signature is valid
	return nil
}

// extractIdentityFromCert extracts the signer identity from a certificate.
// This is typically the email address or subject from the OIDC token.
// Fulcio stores identity in custom OID extensions.
func extractIdentityFromCert(cert *x509.Certificate) (string, error) {
	// Sigstore Fulcio uses custom OID extensions to store OIDC identity
	// OID 1.3.6.1.4.1.57264.1.1 is the Fulcio issuer (v1)
	// OID 1.3.6.1.4.1.57264.1.2 is the GitHub workflow trigger
	// OID 1.3.6.1.4.1.57264.1.8 is the subject alternative name (SAN)

	// Try to extract from SAN extension (OID 1.3.6.1.4.1.57264.1.8)
	for _, ext := range cert.Extensions {
		if ext.Id.String() == "1.3.6.1.4.1.57264.1.8" {
			// The value is typically a URI or email
			identity := string(ext.Value)
			if identity != "" {
				return identity, nil
			}
		}
	}

	// Fallback to email addresses in certificate
	if len(cert.EmailAddresses) > 0 {
		return cert.EmailAddresses[0], nil
	}

	// Check Subject Alternative Names (SAN) for URI
	for _, uri := range cert.URIs {
		if uri != nil {
			return uri.String(), nil
		}
	}

	// Check common name as last resort
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName, nil
	}

	return "", fmt.Errorf("no identity found in certificate")
}

// extractIssuerFromCert extracts the OIDC issuer from a certificate.
func extractIssuerFromCert(cert *x509.Certificate) (string, error) {
	// Sigstore stores the issuer in the certificate extensions
	// OID 1.3.6.1.4.1.57264.1.1 is the Fulcio issuer extension
	for _, ext := range cert.Extensions {
		if ext.Id.String() == "1.3.6.1.4.1.57264.1.1" {
			return string(ext.Value), nil
		}
	}

	// Fallback to certificate issuer
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName, nil
	}

	return "", fmt.Errorf("no issuer found in certificate")
}

// verifyCertificateChain verifies the certificate chain against Fulcio roots.
// This uses TUF (The Update Framework) to get trusted Fulcio root certificates.
func (v *CosignVerifier) verifyCertificateChain(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("certificate is nil")
	}

	// Get TUF roots from Sigstore
	tufClient, err := tuf.NewFromEnv(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create TUF client: %w", err)
	}

	// Get Fulcio root certificates from TUF
	fulcioRoots, err := tufClient.GetTarget("fulcio.crt.pem")
	if err != nil {
		return fmt.Errorf("failed to get Fulcio roots from TUF: %w", err)
	}

	// Build certificate pool from TUF roots
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(fulcioRoots) {
		return fmt.Errorf("failed to parse Fulcio root certificates")
	}

	// Verify certificate chain
	verifyOpts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}

	chains, err := cert.Verify(verifyOpts)
	if err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}

	if len(chains) == 0 {
		return fmt.Errorf("no valid certificate chain found")
	}

	// Additional Fulcio-specific validations
	// This validates SCT (Signed Certificate Timestamp) and Fulcio extensions
	_, err = cosign.ValidateAndUnpackCert(cert, &cosign.CheckOpts{
		IgnoreSCT:  false, // Verify SCT (Certificate Transparency)
		IgnoreTlog: true,  // Don't check Rekor here (handled separately)
	})
	if err != nil {
		return fmt.Errorf("Fulcio certificate validation failed: %w", err)
	}

	// Certificate chain verification passed
	return nil
}
