package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v67/github"
	"github.com/jmgilman/go/errors"
	gh "github.com/jmgilman/go/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSDKProvider(t *testing.T) {
	t.Parallel()

	t.Run("with token", func(t *testing.T) {
		t.Parallel()

		provider, err := NewSDKProvider(WithToken("test-token"))

		require.NoError(t, err)
		assert.NotNil(t, provider)
		// Test behavior: provider should be usable
		// Note: We can't easily test GetRepository without a real server,
		// but the fact that it was created successfully indicates correct behavior
	})

	t.Run("with client", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/repos/testowner/testrepo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 123, "name": "testrepo", "full_name": "testowner/testrepo", "owner": {"login": "testowner"}}`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		repo, err := provider.GetRepository(ctx, "testowner", "testrepo")

		require.NoError(t, err)
		assert.Equal(t, "testrepo", repo.Name)
	})

	tests := []struct {
		name      string
		setupOpts []Option
		wantCode  errors.ErrorCode
	}{
		{
			name:      "with empty token returns error",
			setupOpts: []Option{WithToken("")},
			wantCode:  errors.CodeInvalidInput,
		},
		{
			name:      "with nil client returns error",
			setupOpts: []Option{WithClient(nil)},
			wantCode:  errors.CodeInvalidInput,
		},
		{
			name:      "without token or client returns error",
			setupOpts: []Option{},
			wantCode:  errors.CodeInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewSDKProvider(tt.setupOpts...)

			require.Error(t, err)

			var platformErr errors.PlatformError
			require.True(t, errors.As(err, &platformErr))
			assert.Equal(t, tt.wantCode, platformErr.Code())
		})
	}
}

func TestSDKProvider_GetRepository(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/repos/testowner/testrepo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 12345,
				"name": "testrepo",
				"full_name": "testowner/testrepo",
				"description": "Test repository",
				"owner": {"login": "testowner"},
				"default_branch": "main",
				"private": false,
				"fork": false,
				"archived": false,
				"clone_url": "https://github.com/testowner/testrepo.git",
				"ssh_url": "git@github.com:testowner/testrepo.git",
				"html_url": "https://github.com/testowner/testrepo",
				"created_at": "2020-01-01T00:00:00Z",
				"updated_at": "2020-01-02T00:00:00Z"
			}`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		repo, err := provider.GetRepository(ctx, "testowner", "testrepo")

		require.NoError(t, err)
		assert.Equal(t, int64(12345), repo.ID)
		assert.Equal(t, "testrepo", repo.Name)
		assert.Equal(t, "testowner/testrepo", repo.FullName)
		assert.Equal(t, "Test repository", repo.Description)
		assert.Equal(t, "testowner", repo.Owner)
		assert.Equal(t, "main", repo.DefaultBranch)
		assert.False(t, repo.Private)
		assert.False(t, repo.Fork)
		assert.False(t, repo.Archived)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/repos/testowner/notfound", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "Not Found"}`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		_, err = provider.GetRepository(ctx, "testowner", "notfound")

		require.Error(t, err)

		var platformErr errors.PlatformError
		require.True(t, errors.As(err, &platformErr))
		assert.Equal(t, errors.CodeNotFound, platformErr.Code())
	})
}

func TestSDKProvider_ListRepositories(t *testing.T) {
	t.Parallel()

	t.Run("organization repositories", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/orgs/testorg/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"name": "repo1",
					"full_name": "testorg/repo1",
					"owner": {"login": "testorg"}
				},
				{
					"id": 2,
					"name": "repo2",
					"full_name": "testorg/repo2",
					"owner": {"login": "testorg"}
				}
			]`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		repos, err := provider.ListRepositories(ctx, "testorg", gh.ListOptions{})

		require.NoError(t, err)
		assert.Len(t, repos, 2)
		assert.Equal(t, "repo1", repos[0].Name)
		assert.Equal(t, "repo2", repos[1].Name)
	})

	t.Run("user repositories fallback", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/orgs/testuser/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "Not Found"}`))
		})

		mux.HandleFunc("/users/testuser/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"name": "user-repo",
					"full_name": "testuser/user-repo",
					"owner": {"login": "testuser"}
				}
			]`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		repos, err := provider.ListRepositories(ctx, "testuser", gh.ListOptions{})

		require.NoError(t, err)
		assert.Len(t, repos, 1)
		assert.Equal(t, "user-repo", repos[0].Name)
	})
}

func TestSDKProvider_CreateRepository(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		t.Cleanup(func() { server.Close() })

		mux.HandleFunc("/orgs/testorg/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{
				"id": 999,
				"name": "newrepo",
				"full_name": "testorg/newrepo",
				"description": "A new repository",
				"owner": {"login": "testorg"},
				"private": true
			}`))
		})

		client := github.NewClient(nil)
		baseURL, err := client.BaseURL.Parse(server.URL + "/")
		require.NoError(t, err)
		client.BaseURL = baseURL

		provider, err := NewSDKProvider(WithClient(client))
		require.NoError(t, err)

		ctx := context.Background()
		repo, err := provider.CreateRepository(ctx, "testorg", gh.CreateRepositoryOptions{
			Name:        "newrepo",
			Description: "A new repository",
			Private:     true,
		})

		require.NoError(t, err)
		assert.Equal(t, int64(999), repo.ID)
		assert.Equal(t, "newrepo", repo.Name)
		assert.Equal(t, "testorg/newrepo", repo.FullName)
		assert.True(t, repo.Private)
	})
}
