// Package testutil provides testing utilities for the OCI bundle package.
// This file contains mock implementations of the SignatureVerifier interface.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jmgilman/go/oci/internal/oras"
)

// Signature error sentinel values (duplicated from main package to avoid import cycle)
var (
	ErrSignatureNotFound          = errors.New("signature not found")
	ErrSignatureInvalid           = errors.New("signature verification failed")
	ErrUntrustedSigner            = errors.New("untrusted signer")
	ErrRekorVerificationFailed    = errors.New("rekor verification failed")
	ErrCertificateExpired         = errors.New("certificate expired")
	ErrInvalidAnnotations         = errors.New("required annotations missing or invalid")
)

// SignatureErrorInfo provides detailed context about signature verification failures.
type SignatureErrorInfo struct {
	Digest       string
	Signer       string
	Reason       string
	FailureStage string
}

// BundleError provides detailed context about OCI bundle operation failures.
type BundleError struct {
	Op            string
	Reference     string
	Err           error
	SignatureInfo *SignatureErrorInfo
}

func (e *BundleError) Error() string {
	return e.Err.Error()
}

func (e *BundleError) Unwrap() error {
	return e.Err
}

// MockVerifier is a mock implementation of the SignatureVerifier interface.
// It allows tests to control verification behavior and track verification calls.
type MockVerifier struct {
	// mu protects concurrent access to the mock state
	mu sync.Mutex

	// ShouldSucceed determines whether Verify() returns success or failure
	ShouldSucceed bool

	// ErrorToReturn is the error that will be returned when ShouldSucceed is false
	// If nil and ShouldSucceed is false, a default error is returned
	ErrorToReturn error

	// VerifyCalls tracks all calls to Verify() for assertion purposes
	VerifyCalls []VerifyCall

	// BeforeVerify is an optional callback that runs before verification
	// This can be used to inject custom behavior or assertions
	BeforeVerify func(ctx context.Context, reference string, descriptor *oras.PullDescriptor)
}

// VerifyCall represents a single call to Verify()
type VerifyCall struct {
	Context    context.Context
	Reference  string
	Descriptor *oras.PullDescriptor
}

// NewMockVerifier creates a new MockVerifier that returns the given error.
// If err is nil, the verifier will succeed.
func NewMockVerifier(err error) *MockVerifier {
	if err == nil {
		return &MockVerifier{
			ShouldSucceed: true,
			VerifyCalls:   make([]VerifyCall, 0),
		}
	}

	return &MockVerifier{
		ShouldSucceed: false,
		ErrorToReturn: err,
		VerifyCalls:   make([]VerifyCall, 0),
	}
}

// NewSuccessVerifier creates a MockVerifier that always succeeds.
func NewSuccessVerifier() *MockVerifier {
	return &MockVerifier{
		ShouldSucceed: true,
		VerifyCalls:   make([]VerifyCall, 0),
	}
}

// NewFailureVerifier creates a MockVerifier that always fails with the given error.
// If err is nil, a default ErrSignatureInvalid error is used.
func NewFailureVerifier(err error) *MockVerifier {
	if err == nil {
		err = &BundleError{
			Op:        "verify",
			Reference: "",
			Err:       ErrSignatureInvalid,
			SignatureInfo: &SignatureErrorInfo{
				Digest:       "",
				Signer:       "",
				Reason:       "mock verifier configured to fail",
				FailureStage: "crypto",
			},
		}
	}

	return &MockVerifier{
		ShouldSucceed: false,
		ErrorToReturn: err,
		VerifyCalls:   make([]VerifyCall, 0),
	}
}

// Verify implements the SignatureVerifier interface.
// It records the call and returns success or failure based on ShouldSucceed.
func (m *MockVerifier) Verify(ctx context.Context, reference string, descriptor *oras.PullDescriptor) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record the call
	m.VerifyCalls = append(m.VerifyCalls, VerifyCall{
		Context:    ctx,
		Reference:  reference,
		Descriptor: descriptor,
	})

	// Call optional before callback
	if m.BeforeVerify != nil {
		m.BeforeVerify(ctx, reference, descriptor)
	}

	// Return success or failure based on configuration
	if m.ShouldSucceed {
		return nil
	}

	// Return configured error or default
	if m.ErrorToReturn != nil {
		return m.ErrorToReturn
	}

	// Default error
	return &BundleError{
		Op:        "verify",
		Reference: reference,
		Err:       ErrSignatureInvalid,
		SignatureInfo: &SignatureErrorInfo{
			Digest:       descriptor.Digest,
			Signer:       "",
			Reason:       "mock verifier configured to fail",
			FailureStage: "crypto",
		},
	}
}

// CallCount returns the number of times Verify() has been called.
func (m *MockVerifier) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.VerifyCalls)
}

// GetCall returns the Nth call to Verify() (0-indexed).
// Returns nil if the index is out of bounds.
func (m *MockVerifier) GetCall(index int) *VerifyCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.VerifyCalls) {
		return nil
	}

	return &m.VerifyCalls[index]
}

// LastCall returns the most recent call to Verify().
// Returns nil if Verify() has never been called.
func (m *MockVerifier) LastCall() *VerifyCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.VerifyCalls) == 0 {
		return nil
	}

	return &m.VerifyCalls[len(m.VerifyCalls)-1]
}

// WasCalledWith checks if Verify() was called with the given reference.
// Returns true if any call used the specified reference.
func (m *MockVerifier) WasCalledWith(reference string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, call := range m.VerifyCalls {
		if call.Reference == reference {
			return true
		}
	}
	return false
}

// WasCalledWithDigest checks if Verify() was called with the given digest.
// Returns true if any call used a descriptor with the specified digest.
func (m *MockVerifier) WasCalledWithDigest(digest string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, call := range m.VerifyCalls {
		if call.Descriptor != nil && call.Descriptor.Digest == digest {
			return true
		}
	}
	return false
}

// Reset clears all recorded calls and resets the verifier state.
// This is useful for reusing a mock verifier across multiple test cases.
func (m *MockVerifier) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.VerifyCalls = make([]VerifyCall, 0)
}

// SetSuccess configures the verifier to succeed on future calls.
func (m *MockVerifier) SetSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ShouldSucceed = true
	m.ErrorToReturn = nil
}

// SetFailure configures the verifier to fail on future calls with the given error.
// If err is nil, a default ErrSignatureInvalid error is used.
func (m *MockVerifier) SetFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ShouldSucceed = false
	m.ErrorToReturn = err
}

// String returns a string representation of the mock verifier for debugging.
func (m *MockVerifier) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return fmt.Sprintf("MockVerifier{ShouldSucceed: %v, CallCount: %d}",
		m.ShouldSucceed, len(m.VerifyCalls))
}

// CallbackVerifier is a mock verifier that calls a custom function for verification.
// This allows tests to inject custom logic and assertions.
type CallbackVerifier struct {
	VerifyFunc func(ctx context.Context, reference string, descriptor *oras.PullDescriptor) error
}

// Verify implements the SignatureVerifier interface by calling the custom function.
func (c *CallbackVerifier) Verify(ctx context.Context, reference string, descriptor *oras.PullDescriptor) error {
	if c.VerifyFunc != nil {
		return c.VerifyFunc(ctx, reference, descriptor)
	}
	return nil
}

// TrackingVerifier is a mock verifier that returns different results on each call.
// This is useful for testing multi-signature scenarios.
type TrackingVerifier struct {
	mu         sync.Mutex
	Results    []error
	CallIndex  int
	VerifyCalls []VerifyCall
}

// Verify implements the SignatureVerifier interface.
// It returns results from the Results slice in sequence.
func (t *TrackingVerifier) Verify(ctx context.Context, reference string, descriptor *oras.PullDescriptor) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Record the call
	t.VerifyCalls = append(t.VerifyCalls, VerifyCall{
		Context:    ctx,
		Reference:  reference,
		Descriptor: descriptor,
	})

	// Return the next result
	if t.CallIndex < len(t.Results) {
		result := t.Results[t.CallIndex]
		t.CallIndex++
		return result
	}

	// If we've exhausted the results, return success
	return nil
}

// CallCount returns the number of times Verify() has been called.
func (t *TrackingVerifier) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.VerifyCalls)
}

// Reset clears the call history and resets the call index.
func (t *TrackingVerifier) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.VerifyCalls = make([]VerifyCall, 0)
	t.CallIndex = 0
}
