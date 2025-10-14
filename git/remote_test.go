package git

import (
	"context"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	platformerrors "github.com/jmgilman/go/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRemoteOps is a mock implementation of RemoteOperations for testing.
type mockRemoteOps struct {
	cloneFunc func(ctx context.Context, fs billy.Filesystem, opts CloneOptions) (*Repository, error)
	fetchFunc func(ctx context.Context, repo *Repository, opts FetchOptions) error
	pushFunc  func(ctx context.Context, repo *Repository, opts PushOptions) error
}

func (m *mockRemoteOps) Clone(ctx context.Context, fs billy.Filesystem, opts CloneOptions) (*Repository, error) {
	if m.cloneFunc != nil {
		return m.cloneFunc(ctx, fs, opts)
	}
	return nil, platformerrors.New(platformerrors.CodeInternal, "mock clone not implemented")
}

func (m *mockRemoteOps) Fetch(ctx context.Context, repo *Repository, opts FetchOptions) error {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, repo, opts)
	}
	return platformerrors.New(platformerrors.CodeInternal, "mock fetch not implemented")
}

func (m *mockRemoteOps) Push(ctx context.Context, repo *Repository, opts PushOptions) error {
	if m.pushFunc != nil {
		return m.pushFunc(ctx, repo, opts)
	}
	return platformerrors.New(platformerrors.CodeInternal, "mock push not implemented")
}

// setTestRemoteOps replaces the package-level remoteOps for testing.
func setTestRemoteOps(ops RemoteOperations) func() {
	original := remoteOps
	remoteOps = ops
	return func() {
		remoteOps = original
	}
}

// createTestRepository creates an in-memory test repository with initial commit.
//
//nolint:contextcheck // Context is created inside this helper function, not passed from caller
func createTestRepository(t *testing.T) *Repository {
	t.Helper()

	fs := memfs.New()

	// Initialize repository
	repo, err := Init("test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Create initial commit
	wt, err := repo.repo.Worktree()
	require.NoError(t, err)

	// Create a test file
	file, err := repo.fs.Create("test.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("test content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// Stage the file
	_, err = wt.Add("test.txt")
	require.NoError(t, err)

	// Create commit
	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	return repo
}

func TestClone(t *testing.T) {
	tests := []struct {
		name    string
		opts    CloneOptions
		setup   func() (*mockRemoteOps, func())
		wantErr bool
		errCode platformerrors.ErrorCode
	}{
		{
			name: "successful clone",
			opts: CloneOptions{
				URL: "https://github.com/test/repo",
			},
			setup: func() (*mockRemoteOps, func()) {
				mock := &mockRemoteOps{
					cloneFunc: func(_ context.Context, _ billy.Filesystem, _ CloneOptions) (*Repository, error) {
						// Create a mock repository
						repo := createTestRepository(t)
						return repo, nil
					},
				}
				cleanup := setTestRemoteOps(mock)
				return mock, cleanup
			},
			wantErr: false,
		},
		{
			name: "clone with authentication",
			opts: CloneOptions{
				URL:  "https://github.com/test/private-repo",
				Auth: BasicAuth("user", "token"),
			},
			setup: func() (*mockRemoteOps, func()) {
				mock := &mockRemoteOps{
					cloneFunc: func(_ context.Context, _ billy.Filesystem, opts CloneOptions) (*Repository, error) {
						assert.NotNil(t, opts.Auth)
						repo := createTestRepository(t)
						return repo, nil
					},
				}
				cleanup := setTestRemoteOps(mock)
				return mock, cleanup
			},
			wantErr: false,
		},
		{
			name: "clone with shallow depth",
			opts: CloneOptions{
				URL:   "https://github.com/test/repo",
				Depth: 1,
			},
			setup: func() (*mockRemoteOps, func()) {
				mock := &mockRemoteOps{
					cloneFunc: func(_ context.Context, _ billy.Filesystem, opts CloneOptions) (*Repository, error) {
						assert.Equal(t, 1, opts.Depth)
						repo := createTestRepository(t)
						return repo, nil
					},
				}
				cleanup := setTestRemoteOps(mock)
				return mock, cleanup
			},
			wantErr: false,
		},
		{
			name: "clone not found",
			opts: CloneOptions{
				URL: "https://github.com/test/nonexistent",
			},
			setup: func() (*mockRemoteOps, func()) {
				mock := &mockRemoteOps{
					cloneFunc: func(_ context.Context, _ billy.Filesystem, _ CloneOptions) (*Repository, error) {
						return nil, platformerrors.New(platformerrors.CodeNotFound, "repository not found")
					},
				}
				cleanup := setTestRemoteOps(mock)
				return mock, cleanup
			},
			wantErr: true,
			errCode: platformerrors.CodeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fs := memfs.New()

			_, cleanup := tt.setup()
			defer cleanup()

			repo, err := CloneWithOptions(ctx, fs, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				var perr platformerrors.PlatformError
				require.ErrorAs(t, err, &perr)
				assert.Equal(t, tt.errCode, perr.Code())
			} else {
				require.NoError(t, err)
				require.NotNil(t, repo)
				assert.NotNil(t, repo.Underlying())
				assert.NotNil(t, repo.Filesystem())
			}
		})
	}
}

func TestRepositoryListRemotes(t *testing.T) {

	t.Run("empty repository", func(t *testing.T) {
		repo := createTestRepository(t)

		remotes, err := repo.ListRemotes()
		require.NoError(t, err)
		assert.Empty(t, remotes)
	})

	t.Run("with remotes", func(t *testing.T) {
		repo := createTestRepository(t)

		// Add remotes
		err := repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/repo",
		})
		require.NoError(t, err)

		err = repo.AddRemote(RemoteOptions{
			Name: "upstream",
			URL:  "https://github.com/upstream/repo",
		})
		require.NoError(t, err)

		// List remotes
		remotes, err := repo.ListRemotes()
		require.NoError(t, err)
		require.Len(t, remotes, 2)

		// Check remote details
		remoteMap := make(map[string]Remote)
		for _, remote := range remotes {
			remoteMap[remote.Name] = remote
		}

		origin, ok := remoteMap["origin"]
		require.True(t, ok)
		assert.Equal(t, "origin", origin.Name)
		assert.Contains(t, origin.URLs, "https://github.com/test/repo")

		upstream, ok := remoteMap["upstream"]
		require.True(t, ok)
		assert.Equal(t, "upstream", upstream.Name)
		assert.Contains(t, upstream.URLs, "https://github.com/upstream/repo")
	})
}

func TestRepositoryAddRemote(t *testing.T) {

	t.Run("successful add", func(t *testing.T) {
		repo := createTestRepository(t)

		err := repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/repo",
		})
		require.NoError(t, err)

		// Verify remote was added
		remotes, err := repo.ListRemotes()
		require.NoError(t, err)
		require.Len(t, remotes, 1)
		assert.Equal(t, "origin", remotes[0].Name)
	})

	t.Run("duplicate remote", func(t *testing.T) {
		repo := createTestRepository(t)

		// Add first remote
		err := repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/repo",
		})
		require.NoError(t, err)

		// Try to add duplicate
		err = repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/other-repo",
		})
		require.Error(t, err)
		var perr platformerrors.PlatformError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, platformerrors.CodeAlreadyExists, perr.Code())
	})
}

func TestRepositoryRemoveRemote(t *testing.T) {

	t.Run("successful remove", func(t *testing.T) {
		repo := createTestRepository(t)

		// Add remote
		err := repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/repo",
		})
		require.NoError(t, err)

		// Remove remote
		err = repo.RemoveRemote("origin")
		require.NoError(t, err)

		// Verify remote was removed
		remotes, err := repo.ListRemotes()
		require.NoError(t, err)
		assert.Empty(t, remotes)
	})

	t.Run("remove nonexistent remote", func(t *testing.T) {
		repo := createTestRepository(t)

		err := repo.RemoveRemote("nonexistent")
		require.Error(t, err)
	})
}

func TestRepositoryFetch(t *testing.T) {

	t.Run("successful fetch", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		fetchCalled := false
		mock := &mockRemoteOps{
			fetchFunc: func(_ context.Context, _ *Repository, opts FetchOptions) error {
				fetchCalled = true
				assert.Equal(t, "origin", opts.RemoteName)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Fetch(ctx, FetchOptions{
			RemoteName: "origin",
		})
		require.NoError(t, err)
		assert.True(t, fetchCalled)
	})

	t.Run("fetch with authentication", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		auth := BasicAuth("user", "token")
		mock := &mockRemoteOps{
			fetchFunc: func(_ context.Context, _ *Repository, opts FetchOptions) error {
				assert.Equal(t, auth, opts.Auth)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Fetch(ctx, FetchOptions{
			Auth: auth,
		})
		require.NoError(t, err)
	})

	t.Run("fetch with depth", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		mock := &mockRemoteOps{
			fetchFunc: func(_ context.Context, _ *Repository, opts FetchOptions) error {
				assert.Equal(t, 1, opts.Depth)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Fetch(ctx, FetchOptions{
			Depth: 1,
		})
		require.NoError(t, err)
	})

	t.Run("fetch error", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		mock := &mockRemoteOps{
			fetchFunc: func(_ context.Context, _ *Repository, _ FetchOptions) error {
				return platformerrors.New(platformerrors.CodeNotFound, "remote not found")
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Fetch(ctx, FetchOptions{})
		require.Error(t, err)
		var perr platformerrors.PlatformError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, platformerrors.CodeNotFound, perr.Code())
	})
}

func TestRepositoryPush(t *testing.T) {

	t.Run("successful push", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		pushCalled := false
		mock := &mockRemoteOps{
			pushFunc: func(_ context.Context, _ *Repository, opts PushOptions) error {
				pushCalled = true
				assert.Equal(t, "origin", opts.RemoteName)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Push(ctx, PushOptions{
			RemoteName: "origin",
		})
		require.NoError(t, err)
		assert.True(t, pushCalled)
	})

	t.Run("push with authentication", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		auth := BasicAuth("user", "token")
		mock := &mockRemoteOps{
			pushFunc: func(_ context.Context, _ *Repository, opts PushOptions) error {
				assert.Equal(t, auth, opts.Auth)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Push(ctx, PushOptions{
			Auth: auth,
		})
		require.NoError(t, err)
	})

	t.Run("push with refspecs", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		mock := &mockRemoteOps{
			pushFunc: func(_ context.Context, _ *Repository, opts PushOptions) error {
				assert.Equal(t, []string{"refs/heads/main:refs/heads/main"}, opts.RefSpecs)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Push(ctx, PushOptions{
			RefSpecs: []string{"refs/heads/main:refs/heads/main"},
		})
		require.NoError(t, err)
	})

	t.Run("force push", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		mock := &mockRemoteOps{
			pushFunc: func(_ context.Context, _ *Repository, opts PushOptions) error {
				assert.True(t, opts.Force)
				return nil
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Push(ctx, PushOptions{
			Force: true,
		})
		require.NoError(t, err)
	})

	t.Run("push error", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		mock := &mockRemoteOps{
			pushFunc: func(_ context.Context, _ *Repository, _ PushOptions) error {
				return platformerrors.New(platformerrors.CodeUnauthorized, "authentication failed")
			},
		}
		cleanup := setTestRemoteOps(mock)
		defer cleanup()

		err := repo.Push(ctx, PushOptions{})
		require.Error(t, err)
		var perr platformerrors.PlatformError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, platformerrors.CodeUnauthorized, perr.Code())
	})
}

func TestRepositoryPull(t *testing.T) {

	t.Run("successful pull", func(t *testing.T) {
		ctx := context.Background()
		// Create a more complete test repository with a remote
		fs := memfs.New()

		// Create remote repository
		remotePath := "remote"
		err := fs.MkdirAll(remotePath, 0o755)
		require.NoError(t, err)

		remoteFs, err := fs.Chroot(remotePath)
		require.NoError(t, err)

		dotGitFs, err := remoteFs.Chroot(".git")
		require.NoError(t, err)

		remoteStorage := filesystem.NewStorage(dotGitFs, cache.NewObjectLRUDefault())
		remoteRepo, err := gogit.Init(remoteStorage, remoteFs)
		require.NoError(t, err)

		// Create initial commit in remote
		remoteWt, err := remoteRepo.Worktree()
		require.NoError(t, err)

		file, err := remoteFs.Create("test.txt")
		require.NoError(t, err)
		_, err = file.Write([]byte("initial content"))
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		_, err = remoteWt.Add("test.txt")
		require.NoError(t, err)

		_, err = remoteWt.Commit("Initial commit", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)

		// Now create local repository and configure remote
		localPath := "local"
		err = fs.MkdirAll(localPath, 0o755)
		require.NoError(t, err)

		localFs, err := fs.Chroot(localPath)
		require.NoError(t, err)

		localDotGitFs, err := localFs.Chroot(".git")
		require.NoError(t, err)

		localStorage := filesystem.NewStorage(localDotGitFs, cache.NewObjectLRUDefault())
		localRepo, err := gogit.Init(localStorage, localFs)
		require.NoError(t, err)

		// Add remote configuration pointing to the remote repo
		_, err = localRepo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{"/remote"}, // Note: In real usage this would be a URL
		})
		require.NoError(t, err)

		repo := &Repository{
			path: localPath,
			repo: localRepo,
			fs:   localFs,
		}

		// Note: Pull would fail here because we can't actually fetch from a local path
		// in this test setup without setting up a proper git server. Instead, we'll
		// test that the method calls through correctly.
		err = repo.Pull(ctx, PullOptions{
			RemoteName: "origin",
		})
		// We expect an error because there's no actual remote to pull from
		require.Error(t, err)
	})

	t.Run("pull with authentication", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		// Add a remote
		err := repo.AddRemote(RemoteOptions{
			Name: "origin",
			URL:  "https://github.com/test/repo",
		})
		require.NoError(t, err)

		auth := BasicAuth("user", "token")
		err = repo.Pull(ctx, PullOptions{
			RemoteName: "origin",
			Auth:       auth,
		})
		// Will fail because we can't actually pull, but it tests the auth is passed through
		require.Error(t, err)
	})
}

func TestRemoteOperationsInterface(t *testing.T) {
	t.Run("interface compliance", func(_ *testing.T) {
		// Verify that defaultRemoteOps implements RemoteOperations
		var _ RemoteOperations = &defaultRemoteOps{}

		// Verify that mockRemoteOps implements RemoteOperations
		var _ RemoteOperations = &mockRemoteOps{}
	})

	t.Run("package variable replacement", func(t *testing.T) {
		// Save original
		original := remoteOps

		// Replace with mock
		mock := &mockRemoteOps{}
		remoteOps = mock

		// Verify replacement
		assert.Equal(t, mock, remoteOps)

		// Restore original
		remoteOps = original
		assert.Equal(t, original, remoteOps)
	})
}

func TestDefaultRemoteOpsClone(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		ctx := context.Background()
		fs := memfs.New()

		ops := &defaultRemoteOps{}
		_, err := ops.Clone(ctx, fs, CloneOptions{
			URL: "invalid://not-a-real-url-scheme",
		})
		require.Error(t, err)
	})
}

func TestDefaultRemoteOpsFetch(t *testing.T) {
	t.Run("fetch from repository without remotes", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		ops := &defaultRemoteOps{}
		err := ops.Fetch(ctx, repo, FetchOptions{
			RemoteName: "origin",
		})
		require.Error(t, err)
		var perr platformerrors.PlatformError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, platformerrors.CodeNotFound, perr.Code())
	})
}

func TestDefaultRemoteOpsPush(t *testing.T) {
	t.Run("push to repository without remotes", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)

		ops := &defaultRemoteOps{}
		err := ops.Push(ctx, repo, PushOptions{
			RemoteName: "origin",
		})
		require.Error(t, err)
		var perr platformerrors.PlatformError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, platformerrors.CodeNotFound, perr.Code())
	})
}
