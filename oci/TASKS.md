# Signature Verification Implementation Tasks

This document tracks the implementation tasks for adding signature verification support to the OCI module using a separate submodule architecture.

## Phase 1: Interface & Main Package Integration ✅

### 1.1 Interface Definition ✅
- [x] Create `oci/signature_interface.go`
  - [x] Define `SignatureVerifier` interface with `Verify()` method
  - [x] Add detailed documentation comments
  - [x] Add interface usage examples in comments

### 1.2 Error Types ✅
- [x] Update `oci/errors.go`
  - [x] Add `ErrSignatureNotFound` error
  - [x] Add `ErrSignatureInvalid` error
  - [x] Add `ErrUntrustedSigner` error
  - [x] Add `ErrRekorVerificationFailed` error
  - [x] Add `ErrCertificateExpired` error
  - [x] Add `ErrInvalidAnnotations` error
  - [x] Add `SignatureErrorInfo` struct to `BundleError`
  - [x] Add `IsSignatureError()` method to `BundleError`

### 1.3 Client Options ✅
- [x] Update `oci/options.go`
  - [x] Add `SignatureVerifier` field to `ClientOptions` struct
  - [x] Create `WithSignatureVerifier(verifier SignatureVerifier) ClientOption` function
  - [x] Add documentation for signature verification options

### 1.4 Client Integration ✅
- [x] Update `oci/client.go`
  - [x] Add helper method `shouldVerifySignature()`
  - [x] Integrate verification call in `Pull()` after line 360 (after manifest fetch)
  - [x] Add verification before `extractSelective()` path
  - [x] Add verification before `extractAtomically()` path
  - [x] Handle verification errors appropriately
  - [x] Close descriptor data on verification failure
  - [x] Update `isRetryableError()` to mark signature errors as non-retryable

### 1.5 Testing with Mocks ✅
- [x] Create mock signature verifier for testing
  - [x] Create `oci/internal/testutil/mock_verifier.go`
  - [x] Implement mock that returns success
  - [x] Implement mock that returns failures
  - [x] Implement mock that tracks verification calls

- [x] Update `oci/client_test.go`
  - [x] Test Pull() with nil verifier (existing behavior)
  - [x] Test Pull() with mock verifier that succeeds
  - [x] Test Pull() with mock verifier that fails
  - [x] Test error handling for signature verification failures
  - [x] Test that verification errors are not retried
  - [x] Test that descriptor data is closed on verification failure

## Phase 2: Signature Submodule Foundation

### 2.1 Submodule Setup
- [ ] Create directory structure
  - [ ] Create `oci/signature/` directory
  - [ ] Create `oci/signature/go.mod` with module path `github.com/jmgilman/go/oci/signature`
  - [ ] Create `oci/signature/go.sum` (initially empty)
  - [ ] Create `oci/signature/.gitignore` if needed

### 2.2 Dependencies
- [ ] Add dependencies to `oci/signature/go.mod`
  - [ ] Add `github.com/sigstore/cosign/v2` dependency
  - [ ] Add `github.com/sigstore/sigstore` dependency
  - [ ] Add `github.com/opencontainers/go-digest` dependency
  - [ ] Add `github.com/opencontainers/image-spec` dependency
  - [ ] Add `oras.land/oras-go/v2` dependency
  - [ ] Add parent `github.com/jmgilman/go/oci` dependency (for interface)
  - [ ] Run `go mod tidy`

### 2.3 Core Verifier Structure
- [ ] Create `oci/signature/verifier.go`
  - [ ] Define `CosignVerifier` struct
  - [ ] Implement `Verify()` method (interface compliance)
  - [ ] Add constructor `NewPublicKeyVerifier(keys ...*crypto.PublicKey) *CosignVerifier`
  - [ ] Add internal method `verifySignature()` for crypto operations
  - [ ] Add internal method `fetchSignatureArtifact()` for registry operations
  - [ ] Add internal method `buildSignatureReference()` for Cosign reference format

### 2.4 Policy Types
- [ ] Create `oci/signature/policy.go`
  - [ ] Define `Policy` struct with verification settings
  - [ ] Define `VerificationMode` enum (Optional, Required, Enforce)
  - [ ] Define `MultiSignatureMode` enum (Any, All, Minimum)
  - [ ] Add policy validation logic

### 2.5 Verifier Options
- [ ] Create `oci/signature/options.go`
  - [ ] Define `VerifierOption` functional option type
  - [ ] Implement `WithRequireAll(bool) VerifierOption`
  - [ ] Implement `WithMinimumSignatures(int) VerifierOption`
  - [ ] Implement `WithOptionalMode(bool) VerifierOption`
  - [ ] Implement `WithEnforceMode(bool) VerifierOption`

### 2.6 Utility Functions
- [ ] Create `oci/signature/keys.go`
  - [ ] Implement `LoadPublicKey(path string) (*crypto.PublicKey, error)`
  - [ ] Implement `LoadPublicKeyFromBytes(data []byte) (*crypto.PublicKey, error)`
  - [ ] Add support for PEM and DER formats
  - [ ] Add error handling for invalid key formats

### 2.7 Basic Testing
- [ ] Create `oci/signature/verifier_test.go`
  - [ ] Test `CosignVerifier` struct creation
  - [ ] Test public key loading from file
  - [ ] Test public key loading from bytes
  - [ ] Test signature reference construction
  - [ ] Mock registry operations
  - [ ] Test basic verification flow

## Phase 3: Full Cosign Support

### 3.1 Public Key Verification
- [ ] Implement full public key verification in `verifier.go`
  - [ ] Parse and validate public key format
  - [ ] Fetch signature artifact from registry using ORAS
  - [ ] Extract signature payload from artifact
  - [ ] Verify signature cryptographically using cosign API
  - [ ] Handle multiple signatures (OR logic)
  - [ ] Validate signature format and structure

### 3.2 Keyless Verification
- [ ] Create `oci/signature/keyless.go`
  - [ ] Implement `NewKeylessVerifier(opts ...VerifierOption) *CosignVerifier` constructor
  - [ ] Add Fulcio certificate validation
  - [ ] Add OIDC identity extraction from certificate
  - [ ] Add identity matching against allowed patterns
  - [ ] Add issuer validation (GitHub, Google, etc.)
  - [ ] Add certificate expiration checking

- [ ] Update `oci/signature/options.go`
  - [ ] Add `WithAllowedIdentities(patterns ...string) VerifierOption`
  - [ ] Add `WithRequiredIssuer(issuer string) VerifierOption`
  - [ ] Add `WithFulcioRoot(pool *x509.CertPool) VerifierOption`

### 3.3 Rekor Integration
- [ ] Create `oci/signature/rekor.go`
  - [ ] Implement Rekor client wrapper
  - [ ] Add signature entry lookup in Rekor
  - [ ] Add transparency log verification
  - [ ] Add entry timestamp extraction
  - [ ] Handle Rekor API errors gracefully

- [ ] Update `oci/signature/options.go`
  - [ ] Add `WithRekor(enabled bool) VerifierOption`
  - [ ] Add `WithRekorURL(url string) VerifierOption`
  - [ ] Add `WithRekorPublicKey(key *crypto.PublicKey) VerifierOption`

### 3.4 Annotation Policies
- [ ] Update `oci/signature/policy.go`
  - [ ] Add annotation extraction from signature payload
  - [ ] Add annotation matching logic
  - [ ] Support exact match and regex patterns
  - [ ] Add policy violation error messages

- [ ] Update `oci/signature/options.go`
  - [ ] Add `WithRequiredAnnotations(annotations map[string]string) VerifierOption`

### 3.5 Multiple Signature Support
- [ ] Update `oci/signature/verifier.go`
  - [ ] Implement ANY signature valid (OR logic) - default
  - [ ] Implement ALL signatures valid (AND logic)
  - [ ] Implement minimum N signatures valid
  - [ ] Track which signatures passed/failed
  - [ ] Return detailed verification results

### 3.6 Comprehensive Testing
- [ ] Update `oci/signature/verifier_test.go`
  - [ ] Test public key verification with valid signature
  - [ ] Test public key verification with invalid signature
  - [ ] Test public key verification with missing signature
  - [ ] Test public key verification with multiple signatures

- [ ] Create `oci/signature/keyless_test.go`
  - [ ] Test keyless verification with valid certificate
  - [ ] Test keyless verification with expired certificate
  - [ ] Test keyless verification with wrong identity
  - [ ] Test keyless verification with wrong issuer
  - [ ] Test identity pattern matching

- [ ] Create `oci/signature/rekor_test.go`
  - [ ] Test Rekor entry lookup
  - [ ] Test Rekor verification success
  - [ ] Test Rekor verification failure
  - [ ] Test Rekor API error handling
  - [ ] Mock Rekor API responses

- [ ] Create `oci/signature/policy_test.go`
  - [ ] Test annotation matching (exact)
  - [ ] Test annotation matching (regex)
  - [ ] Test annotation missing error
  - [ ] Test multiple signature modes (ANY/ALL/Minimum)

## Phase 4: Caching Integration

### 4.1 Cache Schema Design
- [ ] Create `oci/internal/cache/verification.go`
  - [ ] Define `VerificationResult` struct
    - [ ] Add `Digest` field
    - [ ] Add `Verified` bool field
    - [ ] Add `Signer` string field
    - [ ] Add `Timestamp` time.Time field
    - [ ] Add `PolicyHash` string field
    - [ ] Add `RekorEntry` optional field
    - [ ] Add `TTL` duration field
  - [ ] Define `RekorLogEntry` struct for transparency log data

### 4.2 Coordinator Extensions
- [ ] Update `oci/internal/cache/coordinator.go`
  - [ ] Add `GetVerificationResult(ctx context.Context, digest string, policyHash string) (*VerificationResult, error)` method
  - [ ] Add `PutVerificationResult(ctx context.Context, result *VerificationResult) error` method
  - [ ] Implement cache key generation: `verify:<digest>:<policy-hash>`
  - [ ] Implement TTL expiration checking
  - [ ] Implement serialization/deserialization

### 4.3 Policy Hashing
- [ ] Create `oci/signature/policy_hash.go`
  - [ ] Implement `ComputePolicyHash(policy *Policy) string`
  - [ ] Hash all policy fields deterministically
  - [ ] Use SHA256 for policy fingerprint
  - [ ] Handle nil/empty policy gracefully

### 4.4 Verifier Cache Integration
- [ ] Update `oci/signature/verifier.go`
  - [ ] Add `cache` field to `CosignVerifier`
  - [ ] Add `WithCache(cache Cache) VerifierOption` option
  - [ ] Check cache before verification
  - [ ] Store verification result after successful verification
  - [ ] Handle cache errors gracefully (don't fail verification)
  - [ ] Log cache hits/misses for observability

### 4.5 Cache Testing
- [ ] Create `oci/internal/cache/verification_test.go`
  - [ ] Test verification result serialization
  - [ ] Test verification result deserialization
  - [ ] Test GetVerificationResult cache hit
  - [ ] Test GetVerificationResult cache miss
  - [ ] Test PutVerificationResult
  - [ ] Test TTL expiration
  - [ ] Test policy hash changes invalidate cache

- [ ] Update `oci/signature/verifier_test.go`
  - [ ] Test verification with cache enabled (cache miss)
  - [ ] Test verification with cache enabled (cache hit)
  - [ ] Test cache stores results after verification
  - [ ] Test cache key includes policy hash
  - [ ] Test different policies use different cache entries

## Phase 5: Documentation & Examples

### 5.1 Main Package Documentation
- [ ] Update `oci/README.md`
  - [ ] Add "Signature Verification" section
  - [ ] Add overview of signature verification feature
  - [ ] Add quick start example with public key
  - [ ] Add quick start example with keyless
  - [ ] Link to signature submodule documentation
  - [ ] Add security considerations section
  - [ ] Add troubleshooting section

- [ ] Update `oci/doc.go`
  - [ ] Add signature verification to package description
  - [ ] Add example in package-level documentation

### 5.2 Signature Submodule Documentation
- [ ] Create `oci/signature/README.md`
  - [ ] Add comprehensive overview
  - [ ] Document all verification modes
  - [ ] Document policy configuration
  - [ ] Add public key verification examples
  - [ ] Add keyless verification examples
  - [ ] Add annotation policy examples
  - [ ] Add Rekor integration examples
  - [ ] Add error handling examples
  - [ ] Add caching configuration examples
  - [ ] Add security best practices
  - [ ] Add FAQ section

- [ ] Create `oci/signature/doc.go`
  - [ ] Add package-level documentation
  - [ ] Add usage examples in comments

### 5.3 Examples
- [ ] Create `oci/examples/signature_verification/`
  - [ ] Create `main.go` with basic example
  - [ ] Create `README.md` explaining the example
  - [ ] Add public key verification example
  - [ ] Add keyless verification example
  - [ ] Add policy-based verification example
  - [ ] Add error handling example
  - [ ] Include test keys/certificates for running example

- [ ] Create `oci/examples/signature_verification_advanced/`
  - [ ] Create advanced multi-signature example
  - [ ] Create annotation policy example
  - [ ] Create Rekor transparency log example
  - [ ] Create cache integration example

### 5.4 API Documentation
- [ ] Ensure all public types have godoc comments
- [ ] Ensure all public functions have godoc comments
- [ ] Add examples to godoc using `Example` functions
- [ ] Review documentation for clarity and completeness

### 5.5 Migration Guide
- [ ] Create `oci/MIGRATION_SIGNATURE.md`
  - [ ] Document how to add signature verification to existing code
  - [ ] Provide before/after code examples
  - [ ] Document breaking changes (none expected)
  - [ ] Document best practices for migration
  - [ ] Add rollout strategy recommendations

## Phase 6: Integration & Testing

### 6.1 Integration Tests
- [ ] Create `oci/signature/integration_test.go`
  - [ ] Test against real registry (localhost registry)
  - [ ] Test signing artifact with cosign CLI
  - [ ] Test verifying signed artifact with library
  - [ ] Test with multiple registries (Docker Hub, GHCR, GCR if possible)
  - [ ] Test keyless signing with Sigstore staging environment
  - [ ] Test Rekor integration end-to-end

### 6.2 End-to-End Tests
- [ ] Create `oci/client_signature_test.go`
  - [ ] Test full Pull() flow with signature verification
  - [ ] Test Pull() with valid signed artifact
  - [ ] Test Pull() with invalid signed artifact
  - [ ] Test Pull() with unsigned artifact
  - [ ] Test Pull() with expired certificate
  - [ ] Test Pull() with wrong signer identity
  - [ ] Test PullWithCache() with signature verification

### 6.3 Security Tests
- [ ] Create `oci/signature/security_test.go`
  - [ ] Test tampered signature detection
  - [ ] Test modified artifact detection
  - [ ] Test cache poisoning prevention
  - [ ] Test policy bypass attempts
  - [ ] Test malformed signature handling

### 6.4 Performance Tests
- [ ] Create `oci/signature/benchmark_test.go`
  - [ ] Benchmark public key verification
  - [ ] Benchmark keyless verification
  - [ ] Benchmark with cache enabled (hit)
  - [ ] Benchmark with cache enabled (miss)
  - [ ] Benchmark Rekor verification
  - [ ] Compare overhead vs non-verified pull

### 6.5 Compatibility Tests
- [ ] Test with different cosign CLI versions
- [ ] Test with different signature formats
- [ ] Test with different registry implementations
- [ ] Test offline verification (no Rekor)

## Phase 7: Polish & Production Readiness

### 7.1 Error Messages
- [ ] Review all error messages for clarity
- [ ] Ensure error messages provide actionable guidance
- [ ] Add error codes for programmatic handling
- [ ] Test error message formatting

### 7.2 Logging & Observability
- [ ] Add structured logging for verification events
- [ ] Log verification success with signer identity
- [ ] Log verification failure with detailed reason
- [ ] Log cache hits/misses
- [ ] Add metrics hooks (optional)

### 7.3 Configuration Validation
- [ ] Validate policy configuration at creation time
- [ ] Validate public key formats
- [ ] Validate identity patterns
- [ ] Provide clear validation error messages

### 7.4 Code Review & Cleanup
- [ ] Review all code for style consistency
- [ ] Remove debug logging
- [ ] Remove commented-out code
- [ ] Ensure all TODOs are addressed
- [ ] Run `go fmt` on all files
- [ ] Run `go vet` on all files
- [ ] Run linter (golangci-lint) if available

### 7.5 Dependencies Audit
- [ ] Review all dependencies for vulnerabilities
- [ ] Ensure dependency versions are pinned
- [ ] Document dependency rationale
- [ ] Check for dependency license compatibility

### 7.6 Final Testing
- [ ] Run all unit tests
- [ ] Run all integration tests
- [ ] Run security tests
- [ ] Run benchmarks
- [ ] Test on multiple platforms (Linux, macOS, Windows)
- [ ] Verify test coverage >90%

### 7.7 Release Preparation
- [ ] Update CHANGELOG.md
- [ ] Tag version for signature submodule
- [ ] Tag version for main oci module
- [ ] Verify go.mod versions are correct
- [ ] Create GitHub release notes

## Completion Checklist

### Phase 1: Interface & Main Package Integration ✅
- [x] All tasks completed
- [x] Tests passing
- [x] Code reviewed

### Phase 2: Signature Submodule Foundation
- [ ] All tasks completed
- [ ] Tests passing
- [ ] Code reviewed

### Phase 3: Full Cosign Support
- [ ] All tasks completed
- [ ] Tests passing
- [ ] Code reviewed

### Phase 4: Caching Integration
- [ ] All tasks completed
- [ ] Tests passing
- [ ] Code reviewed

### Phase 5: Documentation & Examples
- [ ] All tasks completed
- [ ] Documentation reviewed
- [ ] Examples tested

### Phase 6: Integration & Testing
- [ ] All tasks completed
- [ ] All tests passing
- [ ] Performance acceptable

### Phase 7: Polish & Production Readiness
- [ ] All tasks completed
- [ ] Production ready
- [ ] Ready for release

## Notes

- Each checkbox represents a discrete task that can be completed independently
- Tasks should be completed in order within each phase
- Phases can have some overlap, but earlier phases should be mostly complete before starting later phases
- Mark tasks complete with `[x]` as they are finished
- Add notes below each task if needed for tracking issues or decisions
