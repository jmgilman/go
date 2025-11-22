//nolint:contextcheck // Context is properly passed via CommandWrapper.WithContext() but linter cannot verify
package cli

import (
	"context"
	"io"
	"testing"

	"github.com/jmgilman/go/errors"
	"github.com/jmgilman/go/exec"
	"github.com/jmgilman/go/exec/mocks"
	github "github.com/jmgilman/go/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockExecutor creates a mock executor with proper method chaining for testing.
func setupMockExecutor(t *testing.T, runFunc func(args ...string) (*exec.Result, error)) *mocks.ExecutorMock {
	t.Helper()

	var mockExec *mocks.ExecutorMock
	mockExec = &mocks.ExecutorMock{
		WithEnvFunc: func(env map[string]string) exec.Executor {
			return mockExec
		},
		WithDirFunc: func(dir string) exec.Executor {
			return mockExec
		},
		WithContextFunc: func(ctx context.Context) exec.Executor {
			return mockExec
		},
		WithDisableColorsFunc: func() exec.Executor {
			return mockExec
		},
		WithTimeoutFunc: func(timeout string) exec.Executor {
			return mockExec
		},
		WithInheritEnvFunc: func() exec.Executor {
			return mockExec
		},
		WithStdoutFunc: func(w io.Writer) exec.Executor {
			return mockExec
		},
		WithStderrFunc: func(w io.Writer) exec.Executor {
			return mockExec
		},
		WithPassthroughFunc: func() exec.Executor {
			return mockExec
		},
		CloneFunc: func() exec.Executor {
			return mockExec
		},
		RunFunc: runFunc,
	}

	return mockExec
}

func TestNewCLIProvider(t *testing.T) {
	t.Run("success with custom executor", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			// Should call: gh auth status
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{
					Stdout:   "Logged in to github.com",
					Stderr:   "",
					ExitCode: 0,
				}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))

		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("fails when auth status check fails", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{
					Stdout:   "",
					Stderr:   "You are not logged into any GitHub hosts",
					ExitCode: 1,
				}, errors.New(errors.CodeExecutionFailed, "exit status 1")
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Equal(t, errors.CodeUnauthorized, errors.GetCode(err))
	})

	t.Run("fails with nil executor option", func(t *testing.T) {

		provider, err := NewCLIProvider(WithExecutor(nil))

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Equal(t, errors.CodeInvalidInput, errors.GetCode(err))
	})
}

func TestCLIProvider_GetRepository(t *testing.T) {
	tests := []struct {
		name      string
		owner     string
		repo      string
		runFunc   func(args ...string) (*exec.Result, error)
		wantData  *github.RepositoryData
		wantError bool
		errorCode errors.ErrorCode
	}{
		{
			name:  "success",
			owner: "testorg",
			repo:  "testrepo",
			runFunc: func(args ...string) (*exec.Result, error) {
				// Auth status check
				if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
					return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
				}
				// Get repository
				if len(args) >= 3 && args[0] == "gh" && args[1] == "api" {
					return &exec.Result{
						Stdout: `{
							"id": 12345,
							"name": "testrepo",
							"full_name": "testorg/testrepo",
							"description": "Test repository",
							"default_branch": "main",
							"private": false,
							"fork": false,
							"archived": false,
							"clone_url": "https://github.com/testorg/testrepo.git",
							"ssh_url": "git@github.com:testorg/testrepo.git",
							"html_url": "https://github.com/testorg/testrepo",
							"created_at": "2023-01-01T00:00:00Z",
							"updated_at": "2023-01-02T00:00:00Z",
							"owner": {"login": "testorg"}
						}`,
						ExitCode: 0,
					}, nil
				}
				return &exec.Result{}, nil
			},
			wantData: &github.RepositoryData{
				ID:            12345,
				Owner:         "testorg",
				Name:          "testrepo",
				FullName:      "testorg/testrepo",
				Description:   "Test repository",
				DefaultBranch: "main",
				Private:       false,
				Fork:          false,
				Archived:      false,
				CloneURL:      "https://github.com/testorg/testrepo.git",
				SSHURL:        "git@github.com:testorg/testrepo.git",
				HTMLURL:       "https://github.com/testorg/testrepo",
			},
			wantError: false,
		},
		{
			name:  "repository not found",
			owner: "testorg",
			repo:  "notfound",
			runFunc: func(args ...string) (*exec.Result, error) {
				if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
					return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
				}
				if len(args) >= 3 && args[0] == "gh" && args[1] == "api" {
					return &exec.Result{
						Stdout:   "",
						Stderr:   "Not Found",
						ExitCode: 4,
					}, errors.New(errors.CodeExecutionFailed, "exit status 4")
				}
				return &exec.Result{}, nil
			},
			wantData:  nil,
			wantError: true,
			errorCode: errors.CodeNotFound,
		},
		{
			name:  "unauthorized",
			owner: "testorg",
			repo:  "private",
			runFunc: func(args ...string) (*exec.Result, error) {
				if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
					return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
				}
				if len(args) >= 3 && args[0] == "gh" && args[1] == "api" {
					return &exec.Result{
						Stdout:   "",
						Stderr:   "Unauthorized",
						ExitCode: 2,
					}, errors.New(errors.CodeExecutionFailed, "exit status 2")
				}
				return &exec.Result{}, nil
			},
			wantData:  nil,
			wantError: true,
			errorCode: errors.CodeUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mock := setupMockExecutor(t, tt.runFunc)
			provider, err := NewCLIProvider(WithExecutor(mock))
			require.NoError(t, err)

			data, err := provider.GetRepository(context.Background(), tt.owner, tt.repo)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorCode != "" {
					assert.Equal(t, tt.errorCode, errors.GetCode(err))
				}
				assert.Nil(t, data)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, data)
				assert.Equal(t, tt.wantData.ID, data.ID)
				assert.Equal(t, tt.wantData.Owner, data.Owner)
				assert.Equal(t, tt.wantData.Name, data.Name)
				assert.Equal(t, tt.wantData.FullName, data.FullName)
				assert.Equal(t, tt.wantData.Description, data.Description)
				assert.Equal(t, tt.wantData.DefaultBranch, data.DefaultBranch)
			}
		})
	}
}

func TestCLIProvider_GetIssue(t *testing.T) {
	t.Run("success", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
			}
			if len(args) >= 2 && args[0] == "gh" && args[1] == "issue" {
				return &exec.Result{
					Stdout: `{
						"number": 42,
						"title": "Test Issue",
						"body": "Issue body",
						"state": "OPEN",
						"author": {"login": "testuser"},
						"labels": [{"name": "bug"}, {"name": "urgent"}],
						"assignees": [{"login": "dev1"}],
						"milestone": {"title": "v1.0"},
						"createdAt": "2023-01-01T00:00:00Z",
						"updatedAt": "2023-01-02T00:00:00Z",
						"closedAt": null,
						"url": "https://github.com/testorg/testrepo/issues/42"
					}`,
					ExitCode: 0,
				}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))
		require.NoError(t, err)

		data, err := provider.GetIssue(context.Background(), "testorg", "testrepo", 42)

		require.NoError(t, err)

		assert.Equal(t, 42, data.Number)
		assert.Equal(t, "Test Issue", data.Title)
		assert.Equal(t, "Issue body", data.Body)
		assert.Equal(t, "open", data.State)
		assert.Equal(t, "testuser", data.Author)
		assert.Equal(t, []string{"bug", "urgent"}, data.Labels)
		assert.Equal(t, []string{"dev1"}, data.Assignees)
		assert.Equal(t, "v1.0", data.Milestone)
		assert.Nil(t, data.ClosedAt)
	})
}

func TestCLIProvider_ListIssues(t *testing.T) {
	t.Run("success with filters", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
			}
			if len(args) >= 2 && args[0] == "gh" && args[1] == "issue" {
				return &exec.Result{
					Stdout: `[
						{
							"number": 1,
							"title": "Issue 1",
							"body": "Body 1",
							"state": "OPEN",
							"author": {"login": "user1"},
							"labels": [{"name": "bug"}],
							"assignees": [],
							"createdAt": "2023-01-01T00:00:00Z",
							"updatedAt": "2023-01-01T00:00:00Z",
							"closedAt": null,
							"url": "https://github.com/testorg/testrepo/issues/1"
						}
					]`,
					ExitCode: 0,
				}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))
		require.NoError(t, err)

		opts := github.ListIssuesOptions{
			State: "open",
		}

		issues, err := provider.ListIssues(context.Background(), "testorg", "testrepo", opts)

		require.NoError(t, err)

		assert.Len(t, issues, 1)
		assert.Equal(t, 1, issues[0].Number)
		assert.Equal(t, "Issue 1", issues[0].Title)
	})
}

func TestCLIProvider_CloseIssue(t *testing.T) {
	t.Run("success", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
			}
			if len(args) >= 3 && args[0] == "gh" && args[1] == "issue" && args[2] == "close" {
				return &exec.Result{ExitCode: 0}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))
		require.NoError(t, err)

		err = provider.CloseIssue(context.Background(), "testorg", "testrepo", 42)

		assert.NoError(t, err)
	})
}

func TestCLIProvider_GetPullRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
			}
			if len(args) >= 2 && args[0] == "gh" && args[1] == "pr" {
				return &exec.Result{
					Stdout: `{
						"number": 10,
						"title": "Test PR",
						"body": "PR description",
						"state": "OPEN",
						"author": {"login": "developer"},
						"headRefName": "feature",
						"baseRefName": "main",
						"headRefOid": "abc123",
						"labels": [{"name": "enhancement"}],
						"isDraft": false,
						"mergeable": "MERGEABLE",
						"merged": false,
						"mergedAt": null,
						"createdAt": "2023-01-01T00:00:00Z",
						"updatedAt": "2023-01-02T00:00:00Z",
						"closedAt": null,
						"url": "https://github.com/testorg/testrepo/pull/10"
					}`,
					ExitCode: 0,
				}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))
		require.NoError(t, err)

		data, err := provider.GetPullRequest(context.Background(), "testorg", "testrepo", 10)

		require.NoError(t, err)

		assert.Equal(t, 10, data.Number)
		assert.Equal(t, "Test PR", data.Title)
		assert.Equal(t, "PR description", data.Body)
		assert.Equal(t, "open", data.State)
		assert.Equal(t, "developer", data.Author)
		assert.Equal(t, "feature", data.HeadRef)
		assert.Equal(t, "main", data.BaseRef)
		assert.False(t, data.Draft)
		assert.False(t, data.Merged)
		assert.NotNil(t, data.Mergeable)
		assert.True(t, *data.Mergeable)
	})
}

func TestCLIProvider_GetWorkflowRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {

		mock := setupMockExecutor(t, func(args ...string) (*exec.Result, error) {
			if len(args) >= 2 && args[0] == "gh" && args[1] == "auth" {
				return &exec.Result{Stdout: "Logged in", ExitCode: 0}, nil
			}
			if len(args) >= 2 && args[0] == "gh" && args[1] == "run" {
				return &exec.Result{
					Stdout: `{
						"databaseId": 123456,
						"name": "CI",
						"workflowDatabaseId": 789,
						"status": "COMPLETED",
						"conclusion": "SUCCESS",
						"headBranch": "main",
						"headSha": "abc123",
						"number": 42,
						"event": "push",
						"createdAt": "2023-01-01T00:00:00Z",
						"updatedAt": "2023-01-01T00:05:00Z",
						"url": "https://github.com/testorg/testrepo/actions/runs/123456"
					}`,
					ExitCode: 0,
				}, nil
			}
			return &exec.Result{}, nil
		})

		provider, err := NewCLIProvider(WithExecutor(mock))
		require.NoError(t, err)

		data, err := provider.GetWorkflowRun(context.Background(), "testorg", "testrepo", 123456)

		require.NoError(t, err)

		assert.Equal(t, int64(123456), data.ID)
		assert.Equal(t, "CI", data.Name)
		assert.Equal(t, "completed", data.Status)
		assert.Equal(t, "success", data.Conclusion)
		assert.Equal(t, "main", data.HeadBranch)
		assert.Equal(t, 42, data.RunNumber)
	})
}
