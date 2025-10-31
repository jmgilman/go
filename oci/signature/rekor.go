// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
//
// Rekor transparency log verification is now handled by Cosign's high-level APIs.
// See cosign_adapter.go:configureRekor() for the Rekor configuration logic that
// maps our Policy.RekorEnabled and Policy.RekorURL settings to Cosign's CheckOpts.
//
// The previous manual Rekor verification code has been removed as it is now
// delegated to Cosign's VerifyImageSignatures function, which handles:
// - Bundle extraction and validation
// - Rekor client creation and configuration
// - Bundle verification (entry existence, signature, content matching, timestamp)
// - Inclusion proof verification
//
// This reduces our security-critical bespoke code and aligns with industry-standard
// verification practices.
package signature
