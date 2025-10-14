package git

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// SSHKeyOption configures SSH key authentication.
type SSHKeyOption func(*sshKeyOptions)

type sshKeyOptions struct {
	password string
}

// WithSSHPassword sets the password for encrypted SSH keys.
func WithSSHPassword(password string) SSHKeyOption {
	return func(opts *sshKeyOptions) {
		opts.password = password
	}
}

// SSHKeyAuth creates SSH authentication from PEM-encoded key bytes.
// It handles both password-protected and unprotected private keys.
//
// Parameters:
//   - user: SSH username (typically "git" for Git hosting services)
//   - pemBytes: PEM-encoded private key bytes
//   - opts: optional configuration (use WithSSHPassword for encrypted keys)
//
// Returns an Auth object that can be used with Clone, Fetch, Push operations.
// Errors are wrapped with context for easier debugging.
//
// Example (unencrypted key):
//
//	keyBytes, _ := os.ReadFile("~/.ssh/id_rsa")
//	auth, err := git.SSHKeyAuth("git", keyBytes)
//	if err != nil {
//	    return err
//	}
//
// Example (encrypted key):
//
//	auth, err := git.SSHKeyAuth("git", keyBytes, git.WithSSHPassword("mypassphrase"))
func SSHKeyAuth(user string, pemBytes []byte, opts ...SSHKeyOption) (Auth, error) {
	// Apply options
	options := &sshKeyOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Use go-git's ssh.NewPublicKeys to parse and validate the key
	publicKeys, err := ssh.NewPublicKeys(user, pemBytes, options.password)
	if err != nil {
		return nil, wrapError(err, "failed to parse SSH key")
	}

	return publicKeys, nil
}

// SSHKeyFile creates SSH authentication by reading a key from a file.
// This is a convenience wrapper around SSHKeyAuth that handles file I/O.
//
// Parameters:
//   - user: SSH username (typically "git" for Git hosting services)
//   - keyPath: path to PEM-encoded private key file
//   - opts: optional configuration (use WithSSHPassword for encrypted keys)
//
// Returns an Auth object that can be used with Clone, Fetch, Push operations.
// File read errors and key parsing errors are wrapped with context.
//
// Example (unencrypted key):
//
//	auth, err := git.SSHKeyFile("git", "~/.ssh/id_rsa")
//	if err != nil {
//	    return err
//	}
//
// Example (encrypted key):
//
//	auth, err := git.SSHKeyFile("git", "~/.ssh/id_rsa", git.WithSSHPassword("mypassphrase"))
func SSHKeyFile(user string, keyPath string, opts ...SSHKeyOption) (Auth, error) {
	// Read the key file
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key file %q: %w", keyPath, err)
	}

	// Delegate to SSHKeyAuth for parsing (pass options through)
	return SSHKeyAuth(user, pemBytes, opts...)
}

// BasicAuth creates HTTP basic authentication.
// This is commonly used with personal access tokens for HTTPS Git operations.
//
// Parameters:
//   - username: username or token name
//   - password: password or personal access token
//
// Returns an Auth object that can be used with Clone, Fetch, Push operations.
//
// Example:
//
//	auth := git.BasicAuth("myuser", "ghp_mytoken")
func BasicAuth(username, password string) Auth {
	return &http.BasicAuth{
		Username: username,
		Password: password,
	}
}

// EmptyAuth returns nil authentication for public repositories.
// go-git interprets nil Auth as "no authentication needed".
//
// This function exists for explicit documentation purposes - you can
// also just pass nil as the Auth field in options structs.
//
// Example:
//
//	repo, err := git.Clone(ctx, fs, git.CloneOptions{
//	    URL:  "https://github.com/public/repo",
//	    Auth: git.EmptyAuth(),
//	})
func EmptyAuth() Auth {
	return nil
}

// Ensure our Auth interface is satisfied by go-git's transport.AuthMethod.
// This is a compile-time check.
var _ Auth = (transport.AuthMethod)(nil)
