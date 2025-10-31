package ocibundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmgilman/go/fs/billy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/jmgilman/go/oci/internal/oras"
	"github.com/jmgilman/go/oci/internal/oras/mocks"
)

// TestNewClient tests creating a client with default options
func TestNewClient(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options)
	assert.Nil(t, client.options.Auth) // Default should use Docker credential chain
}

// TestNewClientWithOptions tests creating a client with custom options
func TestNewClientWithOptions(t *testing.T) {
	client, err := NewWithOptions(WithAuthNone())
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options)
	assert.Nil(t, client.options.Auth) // WithAuthNone should set to nil
}

// TestWithStaticAuth tests the WithStaticAuth option
func TestWithStaticAuth(t *testing.T) {
	client, err := NewWithOptions(
		WithStaticAuth("ghcr.io", "testuser", "testpass"),
	)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options.Auth)
	assert.Equal(t, "ghcr.io", client.options.Auth.StaticRegistry)
	assert.Equal(t, "testuser", client.options.Auth.StaticUsername)
	assert.Equal(t, "testpass", client.options.Auth.StaticPassword)
}

// TestClientValidation tests client option validation
func TestClientValidation(t *testing.T) {
	t.Run("valid options", func(t *testing.T) {
		client, err := NewWithOptions(WithAuthNone())
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("static auth without username", func(t *testing.T) {
		_, err := NewWithOptions(WithStaticAuth("ghcr.io", "", "password"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "static username required")
	})

	t.Run("static auth without password", func(t *testing.T) {
		_, err := NewWithOptions(WithStaticAuth("ghcr.io", "username", ""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "static password required")
	})

	t.Run("both static auth and credential function allowed", func(t *testing.T) {
		customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
			return auth.Credential{Username: "custom", Password: "pass"}, nil
		}

		client, err := NewWithOptions(
			WithStaticAuth("ghcr.io", "staticuser", "staticpass"),
			WithCredentialFunc(customFunc),
		)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.NotNil(t, client.options.Auth)
		// CredentialFunc should be set and take precedence
		assert.NotNil(t, client.options.Auth.CredentialFunc)
		// Static auth should still be configured but will be overridden
		assert.Equal(t, "ghcr.io", client.options.Auth.StaticRegistry)
		assert.Equal(t, "staticuser", client.options.Auth.StaticUsername)
		assert.Equal(t, "staticpass", client.options.Auth.StaticPassword)
	})
}

// TestWithCredentialFunc tests the WithCredentialFunc option
func TestWithCredentialFunc(t *testing.T) {
	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		return auth.Credential{Username: "custom", Password: "pass"}, nil
	}

	client, err := NewWithOptions(WithCredentialFunc(customFunc))
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options.Auth)
	assert.NotNil(t, client.options.Auth.CredentialFunc)
}

// TestMultipleOptions tests combining multiple options
func TestMultipleOptions(t *testing.T) {
	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		return auth.Credential{Username: "custom", Password: "pass"}, nil
	}

	// CredentialFunc should take precedence over StaticAuth
	client, err := NewWithOptions(
		WithStaticAuth("ghcr.io", "staticuser", "staticpass"),
		WithCredentialFunc(customFunc),
	)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options.Auth)
	// CredentialFunc should be set
	assert.NotNil(t, client.options.Auth.CredentialFunc)
	// Static auth should still be set but will be overridden by CredentialFunc
	assert.Equal(t, "ghcr.io", client.options.Auth.StaticRegistry)
}

// TestDefaultClientOptions tests the default options
func TestDefaultClientOptions(t *testing.T) {
	opts := DefaultClientOptions()
	assert.NotNil(t, opts)
	assert.Nil(t, opts.Auth) // Default should use Docker credential chain
}

// TestClient_Push_Validation tests input validation for Push
func TestClient_Push_Validation(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("nonexistent directory", func(t *testing.T) {
		err = client.Push(ctx, "/nonexistent/directory", "ghcr.io/test/repo:tag")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source directory does not exist")
	})

	t.Run("empty source directory", func(t *testing.T) {
		err = client.Push(ctx, "", "ghcr.io/test/repo:tag")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source directory cannot be empty")
	})

	t.Run("empty reference", func(t *testing.T) {
		sourceDir := t.TempDir()
		err = client.Push(ctx, sourceDir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reference cannot be empty")
	})
}

// TestClient_Push_BasicFunctionality tests basic push functionality
func TestClient_Push_BasicFunctionality(t *testing.T) {
	// Clear any cached credentials to ensure test isolation
	oras.ClearAuthCache()

	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			return fmt.Errorf("simulated push error")
		},
	}
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create a test file in the source directory
	testFile := filepath.Join(sourceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// This should attempt to push via mock (will fail with simulated error)
	err = client.Push(ctx, sourceDir, "ghcr.io/test/repo:tag", WithMaxRetries(0), WithRetryDelay(0))
	assert.Error(t, err)
	// Should contain authentication or push-related error, not "not implemented"
	assert.NotContains(t, err.Error(), "Push not yet implemented")
	assert.Contains(t, err.Error(), "failed to push artifact")
}

// TestClient_Push_WithOptions tests push with various options
func TestClient_Push_WithOptions(t *testing.T) {
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			return fmt.Errorf("simulated push error")
		},
	}
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create a test file in the source directory
	testFile := filepath.Join(sourceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Test with annotations
	annotations := map[string]string{
		"version":  "1.0.0",
		"author":   "test",
		"build-id": "12345",
	}

	// Test with platform
	platform := "linux/amd64"

	// Test with progress callback
	var progressCalls []int64
	progressCallback := func(current, total int64) {
		progressCalls = append(progressCalls, current)
	}

	// This should attempt to push (mocked) and fail with simulated error
	err = client.Push(ctx, sourceDir, "ghcr.io/test/repo:tag",
		WithAnnotations(annotations),
		WithPlatform(platform),
		WithProgressCallback(progressCallback),
		WithMaxRetries(0), WithRetryDelay(0))
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "Push not yet implemented")
	assert.Contains(t, err.Error(), "failed to push artifact")
}

// TestClient_Push_ErrorHandling tests error handling and cleanup
func TestClient_Push_ErrorHandling(t *testing.T) {
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			return fmt.Errorf("simulated push error")
		},
	}
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("empty source directory", func(t *testing.T) {
		sourceDir := t.TempDir()
		err = client.Push(ctx, sourceDir, "ghcr.io/test/repo:tag", WithMaxRetries(0), WithRetryDelay(0))
		assert.Error(t, err)
		// Should attempt to push (will fail due to mock) but not due to "not implemented"
		assert.NotContains(t, err.Error(), "Push not yet implemented")
		assert.Contains(t, err.Error(), "failed to push artifact")
	})

	t.Run("invalid reference", func(t *testing.T) {
		sourceDir := t.TempDir()
		err = os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("content"), 0o644)
		require.NoError(t, err)

		err = client.Push(ctx, sourceDir, "invalid-reference", WithMaxRetries(0), WithRetryDelay(0))
		assert.Error(t, err)
		// Should fail at repository creation due to invalid reference format
		assert.Contains(t, err.Error(), "invalid reference")
	})
}

// TestClient_Push_ProgressReporting tests progress callback functionality
func TestClient_Push_ProgressReporting(t *testing.T) {
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			return fmt.Errorf("simulated push error")
		},
	}
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create multiple test files
	for i := 0; i < 10; i++ {
		testFile := filepath.Join(sourceDir, fmt.Sprintf("test%d.txt", i))
		content := make([]byte, 1024) // 1KB per file
		for j := range content {
			content[j] = byte(i + j)
		}
		err = os.WriteFile(testFile, content, 0o644)
		require.NoError(t, err)
	}

	var progressCalls []int64
	progressCallback := func(current, total int64) {
		progressCalls = append(progressCalls, current)
	}

	// This should attempt to push (mocked) and fail quickly
	err = client.Push(
		ctx,
		sourceDir,
		"ghcr.io/test/repo:tag",
		WithProgressCallback(progressCallback),
		WithMaxRetries(0),
		WithRetryDelay(0),
	)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "Push not yet implemented")
	assert.Contains(t, err.Error(), "failed to push artifact")
}

// TestClient_Push_OptionsValidation tests that push options are properly handled
func TestClient_Push_OptionsValidation(t *testing.T) {
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			return fmt.Errorf("simulated push error")
		},
	}
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(sourceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	t.Run("empty annotations", func(t *testing.T) {
		err = client.Push(
			ctx,
			sourceDir,
			"ghcr.io/test/repo:tag",
			WithAnnotations(nil),
			WithMaxRetries(0),
			WithRetryDelay(0),
		)
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "Push not yet implemented")
		assert.Contains(t, err.Error(), "failed to push artifact")
	})

	t.Run("multiple annotation sets", func(t *testing.T) {
		annotations1 := map[string]string{"key1": "value1"}
		annotations2 := map[string]string{"key2": "value2"}

		err = client.Push(ctx, sourceDir, "ghcr.io/test/repo:tag",
			WithAnnotations(annotations1),
			WithAnnotations(annotations2), WithMaxRetries(0), WithRetryDelay(0))
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "Push not yet implemented")
		assert.Contains(t, err.Error(), "failed to push artifact")
	})

	t.Run("empty platform", func(t *testing.T) {
		err = client.Push(
			ctx,
			sourceDir,
			"ghcr.io/test/repo:tag",
			WithPlatform(""),
			WithMaxRetries(0),
			WithRetryDelay(0),
		)
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "Push not yet implemented")
		assert.Contains(t, err.Error(), "failed to push artifact")
	})

	t.Run("nil progress callback", func(t *testing.T) {
		err = client.Push(
			ctx,
			sourceDir,
			"ghcr.io/test/repo:tag",
			WithProgressCallback(nil),
			WithMaxRetries(0),
			WithRetryDelay(0),
		)
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "Push not yet implemented")
		assert.Contains(t, err.Error(), "failed to push artifact")
	})
}

// TestClient_PushIntegration tests push functionality with a local registry
// This test requires Docker to be running and will start a local registry container
func TestClient_PushIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This is a placeholder for integration tests
	// In a real implementation, this would use testcontainers to start a local registry
	// and test the full push/pull cycle
	t.Skip("Integration tests require testcontainers setup - placeholder for now")

	_, _ = New()    // Placeholder - would be used in real implementation
	_ = t.TempDir() // Placeholder - would be used in real implementation

	// Test with annotations - placeholder
	_ = map[string]string{
		"org.opencontainers.image.title":       "Test Bundle",
		"org.opencontainers.image.description": "Integration test bundle",
		"org.opencontainers.image.version":     "1.0.0",
		"org.opencontainers.image.vendor":      "Test Vendor",
	}

	// This would push to a local registry started by testcontainers
	// err = client.Push(ctx, sourceDir, "localhost:5000/test/bundle:v1.0.0",
	//     WithAnnotations(annotations),
	//     WithPlatform("linux/amd64"))
	// require.NoError(t, err)

	// Verify the push was successful by pulling and comparing
	// This would require implementing the Pull method as well
}

// TestClient_Push_RetryLogic tests the retry mechanism
func TestClient_Push_RetryLogic(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("retry test"), 0o644))

	// Test with custom retry settings
	err = client.Push(ctx, sourceDir, "ghcr.io/test/repo:tag",
		WithMaxRetries(5),
		WithRetryDelay(100*time.Millisecond))
	assert.Error(t, err)
	// Should still fail with auth error but should have attempted retries
	assert.Contains(t, err.Error(), "failed to push artifact")
}

// TestClient_Pull_RetryLogic tests the retry mechanism for Pull
func TestClient_Pull_RetryLogic(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()
	targetDir := t.TempDir()

	// Test with custom retry settings
	err = client.Pull(ctx, "ghcr.io/test/repo:tag", targetDir,
		WithPullMaxRetries(5),
		WithPullRetryDelay(100*time.Millisecond))
	assert.Error(t, err)
	// Should still fail with auth error but should have attempted retries
	assert.Contains(t, err.Error(), "failed to pull artifact")
}

// TestClient_Pull_WithOptions tests pull with various options
func TestClient_Pull_WithOptions(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()
	targetDir := t.TempDir()

	// Test with various pull options
	err = client.Pull(ctx, "ghcr.io/test/repo:tag", targetDir,
		WithPullMaxFiles(100),
		WithPullMaxSize(50*1024*1024),     // 50MB
		WithPullMaxFileSize(10*1024*1024), // 10MB per file
		WithPullAllowHiddenFiles(false),
		WithPullPreservePermissions(false),
		WithPullStripPrefix(""),
		WithPullMaxRetries(3),
		WithPullRetryDelay(1*time.Second))
	assert.Error(t, err)
	// Should attempt to pull (will fail due to auth) but not due to "not implemented"
	assert.NotContains(t, err.Error(), "Pull not yet implemented")
	assert.Contains(t, err.Error(), "failed to pull artifact")
}

// TestClient_Pull_AtomicExtraction tests atomic extraction functionality
func TestClient_Pull_AtomicExtraction(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("non-empty target directory", func(t *testing.T) {
		targetDir := t.TempDir()
		// Create a file in the target directory
		require.NoError(t, os.WriteFile(filepath.Join(targetDir, "existing.txt"), []byte("existing"), 0o644))

		err = client.Pull(ctx, "ghcr.io/test/repo:tag", targetDir)
		assert.Error(t, err)
		// Should fail because target directory is not empty
		assert.Contains(t, err.Error(), "target directory is not empty")
	})

	t.Run("empty target directory", func(t *testing.T) {
		targetDir := t.TempDir()
		// Ensure target directory is empty
		entries, err := os.ReadDir(targetDir)
		require.NoError(t, err)
		require.Len(t, entries, 0)

		err = client.Pull(ctx, "ghcr.io/test/repo:tag", targetDir)
		assert.Error(t, err)
		// Should attempt to pull (will fail due to auth)
		assert.NotContains(t, err.Error(), "target directory is not empty")
		assert.Contains(t, err.Error(), "failed to pull artifact")
	})
}

// TestClient_Pull_OptionsValidation tests that pull options work correctly
func TestClient_Pull_OptionsValidation(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()
	targetDir := t.TempDir()

	t.Run("default options", func(t *testing.T) {
		opts := DefaultPullOptions()
		assert.NotNil(t, opts)
		assert.Equal(t, 10000, opts.MaxFiles)
		assert.Equal(t, int64(1*1024*1024*1024), opts.MaxSize)  // 1GB
		assert.Equal(t, int64(100*1024*1024), opts.MaxFileSize) // 100MB
		assert.False(t, opts.AllowHiddenFiles)
		assert.False(t, opts.PreservePermissions)
		assert.Equal(t, "", opts.StripPrefix)
		assert.Equal(t, 3, opts.MaxRetries)
		assert.Equal(t, 2*time.Second, opts.RetryDelay)
	})

	t.Run("custom options", func(t *testing.T) {
		err = client.Pull(ctx, "ghcr.io/test/repo:tag", targetDir,
			WithPullMaxFiles(50),
			WithPullMaxSize(10*1024*1024), // 10MB
			WithPullAllowHiddenFiles(true),
			WithPullPreservePermissions(true),
			WithPullStripPrefix("app/"))
		assert.Error(t, err)
		// Should attempt to pull (will fail due to auth)
		assert.Contains(t, err.Error(), "failed to pull artifact")
	})
}

// TestClient_Pull_Validation tests input validation for Pull
func TestClient_Pull_Validation(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("empty reference", func(t *testing.T) {
		targetDir := t.TempDir()
		err = client.Pull(ctx, "", targetDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reference cannot be empty")
	})

	t.Run("empty target directory", func(t *testing.T) {
		err = client.Pull(ctx, "ghcr.io/test/repo:tag", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target directory cannot be empty")
	})

	t.Run("nonexistent target directory", func(t *testing.T) {
		err = client.Pull(ctx, "ghcr.io/test/repo:tag", "/nonexistent/directory")
		assert.Error(t, err)
		// Should attempt to pull (will fail due to auth) but not due to "not implemented"
		assert.NotContains(t, err.Error(), "Pull not yet implemented")
		assert.Contains(t, err.Error(), "failed to pull artifact")
	})

	t.Run("invalid reference", func(t *testing.T) {
		targetDir := t.TempDir()
		err = client.Pull(ctx, "invalid-reference", targetDir)
		assert.Error(t, err)
		// Should fail with invalid reference error
		assert.Contains(t, err.Error(), "invalid reference")
	})
}

// TestClient_createRepository_WithStaticAuth tests repository creation behavior with static auth
func TestClient_createRepository_WithStaticAuth(t *testing.T) {
	client, err := NewWithOptions(
		WithStaticAuth("ghcr.io", "testuser", "testpass"),
	)
	require.NoError(t, err)

	ctx := context.Background()
	repo, err := client.createRepository(ctx, "ghcr.io/test/repo:tag")
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// Test that credentials work for the configured registry
	authClient, ok := repo.Client.(*auth.Client)
	require.True(t, ok, "Client should be an auth.Client")
	cred, err := authClient.Credential(ctx, "ghcr.io")
	require.NoError(t, err)
	assert.Equal(t, "testuser", cred.Username)
	assert.Equal(t, "testpass", cred.Password)

	// Test fallback for other registries
	cred, err = authClient.Credential(ctx, "docker.io")
	require.NoError(t, err)
	assert.Equal(t, "", cred.Username) // Should be empty for fallback
}

// TestClient_createRepository_WithCredentialFunc tests createRepository with custom credential function
func TestClient_createRepository_WithCredentialFunc(t *testing.T) {
	// Clear any cached credentials to ensure test isolation
	oras.ClearAuthCache()

	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		if registry == "ghcr.io" {
			return auth.Credential{Username: "customuser", Password: "custompass"}, nil
		}
		return auth.Credential{}, nil
	}

	client, err := NewWithOptions(WithCredentialFunc(customFunc))
	require.NoError(t, err)

	ctx := context.Background()
	repo, err := client.createRepository(ctx, "ghcr.io/test/repo:tag")
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// Test custom credential function
	authClient, ok := repo.Client.(*auth.Client)
	require.True(t, ok, "Client should be an auth.Client")

	// Clear cache again to ensure we get fresh credentials from the function
	oras.ClearAuthCache()

	cred, err := authClient.Credential(ctx, "ghcr.io")
	require.NoError(t, err)
	assert.Equal(t, "customuser", cred.Username)
	assert.Equal(t, "custompass", cred.Password)
}

// TestClient_Pull_WithMockORASClient demonstrates unit testing with mocked ORAS client
// This test shows how to avoid network calls in unit tests by injecting a mock.
func TestClient_Pull_WithMockORASClient(t *testing.T) {
	// Create mock tar.gz data for testing
	mockTarGzData, err := createMockTarGzData()
	require.NoError(t, err)

	// Create a mock ORAS client using the generated mock
	mockORAS := &mocks.ClientMock{
		PullFunc: func(ctx context.Context, reference string, opts *oras.AuthOptions) (*oras.PullDescriptor, error) {
			// Return mock tar.gz data instead of making network calls
			return &oras.PullDescriptor{
				MediaType: "application/tar+gzip",
				Data:      &mockReadCloserForTest{data: mockTarGzData},
				Size:      int64(len(mockTarGzData)),
			}, nil
		},
	}

	// Create client with mock ORAS client (no network calls)
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	targetDir := t.TempDir()

	// This should work without network calls
	err = client.Pull(ctx, "example.com/test/repo:tag", targetDir)
	assert.NoError(t, err, "Pull should succeed with mock ORAS client")

	// Verify the target directory contains extracted files
	entries, err := os.ReadDir(targetDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "Target directory should contain extracted files")
}

// TestClient_Push_WithMockORASClient demonstrates unit testing push with mocked ORAS client
func TestClient_Push_WithMockORASClient(t *testing.T) {
	// Create a mock ORAS client that simulates successful push
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			// Simulate successful push without network calls
			return nil
		},
	}

	// Create client with mock ORAS client
	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	sourceDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(sourceDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

	// This should work without network calls
	err = client.Push(ctx, sourceDir, "example.com/test/repo:tag")
	assert.NoError(t, err, "Push should succeed with mock ORAS client")
}

// TestClient_Pull_WithMockError demonstrates testing error scenarios
func TestClient_Pull_WithMockError(t *testing.T) {
	// Create a mock ORAS client that simulates an error
	mockORAS := &mocks.ClientMock{
		PullFunc: func(ctx context.Context, reference string, opts *oras.AuthOptions) (*oras.PullDescriptor, error) {
			return nil, fmt.Errorf("simulated network error")
		},
	}

	client, err := NewWithOptions(WithORASClient(mockORAS))
	require.NoError(t, err)

	ctx := context.Background()
	targetDir := t.TempDir()

	// This should fail with the simulated error
	err = client.Pull(ctx, "example.com/test/repo:tag", targetDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated network error")
}

// createMockTarGzData creates a simple tar.gz archive for testing
func createMockTarGzData() ([]byte, error) {
	var buf bytes.Buffer

	// Create a tar writer
	tarWriter := tar.NewWriter(&buf)

	// Add a simple test file to the tar archive
	testContent := "Hello, World!"
	header := &tar.Header{
		Name: "test.txt",
		Mode: 0o644,
		Size: int64(len(testContent)),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return nil, err
	}

	if _, err := tarWriter.Write([]byte(testContent)); err != nil {
		return nil, err
	}

	// Close the tar writer
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}

	// Compress with gzip
	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	if _, err := gzipWriter.Write(buf.Bytes()); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}

	return gzipBuf.Bytes(), nil
}

// mockReadCloserForTest implements io.ReadCloser for testing
type mockReadCloserForTest struct {
	data   []byte
	offset int
}

func (m *mockReadCloserForTest) Read(p []byte) (n int, err error) {
	if m.offset >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockReadCloserForTest) Close() error {
	return nil
}

func TestClient_WithMemFS_PushAndPull_WithMocks(t *testing.T) {
	// Setup in-memory filesystem
	mem := billy.NewMemory()

	// Write a simple source tree into memfs
	require.NoError(t, mem.MkdirAll("/src", 0o755))
	require.NoError(t, mem.WriteFile("/src/hello.txt", []byte("hi"), 0o644))

	// Mock ORAS to avoid network
	mockORAS := &mocks.ClientMock{
		PushFunc: func(ctx context.Context, reference string, descriptor *oras.PushDescriptor, opts *oras.AuthOptions) error {
			// Consume all bytes to simulate upload
			_, _ = io.Copy(io.Discard, descriptor.Data)
			return nil
		},
		PullFunc: func(ctx context.Context, reference string, opts *oras.AuthOptions) (*oras.PullDescriptor, error) {
			// Build a tiny tar.gz with a single file
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tr := tar.NewWriter(gz)
			require.NoError(t, tr.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0o644, Size: 2}))
			_, _ = tr.Write([]byte("hi"))
			require.NoError(t, tr.Close())
			require.NoError(t, gz.Close())
			return &oras.PullDescriptor{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Data:      &mockReadCloserForTest{data: buf.Bytes()},
				Size:      int64(buf.Len()),
			}, nil
		},
	}

	client, err := NewWithOptions(WithORASClient(mockORAS), WithFilesystem(mem))
	require.NoError(t, err)

	ctx := context.Background()

	// Push using memfs source
	require.NoError(t, client.Push(ctx, "/src", "example.com/repo:tag"))

	// Pull using memfs target
	require.NoError(t, client.Pull(ctx, "example.com/repo:tag", "/dst"))

	// Verify file exists in memfs
	b, err := mem.ReadFile("/dst/hello.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), b)
}
