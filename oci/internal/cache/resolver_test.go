package cache

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTagCache implements TagCache for testing
type mockTagCache struct {
	mappings  map[string]*TagMapping
	histories map[string][]TagHistoryEntry
	mu        sync.RWMutex
}

func newMockTagCache() *mockTagCache {
	return &mockTagCache{
		mappings:  make(map[string]*TagMapping),
		histories: make(map[string][]TagHistoryEntry),
	}
}

func (m *mockTagCache) GetTagMapping(ctx context.Context, reference string) (*TagMapping, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mapping, exists := m.mappings[reference]; exists {
		return mapping, nil
	}
	return nil, ErrCacheExpired
}

func (m *mockTagCache) PutTagMapping(ctx context.Context, reference, digest string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	mapping := &TagMapping{
		Reference:   reference,
		Digest:      digest,
		CreatedAt:   now,
		UpdatedAt:   now,
		AccessCount: 0,
	}
	m.mappings[reference] = mapping
	return nil
}

func (m *mockTagCache) HasTagMapping(ctx context.Context, reference string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.mappings[reference]
	return exists, nil
}

func (m *mockTagCache) DeleteTagMapping(ctx context.Context, reference string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mappings, reference)
	delete(m.histories, reference)
	return nil
}

func (m *mockTagCache) GetTagHistory(
	ctx context.Context,
	reference string,
) ([]TagHistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if history, exists := m.histories[reference]; exists {
		return history, nil
	}
	return []TagHistoryEntry{}, nil
}

// mockManifestCache implements ManifestCache for testing
type mockManifestCache struct {
	manifests map[string]bool
	mu        sync.RWMutex
}

func newMockManifestCache() *mockManifestCache {
	return &mockManifestCache{
		manifests: make(map[string]bool),
	}
}

func (m *mockManifestCache) GetManifest(
	ctx context.Context,
	digest string,
) (*ocispec.Manifest, error) {
	return nil, ErrCacheExpired
}

func (m *mockManifestCache) PutManifest(
	ctx context.Context,
	digest string,
	manifest *ocispec.Manifest,
) error {
	return nil
}

func (m *mockManifestCache) HasManifest(ctx context.Context, digest string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.manifests[digest], nil
}

func (m *mockManifestCache) ValidateManifest(
	ctx context.Context,
	reference, digest string,
) (bool, error) {
	return m.manifests[digest], nil
}

// mockRoundTripper implements http.RoundTripper for mocking HTTP responses
type mockRoundTripper struct {
	responses map[string]*http.Response
	mu        sync.RWMutex
}

func newMockRoundTripper() *mockRoundTripper {
	return &mockRoundTripper{
		responses: make(map[string]*http.Response),
	}
}

func (m *mockRoundTripper) addResponse(method, url string, statusCode int, headers map[string]string, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := method + "|" + url
	response := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	for k, v := range headers {
		response.Header.Set(k, v)
	}

	m.responses[key] = response
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.RLock()
	key := req.Method + "|" + req.URL.String()

	if response, exists := m.responses[key]; exists {
		m.mu.RUnlock()
		// Return a copy to avoid issues with concurrent use
		respCopy := &http.Response{
			StatusCode: response.StatusCode,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}
		// Copy headers
		for k, v := range response.Header {
			respCopy.Header[k] = v
		}
		if response.Body != nil {
			response.Body.Close()
		}
		return respCopy, nil
	}
	m.mu.RUnlock()

	// Default 404 response
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func setupResolverTest(
	t *testing.T,
) (*TagResolver, *mockTagCache, *mockManifestCache, func()) {
	t.Helper()

	// Create mocks
	tagCache := newMockTagCache()
	manifestCache := newMockManifestCache()

	// Create mock HTTP round tripper
	mockTransport := newMockRoundTripper()

	// Setup mock responses
	mockTransport.addResponse(
		http.MethodHead,
		"https://registry-1.docker.io/v2/library/nginx/manifests/latest",
		http.StatusOK,
		map[string]string{
			"Docker-Content-Digest": "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		"",
	)
	mockTransport.addResponse(
		http.MethodHead,
		"https://registry-1.docker.io/v2/library/nginx/manifests/v1.20",
		http.StatusOK,
		map[string]string{
			"Docker-Content-Digest": "sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
		},
		"",
	)
	mockTransport.addResponse(
		http.MethodHead,
		"https://registry-1.docker.io/v2/library/notfound/manifests/latest",
		http.StatusNotFound,
		map[string]string{},
		"",
	)

	// Create resolver config
	config := TagResolverConfig{
		DefaultTTL:     time.Hour,
		MaxHistorySize: 5,
		EnableHistory:  true,
	}

	// Create resolver
	resolver := NewTagResolver(tagCache, manifestCache, config)

	// Override HTTP client transport to use mock transport
	resolver.httpClient.Transport = mockTransport

	cleanup := func() {
		// No cleanup needed for mock transport
	}

	return resolver, tagCache, manifestCache, cleanup
}

func TestTagResolver_ResolveTag(t *testing.T) {
	resolver, tagCache, manifestCache, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Mock manifest as available
	expectedDigest := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	manifestCache.manifests[expectedDigest] = true

	// Resolve tag
	digest, err := resolver.ResolveTag(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, expectedDigest, digest)

	// Verify cache was updated
	mapping, exists := tagCache.mappings[reference]
	require.True(t, exists)
	assert.Equal(t, expectedDigest, mapping.Digest)
}

func TestTagResolver_ResolveTag_CacheHit(t *testing.T) {
	resolver, tagCache, manifestCache, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	cachedDigest := "sha256:cacheddigest"

	// Pre-populate cache
	err := tagCache.PutTagMapping(ctx, reference, cachedDigest)
	require.NoError(t, err)

	// Mock manifest as available
	manifestCache.manifests[cachedDigest] = true

	// Resolve tag - should use cache
	digest, err := resolver.ResolveTag(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, cachedDigest, digest)
}

func TestTagResolver_ResolveTag_CacheMissManifestMissing(t *testing.T) {
	resolver, tagCache, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"
	cachedDigest := "sha256:cacheddigest"

	// Pre-populate cache but don't mock manifest as available
	err := tagCache.PutTagMapping(ctx, reference, cachedDigest)
	require.NoError(t, err)

	// Resolve tag - should go to registry since manifest check fails
	digest, err := resolver.ResolveTag(ctx, reference)
	require.NoError(t, err)
	assert.Equal(t, "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", digest)
}

func TestTagResolver_ValidateTag(t *testing.T) {
	resolver, tagCache, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Test cache hit
	err := tagCache.PutTagMapping(ctx, reference, "sha256:test")
	require.NoError(t, err)

	valid, err := resolver.ValidateTag(ctx, reference)
	require.NoError(t, err)
	assert.True(t, valid)

	// Test cache miss -> registry validation
	valid, err = resolver.ValidateTag(ctx, "docker.io/library/nginx:v1.20")
	require.NoError(t, err)
	assert.True(t, valid)

	// Test non-existent tag
	valid, err = resolver.ValidateTag(ctx, "docker.io/library/notfound:latest")
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestTagResolver_InvalidateTag(t *testing.T) {
	resolver, tagCache, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Put mapping
	err := tagCache.PutTagMapping(ctx, reference, "sha256:test")
	require.NoError(t, err)

	// Verify exists
	exists, err := tagCache.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.True(t, exists)

	// Invalidate
	err = resolver.InvalidateTag(ctx, reference)
	require.NoError(t, err)

	// Verify no longer exists
	exists, err = tagCache.HasTagMapping(ctx, reference)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestTagResolver_DetectTagMovement(t *testing.T) {
	resolver, _, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:v1.20"

	// Detect movement from expected digest to actual registry digest
	moved, err := resolver.DetectTagMovement(ctx, reference, "sha256:olddigest")
	require.NoError(t, err)
	assert.True(t, moved)

	// Detect no movement when digest matches
	moved, err = resolver.DetectTagMovement(
		ctx,
		reference,
		"sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
	)
	require.NoError(t, err)
	assert.False(t, moved)
}

func TestTagResolver_GetTagHistory(t *testing.T) {
	resolver, tagCache, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Add some history entries
	tagCache.histories[reference] = []TagHistoryEntry{
		{Digest: "sha256:old1", ChangedAt: time.Now().Add(-time.Hour)},
		{Digest: "sha256:old2", ChangedAt: time.Now().Add(-time.Hour * 2)},
	}

	history, err := resolver.GetTagHistory(ctx, reference)
	require.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "sha256:old1", history[0].Digest)
	assert.Equal(t, "sha256:old2", history[1].Digest)
}

func TestParseReference(t *testing.T) {
	testCases := []struct {
		reference        string
		expectedRegistry string
		expectedRepo     string
		expectedRef      string
		expectError      bool
	}{
		{"docker.io/library/nginx:latest", "docker.io", "library/nginx", "latest", false},
		{"nginx:latest", "docker.io", "nginx", "latest", false},
		{"registry.example.com/my/repo:v1.0.0", "registry.example.com", "my/repo", "v1.0.0", false},
		{
			"localhost:5000/test/image@sha256:abc123",
			"localhost:5000",
			"test/image",
			"sha256:abc123",
			false,
		},
		{"", "", "", "", true},
		{"invalid", "", "", "", true},
	}

	for _, tc := range testCases {
		registry, repo, ref, err := parseReference(tc.reference)
		if tc.expectError {
			assert.Error(t, err, "Reference: %s", tc.reference)
		} else {
			require.NoError(t, err, "Reference: %s", tc.reference)
			assert.Equal(t, tc.expectedRegistry, registry)
			assert.Equal(t, tc.expectedRepo, repo)
			assert.Equal(t, tc.expectedRef, ref)
		}
	}
}

func TestTagResolver_BuildRegistryURL(t *testing.T) {
	resolver, _, _, cleanup := setupResolverTest(t)
	defer cleanup()

	testCases := []struct {
		reference   string
		expectedURL string
		expectError bool
	}{
		{
			"docker.io/library/nginx:latest",
			"https://registry-1.docker.io/v2/library/nginx/manifests/latest",
			false,
		},
		{"nginx:latest", "https://registry-1.docker.io/v2/nginx/manifests/latest", false},
		{
			"registry.example.com/my/repo:v1.0.0",
			"https://registry.example.com/v2/my/repo/manifests/v1.0.0",
			false,
		},
		{
			"localhost:5000/test/image@sha256:abc123",
			"https://localhost:5000/v2/test/image/manifests/sha256:abc123",
			false,
		},
		{"", "", true},
	}

	for _, tc := range testCases {
		url, err := resolver.buildRegistryURL(tc.reference)
		if tc.expectError {
			assert.Error(t, err, "Reference: %s", tc.reference)
		} else {
			require.NoError(t, err, "Reference: %s", tc.reference)
			assert.Equal(t, tc.expectedURL, url)
		}
	}
}

func TestTagResolver_BatchResolveTags(t *testing.T) {
	resolver, _, manifestCache, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	references := []string{
		"docker.io/library/nginx:latest",
		"docker.io/library/nginx:v1.20",
	}

	// Mock manifests as available
	manifestCache.manifests["sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"] = true
	manifestCache.manifests["sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"] = true

	results, err := resolver.BatchResolveTags(ctx, references)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(
		t,
		"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		results["docker.io/library/nginx:latest"],
	)
	assert.Equal(
		t,
		"sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
		results["docker.io/library/nginx:v1.20"],
	)
}

func TestTagResolver_InvalidInputs(t *testing.T) {
	resolver, _, _, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test empty reference
	_, err := resolver.ResolveTag(ctx, "")
	assert.Error(t, err)

	// Test invalid reference
	_, err = resolver.ResolveTag(ctx, "invalid")
	assert.Error(t, err)

	// Test invalid reference for validation
	valid, err := resolver.ValidateTag(ctx, "")
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestTagResolver_ConcurrentOperations(t *testing.T) {
	resolver, _, manifestCache, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Mock manifest as available
	manifestCache.manifests["sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"] = true

	// Run concurrent operations
	const numGoroutines = 10
	const numOperations = 20

	errChan := make(chan error, numGoroutines*numOperations)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numOperations; j++ {
				_, err := resolver.ResolveTag(ctx, reference)
				errChan <- err
			}
		}()
	}

	// Collect all errors
	for i := 0; i < numGoroutines*numOperations; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}
}

func TestTagResolver_HealthCheck(t *testing.T) {
	resolver, _, manifestCache, cleanup := setupResolverTest(t)
	defer cleanup()

	ctx := context.Background()

	// Mock the health check manifest as available
	manifestCache.manifests["sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"] = true

	// Health check should pass
	err := resolver.HealthCheck(ctx)
	assert.NoError(t, err)
}

// Test with network failure simulation
func TestTagResolver_NetworkFailure(t *testing.T) {
	tagCache := newMockTagCache()
	manifestCache := newMockManifestCache()

	// Create mock transport that simulates network failure
	mockTransport := newMockRoundTripper()

	config := TagResolverConfig{
		DefaultTTL:     time.Hour,
		MaxHistorySize: 5,
		EnableHistory:  true,
	}

	resolver := NewTagResolver(tagCache, manifestCache, config)
	// Override with mock transport that returns connection error
	resolver.httpClient.Transport = mockTransport

	ctx := context.Background()
	reference := "docker.io/library/nginx:latest"

	// Resolution should fail due to "network" issues (no mock response configured)
	_, err := resolver.ResolveTag(ctx, reference)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag not found in registry")
}
