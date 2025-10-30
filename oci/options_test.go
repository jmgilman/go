package ocibundle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/jmgilman/go/oci/internal/oras"
)

// TestClientOptionsStruct tests the ClientOptions struct
func TestClientOptionsStruct(t *testing.T) {
	opts := &ClientOptions{
		Auth: &oras.AuthOptions{
			StaticRegistry: "ghcr.io",
			StaticUsername: "user",
			StaticPassword: "pass",
		},
	}

	assert.NotNil(t, opts.Auth)
	assert.Equal(t, "ghcr.io", opts.Auth.StaticRegistry)
	assert.Equal(t, "user", opts.Auth.StaticUsername)
	assert.Equal(t, "pass", opts.Auth.StaticPassword)
}

// TestWithAuthNone tests the WithAuthNone option
func TestWithAuthNone(t *testing.T) {
	opts := DefaultClientOptions()
	opt := WithAuthNone()
	opt(opts)

	assert.Nil(t, opts.Auth)
}

// TestWithStaticAuth_Options tests the WithStaticAuth option
func TestWithStaticAuth_Options(t *testing.T) {
	opts := DefaultClientOptions()
	opt := WithStaticAuth("ghcr.io", "testuser", "testpass")
	opt(opts)

	assert.NotNil(t, opts.Auth)
	assert.Equal(t, "ghcr.io", opts.Auth.StaticRegistry)
	assert.Equal(t, "testuser", opts.Auth.StaticUsername)
	assert.Equal(t, "testpass", opts.Auth.StaticPassword)
}

// TestWithStaticAuth_NilAuth tests WithStaticAuth when Auth is initially nil
func TestWithStaticAuth_NilAuth(t *testing.T) {
	opts := &ClientOptions{Auth: nil}
	opt := WithStaticAuth("registry.example.com", "user", "pass")
	opt(opts)

	assert.NotNil(t, opts.Auth)
	assert.Equal(t, "registry.example.com", opts.Auth.StaticRegistry)
	assert.Equal(t, "user", opts.Auth.StaticUsername)
	assert.Equal(t, "pass", opts.Auth.StaticPassword)
}

// TestWithCredentialFunc_Options tests the WithCredentialFunc option
func TestWithCredentialFunc_Options(t *testing.T) {
	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		return auth.Credential{Username: "custom", Password: "pass"}, nil
	}

	opts := DefaultClientOptions()
	opt := WithCredentialFunc(customFunc)
	opt(opts)

	assert.NotNil(t, opts.Auth)
	assert.NotNil(t, opts.Auth.CredentialFunc)
}

// TestWithCredentialFunc_NilAuth tests WithCredentialFunc when Auth is initially nil
func TestWithCredentialFunc_NilAuth(t *testing.T) {
	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		return auth.Credential{Username: "custom", Password: "pass"}, nil
	}

	opts := &ClientOptions{Auth: nil}
	opt := WithCredentialFunc(customFunc)
	opt(opts)

	assert.NotNil(t, opts.Auth)
	assert.NotNil(t, opts.Auth.CredentialFunc)
}

// TestOptionsCombination tests combining multiple options
func TestOptionsCombination(t *testing.T) {
	customFunc := func(ctx context.Context, registry string) (auth.Credential, error) {
		return auth.Credential{Username: "custom", Password: "pass"}, nil
	}

	opts := DefaultClientOptions()

	// Apply static auth first
	opt1 := WithStaticAuth("ghcr.io", "staticuser", "staticpass")
	opt1(opts)

	// Then apply credential func (should override static auth behavior)
	opt2 := WithCredentialFunc(customFunc)
	opt2(opts)

	assert.NotNil(t, opts.Auth)
	assert.NotNil(t, opts.Auth.CredentialFunc)
	// Static auth fields should still be set but will be overridden by CredentialFunc
	assert.Equal(t, "ghcr.io", opts.Auth.StaticRegistry)
	assert.Equal(t, "staticuser", opts.Auth.StaticUsername)
	assert.Equal(t, "staticpass", opts.Auth.StaticPassword)
}

// TestDefaultClientOptions_Options tests the DefaultClientOptions function
func TestDefaultClientOptions_Options(t *testing.T) {
	opts := DefaultClientOptions()
	assert.NotNil(t, opts)
	assert.Nil(t, opts.Auth)
}

// TestNewWithOptions_NoOptions tests NewWithOptions with no options
func TestNewWithOptions_NoOptions(t *testing.T) {
	client, err := NewWithOptions()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.options)
	assert.Nil(t, client.options.Auth)
}

// TestNewWithOptions_WithOptions tests NewWithOptions with options
func TestNewWithOptions_WithOptions(t *testing.T) {
	client, err := NewWithOptions(
		WithStaticAuth("ghcr.io", "user", "pass"),
		WithAuthNone(), // This should override the static auth
	)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Nil(t, client.options.Auth) // WithAuthNone should set to nil
}
