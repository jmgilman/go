package oras

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// TestNewRepository_DefaultAuth tests the default authentication behavior
func TestNewRepository_DefaultAuth(t *testing.T) {
	ctx := context.Background()

	// Test with default (nil) auth options
	repo, err := NewRepository(ctx, "ghcr.io/test/repo:tag", nil)
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// Repository should have auth client configured
	assert.NotNil(t, repo.Client)

	// Test that we can create repositories for different references
	repo2, err := NewRepository(ctx, "docker.io/library/alpine:latest", nil)
	require.NoError(t, err)
	assert.NotNil(t, repo2)
}

// TestNewRepository_StaticAuth tests static authentication for specific registry
func TestNewRepository_StaticAuth(t *testing.T) {
	ctx := context.Background()

	authOpts := &AuthOptions{
		StaticRegistry: "ghcr.io",
		StaticUsername: "testuser",
		StaticPassword: "testpass",
	}

	repo, err := NewRepository(ctx, "ghcr.io/test/repo:tag", authOpts)
	require.NoError(t, err)
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.Client)

	// Test credential function is set (cast to auth client)
	authClient, ok := repo.Client.(*auth.Client)
	require.True(t, ok, "Client should be an auth.Client")
	cred, err := authClient.Credential(ctx, "ghcr.io")
	require.NoError(t, err)
	assert.Equal(t, "testuser", cred.Username)
	assert.Equal(t, "testpass", cred.Password)
}

// TestNewRepository_StaticAuth_Fallback tests that static auth only applies to specified registry
func TestNewRepository_StaticAuth_Fallback(t *testing.T) {
	ctx := context.Background()

	authOpts := &AuthOptions{
		StaticRegistry: "ghcr.io",
		StaticUsername: "testuser",
		StaticPassword: "testpass",
	}

	repo, err := NewRepository(ctx, "docker.io/library/alpine:latest", authOpts)
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// For non-matching registry, should return empty credentials (fallback to default chain)
	authClient, ok := repo.Client.(*auth.Client)
	require.True(t, ok, "Client should be an auth.Client")
	cred, err := authClient.Credential(ctx, "docker.io")
	require.NoError(t, err)
	assert.Equal(t, "", cred.Username)
	assert.Equal(t, "", cred.Password)
}

// TestNewRepository_CustomCredentialFunc tests custom credential function
func TestNewRepository_CustomCredentialFunc(t *testing.T) {
	ctx := context.Background()

	// Clear any cached credentials to ensure test isolation
	ClearAuthCache()

	customCredFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		if registry == "ghcr.io" {
			return auth.Credential{Username: "customuser", Password: "custompass"}, nil
		}
		return auth.Credential{}, nil
	}

	authOpts := &AuthOptions{
		CredentialFunc: customCredFunc,
	}

	repo, err := NewRepository(ctx, "ghcr.io/test/repo:tag", authOpts)
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// Test custom credential function
	authClient, ok := repo.Client.(*auth.Client)
	require.True(t, ok, "Client should be an auth.Client")

	// Clear cache again to ensure we get fresh credentials from the function
	ClearAuthCache()

	cred, err := authClient.Credential(ctx, "ghcr.io")
	require.NoError(t, err)
	assert.Equal(t, "customuser", cred.Username)
	assert.Equal(t, "custompass", cred.Password)

	// Test fallback for other registries
	cred, err = authClient.Credential(ctx, "docker.io")
	require.NoError(t, err)
	assert.Equal(t, "", cred.Username)
	assert.Equal(t, "", cred.Password)
}

// TestNewRepository_InvalidReference tests error handling for invalid references
func TestNewRepository_InvalidReference(t *testing.T) {
	ctx := context.Background()

	// Test with invalid reference
	_, err := NewRepository(ctx, "invalid-reference", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create repository")
}

// TestAuthOptionsStruct tests the AuthOptions struct
func TestAuthOptionsStruct(t *testing.T) {
	opts := &AuthOptions{
		StaticRegistry: "registry.example.com",
		StaticUsername: "user",
		StaticPassword: "pass",
		CredentialFunc: func(ctx context.Context, reg string) (auth.Credential, error) {
			return auth.Credential{}, nil
		},
	}

	assert.Equal(t, "registry.example.com", opts.StaticRegistry)
	assert.Equal(t, "user", opts.StaticUsername)
	assert.Equal(t, "pass", opts.StaticPassword)
	assert.NotNil(t, opts.CredentialFunc)
}

// TestAuthConfigStruct tests the AuthConfig struct (for compatibility)
func TestAuthConfigStruct(t *testing.T) {
	config := AuthConfig{
		Username: "testuser",
		Password: "testpass",
	}

	assert.Equal(t, "testuser", config.Username)
	assert.Equal(t, "testpass", config.Password)
}

// TestPushOperation tests the push operation wrapper
func TestPushOperation(t *testing.T) {
	ctx := context.Background()

	// Test nil descriptor error
	t.Run("nil descriptor", func(t *testing.T) {
		err := Push(ctx, "ghcr.io/test/repo:tag", nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "descriptor cannot be nil")
	})

	// Test with valid descriptor but non-existent registry
	t.Run("invalid reference", func(t *testing.T) {
		desc := &PushDescriptor{
			MediaType: "application/octet-stream",
			Data:      strings.NewReader("test data"),
			Size:      9,
		}
		err := Push(ctx, "invalid-reference", desc, nil)
		assert.Error(t, err)
		// Should be mapped to our error format
		assert.Contains(t, err.Error(), "push invalid-reference:")
	})
}

// TestPullOperation tests the pull operation wrapper
func TestPullOperation(t *testing.T) {
	ctx := context.Background()

	// Test with invalid reference
	t.Run("invalid reference", func(t *testing.T) {
		_, err := Pull(ctx, "invalid-reference", nil)
		assert.Error(t, err)
		// Should be mapped to our error format
		assert.Contains(t, err.Error(), "pull invalid-reference:")
	})

	// Test with non-existent reference (will try to connect to registry)
	t.Run("non-existent reference", func(t *testing.T) {
		_, err := Pull(ctx, "ghcr.io/test/nonexistent:tag", nil)
		assert.Error(t, err)
		// Should be mapped to our error format or contain registry error
		assert.True(t, strings.Contains(err.Error(), "pull") ||
			strings.Contains(err.Error(), "authentication") ||
			strings.Contains(err.Error(), "registry"))
	})
}

// TestErrorMapping tests the error mapping functionality
func TestErrorMapping(t *testing.T) {
	ctx := context.Background()

	// Test authentication error mapping
	t.Run("authentication error", func(t *testing.T) {
		// Try to access a private repository without auth
		_, err := Pull(ctx, "ghcr.io/test/private-repo:latest", nil)
		assert.Error(t, err)
		// Should contain our mapped error message or ORAS error
		assert.True(t, strings.Contains(err.Error(), "pull") ||
			strings.Contains(err.Error(), "denied") ||
			strings.Contains(err.Error(), "authentication"))
	})
}
