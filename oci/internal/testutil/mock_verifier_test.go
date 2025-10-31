package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/jmgilman/go/oci/internal/oras"
)

func TestNewMockVerifier(t *testing.T) {
	verifier := NewMockVerifier(nil)

	if verifier == nil {
		t.Fatal("NewMockVerifier returned nil")
	}

	if !verifier.ShouldSucceed {
		t.Error("Expected ShouldSucceed to be true by default")
	}

	if verifier.CallCount() != 0 {
		t.Errorf("Expected CallCount to be 0, got %d", verifier.CallCount())
	}
}

func TestNewSuccessVerifier(t *testing.T) {
	verifier := NewSuccessVerifier()

	ctx := context.Background()
	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123",
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Size:      1024,
	}

	err := verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if verifier.CallCount() != 1 {
		t.Errorf("Expected CallCount to be 1, got %d", verifier.CallCount())
	}
}

func TestNewFailureVerifier(t *testing.T) {
	customErr := errors.New("custom error")
	verifier := NewFailureVerifier(customErr)

	ctx := context.Background()
	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123",
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Size:      1024,
	}

	err := verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !errors.Is(err, customErr) {
		t.Errorf("Expected custom error, got: %v", err)
	}
}

func TestNewFailureVerifier_DefaultError(t *testing.T) {
	verifier := NewFailureVerifier(nil)

	ctx := context.Background()
	descriptor := &oras.PullDescriptor{
		Digest:    "sha256:abc123",
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Size:      1024,
	}

	err := verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	var bundleErr *BundleError
	if !errors.As(err, &bundleErr) {
		t.Errorf("Expected BundleError, got: %T", err)
	}

	if !errors.Is(bundleErr.Err, ErrSignatureInvalid) {
		t.Error("Expected signature error")
	}
}

func TestMockVerifier_Verify_RecordsCalls(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	testCases := []struct {
		reference string
		digest    string
	}{
		{"ghcr.io/org/repo1:v1.0", "sha256:abc123"},
		{"ghcr.io/org/repo2:v2.0", "sha256:def456"},
		{"ghcr.io/org/repo3:v3.0", "sha256:ghi789"},
	}

	for _, tc := range testCases {
		descriptor := &oras.PullDescriptor{
			Digest:    tc.digest,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Size:      1024,
		}

		err := verifier.Verify(ctx, tc.reference, descriptor)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	if verifier.CallCount() != len(testCases) {
		t.Errorf("Expected CallCount to be %d, got %d", len(testCases), verifier.CallCount())
	}

	for i, tc := range testCases {
		call := verifier.GetCall(i)
		if call == nil {
			t.Errorf("Call %d is nil", i)
			continue
		}

		if call.Reference != tc.reference {
			t.Errorf("Call %d: expected reference %s, got %s", i, tc.reference, call.Reference)
		}

		if call.Descriptor.Digest != tc.digest {
			t.Errorf("Call %d: expected digest %s, got %s", i, tc.digest, call.Descriptor.Digest)
		}
	}
}

func TestMockVerifier_LastCall(t *testing.T) {
	verifier := NewSuccessVerifier()

	// No calls yet
	if verifier.LastCall() != nil {
		t.Error("Expected LastCall to be nil when no calls have been made")
	}

	ctx := context.Background()
	descriptor1 := &oras.PullDescriptor{Digest: "sha256:first", Size: 100}
	descriptor2 := &oras.PullDescriptor{Digest: "sha256:second", Size: 200}

	_ = verifier.Verify(ctx, "first-ref", descriptor1)
	_ = verifier.Verify(ctx, "second-ref", descriptor2)

	lastCall := verifier.LastCall()
	if lastCall == nil {
		t.Fatal("LastCall returned nil")
	}

	if lastCall.Reference != "second-ref" {
		t.Errorf("Expected last reference to be 'second-ref', got %s", lastCall.Reference)
	}

	if lastCall.Descriptor.Digest != "sha256:second" {
		t.Errorf("Expected last digest to be 'sha256:second', got %s", lastCall.Descriptor.Digest)
	}
}

func TestMockVerifier_WasCalledWith(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	descriptor := &oras.PullDescriptor{Digest: "sha256:test", Size: 100}
	_ = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)

	if !verifier.WasCalledWith("ghcr.io/org/repo:v1.0") {
		t.Error("Expected WasCalledWith to return true for 'ghcr.io/org/repo:v1.0'")
	}

	if verifier.WasCalledWith("ghcr.io/org/other:v1.0") {
		t.Error("Expected WasCalledWith to return false for 'ghcr.io/org/other:v1.0'")
	}
}

func TestMockVerifier_WasCalledWithDigest(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	descriptor := &oras.PullDescriptor{Digest: "sha256:abc123", Size: 100}
	_ = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)

	if !verifier.WasCalledWithDigest("sha256:abc123") {
		t.Error("Expected WasCalledWithDigest to return true for 'sha256:abc123'")
	}

	if verifier.WasCalledWithDigest("sha256:other") {
		t.Error("Expected WasCalledWithDigest to return false for 'sha256:other'")
	}
}

func TestMockVerifier_Reset(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	descriptor := &oras.PullDescriptor{Digest: "sha256:test", Size: 100}
	_ = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)

	if verifier.CallCount() != 1 {
		t.Errorf("Expected CallCount to be 1, got %d", verifier.CallCount())
	}

	verifier.Reset()

	if verifier.CallCount() != 0 {
		t.Errorf("Expected CallCount to be 0 after Reset, got %d", verifier.CallCount())
	}

	if verifier.WasCalledWith("ghcr.io/org/repo:v1.0") {
		t.Error("Expected WasCalledWith to return false after Reset")
	}
}

func TestMockVerifier_SetSuccess(t *testing.T) {
	verifier := NewFailureVerifier(errors.New("test error"))
	ctx := context.Background()

	descriptor := &oras.PullDescriptor{Digest: "sha256:test", Size: 100}

	// Should fail initially
	err := verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Change to success
	verifier.SetSuccess()

	// Should succeed now
	err = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
}

func TestMockVerifier_SetFailure(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	descriptor := &oras.PullDescriptor{Digest: "sha256:test", Size: 100}

	// Should succeed initially
	err := verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	// Change to failure
	customErr := errors.New("custom error")
	verifier.SetFailure(customErr)

	// Should fail now
	err = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !errors.Is(err, customErr) {
		t.Errorf("Expected custom error, got: %v", err)
	}
}

func TestMockVerifier_BeforeVerify(t *testing.T) {
	verifier := NewSuccessVerifier()
	ctx := context.Background()

	callbackExecuted := false
	var callbackReference string
	var callbackDigest string

	verifier.BeforeVerify = func(_ context.Context, reference string, descriptor *oras.PullDescriptor) {
		callbackExecuted = true
		callbackReference = reference
		callbackDigest = descriptor.Digest
	}

	descriptor := &oras.PullDescriptor{Digest: "sha256:test123", Size: 100}
	_ = verifier.Verify(ctx, "ghcr.io/org/repo:v1.0", descriptor)

	if !callbackExecuted {
		t.Error("BeforeVerify callback was not executed")
	}

	if callbackReference != "ghcr.io/org/repo:v1.0" {
		t.Errorf("Expected callback reference 'ghcr.io/org/repo:v1.0', got %s", callbackReference)
	}

	if callbackDigest != "sha256:test123" {
		t.Errorf("Expected callback digest 'sha256:test123', got %s", callbackDigest)
	}
}
