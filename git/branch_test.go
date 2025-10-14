package git

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test repository with an initial commit.
func createTestRepoWithCommit(t *testing.T) (*Repository, plumbing.Hash) {
	t.Helper()
	fs := memfs.New()

	// Initialize repository
	repo, err := Init("/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Create a test file
	file, err := fs.Create("/test-repo/test.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("test content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// Get worktree and add file
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("test.txt")
	require.NoError(t, err)

	// Create initial commit
	hash, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	return repo, hash
}

func TestCreateBranch_FromHEAD(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create branch from HEAD
	err := repo.CreateBranch( "feature", "HEAD")
	require.NoError(t, err)

	// Verify branch was created
	branches, err := repo.ListBranches()
	require.NoError(t, err)
	
	var found bool
	for _, b := range branches {
		if b.Name == "feature" && !b.IsRemote {
			found = true
			break
		}
	}
	assert.True(t, found, "feature branch should exist")
}

func TestCreateBranch_FromCommitHash(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create branch from specific commit hash
	err := repo.CreateBranch( "from-hash", hash.String())
	require.NoError(t, err)

	// Verify branch was created at the correct commit
	branchRef, err := repo.Underlying().Reference(plumbing.NewBranchReferenceName("from-hash"), false)
	require.NoError(t, err)
	assert.Equal(t, hash, branchRef.Hash())
}

func TestCreateBranch_FromAnotherBranch(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create first branch
	err := repo.CreateBranch( "develop", "HEAD")
	require.NoError(t, err)

	// Create second branch from first branch
	err = repo.CreateBranch( "feature", "develop")
	require.NoError(t, err)

	// Verify both branches point to the same commit
	developRef, err := repo.Underlying().Reference(plumbing.NewBranchReferenceName("develop"), false)
	require.NoError(t, err)
	
	featureRef, err := repo.Underlying().Reference(plumbing.NewBranchReferenceName("feature"), false)
	require.NoError(t, err)
	
	assert.Equal(t, developRef.Hash(), featureRef.Hash())
}

func TestCreateBranch_AlreadyExists(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create branch
	err := repo.CreateBranch( "existing", "HEAD")
	require.NoError(t, err)

	// Try to create the same branch again
	err = repo.CreateBranch( "existing", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateBranch_InvalidRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to create branch from non-existent reference
	err := repo.CreateBranch( "feature", "nonexistent-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateBranch_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to create branch with empty name
	err := repo.CreateBranch( "", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestCreateBranch_EmptyRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to create branch with empty ref
	err := repo.CreateBranch( "feature", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestCreateBranchFromRemote_Success(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Simulate a remote branch by creating a remote reference
	remoteRef := plumbing.NewRemoteReferenceName("origin", "main")
	err := repo.Underlying().Storer.SetReference(plumbing.NewHashReference(remoteRef, hash))
	require.NoError(t, err)

	// Create local branch tracking the remote branch
	err = repo.CreateBranchFromRemote( "main", "origin/main")
	require.NoError(t, err)

	// Verify local branch was created
	localRef, err := repo.Underlying().Reference(plumbing.NewBranchReferenceName("main"), false)
	require.NoError(t, err)
	assert.Equal(t, hash, localRef.Hash())

	// Verify tracking configuration
	cfg, err := repo.Underlying().Config()
	require.NoError(t, err)
	branch, exists := cfg.Branches["main"]
	require.True(t, exists, "branch tracking configuration should exist")
	assert.Equal(t, "origin", branch.Remote)
	assert.Equal(t, plumbing.NewBranchReferenceName("main"), branch.Merge)
}

func TestCreateBranchFromRemote_AlreadyExists(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create remote reference
	remoteRef := plumbing.NewRemoteReferenceName("origin", "main")
	err := repo.Underlying().Storer.SetReference(plumbing.NewHashReference(remoteRef, hash))
	require.NoError(t, err)

	// Create local branch first
	err = repo.CreateBranch( "main", "HEAD")
	require.NoError(t, err)

	// Try to create from remote when local already exists
	err = repo.CreateBranchFromRemote( "main", "origin/main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateBranchFromRemote_RemoteNotFound(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to create branch from non-existent remote branch
	err := repo.CreateBranchFromRemote( "main", "origin/main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateBranchFromRemote_InvalidFormat(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try with invalid remote branch format (missing slash)
	err := repo.CreateBranchFromRemote( "main", "invalidformat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format")
}

func TestCreateBranchFromRemote_EmptyLocalName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateBranchFromRemote( "", "origin/main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestCreateBranchFromRemote_EmptyRemoteBranch(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CreateBranchFromRemote( "main", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestListBranches_LocalOnly(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create several local branches
	err := repo.CreateBranch( "feature1", "HEAD")
	require.NoError(t, err)
	err = repo.CreateBranch( "feature2", "HEAD")
	require.NoError(t, err)

	// List branches
	branches, err := repo.ListBranches()
	require.NoError(t, err)

	// Should have master/main (default) + feature1 + feature2
	assert.GreaterOrEqual(t, len(branches), 3)

	// Count local branches
	localCount := 0
	for _, b := range branches {
		if !b.IsRemote {
			localCount++
		}
	}
	assert.GreaterOrEqual(t, localCount, 3)
}

func TestListBranches_LocalAndRemote(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create local branches
	err := repo.CreateBranch( "local", "HEAD")
	require.NoError(t, err)

	// Create remote references
	remoteRef1 := plumbing.NewRemoteReferenceName("origin", "main")
	err = repo.Underlying().Storer.SetReference(plumbing.NewHashReference(remoteRef1, hash))
	require.NoError(t, err)

	remoteRef2 := plumbing.NewRemoteReferenceName("origin", "develop")
	err = repo.Underlying().Storer.SetReference(plumbing.NewHashReference(remoteRef2, hash))
	require.NoError(t, err)

	// List branches
	branches, err := repo.ListBranches()
	require.NoError(t, err)

	// Verify we have both local and remote branches
	var localBranches, remoteBranches []Branch
	for _, b := range branches {
		if b.IsRemote {
			remoteBranches = append(remoteBranches, b)
		} else {
			localBranches = append(localBranches, b)
		}
	}

	assert.NotEmpty(t, localBranches, "should have local branches")
	assert.GreaterOrEqual(t, len(remoteBranches), 2, "should have at least 2 remote branches")

	// Verify remote branch names
	remoteNames := make(map[string]bool)
	for _, b := range remoteBranches {
		remoteNames[b.Name] = true
	}
	assert.True(t, remoteNames["origin/main"], "should have origin/main")
	assert.True(t, remoteNames["origin/develop"], "should have origin/develop")
}

func TestListBranches_BranchProperties(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create a branch
	err := repo.CreateBranch( "test-branch", "HEAD")
	require.NoError(t, err)

	// List branches
	branches, err := repo.ListBranches()
	require.NoError(t, err)

	// Find our test branch
	var testBranch *Branch
	for i := range branches {
		if branches[i].Name == "test-branch" {
			testBranch = &branches[i]
			break
		}
	}
	require.NotNil(t, testBranch, "test-branch should exist")

	// Verify properties
	assert.Equal(t, "test-branch", testBranch.Name)
	assert.Equal(t, hash, testBranch.Hash)
	assert.False(t, testBranch.IsRemote)
}

func TestCheckoutBranch_Success(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create a new branch
	err := repo.CreateBranch( "develop", "HEAD")
	require.NoError(t, err)

	// Checkout the new branch
	err = repo.CheckoutBranch( "develop")
	require.NoError(t, err)

	// Verify HEAD points to the new branch
	head, err := repo.Underlying().Head()
	require.NoError(t, err)
	assert.Equal(t, plumbing.NewBranchReferenceName("develop"), head.Name())
}

func TestCheckoutBranch_NonExistent(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to checkout non-existent branch
	err := repo.CheckoutBranch( "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCheckoutBranch_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.CheckoutBranch( "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestDeleteBranch_Merged(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create and checkout a new branch
	err := repo.CreateBranch( "feature", "HEAD")
	require.NoError(t, err)

	// Get default branch (should be master or main)
	head, err := repo.Underlying().Head()
	require.NoError(t, err)
	defaultBranch := head.Name().Short()

	// Make sure we're on the default branch before deleting
	err = repo.CheckoutBranch( defaultBranch)
	require.NoError(t, err)

	// Delete the feature branch (should succeed since it's merged)
	err = repo.DeleteBranch( "feature", false)
	require.NoError(t, err)

	// Verify branch is deleted
	_, err = repo.Underlying().Reference(plumbing.NewBranchReferenceName("feature"), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteBranch_UnmergedWithForce(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create a new branch
	err := repo.CreateBranch( "feature", "HEAD")
	require.NoError(t, err)

	// Checkout the new branch
	err = repo.CheckoutBranch( "feature")
	require.NoError(t, err)

	// Create a commit on the feature branch
	fs := repo.Filesystem()
	file, err := fs.Create("feature.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("feature content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("feature.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Feature commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Go back to default branch for simplicity (HEAD was pointing to feature)
	// We need to get the default branch from config or references
	refs, err := repo.Underlying().References()
	require.NoError(t, err)
	var defaultBranch string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() && ref.Name().Short() != "feature" {
			defaultBranch = ref.Name().Short()
			return nil
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, defaultBranch, "should have found default branch")

	err = repo.CheckoutBranch( defaultBranch)
	require.NoError(t, err)

	// Delete with force should succeed even though there are unmerged changes
	err = repo.DeleteBranch( "feature", true)
	require.NoError(t, err)

	// Verify branch is deleted
	_, err = repo.Underlying().Reference(plumbing.NewBranchReferenceName("feature"), false)
	require.Error(t, err)
}

func TestDeleteBranch_UnmergedWithoutForce(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create a new branch
	err := repo.CreateBranch( "feature", "HEAD")
	require.NoError(t, err)

	// Checkout the new branch
	err = repo.CheckoutBranch( "feature")
	require.NoError(t, err)

	// Create a commit on the feature branch
	fs := repo.Filesystem()
	file, err := fs.Create("feature.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("feature content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("feature.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Feature commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Find and checkout default branch
	refs, err := repo.Underlying().References()
	require.NoError(t, err)
	var defaultBranch string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() && ref.Name().Short() != "feature" {
			defaultBranch = ref.Name().Short()
			return nil
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, defaultBranch)

	err = repo.CheckoutBranch( defaultBranch)
	require.NoError(t, err)

	// Delete without force should fail
	err = repo.DeleteBranch( "feature", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmerged")
}

func TestDeleteBranch_CurrentBranch(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Get current branch
	head, err := repo.Underlying().Head()
	require.NoError(t, err)
	currentBranch := head.Name().Short()

	// Try to delete current branch
	err = repo.DeleteBranch( currentBranch, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "current branch")
}

func TestDeleteBranch_NonExistent(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Try to delete non-existent branch
	err := repo.DeleteBranch( "nonexistent", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteBranch_EmptyName(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	err := repo.DeleteBranch( "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}
