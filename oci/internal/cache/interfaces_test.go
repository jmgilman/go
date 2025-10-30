package cache

import (
	"context"
	"io"
	"testing"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCache is a mock implementation of the Cache interface for testing
type MockCache struct {
	mock.Mock
}

func (m *MockCache) Get(ctx context.Context, key string) (*Entry, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Entry), args.Error(1)
}

func (m *MockCache) Put(ctx context.Context, key string, entry *Entry) error {
	args := m.Called(ctx, key, entry)
	return args.Error(0)
}

func (m *MockCache) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockCache) Clear(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCache) Size(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// MockManifestCache is a mock implementation of the ManifestCache interface for testing
type MockManifestCache struct {
	mock.Mock
}

func (m *MockManifestCache) GetManifest(
	ctx context.Context,
	digest string,
) (*ocispec.Manifest, error) {
	args := m.Called(ctx, digest)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ocispec.Manifest), args.Error(1)
}

func (m *MockManifestCache) PutManifest(
	ctx context.Context,
	digest string,
	manifest *ocispec.Manifest,
) error {
	args := m.Called(ctx, digest, manifest)
	return args.Error(0)
}

func (m *MockManifestCache) HasManifest(ctx context.Context, digest string) (bool, error) {
	args := m.Called(ctx, digest)
	return args.Bool(0), args.Error(1)
}

func (m *MockManifestCache) ValidateManifest(
	ctx context.Context,
	reference, digest string,
) (bool, error) {
	args := m.Called(ctx, reference, digest)
	return args.Bool(0), args.Error(1)
}

// MockBlobCache is a mock implementation of the BlobCache interface for testing
type MockBlobCache struct {
	mock.Mock
}

func (m *MockBlobCache) GetBlob(ctx context.Context, digest string) (io.ReadCloser, error) {
	args := m.Called(ctx, digest)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockBlobCache) PutBlob(ctx context.Context, digest string, reader io.Reader) error {
	args := m.Called(ctx, digest, reader)
	return args.Error(0)
}

func (m *MockBlobCache) HasBlob(ctx context.Context, digest string) (bool, error) {
	args := m.Called(ctx, digest)
	return args.Bool(0), args.Error(1)
}

func (m *MockBlobCache) DeleteBlob(ctx context.Context, digest string) error {
	args := m.Called(ctx, digest)
	return args.Error(0)
}

// MockEvictionStrategy is a mock implementation of the EvictionStrategy interface for testing
type MockEvictionStrategy struct {
	mock.Mock
}

func (m *MockEvictionStrategy) SelectForEviction(entries map[string]*Entry) []string {
	args := m.Called(entries)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}

func (m *MockEvictionStrategy) OnAccess(entry *Entry) {
	m.Called(entry)
}

func (m *MockEvictionStrategy) OnAdd(entry *Entry) {
	m.Called(entry)
}

func (m *MockEvictionStrategy) OnRemove(entry *Entry) {
	m.Called(entry)
}

// Test that the interfaces are properly defined and can be implemented
func TestInterfaceImplementation(t *testing.T) {
	// These tests verify that our mocks properly implement the interfaces
	// If they compile and run without panics, the interfaces are properly defined

	t.Run("Cache interface", func(t *testing.T) {
		var _ Cache = &MockCache{}
	})

	t.Run("ManifestCache interface", func(t *testing.T) {
		var _ ManifestCache = &MockManifestCache{}
	})

	t.Run("BlobCache interface", func(t *testing.T) {
		var _ BlobCache = &MockBlobCache{}
	})

	t.Run("EvictionStrategy interface", func(t *testing.T) {
		var _ EvictionStrategy = &MockEvictionStrategy{}
	})
}

// Test that mocks can be used in typical scenarios
func TestMockUsage(t *testing.T) {
	ctx := context.Background()

	t.Run("Cache mock", func(t *testing.T) {
		mockCache := &MockCache{}
		defer mockCache.AssertExpectations(t)

		expectedEntry := &Entry{Key: "test-key"}
		mockCache.On("Get", ctx, "test-key").Return(expectedEntry, nil)
		mockCache.On("Put", ctx, "test-key", expectedEntry).Return(nil)
		mockCache.On("Delete", ctx, "test-key").Return(nil)
		mockCache.On("Clear", ctx).Return(nil)
		mockCache.On("Size", ctx).Return(int64(1024), nil)

		entry, err := mockCache.Get(ctx, "test-key")
		assert.NoError(t, err)
		assert.Equal(t, expectedEntry, entry)

		err = mockCache.Put(ctx, "test-key", expectedEntry)
		assert.NoError(t, err)

		err = mockCache.Delete(ctx, "test-key")
		assert.NoError(t, err)

		err = mockCache.Clear(ctx)
		assert.NoError(t, err)

		size, err := mockCache.Size(ctx)
		assert.NoError(t, err)
		assert.Equal(t, int64(1024), size)
	})

	t.Run("ManifestCache mock", func(t *testing.T) {
		mockManifest := &MockManifestCache{}
		defer mockManifest.AssertExpectations(t)

		manifest := &ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
		}
		mockManifest.On("GetManifest", ctx, "sha256:abc").Return(manifest, nil)
		mockManifest.On("PutManifest", ctx, "sha256:abc", manifest).Return(nil)
		mockManifest.On("HasManifest", ctx, "sha256:abc").Return(true, nil)
		mockManifest.On("ValidateManifest", ctx, "ref:latest", "sha256:abc").Return(true, nil)

		data, err := mockManifest.GetManifest(ctx, "sha256:abc")
		assert.NoError(t, err)
		assert.Equal(t, manifest, data)

		err = mockManifest.PutManifest(ctx, "sha256:abc", manifest)
		assert.NoError(t, err)

		has, err := mockManifest.HasManifest(ctx, "sha256:abc")
		assert.NoError(t, err)
		assert.True(t, has)

		valid, err := mockManifest.ValidateManifest(ctx, "ref:latest", "sha256:abc")
		assert.NoError(t, err)
		assert.True(t, valid)
	})
}
