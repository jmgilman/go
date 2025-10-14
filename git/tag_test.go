package git

import (
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCommitOptions returns standard commit options for testing.
func testCommitOptions() *gogit.CommitOptions {
	return &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	}
}

func TestCreateTag_Annotated(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create annotated tag
	err := repo.CreateTag("v1.0.0", "HEAD", "Release version 1.0.0")
	require.NoError(t, err)

	// Verify tag was created
	tags, err := repo.ListTags()
	require.NoError(t, err)

	var found bool
	for _, tag := range tags {
		if tag.Name == "v1.0.0" {
			found = true
			assert.Equal(t, "Release version 1.0.0", tag.Message)
			break
		}
	}
	assert.True(t, found, "tag v1.0.0 should exist")
}

func TestCreateTag_FromCommitHash(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create tag from specific commit hash
	err := repo.CreateTag("v0.1.0", hash.String(), "Initial release")
	require.NoError(t, err)

	// Verify tag points to the correct commit
	tagRef, err := repo.Underlying().Reference(plumbing.NewTagReferenceName("v0.1.0"), false)
	require.NoError(t, err)
	assert.NotNil(t, tagRef)

	// Get the tag object and verify it points to the correct commit
	tagObj, err := repo.Underlying().TagObject(tagRef.Hash())
	require.NoError(t, err)
	commit, err := tagObj.Commit()
	require.NoError(t, err)
	assert.Equal(t, hash, commit.Hash)
}

func TestCreateTag_AlreadyExists(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create tag
	err := repo.CreateTag("v1.0.0", "HEAD", "First message")
	require.NoError(t, err)

	// Try to create tag with same name
	err = repo.CreateTag("v1.0.0", "HEAD", "Second message")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateTag_InvalidReference(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to create tag from non-existent reference
	err := repo.CreateTag("v1.0.0", "nonexistent", "Message")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve reference")
}

func TestCreateTag_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateTag("", "HEAD", "Message")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag name is required")
}

func TestCreateTag_EmptyRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateTag("v1.0.0", "", "Message")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference is required")
}

func TestCreateTag_EmptyMessage(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateTag("v1.0.0", "HEAD", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message is required")
}

func TestCreateLightweightTag(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create lightweight tag
	err := repo.CreateLightweightTag("build-123", "HEAD")
	require.NoError(t, err)

	// Verify tag was created
	tags, err := repo.ListTags()
	require.NoError(t, err)

	var found bool
	for _, tag := range tags {
		if tag.Name == "build-123" {
			found = true
			assert.Empty(t, tag.Message, "lightweight tag should have empty message")
			break
		}
	}
	assert.True(t, found, "tag build-123 should exist")
}

func TestCreateLightweightTag_FromCommitHash(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create lightweight tag from specific commit
	err := repo.CreateLightweightTag("tested", hash.String())
	require.NoError(t, err)

	// Verify tag points directly to commit (not a tag object)
	tagRef, err := repo.Underlying().Reference(plumbing.NewTagReferenceName("tested"), false)
	require.NoError(t, err)
	assert.Equal(t, hash, tagRef.Hash())

	// Verify it's not an annotated tag (TagObject should fail)
	_, err = repo.Underlying().TagObject(tagRef.Hash())
	assert.Error(t, err, "lightweight tag should not have a tag object")
}

func TestCreateLightweightTag_AlreadyExists(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create tag
	err := repo.CreateLightweightTag("build-123", "HEAD")
	require.NoError(t, err)

	// Try to create tag with same name
	err = repo.CreateLightweightTag("build-123", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateLightweightTag_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateLightweightTag("", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag name is required")
}

func TestCreateLightweightTag_EmptyRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateLightweightTag("build-123", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference is required")
}

func TestListTags_Empty(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// List tags on empty repository (no tags yet)
	tags, err := repo.ListTags()
	require.NoError(t, err)
	assert.Empty(t, tags)
}

func TestListTags_Mixed(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create annotated tag
	err := repo.CreateTag("v1.0.0", "HEAD", "Release 1.0.0")
	require.NoError(t, err)

	// Create lightweight tag
	err = repo.CreateLightweightTag("build-123", "HEAD")
	require.NoError(t, err)

	// Create another annotated tag
	err = repo.CreateTag("v1.1.0", "HEAD", "Release 1.1.0")
	require.NoError(t, err)

	// List all tags
	tags, err := repo.ListTags()
	require.NoError(t, err)
	assert.Len(t, tags, 3)

	// Verify each tag
	tagMap := make(map[string]Tag)
	for _, tag := range tags {
		tagMap[tag.Name] = tag
	}

	assert.Contains(t, tagMap, "v1.0.0")
	assert.Equal(t, "Release 1.0.0", tagMap["v1.0.0"].Message)

	assert.Contains(t, tagMap, "build-123")
	assert.Empty(t, tagMap["build-123"].Message)

	assert.Contains(t, tagMap, "v1.1.0")
	assert.Equal(t, "Release 1.1.0", tagMap["v1.1.0"].Message)
}

func TestDeleteTag_Annotated(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create and delete annotated tag
	err := repo.CreateTag("v1.0.0", "HEAD", "Release")
	require.NoError(t, err)

	err = repo.DeleteTag("v1.0.0")
	require.NoError(t, err)

	// Verify tag was deleted
	tags, err := repo.ListTags()
	require.NoError(t, err)
	for _, tag := range tags {
		assert.NotEqual(t, "v1.0.0", tag.Name)
	}
}

func TestDeleteTag_Lightweight(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create and delete lightweight tag
	err := repo.CreateLightweightTag("build-123", "HEAD")
	require.NoError(t, err)

	err = repo.DeleteTag("build-123")
	require.NoError(t, err)

	// Verify tag was deleted
	tags, err := repo.ListTags()
	require.NoError(t, err)
	for _, tag := range tags {
		assert.NotEqual(t, "build-123", tag.Name)
	}
}

func TestDeleteTag_NotFound(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to delete non-existent tag
	err := repo.DeleteTag("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find tag")
}

func TestDeleteTag_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.DeleteTag("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag name is required")
}

func TestWalkCommits_BetweenTags(t *testing.T) {
	repo, hash1 := createTestRepoWithCommit(t)

	// Create first tag
	err := repo.CreateTag("v1.0.0", hash1.String(), "Version 1.0.0")
	require.NoError(t, err)

	// Create second commit
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)

	file, err := repo.Filesystem().Create("test2.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("second commit"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	_, err = wt.Add("test2.txt")
	require.NoError(t, err)
	hash2, err := wt.Commit("Second commit", testCommitOptions())
	require.NoError(t, err)

	// Create second tag
	err = repo.CreateTag("v1.1.0", hash2.String(), "Version 1.1.0")
	require.NoError(t, err)

	// Walk between tags using WalkCommits (iterator yields newest→oldest)
	var commits []Commit
	for commit, err := range repo.WalkCommits("v1.0.0", "v1.1.0") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}

	// Should have 1 commit (second commit, excluding first)
	assert.Len(t, commits, 1)
	assert.Equal(t, hash2.String(), commits[0].Hash)
	assert.Equal(t, "Second commit", commits[0].Message)
}

