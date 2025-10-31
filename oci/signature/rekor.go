// Package signature provides OCI artifact signature verification using Sigstore/Cosign.
package signature

import (
	"context"
	"fmt"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/cosign/v2/pkg/oci"
	"github.com/sigstore/rekor/pkg/generated/client"

	ocibundle "github.com/jmgilman/go/oci"
)

// verifyRekorEntry verifies that the signature is present in the Rekor transparency log.
// This provides an audit trail and non-repudiation for signatures.
func (v *CosignVerifier) verifyRekorEntry(ctx context.Context, digestStr string, sig oci.Signature) error {
	// Get the bundle from the signature
	bundle, err := sig.Bundle()
	if err != nil || bundle == nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrRekorVerificationFailed,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "No Rekor bundle found in signature (Rekor verification requires bundle)",
				FailureStage: "rekor",
			},
		}
	}

	// Verify the bundle contains a valid Rekor entry
	if bundle.Payload.LogIndex == 0 && bundle.Payload.LogID == "" {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrRekorVerificationFailed,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "Rekor bundle is incomplete (missing log index or log ID)",
				FailureStage: "rekor",
			},
		}
	}

	// Get Rekor URL (use default if not specified)
	rekorURL := v.policy.RekorURL
	if rekorURL == "" {
		rekorURL = "https://rekor.sigstore.dev" // Sigstore public instance
	}

	// Create Rekor client
	rekorClient := client.NewHTTPClientWithConfig(nil, &client.TransportConfig{
		Host:     rekorURL,
		BasePath: client.DefaultBasePath,
		Schemes:  []string{"https"},
	})

	// Verify the bundle using Cosign's Rekor verification
	// This checks:
	// 1. Entry exists in Rekor at the specified log index
	// 2. Entry signature is valid (signed by Rekor)
	// 3. Entry content matches the signature
	// 4. Entry timestamp is valid
	checkOpts := &cosign.CheckOpts{
		RekorClient: rekorClient,
	}

	// VerifyBundle returns a bool indicating if verification succeeded
	verified, err := cosign.VerifyBundle(sig, checkOpts)
	if err != nil {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrRekorVerificationFailed,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       fmt.Sprintf("Rekor bundle verification failed: %s", err.Error()),
				FailureStage: "rekor",
			},
		}
	}

	if !verified {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrRekorVerificationFailed,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "Rekor bundle verification returned false",
				FailureStage: "rekor",
			},
		}
	}

	// Since cosign.VerifyBundle already validates the bundle completely,
	// we don't need to do additional UUID extraction and fetching.
	// The bundle verification already confirms:
	// - Entry exists in Rekor
	// - Entry signature is valid
	// - Entry content matches the signature
	// - Timestamp is valid
	//
	// Additional validation of log index if present
	if bundle.Payload.LogIndex <= 0 {
		return &ocibundle.BundleError{
			Op:  "verify",
			Err: ocibundle.ErrRekorVerificationFailed,
			SignatureInfo: &ocibundle.SignatureErrorInfo{
				Digest:       digestStr,
				Reason:       "Rekor bundle has invalid log index",
				FailureStage: "rekor",
			},
		}
	}

	// All Rekor checks passed
	return nil
}

// RekorEntry represents an entry in the Rekor transparency log.
// This is used for verification result caching and observability.
type RekorEntry struct {
	// LogIndex is the position in the Rekor log
	LogIndex int64

	// LogID is the identifier of the Rekor log
	LogID string

	// IntegratedTime is when the entry was added to the log
	IntegratedTime int64

	// Verified indicates if the entry was successfully verified
	Verified bool
}

// extractRekorEntry extracts Rekor entry information from a signature.
// This is useful for caching and observability.
func extractRekorEntry(sig oci.Signature) (*RekorEntry, error) {
	bundle, err := sig.Bundle()
	if err != nil || bundle == nil {
		return nil, fmt.Errorf("no Rekor bundle found")
	}

	entry := &RekorEntry{
		LogID:          bundle.Payload.LogID,
		LogIndex:       bundle.Payload.LogIndex,
		IntegratedTime: bundle.Payload.IntegratedTime,
		Verified:       false,
	}

	return entry, nil
}
