# Signature Verification Example

This example demonstrates basic signature verification for OCI artifacts using both public key and keyless verification modes.

## Overview

The example shows how to:
- Verify artifacts signed with Cosign
- Use public key verification (traditional cryptography)
- Use keyless verification (OIDC-based with Sigstore)
- Handle verification errors with detailed diagnostics
- Implement security best practices

## Prerequisites

1. **Cosign CLI** (for signing artifacts):
```bash
# Install Cosign
go install github.com/sigstore/cosign/v2/cmd/cosign@latest
```

2. **A signed OCI artifact** (or use the test artifacts below)

## Building

```bash
cd examples/signature_verification
go build -o verify-artifact .
```

## Usage

### Public Key Verification

#### 1. Generate a key pair (first time only):

```bash
cosign generate-key-pair
# Creates: cosign.key (private) and cosign.pub (public)
# Store cosign.key securely and share cosign.pub
```

#### 2. Sign an artifact:

```bash
# Sign with your private key
cosign sign --key cosign.key ghcr.io/yourorg/app:v1.0

# Or sign locally built artifacts
docker build -t localhost:5000/app:v1.0 .
docker push localhost:5000/app:v1.0
cosign sign --key cosign.key localhost:5000/app:v1.0
```

#### 3. Verify and pull:

```bash
./verify-artifact \
  -mode public-key \
  -key cosign.pub \
  -ref ghcr.io/yourorg/app:v1.0 \
  -target ./app
```

### Keyless Verification

#### 1. Sign an artifact with keyless signing:

```bash
# Sign using your OIDC identity (GitHub, Google, etc.)
cosign sign ghcr.io/yourorg/app:v1.0

# Follow the browser prompt to authenticate
# Your identity will be embedded in the certificate
```

#### 2. Verify and pull:

```bash
./verify-artifact \
  -mode keyless \
  -identity "your-email@example.com" \
  -rekor=true \
  -ref ghcr.io/yourorg/app:v1.0 \
  -target ./app
```

#### 3. Verify with wildcard identity:

```bash
# Allow any email from a domain
./verify-artifact \
  -mode keyless \
  -identity "*@example.com" \
  -rekor=true \
  -ref ghcr.io/yourorg/app:v1.0 \
  -target ./app
```

## Command-Line Options

```
-mode string
    Verification mode: "public-key" or "keyless" (default "public-key")

-key string
    Path to public key file for public-key mode (default "cosign.pub")

-identity string
    Allowed identity pattern for keyless mode (default "*@example.com")
    Supports wildcards: "*@domain.com", "user@domain.com"

-rekor bool
    Enable Rekor transparency log verification for keyless mode (default true)

-ref string
    OCI reference to pull (required)
    Examples: ghcr.io/org/app:v1.0, registry.io/repo:tag

-target string
    Target directory for extraction (default "./output")
```

## Example Scenarios

### Scenario 1: Verify artifact from trusted organization

```bash
./verify-artifact \
  -mode keyless \
  -identity "*@trusted-org.com" \
  -ref ghcr.io/trusted-org/app:v2.0 \
  -target ./trusted-app
```

### Scenario 2: Verify with specific public key

```bash
./verify-artifact \
  -mode public-key \
  -key ./keys/prod.pub \
  -ref ghcr.io/myorg/app:production \
  -target ./production-app
```

### Scenario 3: Offline verification (no Rekor)

```bash
# For environments without internet access
./verify-artifact \
  -mode keyless \
  -identity "ci-bot@example.com" \
  -rekor=false \
  -ref ghcr.io/myorg/app:v1.0 \
  -target ./app
```

## Error Handling

The example demonstrates comprehensive error handling:

### Signature Not Found

```
✗ Verification failed!

Error Type:
  No signature found for artifact

Troubleshooting:
  1. Ensure the artifact was signed with: cosign sign <image>
  2. Check that you have read access to the signature artifact
  3. Verify the signature reference format: repo:sha256-<digest>.sig
```

### Untrusted Signer

```
✗ Verification failed!

Error Type:
  Signature valid but signer not trusted

Troubleshooting:
  1. Check the allowed identity patterns match the actual signer
  2. Verify the certificate issuer matches your policy
  3. Update allowed identities to trust this signer
```

### Invalid Signature

```
✗ Verification failed!

Error Type:
  Signature verification failed

Troubleshooting:
  1. The artifact may have been tampered with
  2. Ensure you're using the correct public key
  3. Try re-signing the artifact
```

## Testing

### Create Test Artifacts

```bash
# Start a local registry
docker run -d -p 5000:5000 --name registry registry:2

# Build and push a test image
echo "FROM alpine" > Dockerfile
echo "RUN echo 'test artifact'" >> Dockerfile
docker build -t localhost:5000/test:v1.0 .
docker push localhost:5000/test:v1.0

# Sign with public key
cosign generate-key-pair
cosign sign --key cosign.key localhost:5000/test:v1.0

# Verify
./verify-artifact \
  -mode public-key \
  -key cosign.pub \
  -ref localhost:5000/test:v1.0 \
  -target ./test-output
```

### Test Keyless Signing

```bash
# Sign with keyless (requires OIDC authentication)
cosign sign localhost:5000/test:v1.0

# Verify with your identity
./verify-artifact \
  -mode keyless \
  -identity "$(git config user.email)" \
  -ref localhost:5000/test:v1.0 \
  -target ./test-output
```

## Security Best Practices

1. **Key Management (Public Key Mode)**:
   - Store private keys securely (use HSM or key vault in production)
   - Rotate keys regularly (every 90 days)
   - Never commit private keys to version control
   - Use different keys for different environments

2. **Identity Restrictions (Keyless Mode)**:
   - Use specific identities, not wildcards in production
   - Combine identity and issuer restrictions
   - Enable Rekor for transparency and audit trail

3. **Production Configuration**:
   ```bash
   # Production: Strict identity, Rekor required
   ./verify-artifact \
     -mode keyless \
     -identity "release-bot@company.com" \
     -rekor=true \
     -ref ghcr.io/company/app:production

   # Development: More permissive for testing
   ./verify-artifact \
     -mode keyless \
     -identity "*@company.com" \
     -rekor=false \
     -ref ghcr.io/company/app:dev
   ```

4. **Monitoring**:
   - Log all verification attempts
   - Alert on verification failures
   - Track signer identities
   - Monitor Rekor availability

## Next Steps

- See [Advanced Examples](../signature_verification_advanced/) for:
  - Multi-signature verification
  - Annotation policies
  - Cache integration
  - Custom error handling
- Read the [Signature Module README](../../signature/README.md) for complete documentation
- Explore the [Cosign documentation](https://docs.sigstore.dev/cosign/overview/)

## Troubleshooting

### "Failed to load public key"
- Ensure the key file exists and is readable
- Verify the key is in PEM format
- Check file permissions

### "Failed to resolve signature manifest"
- Verify network connectivity to the registry
- Ensure you have read access to the signature artifact
- Check that the artifact was actually signed

### "Rekor verification failed"
- Ensure network connectivity to rekor.sigstore.dev
- Try with `-rekor=false` for testing
- Check Sigstore status: https://status.sigstore.dev/

### "Context deadline exceeded"
- Increase timeout (see advanced examples)
- Check network connectivity
- Verify registry is accessible

## License

See [LICENSE](../../../LICENSE) for license information.
