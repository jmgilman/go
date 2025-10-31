// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
//
// This file contains utility functions for extracting identity and issuer information
// from Fulcio certificates. The main verification logic has been delegated to Cosign's
// high-level APIs (see verifier.go and cosign_adapter.go).
package signature

import (
	"crypto/x509"
	"fmt"
	"strings"
	"unicode/utf8"
)

// extractIdentityFromCert extracts the signer identity from a Fulcio certificate.
// This is typically the email address or subject from the OIDC token.
// Fulcio stores identity in custom OID extensions.
//
// This function is still used for audit logging and extracting signer information
// from verified signatures, even though the main verification is now handled by Cosign.
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

			// Validate identity to prevent injection attacks
			if identity != "" {
				// Check for null bytes (common injection vector)
				if strings.ContainsRune(identity, 0) {
					return "", fmt.Errorf("identity contains null byte")
				}

				// Validate UTF-8 encoding
				if !utf8.ValidString(identity) {
					return "", fmt.Errorf("identity is not valid UTF-8")
				}

				// Check for control characters (except common whitespace)
				for _, r := range identity {
					if r < 32 && r != '\t' && r != '\n' && r != '\r' {
						return "", fmt.Errorf("identity contains control character")
					}
				}

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

// extractIssuerFromCert extracts the OIDC issuer from a Fulcio certificate.
// This function is kept for potential future use in policy validation or audit logging.
func extractIssuerFromCert(cert *x509.Certificate) (string, error) {
	// Sigstore stores the issuer in the certificate extensions
	// OID 1.3.6.1.4.1.57264.1.1 is the Fulcio issuer extension
	for _, ext := range cert.Extensions {
		if ext.Id.String() == "1.3.6.1.4.1.57264.1.1" {
			issuer := string(ext.Value)

			// Validate issuer to prevent injection attacks
			if issuer != "" {
				// Check for null bytes
				if strings.ContainsRune(issuer, 0) {
					return "", fmt.Errorf("issuer contains null byte")
				}

				// Validate UTF-8 encoding
				if !utf8.ValidString(issuer) {
					return "", fmt.Errorf("issuer is not valid UTF-8")
				}

				// Check for control characters
				for _, r := range issuer {
					if r < 32 && r != '\t' && r != '\n' && r != '\r' {
						return "", fmt.Errorf("issuer contains control character")
					}
				}

				return issuer, nil
			}
		}
	}

	// Fallback to certificate issuer
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName, nil
	}

	return "", fmt.Errorf("no issuer found in certificate")
}
