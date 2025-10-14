package git

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCommit_Success(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create a new file to commit
	fs := repo.Filesystem()
	file, err := fs.Create("new-file.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("new content"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// Stage the file
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("new-file.txt")
	require.NoError(t, err)

	// Create commit
	hash, err := repo.CreateCommit(CommitOptions{
		Author:  "Test Author",
		Email:   "test@example.com",
		Message: "Add new file",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Verify commit was created
	commit, err := repo.GetCommit(hash)
	require.NoError(t, err)
	assert.Equal(t, hash, commit.Hash)
	assert.Equal(t, "Test Author", commit.Author)
	assert.Equal(t, "test@example.com", commit.Email)
	assert.Equal(t, "Add new file", commit.Message)
}

func TestCreateCommit_AllowEmpty(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create an empty commit (no changes)
	hash, err := repo.CreateCommit(CommitOptions{
		Author:     "Test Author",
		Email:      "test@example.com",
		Message:    "Empty commit for trigger",
		AllowEmpty: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Verify commit was created
	commit, err := repo.GetCommit(hash)
	require.NoError(t, err)
	assert.Equal(t, hash, commit.Hash)
	assert.Equal(t, "Empty commit for trigger", commit.Message)
}

func TestCreateCommit_MissingAuthor(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	_, err := repo.CreateCommit(CommitOptions{
		Email:   "test@example.com",
		Message: "Missing author",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "author is required")
}

func TestCreateCommit_MissingEmail(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	_, err := repo.CreateCommit(CommitOptions{
		Author:  "Test Author",
		Message: "Missing email",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email is required")
}

func TestCreateCommit_MissingMessage(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	_, err := repo.CreateCommit(CommitOptions{
		Author: "Test Author",
		Email:  "test@example.com",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message is required")
}

func TestWalkCommits_BetweenTwoCommits(t *testing.T) {
	repo, firstHash := createTestRepoWithCommit(t)

	// Create second commit
	fs := repo.Filesystem()
	file, err := fs.Create("file2.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("content 2"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file2.txt")
	require.NoError(t, err)
	secondHash, err := wt.Commit("Second commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create third commit
	file3, err := fs.Create("file3.txt")
	require.NoError(t, err)
	_, err = file3.Write([]byte("content 3"))
	require.NoError(t, err)
	err = file3.Close()
	require.NoError(t, err)

	_, err = wt.Add("file3.txt")
	require.NoError(t, err)
	thirdHash, err := wt.Commit("Third commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Walk from first to third (iterator yields newest→oldest)
	var commits []Commit
	for commit, err := range repo.WalkCommits(firstHash.String(), thirdHash.String()) {
		require.NoError(t, err)
		commits = append(commits, commit)
	}

	// Should return second and third commits (excluding first), newest→oldest
	assert.Len(t, commits, 2)
	assert.Equal(t, thirdHash.String(), commits[0].Hash)
	assert.Equal(t, secondHash.String(), commits[1].Hash)
}

func TestWalkCommits_SameCommit(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Walk from hash to itself (should yield nothing)
	var commits []Commit
	for commit, err := range repo.WalkCommits(hash.String(), hash.String()) {
		require.NoError(t, err)
		commits = append(commits, commit)
	}
	assert.Empty(t, commits)
}

func TestWalkCommits_InvalidFromRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Iterator should yield an error
	for _, err := range repo.WalkCommits("nonexistent", "HEAD") {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		break
	}
}

func TestWalkCommits_InvalidToRef(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Iterator should yield an error
	for _, err := range repo.WalkCommits(hash.String(), "nonexistent") {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		break
	}
}

func TestWalkCommits_EmptyFromRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Empty from should walk all commits (no error)
	var commits []Commit
	for commit, err := range repo.WalkCommits("", "HEAD") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}
	// Should have at least one commit (the initial commit)
	assert.GreaterOrEqual(t, len(commits), 1)
}

func TestWalkCommits_EmptyToRef(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Empty to should yield an error (to is required)
	for _, err := range repo.WalkCommits(hash.String(), "") {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
		break
	}
}

func TestWalkCommits_WithBranchRefs(t *testing.T) {
	repo, firstHash := createTestRepoWithCommit(t)

	// Create a branch at first commit
	err := repo.CreateBranch("start", firstHash.String())
	require.NoError(t, err)

	// Create another commit
	fs := repo.Filesystem()
	file, err := fs.Create("file2.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("content 2"))
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	_, err = wt.Add("file2.txt")
	require.NoError(t, err)
	secondHash, err := wt.Commit("Second commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create branch at second commit
	err = repo.CreateBranch("end", secondHash.String())
	require.NoError(t, err)

	// Walk using branch names
	var commits []Commit
	for commit, err := range repo.WalkCommits("start", "end") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}

	assert.Len(t, commits, 1)
	assert.Equal(t, secondHash.String(), commits[0].Hash)
}

func TestWalkCommits_LimitCount(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create additional commits
	fs := repo.Filesystem()
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)

	for i := 2; i <= 5; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		file, err := fs.Create(filename)
		require.NoError(t, err)
		_, err = fmt.Fprintf(file, "content %d", i)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		_, err = wt.Add(filename)
		require.NoError(t, err)
		_, err = wt.Commit(fmt.Sprintf("Commit %d", i), &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
	}

	// Should have 5 commits total (1 initial + 4 new)

	// Get last 3 commits using iterator with break
	var commits []Commit
	count := 0
	for commit, err := range repo.WalkCommits("", "HEAD") {
		require.NoError(t, err)
		if count >= 3 {
			break
		}
		commits = append(commits, commit)
		count++
	}
	assert.Len(t, commits, 3)

	// Verify they are in reverse chronological order (newest to oldest)
	assert.Contains(t, commits[0].Message, "Commit 5")
	assert.Contains(t, commits[1].Message, "Commit 4")
	assert.Contains(t, commits[2].Message, "Commit 3")
}

func TestWalkCommits_NoLimit(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Create two more commits
	fs := repo.Filesystem()
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)

	for i := 2; i <= 3; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		file, err := fs.Create(filename)
		require.NoError(t, err)
		_, err = fmt.Fprintf(file, "content %d", i)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		_, err = wt.Add(filename)
		require.NoError(t, err)
		_, err = wt.Commit(fmt.Sprintf("Commit %d", i), &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
	}

	// Get all commits (no limit)
	var commits []Commit
	for commit, err := range repo.WalkCommits("", "HEAD") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}

	// Should have 3 commits total
	assert.Len(t, commits, 3)

	// Verify reverse chronological order (newest to oldest)
	assert.Contains(t, commits[0].Message, "Commit 3")
	assert.Contains(t, commits[1].Message, "Commit 2")
	assert.Contains(t, commits[2].Message, "Initial commit")
}

func TestWalkCommits_AllCommits(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	// Walk all commits from HEAD
	var commits []Commit
	for commit, err := range repo.WalkCommits("", "HEAD") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}
	assert.GreaterOrEqual(t, len(commits), 1)
}

func TestGetCommit_ByHash(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	commit, err := repo.GetCommit(hash.String())
	require.NoError(t, err)

	assert.Equal(t, hash.String(), commit.Hash)
	assert.Equal(t, "Test User", commit.Author)
	assert.Equal(t, "test@example.com", commit.Email)
	assert.Contains(t, commit.Message, "Initial commit")
	assert.False(t, commit.Timestamp.IsZero())
}

func TestGetCommit_ByHEAD(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	commit, err := repo.GetCommit("HEAD")
	require.NoError(t, err)

	// HEAD should point to the initial commit
	assert.Equal(t, hash.String(), commit.Hash)
	assert.Equal(t, "Test User", commit.Author)
}

func TestGetCommit_ByBranch(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Get HEAD branch name
	head, err := repo.Underlying().Head()
	require.NoError(t, err)
	branchName := head.Name().Short()

	commit, err := repo.GetCommit(branchName)
	require.NoError(t, err)

	assert.Equal(t, hash.String(), commit.Hash)
}

func TestGetCommit_ByTag(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Create a lightweight tag
	err := repo.CreateLightweightTag("v1.0.0", hash.String())
	require.NoError(t, err)

	commit, err := repo.GetCommit("v1.0.0")
	require.NoError(t, err)

	assert.Equal(t, hash.String(), commit.Hash)
}

func TestGetCommit_InvalidRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	_, err := repo.GetCommit("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetCommit_EmptyRef(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	_, err := repo.GetCommit("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestCommit_Underlying(t *testing.T) {
	repo, hash := createTestRepoWithCommit(t)

	// Get commit using our wrapper
	commit, err := repo.GetCommit(hash.String())
	require.NoError(t, err)

	// Get underlying go-git commit
	gogitCommit := commit.Underlying()
	require.NotNil(t, gogitCommit)

	// Verify it's the same commit
	assert.Equal(t, hash, gogitCommit.Hash)
	assert.Equal(t, "Test User", gogitCommit.Author.Name)
	assert.Equal(t, "test@example.com", gogitCommit.Author.Email)

	// Verify we can use go-git API
	tree, err := gogitCommit.Tree()
	require.NoError(t, err)
	assert.NotNil(t, tree)
}

func TestCommit_Properties(t *testing.T) {
	repo, _ := createTestRepoWithCommit(t)

	commit, err := repo.GetCommit("HEAD")
	require.NoError(t, err)

	// Verify all fields are populated
	assert.NotEmpty(t, commit.Hash)
	assert.Len(t, commit.Hash, 40) // SHA-1 hash length
	assert.NotEmpty(t, commit.Author)
	assert.NotEmpty(t, commit.Email)
	assert.NotEmpty(t, commit.Message)
	assert.False(t, commit.Timestamp.IsZero())
	assert.NotNil(t, commit.raw)
}

func TestWalkCommits_ReverseChronologicalOrder(t *testing.T) {
	repo, firstHash := createTestRepoWithCommit(t)

	// Create multiple commits with known timestamps
	fs := repo.Filesystem()
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)

	var hashes []plumbing.Hash
	hashes = append(hashes, firstHash)

	for i := 2; i <= 4; i++ {
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		filename := fmt.Sprintf("file%d.txt", i)
		file, err := fs.Create(filename)
		require.NoError(t, err)
		_, err = fmt.Fprintf(file, "content %d", i)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		_, err = wt.Add(filename)
		require.NoError(t, err)
		hash, err := wt.Commit(fmt.Sprintf("Commit %d", i), &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
		hashes = append(hashes, hash)
	}

	// Walk all commits (iterator yields newest→oldest)
	var commits []Commit
	for commit, err := range repo.WalkCommits(firstHash.String(), hashes[len(hashes)-1].String()) {
		require.NoError(t, err)
		commits = append(commits, commit)
	}

	// Verify reverse chronological order (newest to oldest)
	for i := 1; i < len(commits); i++ {
		assert.True(t, commits[i-1].Timestamp.After(commits[i].Timestamp) ||
			commits[i-1].Timestamp.Equal(commits[i].Timestamp),
			"Commits should be in reverse chronological order")
	}
}

func TestWalkCommits_EmptyRepository(t *testing.T) {
	fs := memfs.New()
	repo, err := Init("/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Try to walk commits in empty repository
	for _, err := range repo.WalkCommits("", "HEAD") {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD")
		break
	}
}
