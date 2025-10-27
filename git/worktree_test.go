package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isGitAvailable checks if the git CLI is available on the system.
func isGitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// requireGit skips the test if git CLI is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if !isGitAvailable() {
		t.Skip("git CLI not available, skipping test")
	}
}

// createRealTestRepoWithCommits creates a test repository in a temp directory with commits.
// Returns the repository, two commit hashes, and the temp directory path.
// The caller should defer os.RemoveAll(tmpDir).
func createRealTestRepoWithCommits(t *testing.T) (*Repository, plumbing.Hash, plumbing.Hash, string) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "git-worktree-test-*")
	require.NoError(t, err)

	repoPath := filepath.Join(tmpDir, "test-repo")

	// Create repository (Init will create its own filesystem scoped to repoPath)
	repo, err := Init(repoPath)
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

	// Create a "main" branch at the current commit for tests that need it
	// Don't checkout the branch so it can be used for creating worktrees
	err = repo.CreateBranch("main", hash2.String())
	require.NoError(t, err)

	return repo, hash1, hash2, tmpDir
}

// mockWorktreeOps is a mock implementation of WorktreeOperations for testing.
type mockWorktreeOps struct {
	addFunc    func(ctx context.Context, path, ref string, opts WorktreeOptions) error
	listFunc   func(ctx context.Context) ([]WorktreeInfo, error)
	removeFunc func(ctx context.Context, path string, force bool) error
	lockFunc   func(ctx context.Context, path, reason string) error
	unlockFunc func(ctx context.Context, path string) error
	pruneFunc  func(ctx context.Context) error
}

func (m *mockWorktreeOps) Add(ctx context.Context, path, ref string, opts WorktreeOptions) error {
	if m.addFunc != nil {
		return m.addFunc(ctx, path, ref, opts)
	}
	return nil
}

func (m *mockWorktreeOps) List(ctx context.Context) ([]WorktreeInfo, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []WorktreeInfo{}, nil
}

func (m *mockWorktreeOps) Remove(ctx context.Context, path string, force bool) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, path, force)
	}
	return nil
}

func (m *mockWorktreeOps) Lock(ctx context.Context, path, reason string) error {
	if m.lockFunc != nil {
		return m.lockFunc(ctx, path, reason)
	}
	return nil
}

func (m *mockWorktreeOps) Unlock(ctx context.Context, path string) error {
	if m.unlockFunc != nil {
		return m.unlockFunc(ctx, path)
	}
	return nil
}

func (m *mockWorktreeOps) Prune(ctx context.Context) error {
	if m.pruneFunc != nil {
		return m.pruneFunc(ctx)
	}
	return nil
}

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
	requireGit(t)

	repo, hash1, _, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree1")

	// Create worktree at specific commit
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash1,
	})
	require.NoError(t, err)
	require.NotNil(t, wt)
	defer func() { _ = wt.Remove() }()

	// Verify the worktree path
	assert.Equal(t, worktreePath, wt.Path())

	// Verify only file1.txt exists (from first commit)
	_, err = os.Stat(filepath.Join(worktreePath, "file1.txt"))
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(worktreePath, "file2.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestCreateWorktree_WithBranch(t *testing.T) {
	requireGit(t)

	repo, _, _, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a branch
	branchRef := plumbing.NewBranchReferenceName("test-branch")
	headRef, err := repo.repo.Head()
	require.NoError(t, err)

	err = repo.repo.Storer.SetReference(plumbing.NewHashReference(branchRef, headRef.Hash()))
	require.NoError(t, err)

	worktreePath := filepath.Join(tmpDir, "worktree-branch")

	// Create worktree on branch
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Branch: branchRef,
	})
	require.NoError(t, err)
	require.NotNil(t, wt)
	defer func() { _ = wt.Remove() }()

	// Verify the worktree was created
	assert.Equal(t, worktreePath, wt.Path())
	_, err = os.Stat(worktreePath)
	assert.NoError(t, err)
}

func TestCreateWorktree_InvalidOptions_BothHashAndBranch(t *testing.T) {
	repo, hash1, _ := createTestRepoWithCommits(t)

	// Try to create worktree with both hash and branch
	wt, err := repo.CreateWorktree("test-repo", WorktreeOptions{
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
	wt, err := repo.CreateWorktree("test-repo", WorktreeOptions{})
	assert.Error(t, err)
	assert.Nil(t, wt)
	assert.Contains(t, err.Error(), "neither Hash nor Branch specified")
}

func TestCreateWorktree_InvalidHash(t *testing.T) {
	t.Skip("Skipping test that requires real filesystem - worktree operations now use git CLI")
}

func TestListWorktrees(t *testing.T) {
	requireGit(t)

	repo, _, _, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a couple of worktrees
	wt1Path := filepath.Join(tmpDir, "worktree1")
	wt1, err := repo.CreateWorktree(wt1Path, WorktreeOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	require.NoError(t, err)
	defer func() { _ = wt1.Remove() }()

	wt2Path := filepath.Join(tmpDir, "worktree2")
	wt2, err := repo.CreateWorktree(wt2Path, WorktreeOptions{
		Branch:       plumbing.NewBranchReferenceName("main"),
		CreateBranch: "feature",
	})
	require.NoError(t, err)
	defer func() { _ = wt2.Remove() }()

	// List worktrees
	worktrees, err := repo.ListWorktrees()
	require.NoError(t, err)

	// Should have 3 worktrees: main + 2 linked
	require.Len(t, worktrees, 3)

	// Verify we can access them
	paths := make(map[string]bool)
	for _, wt := range worktrees {
		paths[wt.Path()] = true
		assert.NotNil(t, wt.Underlying())
	}

	// Resolve symlinks in expected paths (macOS has /var -> /private/var)
	wt1PathResolved, err := filepath.EvalSymlinks(wt1Path)
	require.NoError(t, err)
	wt2PathResolved, err := filepath.EvalSymlinks(wt2Path)
	require.NoError(t, err)

	// Check that our created worktrees are in the list
	assert.True(t, paths[wt1PathResolved] || paths[wt2PathResolved], "at least one linked worktree should be in the list")
}

func TestWorktree_Checkout(t *testing.T) {
	requireGit(t)

	repo, hash1, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-checkout")

	// Create worktree at second commit
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }()

	// Verify both files exist initially
	_, err = os.Stat(filepath.Join(worktreePath, "file1.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreePath, "file2.txt"))
	require.NoError(t, err)

	// Checkout first commit
	err = wt.Checkout(hash1.String())
	require.NoError(t, err)

	// Verify only file1 exists now
	_, err = os.Stat(filepath.Join(worktreePath, "file1.txt"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreePath, "file2.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestWorktree_Checkout_Branch(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a branch
	branchRef := plumbing.NewBranchReferenceName("test-branch")
	err := repo.repo.Storer.SetReference(plumbing.NewHashReference(branchRef, hash2))
	require.NoError(t, err)

	worktreePath := filepath.Join(tmpDir, "worktree-branch")

	// Create worktree at hash2
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }()

	// Checkout the branch
	err = wt.Checkout("test-branch")
	require.NoError(t, err)

	// Verify the checkout succeeded
	assert.Equal(t, worktreePath, wt.Path())
}

func TestWorktree_Checkout_InvalidRef(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-invalid")

	// Create worktree
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }()

	// Try to checkout invalid reference
	err = wt.Checkout("nonexistent")
	assert.Error(t, err)
}

func TestWorktree_Remove_Clean(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-remove")

	// Create worktree
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(worktreePath)
	require.NoError(t, err)

	// Remove should succeed when worktree is clean
	err = wt.Remove()
	assert.NoError(t, err)
}

func TestWorktree_Remove_DirtyWorktree(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-dirty")

	// Create worktree
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }() // cleanup in case test fails

	// Make worktree dirty by creating a new file
	err = os.WriteFile(filepath.Join(worktreePath, "newfile.txt"), []byte("new content"), 0644)
	require.NoError(t, err)

	// Remove should fail when worktree has uncommitted changes
	err = wt.Remove()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uncommitted changes")
}

func TestWorktree_Path(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-path")

	// Create worktree
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }()

	// Verify path
	assert.Equal(t, worktreePath, wt.Path())
}

func TestWorktree_Underlying(t *testing.T) {
	requireGit(t)

	repo, _, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-underlying")

	// Create worktree
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)
	defer func() { _ = wt.Remove() }()

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
	requireGit(t)

	repo, hash1, hash2, tmpDir := createRealTestRepoWithCommits(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	worktreePath := filepath.Join(tmpDir, "worktree-integration")

	// Create worktree at second commit
	wt, err := repo.CreateWorktree(worktreePath, WorktreeOptions{
		Hash: hash2,
	})
	require.NoError(t, err)

	// Verify both files exist
	_, err = os.Stat(filepath.Join(worktreePath, "file1.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreePath, "file2.txt"))
	require.NoError(t, err)

	// Checkout first commit
	err = wt.Checkout(hash1.String())
	require.NoError(t, err)

	// Verify only file1 exists
	_, err = os.Stat(filepath.Join(worktreePath, "file1.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreePath, "file2.txt"))
	assert.True(t, os.IsNotExist(err))

	// Worktree should be clean
	status, err := wt.worktree.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean())

	// Remove should succeed
	err = wt.Remove()
	assert.NoError(t, err)
}

// New tests for the interface-based worktree operations

func TestCreateWorktree_WithMock_NeitherHashNorBranch(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Set up mock worktreeOps that should not be called
	mock := &mockWorktreeOps{
		addFunc: func(ctx context.Context, path, ref string, opts WorktreeOptions) error {
			t.Fatal("Add should not be called when neither Hash nor Branch is provided")
			return nil
		},
	}
	repo.worktreeOps = mock

	// Try to create worktree without Hash or Branch
	wt, err := repo.CreateWorktree("/tmp/worktree", WorktreeOptions{})
	assert.Error(t, err)
	assert.Nil(t, wt)
	assert.Contains(t, err.Error(), "neither Hash nor Branch specified")
}

func TestListWorktrees_WithMock(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Set up mock to verify it gets called
	listCalled := false
	mock := &mockWorktreeOps{
		listFunc: func(ctx context.Context) ([]WorktreeInfo, error) {
			listCalled = true
			return []WorktreeInfo{
				{
					Path:   "test-repo",
					Head:   "abc123",
					Branch: "main",
				},
			}, nil
		},
	}
	repo.worktreeOps = mock

	// Note: We can't test the full ListWorktrees() flow with a mock because it tries
	// to Open() the paths returned, which requires actual filesystem access.
	// This test just verifies the mock interface works correctly.
	infos, err := repo.worktreeOps.List(context.Background())
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "test-repo", infos[0].Path)
	assert.Equal(t, "abc123", infos[0].Head)
	assert.Equal(t, "main", infos[0].Branch)
	assert.True(t, listCalled, "List should have been called on mock")
}

func TestWorktree_Remove_WithMock(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Get the main worktree (it will be clean)
	wt, err := repo.repo.Worktree()
	require.NoError(t, err)

	// Set up mock
	removeCalled := false
	mock := &mockWorktreeOps{
		removeFunc: func(ctx context.Context, path string, force bool) error {
			removeCalled = true
			assert.Equal(t, "test-repo", path)
			// Force is now always true to handle index state differences between go-git and git CLI
			assert.True(t, force)
			return nil
		},
	}
	repo.worktreeOps = mock

	// Create worktree wrapper
	worktree := &Worktree{
		path:     "test-repo",
		worktree: wt,
		repo:     repo,
	}

	// Remove should succeed
	err = worktree.Remove()
	assert.NoError(t, err)
	assert.True(t, removeCalled, "Remove should have been called on mock")
}

func TestWorktree_Lock_WithMock(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	wt, err := repo.repo.Worktree()
	require.NoError(t, err)

	// Set up mock
	lockCalled := false
	mock := &mockWorktreeOps{
		lockFunc: func(ctx context.Context, path, reason string) error {
			lockCalled = true
			assert.Equal(t, "/tmp/worktree", path)
			assert.Equal(t, "test reason", reason)
			return nil
		},
	}
	repo.worktreeOps = mock

	// Create worktree wrapper
	worktree := &Worktree{
		path:     "/tmp/worktree",
		worktree: wt,
		repo:     repo,
	}

	// Lock worktree
	err = worktree.Lock("test reason")
	assert.NoError(t, err)
	assert.True(t, lockCalled, "Lock should have been called on mock")
}

func TestWorktree_Unlock_WithMock(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	wt, err := repo.repo.Worktree()
	require.NoError(t, err)

	// Set up mock
	unlockCalled := false
	mock := &mockWorktreeOps{
		unlockFunc: func(ctx context.Context, path string) error {
			unlockCalled = true
			assert.Equal(t, "/tmp/worktree", path)
			return nil
		},
	}
	repo.worktreeOps = mock

	// Create worktree wrapper
	worktree := &Worktree{
		path:     "/tmp/worktree",
		worktree: wt,
		repo:     repo,
	}

	// Unlock worktree
	err = worktree.Unlock()
	assert.NoError(t, err)
	assert.True(t, unlockCalled, "Unlock should have been called on mock")
}

func TestRepository_PruneWorktrees_WithMock(t *testing.T) {
	repo, _, _ := createTestRepoWithCommits(t)

	// Set up mock
	pruneCalled := false
	mock := &mockWorktreeOps{
		pruneFunc: func(ctx context.Context) error {
			pruneCalled = true
			return nil
		},
	}
	repo.worktreeOps = mock

	// Prune worktrees
	err := repo.PruneWorktrees()
	assert.NoError(t, err)
	assert.True(t, pruneCalled, "Prune should have been called on mock")
}
