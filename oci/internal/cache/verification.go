package cache

import (
	"context"
	"encoding/json"
	"time"
)

// VerificationResult represents the cached result of a signature verification operation.
// This allows verification results to be cached to avoid redundant cryptographic operations.
//
// The cache key is constructed as: "verify:<digest>:<policy-hash>"
// Where:
//   - <digest> is the artifact content digest (immutable)
//   - <policy-hash> is SHA256 of the policy configuration
//
// This ensures cache invalidation when:
//   - The artifact changes (different digest)
//   - The verification policy changes (different policy hash)
type VerificationResult struct {
	// Digest is the content digest of the verified artifact (e.g., "sha256:abc123...")
	Digest string `json:"digest"`

	// Verified indicates whether signature verification passed
	Verified bool `json:"verified"`

	// Signer is the identity of the entity that signed the artifact
	// For keyless signing: email address or OIDC subject
	// For public key signing: key fingerprint or identifier
	Signer string `json:"signer,omitempty"`

	// Timestamp is when this verification was performed
	Timestamp time.Time `json:"timestamp"`

	// PolicyHash is the SHA256 hash of the policy used for verification
	// This ensures cache invalidation when policy changes
	PolicyHash string `json:"policy_hash"`

	// RekorEntry contains transparency log information if Rekor verification was performed
	// This is optional and only populated when Rekor verification is enabled
	RekorEntry *RekorLogEntry `json:"rekor_entry,omitempty"`

	// TTL is the time-to-live for this cached result
	// Different TTLs are used based on verification mode:
	//   - Public key verification: 24 hours (keys don't expire)
	//   - Keyless verification: 1 hour (certificates expire, Rekor may change)
	TTL time.Duration `json:"ttl"`
}

// IsExpired checks if the cached verification result has expired.
// Expiration is determined by comparing the current time against the
// timestamp plus TTL.
func (vr *VerificationResult) IsExpired() bool {
	if vr.TTL <= 0 {
		return false // Zero TTL means never expires
	}
	return time.Since(vr.Timestamp) > vr.TTL
}

// Size returns the approximate size of the verification result in bytes.
// This is used for cache size tracking and eviction decisions.
func (vr *VerificationResult) Size() int64 {
	size := int64(len(vr.Digest))
	size += int64(len(vr.Signer))
	size += int64(len(vr.PolicyHash))

	// Add overhead for struct fields
	size += 8  // bool (8 bytes aligned)
	size += 16 // time.Time (approximate)
	size += 8  // TTL (int64)

	// Add RekorEntry size if present
	if vr.RekorEntry != nil {
		size += vr.RekorEntry.Size()
	}

	return size
}

// ToEntry converts a VerificationResult to a cache Entry for storage.
// The verification result is serialized to JSON and stored as the entry's data.
func (vr *VerificationResult) ToEntry(key string) (*Entry, error) {
	data, err := json.Marshal(vr)
	if err != nil {
		return nil, err
	}

	return &Entry{
		Key:        key,
		Data:       data,
		Metadata: map[string]string{
			"type":        "verification",
			"digest":      vr.Digest,
			"verified":    jsonBool(vr.Verified),
			"signer":      vr.Signer,
			"policy_hash": vr.PolicyHash,
		},
		CreatedAt:  vr.Timestamp,
		AccessedAt: vr.Timestamp,
		TTL:        vr.TTL,
	}, nil
}

// VerificationResultFromEntry reconstructs a VerificationResult from a cache Entry.
// This deserializes the JSON data stored in the entry.
func VerificationResultFromEntry(entry *Entry) (*VerificationResult, error) {
	var result VerificationResult
	if err := json.Unmarshal(entry.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RekorLogEntry represents an entry in the Rekor transparency log.
// Rekor is Sigstore's signature transparency log that provides
// non-repudiation and auditability for signature operations.
//
// This structure captures the essential information from a Rekor entry
// needed for verification and auditing purposes.
type RekorLogEntry struct {
	// LogIndex is the index of this entry in the Rekor log
	// This is a unique, monotonically increasing identifier
	LogIndex int64 `json:"log_index"`

	// UUID is the unique identifier for this Rekor entry
	// Format: <tree_id>-<entry_id>
	UUID string `json:"uuid"`

	// IntegratedTime is when the entry was added to the log
	// This provides a trusted timestamp for the signature
	IntegratedTime time.Time `json:"integrated_time"`

	// LogID is the identifier of the Rekor log that contains this entry
	// This is the SHA256 hash of the log's public key
	LogID string `json:"log_id"`

	// Body contains the entry body (base64 encoded)
	// This includes the signature and artifact information
	Body string `json:"body,omitempty"`
}

// Size returns the approximate size of the Rekor log entry in bytes.
func (r *RekorLogEntry) Size() int64 {
	size := int64(8)                // LogIndex (int64)
	size += int64(len(r.UUID))      // UUID string
	size += int64(16)               // IntegratedTime (approximate)
	size += int64(len(r.LogID))     // LogID string
	size += int64(len(r.Body))      // Body string
	return size
}

// jsonBool converts a boolean to a JSON string representation.
// This is a helper for storing boolean values in string-based metadata.
func jsonBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// GetCachedVerification retrieves a cached verification result.
// This method implements the signature.VerificationCache interface, allowing
// the Coordinator to be used as a cache for signature verification.
//
// Returns:
//   - verified=true, signer, nil if verification previously passed and is cached
//   - verified=false, "", nil if verification previously failed and is cached
//   - verified=false, "", error if cache miss or error accessing cache
func (cm *Coordinator) GetCachedVerification(ctx context.Context, digest, policyHash string) (verified bool, signer string, err error) {
	result, err := cm.GetVerificationResult(ctx, digest, policyHash)
	if err != nil {
		// Cache miss or error
		return false, "", err
	}

	return result.Verified, result.Signer, nil
}

// PutCachedVerification stores a verification result in the cache.
// This method implements the signature.VerificationCache interface, allowing
// the Coordinator to be used as a cache for signature verification.
//
// Parameters:
//   - digest: The artifact content digest
//   - policyHash: The hash of the verification policy
//   - verified: Whether verification passed
//   - signer: The identity of the signer (empty if unknown)
//   - ttl: Time-to-live for this cache entry
func (cm *Coordinator) PutCachedVerification(ctx context.Context, digest, policyHash string, verified bool, signer string, ttl time.Duration) error {
	result := &VerificationResult{
		Digest:     digest,
		Verified:   verified,
		Signer:     signer,
		Timestamp:  time.Now(),
		PolicyHash: policyHash,
		TTL:        ttl,
	}

	return cm.PutVerificationResult(ctx, result)
}
