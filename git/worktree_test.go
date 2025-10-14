package git

import (
	"os"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestRepoWithCommits creates a test repository with multiple commits.
func createTestRepoWithCommits(t *testing.T) (*Repository, plumbing.Hash, plumbing.Hash) {
	t.Helper()

	// Create repository
	fs := memfs.New()
	repo, err := Init("test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Create first commit
	wt, err := repo.repo.Worktree()
	require.NoError(t, err)

	// Create a file
	file, err := wt.Filesystem.Create("file1.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("content1"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// Add and commit
	_, err = wt.Add("file1.txt")
	require.NoError(t, err)

	hash1, err := wt.Commit("first commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	// Create second commit
	file2, err := wt.Filesystem.Create("file2.txt")
	require.NoError(t, err)
	_, err = file2.Write([]byte("content2"))
	require.NoError(t, err)
	err = file2.Close()
	require.NoError(t, err)

	_, err = wt.Add("file2.txt")
	require.NoError(t, err)

	hash2, err := wt.Commit("second commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	return repo, hash1, hash2
}

func TestCreateWorktree_WithHash(t *testing.T) {
	repo, hash1, _ := createTestRepoWithCommits(t)

	// Create worktree at specific commit
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash1,
	})
	require.NoError(t, err)
	require.NotNil(t, wt)

	// Verify the worktree is at the correct commit
	head, err := repo.repo.Head()
	require.NoError(t, err)
	assert.Equal(t, hash1, head.Hash())

	// Verify only file1.txt exists (from first commit)
	_, err = wt.worktree.Filesystem.Stat("file1.txt")
	assert.NoError(t, err)

	_, err = wt.worktree.Filesystem.Stat("file2.txt")
	assert.True(t, os.IsNotExist(err))
}

func TestCreateWorktree_WithBranch(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Create a branch
	branchRef := plumbing.NewBranchReferenceName("test-branch")
	headRef, err := repo.repo.Head()
	require.NoError(t, err)

	err = repo.repo.Storer.SetReference(plumbing.NewHashReference(branchRef, headRef.Hash()))
	require.NoError(t, err)

	// Create worktree on branch
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Branch: branchRef,
	})
	require.NoError(t, err)
	require.NotNil(t, wt)

	// Verify the worktree is on the correct branch
	head, err := repo.repo.Head()
	require.NoError(t, err)
	assert.Equal(t, branchRef, head.Name())
}

func TestCreateWorktree_InvalidOptions_BothHashAndBranch(t *testing.T) {
	repo, hash1, _ := createTestRepoWithCommits(t)

	// Try to create worktree with both hash and branch
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash:   hash1,
		Branch: plumbing.NewBranchReferenceName("test"),
	})
	assert.Error(t, err)
	assert.Nil(t, wt)
	assert.Contains(t, err.Error(), "both Hash and Branch specified")
}

func TestCreateWorktree_InvalidOptions_NeitherHashNorBranch(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Try to create worktree with neither hash nor branch
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{})
	assert.Error(t, err)
	assert.Nil(t, wt)
	assert.Contains(t, err.Error(), "neither Hash nor Branch specified")
}

func TestCreateWorktree_InvalidHash(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Try to create worktree with invalid hash
	invalidHash := plumbing.NewHash("0000000000000000000000000000000000000000")
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: invalidHash,
	})
	assert.Error(t, err)
	assert.Nil(t, wt)
}

func TestListWorktrees(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// List worktrees
	worktrees, err := repo.ListWorktrees()
	require.NoError(t, err)
	require.Len(t, worktrees, 1)

	// Verify the main worktree is returned
	wt := worktrees[0]
	assert.Equal(t, "test-repo", wt.Path())
	assert.NotNil(t, wt.Underlying())
}

func TestWorktree_Checkout(t *testing.T) {
	repo, hash1, hash2 := createTestRepoWithCommits(t)

	// Get the main worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Checkout first commit
	err = wt.Checkout( hash1.String())
	require.NoError(t, err)

	// Verify we're at the first commit
	head, err := repo.repo.Head()
	require.NoError(t, err)
	assert.Equal(t, hash1, head.Hash())

	// Verify only file1.txt exists
	_, err = wt.worktree.Filesystem.Stat("file1.txt")
	assert.NoError(t, err)

	_, err = wt.worktree.Filesystem.Stat("file2.txt")
	assert.True(t, os.IsNotExist(err))
}

func TestWorktree_Checkout_Branch(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Create a branch at first commit
	branchRef := plumbing.NewBranchReferenceName("test-branch")
	err := repo.repo.Storer.SetReference(plumbing.NewHashReference(branchRef, hash2))
	require.NoError(t, err)

	// Get the main worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Checkout the branch
	err = wt.Checkout( "test-branch")
	require.NoError(t, err)

	// Verify we're on the branch
	head, err := repo.repo.Head()
	require.NoError(t, err)
	assert.Equal(t, branchRef, head.Name())
}

func TestWorktree_Checkout_InvalidRef(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Get the main worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Try to checkout invalid reference
	err = wt.Checkout( "nonexistent")
	assert.Error(t, err)
}

func TestWorktree_Remove_Clean(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Get the main worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Remove should succeed when worktree is clean
	err = wt.Remove()
	assert.NoError(t, err)
}

func TestWorktree_Remove_DirtyWorktree(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Get the main worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Make changes to make worktree dirty
	file, err := wt.worktree.Filesystem.Create("newfile.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("new content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// Remove should fail when worktree has uncommitted changes
	err = wt.Remove()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestWorktree_Path(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Create worktree
	wt, err := repo.CreateWorktree( "/tmp/test-worktree", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Verify path
	assert.Equal(t, "/tmp/test-worktree", wt.Path())
}

func TestWorktree_Underlying(t *testing.T) {
	repo, _, hash2 := createTestRepoWithCommits(t)

	// Create worktree
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Get underlying worktree
	gogitWt := wt.Underlying()
	require.NotNil(t, gogitWt)

	// Verify it's the same worktree
	assert.Equal(t, wt.worktree, gogitWt)

	// Verify we can use the underlying worktree
	status, err := gogitWt.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())
}

func TestWorktree_Integration(t *testing.T) {
	repo, hash1, hash2 := createTestRepoWithCommits(t)

	// Create worktree at second commit
	wt, err := repo.CreateWorktree( "test-repo", WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Verify both files exist
	_, err = wt.worktree.Filesystem.Stat("file1.txt")
	require.NoError(t, err)
	_, err = wt.worktree.Filesystem.Stat("file2.txt")
	require.NoError(t, err)

	// Checkout first commit
	err = wt.Checkout( hash1.String())
	require.NoError(t, err)

	// Verify only file1 exists
	_, err = wt.worktree.Filesystem.Stat("file1.txt")
	require.NoError(t, err)
	_, err = wt.worktree.Filesystem.Stat("file2.txt")
	assert.True(t, os.IsNotExist(err))

	// Worktree should be clean
	status, err := wt.worktree.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())

	// Remove should succeed
	err = wt.Remove()
	assert.NoError(t, err)
}
