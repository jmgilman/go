// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// LoadPublicKey loads a public key from a file.
// Supports PEM and DER encoded keys in various formats:
//   - PKCS#1 RSA public keys
//   - PKIX/SPKI format (most common, used by ssh-keygen -e)
//   - EC public keys
//   - Ed25519 public keys
//
// The file should contain a single public key. If the file contains a PEM block,
// the PEM encoding is stripped before parsing. Raw DER encoding is also supported.
//
// Example:
//
//	pubKey, err := LoadPublicKey("cosign.pub")
//	if err != nil {
//	    return fmt.Errorf("failed to load public key: %w", err)
//	}
//	verifier := NewPublicKeyVerifier(pubKey)
func LoadPublicKey(path string) (crypto.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file %s: %w", path, err)
	}

	key, err := LoadPublicKeyFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key from %s: %w", path, err)
	}

	return key, nil
}

// LoadPublicKeyFromBytes loads a public key from raw bytes.
// Supports both PEM-encoded and raw DER-encoded keys.
//
// The function attempts to parse the key in the following order:
//  1. PEM-encoded PKIX/SPKI public key (most common)
//  2. Raw DER-encoded PKIX public key
//  3. PEM-encoded PKCS#1 RSA public key
//  4. Raw DER-encoded PKCS#1 RSA public key
//
// Example:
//
//	keyBytes := []byte(`-----BEGIN PUBLIC KEY-----
//	MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
//	-----END PUBLIC KEY-----`)
//	pubKey, err := LoadPublicKeyFromBytes(keyBytes)
func LoadPublicKeyFromBytes(data []byte) (crypto.PublicKey, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("public key data is empty")
	}

	// Try PEM decoding first
	block, rest := pem.Decode(data)
	if block != nil {
		// Found PEM block - parse it
		key, err := parsePublicKeyDER(block.Bytes, block.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PEM public key (type: %s): %w", block.Type, err)
		}

		// Warn if there's extra data after the first PEM block
		if len(rest) > 0 {
			trimmed := len(data) - len(rest)
			if trimmed > 0 {
				// This is just informational - we successfully parsed the first key
				_ = fmt.Sprintf("warning: found extra data after PEM block (parsed %d bytes, %d remaining)", trimmed, len(rest))
			}
		}

		return key, nil
	}

	// No PEM encoding found - try raw DER
	key, err := parsePublicKeyDER(data, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key as PEM or DER: %w", err)
	}

	return key, nil
}

// parsePublicKeyDER parses a DER-encoded public key.
// The pemType parameter is optional and provides hints about the key format when
// the data comes from a PEM block.
func parsePublicKeyDER(der []byte, pemType string) (crypto.PublicKey, error) {
	// Try PKIX format first (most common, RFC 5280)
	// This is the format used by: openssl, ssh-keygen, cosign, etc.
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		return validatePublicKey(key)
	}

	// Try PKCS#1 RSA public key format
	// This is an older format but still used by some tools
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return validatePublicKey(key)
	}

	// If we have a PEM type hint, provide a more specific error
	if pemType != "" {
		return nil, fmt.Errorf("unsupported PEM type %q or invalid key format", pemType)
	}

	return nil, fmt.Errorf("unable to parse public key: not a valid PKIX or PKCS#1 format")
}

// validatePublicKey ensures the parsed key is of a supported type and has adequate strength.
// Returns the key if valid, or an error if the type is unsupported or key is too weak.
func validatePublicKey(key any) (crypto.PublicKey, error) {
	switch pub := key.(type) {
	case *rsa.PublicKey:
		if pub.N == nil {
			return nil, fmt.Errorf("invalid RSA public key: missing modulus")
		}
		// Validate RSA key strength (minimum 2048 bits for security)
		keySize := pub.N.BitLen()
		if keySize < 2048 {
			return nil, fmt.Errorf("RSA key size too small: %d bits (minimum: 2048)", keySize)
		}
		return pub, nil

	case *ecdsa.PublicKey:
		if pub.X == nil || pub.Y == nil {
			return nil, fmt.Errorf("invalid ECDSA public key: missing curve points")
		}
		// Validate ECDSA curve strength (minimum P-256, which is 256 bits)
		curveSize := pub.Curve.Params().BitSize
		if curveSize < 256 {
			return nil, fmt.Errorf("ECDSA curve too weak: %d bits (minimum: 256)", curveSize)
		}
		return pub, nil

	case ed25519.PublicKey:
		if len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid Ed25519 public key: wrong size (got %d, expected %d)", len(pub), ed25519.PublicKeySize)
		}
		// Ed25519 is always 256 bits - adequate strength
		return pub, nil

	default:
		return nil, fmt.Errorf("unsupported public key type: %T (supported: RSA, ECDSA, Ed25519)", key)
	}
}
