# Advanced Signature Verification Examples

Advanced examples demonstrating multi-signature verification, annotation policies, Rekor integration, and verification caching.

## Overview

This example demonstrates:
- Multi-signature verification with different policies (ANY, ALL, Minimum N)
- Annotation-based policies for build metadata validation
- Rekor transparency log integration
- Verification result caching for performance

## Building

```bash
cd examples/signature_verification_advanced
go build -o verify-advanced .
```

## Examples

### 1. Multi-Signature Verification

Verify artifacts signed by multiple parties with different policy modes.

#### ANY Signature Valid (OR Logic)

Default mode - any valid signature from any key passes verification:

```bash
./verify-advanced \
  -mode multi-sig \
  -keys "key1.pub,key2.pub,key3.pub" \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Use case:** Multiple release managers can sign independently.

#### ALL Signatures Valid (AND Logic)

Require unanimous approval from all signers:

```bash
./verify-advanced \
  -mode multi-sig \
  -keys "dev-key.pub,qa-key.pub,security-key.pub" \
  -require-all \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Use case:** Require sign-off from development, QA, and security teams.

#### Minimum N Signatures (Threshold)

Require at least N signatures to be valid:

```bash
./verify-advanced \
  -mode multi-sig \
  -keys "key1.pub,key2.pub,key3.pub,key4.pub,key5.pub" \
  -min-sigs 3 \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Use case:** Require majority approval (3 of 5 signers).

### 2. Annotation Policy Verification

Enforce required metadata in signatures for compliance and audit requirements.

#### Build Metadata Validation

```bash
./verify-advanced \
  -mode annotations \
  -identity "*@company.com" \
  -annotations "build-system=github-actions,verified=true,security-scan=passed" \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Required annotations:**
- `build-system`: Must be from approved CI/CD system
- `verified`: Build verification status
- `security-scan`: Security scan result

#### Release Compliance

```bash
./verify-advanced \
  -mode annotations \
  -identity "release-bot@company.com" \
  -annotations "release-stage=production,compliance-check=passed,approver=john.doe@company.com" \
  -ref ghcr.io/org/app:production \
  -target ./production-app
```

**Use case:** Ensure production releases meet compliance requirements.

### 3. Rekor Transparency Log Integration

Verify signatures against the Rekor transparency log for audit trails.

#### Public Rekor Instance

```bash
./verify-advanced \
  -mode rekor \
  -identity "*@company.com" \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Benefits:**
- Transparency: All signatures publicly logged
- Non-repudiation: Signers cannot deny signing
- Audit trail: Historical record of all signatures
- Trusted timestamp: Timestamp of signature creation

#### Custom Rekor Instance

For private deployments:

```bash
./verify-advanced \
  -mode rekor \
  -identity "*@company.com" \
  -issuer "https://github.com/login/oauth" \
  -rekor-url "https://rekor.company.internal" \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Use case:** Enterprise deployments with private Sigstore infrastructure.

### 4. Verification Caching

Cache verification results to improve performance for repeated pulls.

```bash
./verify-advanced \
  -mode caching \
  -identity "*@company.com" \
  -cache-dir "/var/cache/oci-signatures" \
  -cache-ttl "1h" \
  -ref ghcr.io/org/app:v1.0 \
  -target ./app
```

**Performance:**
- First pull: Full verification (55-730ms depending on mode)
- Subsequent pulls: Cached verification (<1ms, 99.8% faster)
- Cache hit rate: Typically 90%+ for repeated pulls

#### Cache Configuration

```bash
# Short TTL for security-critical environments
./verify-advanced -mode caching -cache-ttl "15m" -ref <ref> -target <dir>

# Long TTL for development environments
./verify-advanced -mode caching -cache-ttl "24h" -ref <ref> -target <dir>

# Custom cache directory
./verify-advanced -mode caching -cache-dir "$HOME/.cache/oci" -ref <ref> -target <dir>
```

## Command-Line Options

### Global Options

```
-mode string
    Mode: multi-sig, annotations, rekor, or caching (required)

-ref string
    OCI reference to pull (required)

-target string
    Target directory for extraction (default "./output")
```

### Multi-Signature Options

```
-keys string
    Comma-separated list of public key files (required for multi-sig mode)

-require-all
    Require all signatures to be valid (AND logic)

-min-sigs int
    Minimum number of valid signatures required (default 1)
```

### Annotation Options

```
-annotations string
    Required annotations in format: key1=value1,key2=value2 (required for annotations mode)

-identity string
    Allowed identity pattern (default "*@example.com")
```

### Rekor Options

```
-identity string
    Allowed identity pattern (default "*@example.com")

-issuer string
    Required OIDC issuer (optional, e.g., "https://github.com/login/oauth")

-rekor-url string
    Custom Rekor URL (default: public Sigstore instance)
```

### Caching Options

```
-identity string
    Allowed identity pattern (default "*@example.com")

-cache-dir string
    Cache directory (default "/tmp/oci-cache")

-cache-ttl duration
    Cache TTL duration (default "1h")
```

## Real-World Scenarios

### Scenario 1: Multi-Team Release Process

Require sign-off from development, QA, and security:

```bash
# Generate keys for each team
cosign generate-key-pair --output-key-prefix dev
cosign generate-key-pair --output-key-prefix qa
cosign generate-key-pair --output-key-prefix security

# Each team signs the artifact
cosign sign --key dev.key ghcr.io/org/app:v2.0
cosign sign --key qa.key ghcr.io/org/app:v2.0
cosign sign --key security.key ghcr.io/org/app:v2.0

# Verify all signatures are present
./verify-advanced \
  -mode multi-sig \
  -keys "dev.pub,qa.pub,security.pub" \
  -require-all \
  -ref ghcr.io/org/app:v2.0 \
  -target ./release
```

### Scenario 2: Compliance-Driven Deployment

Enforce build and security scan requirements:

```bash
# Sign with required metadata
cosign sign \
  -a build-system=github-actions \
  -a security-scan=passed \
  -a compliance-check=sox-compliant \
  -a build-id=12345 \
  ghcr.io/org/app:production

# Verify compliance
./verify-advanced \
  -mode annotations \
  -identity "ci-bot@company.com" \
  -annotations "security-scan=passed,compliance-check=sox-compliant" \
  -ref ghcr.io/org/app:production \
  -target ./production
```

### Scenario 3: High-Performance Production Environment

Use caching for frequent deployments:

```bash
# Configure persistent cache
export CACHE_DIR="/var/cache/oci-signatures"
mkdir -p $CACHE_DIR

# First deployment (full verification)
./verify-advanced \
  -mode caching \
  -identity "release-bot@company.com" \
  -cache-dir "$CACHE_DIR" \
  -cache-ttl "1h" \
  -ref ghcr.io/org/app:v3.0 \
  -target /opt/app-v3.0

# Subsequent deployments (cached, ~1ms)
# Useful for rolling updates, canary deployments, etc.
for i in {1..10}; do
  ./verify-advanced \
    -mode caching \
    -cache-dir "$CACHE_DIR" \
    -ref ghcr.io/org/app:v3.0 \
    -target "/opt/app-node-$i"
done
```

## Security Best Practices

### 1. Multi-Signature Policies

- Use `require-all` for critical production releases
- Use `min-sigs` for flexibility with large teams
- Rotate keys regularly across all teams
- Document key ownership and responsibilities

### 2. Annotation Policies

- Define required annotations in security policy
- Validate security scan results
- Require build system identification
- Include compliance markers for regulated industries

### 3. Rekor Integration

- Always enable Rekor for keyless signing
- Monitor Rekor availability
- Use custom Rekor for air-gapped environments
- Audit Rekor logs regularly

### 4. Caching

- Use shorter TTLs (15-30min) for security-critical environments
- Monitor cache hit rates
- Clear cache when policies change
- Secure cache directory permissions

## Troubleshooting

### Multi-Signature Issues

**"Signature policy not satisfied"**
- Check that all required keys have valid signatures
- Verify key files are correct
- Use `cosign verify` to check individual signatures

### Annotation Issues

**"Required annotations missing"**
- Verify annotations were added during signing: `cosign verify <image>`
- Check annotation key names match exactly (case-sensitive)
- Ensure all required annotations are present

### Rekor Issues

**"Rekor verification failed"**
- Check network connectivity to Rekor server
- Verify Rekor URL is correct
- Check Sigstore status: https://status.sigstore.dev/
- Try public Rekor if custom instance fails

### Caching Issues

**"Cache not improving performance"**
- Verify cache directory is writable
- Check cache TTL is reasonable
- Monitor cache hit/miss rates
- Ensure policy hasn't changed (invalidates cache)

## Performance Benchmarks

### Multi-Signature Verification

- 1 signature: 55-215ms
- 3 signatures (ANY): 60-220ms
- 3 signatures (ALL): 165-645ms

### Annotation Policy

- Same as base verification mode
- Negligible overhead for annotation checking (<1ms)

### Rekor Verification

- Additional 100-500ms for Rekor lookup and verification
- Varies based on network latency to Rekor server

### Caching

- Cache miss: Full verification time
- Cache hit: <1ms (99.8% reduction)
- Typical improvement: 100-700x faster

## Next Steps

- Explore the [Signature Module README](../../signature/README.md) for complete documentation
- See [DESIGN.md](../../DESIGN.md) for architecture details
- Read about [Sigstore](https://docs.sigstore.dev/) and [Cosign](https://github.com/sigstore/cosign)
- Integrate with CI/CD pipelines

## License

See [LICENSE](../../../LICENSE) for license information.
