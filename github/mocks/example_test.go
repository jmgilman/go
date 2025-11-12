package mocks_test

import (
	"context"
	"testing"

	"github.com/jmgilman/go/github"
	"github.com/jmgilman/go/github/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Example test showing how to use the ProviderMock
func TestExampleUsingMock(t *testing.T) {
	ctx := context.Background()

	// Create and configure mock provider
	mock := &mocks.ProviderMock{
		GetRepositoryFunc: func(ctx context.Context, owner string, repo string) (*github.RepositoryData, error) {
			return &github.RepositoryData{
				ID:            123,
				Owner:         owner,
				Name:          repo,
				FullName:      owner + "/" + repo,
				DefaultBranch: "main",
			}, nil
		},
	}

	// Use the mock
	client := github.NewClient(mock, "testowner")
	repository := client.Repository("testrepo")
	err := repository.Get(ctx)

	// Assert behavior
	require.NoError(t, err)
	assert.Equal(t, "testowner", repository.Owner())
	assert.Equal(t, "testrepo", repository.Name())
	assert.Equal(t, "main", repository.DefaultBranch())
}
