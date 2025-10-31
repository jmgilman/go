# Security Review: OCI Signature Verification Package

**Date:** 2024-12-19
**Reviewer:** Security Engineer
**Scope:** `oci/signature` package - Cosign signature verification implementation
**Status:** ⚠️ **CRITICAL ISSUES FOUND**

---

## Executive Summary

This security review analyzed the Cosign signature verification implementation in the `oci/signature` package. The review identified **8 critical issues**, **12 medium-severity issues**, and **5 low-severity issues** that require attention.

**Key Findings:**
- ✅ Strong cryptographic verification implementation
- ✅ Good input validation in most areas
- ⚠️ **CRITICAL:** Context propagation issues in certificate chain verification
- ⚠️ **CRITICAL:** Missing certificate timestamp validation for Rekor bundles
- ⚠️ **CRITICAL:** Potential identity injection via certificate extension parsing
- ⚠️ **CRITICAL:** No timeout enforcement on external network calls
- ⚠️ **MEDIUM:** Policy hash doesn't include CacheTTL (cache poisoning risk)
- ⚠️ **MEDIUM:** Error messages leak sensitive information

---

## Critical Issues (P0 - Immediate Fix Required)

### 1. **CRITICAL: Context Not Propagated to TUF Client Creation**

**Location:** `keyless.go:435`
**Severity:** HIGH
**Impact:** Certificate chain verification cannot be cancelled/timeout-controlled

**Issue:**
```go
// Line 435: Uses context.Background() instead of passed context
tufClient, err := tuf.NewFromEnv(context.Background())
```

**Problem:**
- The `verifyCertificateChain` function receives a `cert` parameter but doesn't use the context from `verifyKeylessSignature`
- TUF client creation uses `context.Background()`, preventing cancellation and timeout control
- Network calls to fetch Fulcio roots cannot be cancelled if the parent context is cancelled
- Could lead to resource exhaustion under denial-of-service conditions

**Recommendation:**
```go
func (v *CosignVerifier) verifyCertificateChain(ctx context.Context, cert *x509.Certificate) error {
    // ...
    tufClient, err := tuf.NewFromEnv(ctx) // Use passed context
    // ...
}
```

**Fix:** Update `verifyCertificateChain` to accept and use `context.Context`.

---

### 2. **CRITICAL: Missing Rekor Bundle Timestamp Validation**

**Location:** `rekor.go:69`
**Severity:** HIGH
**Impact:** Replay attacks possible with expired or future-dated signatures

**Issue:**
```go
// verifyRekorEntry verifies bundle but doesn't check timestamp
verified, err := cosign.VerifyBundle(sig, checkOpts)
```

**Problem:**
- Rekor bundle verification validates the entry exists and is signed correctly
- **Does NOT validate the bundle timestamp against current time**
- An attacker could replay old signatures indefinitely if they have the bundle
- No check that `bundle.Payload.IntegratedTime` is reasonable (not in future, not too old)

**Recommendation:**
```go
// After verifying bundle, check timestamp
if bundle.Payload.IntegratedTime > 0 {
    integratedTime := time.Unix(bundle.Payload.IntegratedTime, 0)
    now := time.Now()

    // Reject if timestamp is too far in the future (clock skew protection)
    if integratedTime.After(now.Add(5 * time.Minute)) {
        return &ocibundle.BundleError{...}
    }

    // Optionally reject if timestamp is too old (configurable max age)
    maxAge := 24 * time.Hour * 365 // 1 year default
    if integratedTime.Before(now.Add(-maxAge)) {
        return &ocibundle.BundleError{...}
    }
}
```

**Fix:** Add timestamp validation after `cosign.VerifyBundle` succeeds.

---

### 3. **CRITICAL: Identity Extraction from Certificate Extensions Vulnerable to Injection**

**Location:** `keyless.go:382`
**Severity:** HIGH
**Impact:** Potential injection attacks via malicious certificate extensions

**Issue:**
```go
// Line 380-384: Directly converts extension value to string
if ext.Id.String() == "1.3.6.1.4.1.57264.1.8" {
    identity := string(ext.Value) // No validation!
    if identity != "" {
        return identity, nil
    }
}
```

**Problem:**
- Certificate extension values are DER-encoded and may contain arbitrary bytes
- Converting directly to `string()` without validation could include:
  - Null bytes (`\x00`)
  - Control characters
  - Multi-byte sequences that break UTF-8
- These could be used to bypass identity matching or inject into logs/error messages
- No validation that the extension value is a valid UTF-8 string

**Recommendation:**
```go
if ext.Id.String() == "1.3.6.1.4.1.57264.1.8" {
    // Validate encoding and content
    identity := string(ext.Value)

    // Check for null bytes
    if strings.ContainsRune(identity, 0) {
        return "", fmt.Errorf("identity contains null byte")
    }

    // Validate UTF-8 encoding
    if !utf8.ValidString(identity) {
        return "", fmt.Errorf("identity is not valid UTF-8")
    }

    // Additional validation: check for control characters (except whitespace)
    for _, r := range identity {
        if r < 32 && r != '\t' && r != '\n' && r != '\r' {
            return "", fmt.Errorf("identity contains control character")
        }
    }

    if identity != "" {
        return identity, nil
    }
}
```

**Fix:** Add proper validation and sanitization when extracting identity from certificate extensions.

---

### 4. **CRITICAL: No Timeout Enforcement on External Network Calls**

**Location:** Multiple - `rekor.go:52`, `keyless.go:435`, `verifier.go:522`
**Severity:** HIGH
**Impact:** Denial of service via slow or hanging network calls

**Problem:**
- Rekor client creation has no timeout configuration
- TUF client creation has no timeout
- Registry fetch operations have no timeout
- A malicious or slow Rekor server could hang verification indefinitely
- No protection against network exhaustion attacks

**Current Code:**
```go
// rekor.go:52 - No timeout
rekorClient := client.NewHTTPClientWithConfig(nil, &client.TransportConfig{
    Host:     rekorURL,
    BasePath: client.DefaultBasePath,
    Schemes:  []string{"https"},
})
```

**Recommendation:**
```go
// Add timeout to HTTP client
httpClient := &http.Client{
    Timeout: 30 * time.Second, // Reasonable timeout for Rekor
}
rekorClient := client.NewHTTPClientWithConfig(nil, &client.TransportConfig{
    Host:     rekorURL,
    BasePath: client.DefaultBasePath,
    Schemes:  []string{"https"},
}).WithHTTPClient(httpClient)
```

**Fix:** Add timeouts to all external network calls (Rekor, TUF, registry fetches).

---

### 5. **CRITICAL: Signature Reference Building Vulnerable to Path Traversal**

**Location:** `verifier.go:447-508`
**Severity:** MEDIUM-HIGH
**Impact:** Potential path traversal in signature reference construction

**Issue:**
```go
// Line 496: Basic validation but may not catch all cases
if repo == "" || strings.Contains(repo, "..") || strings.HasPrefix(repo, "-") {
    return ""
}
```

**Problem:**
- Validation checks for `".."` but doesn't handle encoded variants (`%2e%2e`, `%2E%2E`)
- Doesn't validate registry URL format rigorously
- No validation of port numbers in registry URLs
- Could allow malicious references that bypass signature verification

**Recommendation:**
```go
// More rigorous validation
func (v *CosignVerifier) buildSignatureReference(reference, digestStr string) string {
    // ... existing digest validation ...

    // Validate repository format more strictly
    if repo == "" {
        return ""
    }

    // Check for path traversal attempts (encoded and unencoded)
    if strings.Contains(repo, "..") ||
       strings.Contains(repo, "%2e%2e") ||
       strings.Contains(repo, "%2E%2E") ||
       strings.Contains(repo, "%252e%252e") {
        return ""
    }

    // Validate registry format (basic check)
    // Should match: [registry[:port]]/repository
    if !regexp.MustCompile(`^[a-zA-Z0-9._-]+(/[a-zA-Z0-9._/-]+)*$`).MatchString(repo) {
        return ""
    }

    // ... rest of function ...
}
```

**Fix:** Strengthen repository reference validation to prevent path traversal.

---

### 6. **CRITICAL: Certificate Expiration Check Vulnerable to Clock Skew**

**Location:** `keyless.go:42`
**Severity:** MEDIUM-HIGH
**Impact:** Clock skew could cause false positives/negatives

**Issue:**
```go
now := time.Now()
if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
    return &ocibundle.BundleError{...}
}
```

**Problem:**
- Strict time comparison without clock skew tolerance
- Systems with clock skew could incorrectly reject valid certificates
- Or accept expired certificates if clock is behind
- No configuration for acceptable clock skew window

**Recommendation:**
```go
// Add clock skew tolerance (default 5 minutes)
const clockSkewTolerance = 5 * time.Minute

now := time.Now()
// Allow certificates that are slightly expired due to clock skew
if now.Before(cert.NotBefore.Add(-clockSkewTolerance)) {
    return &ocibundle.BundleError{...}
}
// Allow certificates that are slightly expired
if now.After(cert.NotAfter.Add(clockSkewTolerance)) {
    return &ocibundle.BundleError{...}
}
```

**Fix:** Add clock skew tolerance to certificate expiration checks.

---

### 7. **CRITICAL: No Validation of Signature Payload Size**

**Location:** `verifier.go:621`, `keyless.go:126`
**Severity:** MEDIUM
**Impact:** Potential DoS via extremely large payloads

**Issue:**
```go
payload, err := sig.Payload()
// No size check before parsing
if err := json.Unmarshal(payload, &payloadData); err != nil {
    // ...
}
```

**Problem:**
- No maximum size limit on signature payloads
- Malicious signatures could contain extremely large payloads
- JSON unmarshaling could consume excessive memory
- Could lead to denial of service

**Recommendation:**
```go
const maxPayloadSize = 64 * 1024 // 64KB - reasonable limit for Cosign payloads

payload, err := sig.Payload()
if err != nil {
    return fmt.Errorf("failed to get signature payload: %w", err)
}

if len(payload) > maxPayloadSize {
    return fmt.Errorf("signature payload too large: %d bytes (max: %d)",
        len(payload), maxPayloadSize)
}

// Now safe to unmarshal
if err := json.Unmarshal(payload, &payloadData); err != nil {
    // ...
}
```

**Fix:** Add payload size validation before JSON parsing.

---

### 8. **CRITICAL: Public Key Signature Verification Missing Key Strength Validation**

**Location:** `verifier.go:710`
**Severity:** MEDIUM
**Impact:** Weak keys could be accepted

**Issue:**
```go
// Line 710: No key strength validation
verifier, err := signature.LoadVerifier(pubKey, crypto.SHA256)
```

**Problem:**
- No validation of RSA key size (should be >= 2048 bits)
- No validation of ECDSA curve (should be P-256 or stronger)
- Weak keys could be used, compromising security
- No check for deprecated algorithms

**Recommendation:**
```go
// Add key strength validation
func validateKeyStrength(pubKey crypto.PublicKey) error {
    switch k := pubKey.(type) {
    case *rsa.PublicKey:
        if k.N.BitLen() < 2048 {
            return fmt.Errorf("RSA key size too small: %d bits (minimum: 2048)", k.N.BitLen())
        }
    case *ecdsa.PublicKey:
        if k.Curve.Params().BitSize < 256 {
            return fmt.Errorf("ECDSA curve too weak: %d bits (minimum: 256)", k.Curve.Params().BitSize)
        }
    case ed25519.PublicKey:
        // Ed25519 is always 256 bits - OK
    default:
        return fmt.Errorf("unsupported key type: %T", pubKey)
    }
    return nil
}

// Use in verification
if err := validateKeyStrength(pubKey); err != nil {
    return fmt.Errorf("weak key rejected: %w", err)
}
```

**Fix:** Add key strength validation before using keys for verification.

---

## Medium Severity Issues (P1 - Fix Soon)

### 9. **Policy Hash Doesn't Include CacheTTL**

**Location:** `policy_hash.go:36-105`
**Severity:** MEDIUM
**Impact:** Cache poisoning risk

**Issue:**
The `ComputePolicyHash` function doesn't include `CacheTTL` in the hash calculation. This means policies with different TTLs would have the same hash, potentially allowing cache poisoning.

**Recommendation:**
```go
// Add CacheTTL to hash
fmt.Fprintf(h, "cache_ttl:%d\n", policy.CacheTTL)
```

---

### 10. **No Validation of Rekor URL Format**

**Location:** `rekor.go:46-56`
**Severity:** MEDIUM
**Impact:** Could allow HTTP instead of HTTPS

**Recommendation:**
```go
// Validate Rekor URL is HTTPS
if !strings.HasPrefix(rekorURL, "https://") {
    return &ocibundle.BundleError{
        Err: fmt.Errorf("Rekor URL must use HTTPS: %s", rekorURL),
    }
}
```

---

### 11. **Error Messages Leak Sensitive Information**

**Location:** Multiple locations
**Severity:** MEDIUM
**Impact:** Information disclosure

**Issue:**
Error messages include:
- Full digest values
- Signer identities
- Reference URLs
- Certificate details

These could leak sensitive information in logs or error responses.

**Recommendation:**
- Sanitize error messages for production logging
- Provide detailed errors only in debug mode
- Hash sensitive values in logs

---

### 12. **No Rate Limiting on Verification Attempts**

**Location:** `verifier.go:188`
**Severity:** MEDIUM
**Impact:** DoS via excessive verification attempts

**Recommendation:**
Add rate limiting to prevent abuse:
```go
type rateLimiter interface {
    Allow() bool
}

// Add to CosignVerifier
rateLimiter rateLimiter
```

---

### 13. **Identity Pattern Matching Allows Arbitrary Glob Patterns**

**Location:** `policy.go:227-273`
**Severity:** LOW-MEDIUM
**Impact:** Performance DoS via complex patterns

**Issue:**
No validation that glob patterns are reasonable. Complex patterns like `*{a,b,c}*{d,e,f}*` could cause performance issues.

**Recommendation:**
Add pattern complexity limits:
```go
// Limit pattern length and complexity
if len(pattern) > maxPatternLength {
    return false
}

// Limit number of wildcards
wildcardCount := strings.Count(pattern, "*") + strings.Count(pattern, "?")
if wildcardCount > 10 {
    return false // Too complex
}
```

---

### 14. **No Bounds Checking on Signature Manifest Layers**

**Location:** `verifier.go:552-578`
**Severity:** MEDIUM
**Impact:** DoS via excessive signature layers

**Issue:**
No limit on number of signature layers. Malicious manifest could have thousands of layers.

**Recommendation:**
```go
const maxSignatureLayers = 100

if len(manifest.Layers) > maxSignatureLayers {
    return nil, fmt.Errorf("too many signature layers: %d (max: %d)",
        len(manifest.Layers), maxSignatureLayers)
}
```

---

### 15. **Missing Validation of Certificate Extension OID Format**

**Location:** `keyless.go:380`, `keyless.go:414`
**Severity:** LOW-MEDIUM
**Impact:** Potential parsing issues

**Recommendation:**
Validate OID format more strictly:
```go
expectedOID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}
if !ext.Id.Equal(expectedOID) {
    continue
}
```

---

### 16. **No Validation of Rekor Bundle LogID Format**

**Location:** `rekor.go:33`
**Severity:** LOW-MEDIUM
**Impact:** Could accept invalid bundle formats

**Recommendation:**
Validate LogID format:
```go
if bundle.Payload.LogID == "" {
    return &ocibundle.BundleError{...}
}

// Validate LogID is hex-encoded SHA256 hash (64 characters)
if len(bundle.Payload.LogID) != 64 {
    return &ocibundle.BundleError{...}
}
if !hexRegex.MatchString(bundle.Payload.LogID) {
    return &ocibundle.BundleError{...}
}
```

---

### 17. **Cache Implementation Relies on External TTL Enforcement**

**Location:** `verifier.go:273-294`
**Severity:** MEDIUM
**Impact:** Stale cache entries if cache implementation is buggy

**Issue:**
The code trusts the cache implementation to enforce TTL. A buggy cache could return expired entries.

**Recommendation:**
Add defensive TTL check even when cache returns a hit:
```go
// If cache returns hit, verify TTL is still valid
if verified && v.cache != nil {
    // Double-check cache entry age
    // (This requires cache interface extension to return timestamp)
}
```

---

### 18. **No Logging of Security Events**

**Location:** Entire package
**Severity:** MEDIUM
**Impact:** No audit trail for security incidents

**Recommendation:**
Add structured logging for:
- Verification failures
- Policy violations
- Certificate expiration
- Rekor verification failures

---

### 19. **No Metrics for Verification Failures**

**Location:** Entire package
**Severity:** LOW-MEDIUM
**Impact:** Difficult to detect attacks

**Recommendation:**
Add metrics for:
- Verification attempt count
- Failure rate by type
- Average verification time
- Cache hit/miss rates

---

### 20. **Public Key Loading Doesn't Validate File Permissions**

**Location:** `keys.go:33`
**Severity:** LOW-MEDIUM
**Impact:** Keys could be readable by unauthorized users

**Recommendation:**
```go
// Check file permissions
info, err := os.Stat(path)
if err != nil {
    return nil, fmt.Errorf("failed to stat key file: %w", err)
}

mode := info.Mode()
if mode&0077 != 0 { // Others or group have read permission
    return nil, fmt.Errorf("key file has insecure permissions: %v", mode)
}
```

---

## Low Severity Issues (P2 - Nice to Have)

### 21. **Error Messages Could Be More User-Friendly**

**Location:** Multiple
**Severity:** LOW
**Impact:** Poor user experience

---

### 22. **No Support for Certificate Revocation Lists (CRL)**

**Location:** `keyless.go:427-479`
**Severity:** LOW
**Impact:** Revoked certificates may still be accepted

**Note:** This is a feature gap, not a security bug. Sigstore uses short-lived certificates (15 min) which mitigates this.

---

### 23. **No Support for OCSP Stapling**

**Location:** `keyless.go:427-479`
**Severity:** LOW
**Impact:** Same as CRL

---

### 24. **No Validation of Certificate Serial Number Format**

**Location:** `keyless.go:27`
**Severity:** LOW
**Impact:** Minor parsing robustness

---

### 25. **Digest Validation Could Be More Strict**

**Location:** `verifier.go:217-227`
**Severity:** LOW
**Impact:** Could accept malformed digests

**Issue:**
Only checks for `:` separator, doesn't validate hex length.

**Recommendation:**
```go
// SHA256 digests should be exactly 64 hex characters
parts := strings.SplitN(digestStr, ":", 2)
if len(parts) != 2 {
    return &ocibundle.BundleError{...}
}

algorithm := parts[0]
hexPart := parts[1]

if algorithm == "sha256" && len(hexPart) != 64 {
    return &ocibundle.BundleError{
        Err: fmt.Errorf("invalid sha256 digest length: %d (expected 64)", len(hexPart)),
    }
}
```

---

## Positive Security Findings

### ✅ **Strong Cryptographic Implementation**
- Proper use of crypto libraries
- Correct signature verification flow
- Good digest validation

### ✅ **Good Input Validation**
- Validates digest format
- Validates descriptor fields
- Checks for empty/null values

### ✅ **Policy-Based Access Control**
- Flexible identity matching
- Annotation requirements
- Issuer restrictions

### ✅ **Cache Security**
- Policy hash included in cache key
- Digest included in cache key
- TTL enforcement (relies on implementation)

### ✅ **Error Handling**
- Detailed error context
- Proper error wrapping
- Security-sensitive error classification

---

## Recommendations Summary

### Immediate Actions (P0):
1. ✅ Fix context propagation in TUF client creation
2. ✅ Add Rekor bundle timestamp validation
3. ✅ Fix identity extraction injection vulnerability
4. ✅ Add timeout enforcement to all network calls
5. ✅ Strengthen signature reference validation
6. ✅ Add clock skew tolerance to certificate expiration
7. ✅ Add payload size limits
8. ✅ Add key strength validation

### Short-term Actions (P1):
1. Include CacheTTL in policy hash
2. Validate Rekor URL format
3. Sanitize error messages
4. Add rate limiting
5. Add signature layer limits
6. Add security event logging

### Long-term Actions (P2):
1. Improve error messages
2. Add CRL support (if needed)
3. Add metrics collection
4. Strengthen digest validation

---

## Testing Recommendations

### Security Test Cases to Add:
1. **Test context cancellation** - Verify TUF client respects context cancellation
2. **Test Rekor timestamp validation** - Verify old/future timestamps are rejected
3. **Test identity injection** - Verify null bytes/control chars in identity are rejected
4. **Test timeout enforcement** - Verify network calls timeout correctly
5. **Test path traversal** - Verify malicious references are rejected
6. **Test clock skew** - Verify certificate expiration handles skew
7. **Test payload size limits** - Verify oversized payloads are rejected
8. **Test key strength** - Verify weak keys are rejected
9. **Test cache poisoning** - Verify policy changes invalidate cache
10. **Test rate limiting** - Verify excessive requests are rate-limited

---

## Conclusion

The `oci/signature` package implements a robust signature verification system with good cryptographic practices. However, several critical security issues require immediate attention:

1. **Context propagation** - Network calls cannot be cancelled
2. **Timestamp validation** - Replay attacks possible
3. **Identity injection** - Malicious certificates could bypass checks
4. **Timeout enforcement** - DoS vulnerabilities exist

After addressing the critical issues, the package will be significantly more secure and production-ready.

**Risk Assessment:**
- **Current Risk Level:** 🔴 **HIGH** (due to critical issues)
- **Post-Fix Risk Level:** 🟢 **LOW** (after addressing critical issues)

---

## Review Checklist

- [x] Code review completed
- [x] Security test cases reviewed
- [x] Error handling analyzed
- [x] Input validation checked
- [x] Cryptographic implementation verified
- [x] Network security reviewed
- [x] Cache security analyzed
- [x] Logging and monitoring assessed

---

**End of Security Review**

