package cache

import (
	"context"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/billy"
)

func TestVerificationResult_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		result    *VerificationResult
		wantExpired bool
	}{
		{
			name: "not expired",
			result: &VerificationResult{
				Timestamp: time.Now(),
				TTL:       time.Hour,
			},
			wantExpired: false,
		},
		{
			name: "expired",
			result: &VerificationResult{
				Timestamp: time.Now().Add(-2 * time.Hour),
				TTL:       time.Hour,
			},
			wantExpired: true,
		},
		{
			name: "zero TTL never expires",
			result: &VerificationResult{
				Timestamp: time.Now().Add(-100 * time.Hour),
				TTL:       0,
			},
			wantExpired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsExpired(); got != tt.wantExpired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.wantExpired)
			}
		})
	}
}

func TestVerificationResult_Size(t *testing.T) {
	result := &VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "test@example.com",
		PolicyHash: "policy123",
		TTL:        time.Hour,
	}

	size := result.Size()
	if size <= 0 {
		t.Errorf("Size() = %d, want > 0", size)
	}

	// Size should include all string fields
	minExpectedSize := int64(len(result.Digest) + len(result.Signer) + len(result.PolicyHash))
	if size < minExpectedSize {
		t.Errorf("Size() = %d, want >= %d", size, minExpectedSize)
	}
}

func TestVerificationResult_ToEntry(t *testing.T) {
	result := &VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "test@example.com",
		Timestamp:  time.Now(),
		PolicyHash: "policy123",
		TTL:        time.Hour,
	}

	entry, err := result.ToEntry("test-key")
	if err != nil {
		t.Fatalf("ToEntry() error = %v", err)
	}

	if entry.Key != "test-key" {
		t.Errorf("Entry.Key = %v, want %v", entry.Key, "test-key")
	}

	if entry.TTL != result.TTL {
		t.Errorf("Entry.TTL = %v, want %v", entry.TTL, result.TTL)
	}

	// Check metadata
	if entry.Metadata["type"] != "verification" {
		t.Errorf("Metadata[type] = %v, want verification", entry.Metadata["type"])
	}
	if entry.Metadata["digest"] != result.Digest {
		t.Errorf("Metadata[digest] = %v, want %v", entry.Metadata["digest"], result.Digest)
	}
	if entry.Metadata["verified"] != "true" {
		t.Errorf("Metadata[verified] = %v, want true", entry.Metadata["verified"])
	}
}

func TestVerificationResultFromEntry(t *testing.T) {
	original := &VerificationResult{
		Digest:     "sha256:abc123",
		Verified:   true,
		Signer:     "test@example.com",
		Timestamp:  time.Now().Truncate(time.Second), // Truncate for comparison
		PolicyHash: "policy123",
		TTL:        time.Hour,
	}

	entry, err := original.ToEntry("test-key")
	if err != nil {
		t.Fatalf("ToEntry() error = %v", err)
	}

	result, err := VerificationResultFromEntry(entry)
	if err != nil {
		t.Fatalf("VerificationResultFromEntry() error = %v", err)
	}

	if result.Digest != original.Digest {
		t.Errorf("Digest = %v, want %v", result.Digest, original.Digest)
	}
	if result.Verified != original.Verified {
		t.Errorf("Verified = %v, want %v", result.Verified, original.Verified)
	}
	if result.Signer != original.Signer {
		t.Errorf("Signer = %v, want %v", result.Signer, original.Signer)
	}
	if result.PolicyHash != original.PolicyHash {
		t.Errorf("PolicyHash = %v, want %v", result.PolicyHash, original.PolicyHash)
	}
}

func TestRekorLogEntry_Size(t *testing.T) {
	entry := &RekorLogEntry{
		LogIndex:       12345,
		UUID:           "tree-id-entry-id",
		IntegratedTime: time.Now(),
		LogID:          "log-id-hash",
		Body:           "base64-encoded-body",
	}

	size := entry.Size()
	if size <= 0 {
		t.Errorf("Size() = %d, want > 0", size)
	}

	// Size should include string fields
	minExpectedSize := int64(len(entry.UUID) + len(entry.LogID) + len(entry.Body))
	if size < minExpectedSize {
		t.Errorf("Size() = %d, want >= %d", size, minExpectedSize)
	}
}

func TestCoordinator_VerificationCache(t *testing.T) {
	ctx := context.Background()

	// Create test filesystem
	fs := billy.NewMemory()

	// Create cache coordinator
	config := Config{
		MaxSizeBytes: 10 * 1024 * 1024, // 10MB
		DefaultTTL:   time.Hour,
	}

	coordinator, err := NewCoordinator(ctx, config, fs, "/cache", NewNopLogger())
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := coordinator.Close(); closeErr != nil {
			t.Logf("Failed to close coordinator: %v", closeErr)
		}
	})

	// Test data
	digest := "sha256:abc123def456"
	policyHash := "policy-hash-123"
	signer := "test@example.com"
	ttl := time.Hour

	t.Run("cache miss", func(t *testing.T) {
		verified, signerOut, err := coordinator.GetCachedVerification(ctx, digest, "nonexistent-policy")
		if err == nil {
			t.Error("Expected error for cache miss, got nil")
		}
		if verified {
			t.Error("Expected verified=false for cache miss")
		}
		if signerOut != "" {
			t.Errorf("Expected empty signer for cache miss, got %s", signerOut)
		}
	})

	t.Run("put and get successful verification", func(t *testing.T) {
		// Store verification result
		err := coordinator.PutCachedVerification(ctx, digest, policyHash, true, signer, ttl)
		if err != nil {
			t.Fatalf("PutCachedVerification() error = %v", err)
		}

		// Retrieve verification result
		verified, signerOut, err := coordinator.GetCachedVerification(ctx, digest, policyHash)
		if err != nil {
			t.Fatalf("GetCachedVerification() error = %v", err)
		}

		if !verified {
			t.Error("Expected verified=true, got false")
		}
		if signerOut != signer {
			t.Errorf("Signer = %v, want %v", signerOut, signer)
		}
	})

	t.Run("put and get failed verification", func(t *testing.T) {
		failedDigest := "sha256:failed"

		// Store failed verification result
		err := coordinator.PutCachedVerification(ctx, failedDigest, policyHash, false, "", ttl)
		if err != nil {
			t.Fatalf("PutCachedVerification() error = %v", err)
		}

		// Retrieve verification result
		verified, signerOut, err := coordinator.GetCachedVerification(ctx, failedDigest, policyHash)
		if err != nil {
			t.Fatalf("GetCachedVerification() error = %v", err)
		}

		if verified {
			t.Error("Expected verified=false, got true")
		}
		if signerOut != "" {
			t.Errorf("Expected empty signer for failed verification, got %s", signerOut)
		}
	})

	t.Run("cache invalidation on policy change", func(t *testing.T) {
		newDigest := "sha256:new-artifact"

		// Store with one policy
		err := coordinator.PutCachedVerification(ctx, newDigest, "policy-v1", true, signer, ttl)
		if err != nil {
			t.Fatalf("PutCachedVerification() error = %v", err)
		}

		// Try to retrieve with different policy - should miss
		_, _, err = coordinator.GetCachedVerification(ctx, newDigest, "policy-v2")
		if err == nil {
			t.Error("Expected cache miss with different policy hash, got hit")
		}

		// Retrieve with correct policy - should hit
		verified, _, err := coordinator.GetCachedVerification(ctx, newDigest, "policy-v1")
		if err != nil {
			t.Fatalf("GetCachedVerification() error = %v", err)
		}
		if !verified {
			t.Error("Expected verified=true with correct policy hash")
		}
	})

	t.Run("expired entry", func(t *testing.T) {
		expiredDigest := "sha256:expired"
		shortTTL := 1 * time.Millisecond

		// Store with very short TTL
		err := coordinator.PutCachedVerification(ctx, expiredDigest, policyHash, true, signer, shortTTL)
		if err != nil {
			t.Fatalf("PutCachedVerification() error = %v", err)
		}

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		// Try to retrieve - should be expired
		_, _, err = coordinator.GetCachedVerification(ctx, expiredDigest, policyHash)
		if err == nil {
			t.Error("Expected error for expired entry, got nil")
		}
	})
}

func TestCoordinator_GetVerificationResult(t *testing.T) {
	ctx := context.Background()
	fs := billy.NewMemory()

	config := Config{
		MaxSizeBytes: 10 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator, err := NewCoordinator(ctx, config, fs, "/cache", NewNopLogger())
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := coordinator.Close(); closeErr != nil {
			t.Logf("Failed to close coordinator: %v", closeErr)
		}
	})

	result := &VerificationResult{
		Digest:     "sha256:test123",
		Verified:   true,
		Signer:     "signer@example.com",
		Timestamp:  time.Now(),
		PolicyHash: "policy-abc",
		TTL:        time.Hour,
	}

	// Store verification result
	err = coordinator.PutVerificationResult(ctx, result)
	if err != nil {
		t.Fatalf("PutVerificationResult() error = %v", err)
	}

	// Retrieve verification result
	retrieved, err := coordinator.GetVerificationResult(ctx, result.Digest, result.PolicyHash)
	if err != nil {
		t.Fatalf("GetVerificationResult() error = %v", err)
	}

	if retrieved.Digest != result.Digest {
		t.Errorf("Digest = %v, want %v", retrieved.Digest, result.Digest)
	}
	if retrieved.Verified != result.Verified {
		t.Errorf("Verified = %v, want %v", retrieved.Verified, result.Verified)
	}
	if retrieved.Signer != result.Signer {
		t.Errorf("Signer = %v, want %v", retrieved.Signer, result.Signer)
	}
	if retrieved.PolicyHash != result.PolicyHash {
		t.Errorf("PolicyHash = %v, want %v", retrieved.PolicyHash, result.PolicyHash)
	}
}

func TestCoordinator_PutVerificationResult_WithRekor(t *testing.T) {
	ctx := context.Background()
	fs := billy.NewMemory()

	config := Config{
		MaxSizeBytes: 10 * 1024 * 1024,
		DefaultTTL:   time.Hour,
	}

	coordinator, err := NewCoordinator(ctx, config, fs, "/cache", NewNopLogger())
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := coordinator.Close(); closeErr != nil {
			t.Logf("Failed to close coordinator: %v", closeErr)
		}
	})

	rekorEntry := &RekorLogEntry{
		LogIndex:       12345,
		UUID:           "tree-entry-uuid",
		IntegratedTime: time.Now(),
		LogID:          "log-id-hash",
		Body:           "base64-body",
	}

	result := &VerificationResult{
		Digest:      "sha256:with-rekor",
		Verified:    true,
		Signer:      "signer@example.com",
		Timestamp:   time.Now(),
		PolicyHash:  "policy-def",
		RekorEntry:  rekorEntry,
		TTL:         time.Hour,
	}

	// Store verification result with Rekor entry
	err = coordinator.PutVerificationResult(ctx, result)
	if err != nil {
		t.Fatalf("PutVerificationResult() error = %v", err)
	}

	// Retrieve and verify Rekor entry is preserved
	retrieved, err := coordinator.GetVerificationResult(ctx, result.Digest, result.PolicyHash)
	if err != nil {
		t.Fatalf("GetVerificationResult() error = %v", err)
	}

	if retrieved.RekorEntry == nil {
		t.Fatal("RekorEntry is nil, want non-nil")
	}

	if retrieved.RekorEntry.LogIndex != rekorEntry.LogIndex {
		t.Errorf("RekorEntry.LogIndex = %v, want %v", retrieved.RekorEntry.LogIndex, rekorEntry.LogIndex)
	}
	if retrieved.RekorEntry.UUID != rekorEntry.UUID {
		t.Errorf("RekorEntry.UUID = %v, want %v", retrieved.RekorEntry.UUID, rekorEntry.UUID)
	}
}
