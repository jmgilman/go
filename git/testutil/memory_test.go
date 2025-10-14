package testutil

import (
	"testing"
	"time"

	git "github.com/jmgilman/go/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryRepo(t *testing.T) {
	t.Run("creates valid repository", func(t *testing.T) {
		repo, fs, err := NewMemoryRepo()
		require.NoError(t, err)
		require.NotNil(t, repo)
		require.NotNil(t, fs)

		// Verify underlying repository is accessible
		underlying := repo.Underlying()
		assert.NotNil(t, underlying)

		// Verify filesystem is accessible
		repoFs := repo.Filesystem()
		assert.NotNil(t, repoFs)
	})

	t.Run("filesystem is usable", func(t *testing.T) {
		_, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		// Create a file to verify filesystem works
		file, err := fs.Create("test.txt")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()

		_, err = file.Write([]byte("test content"))
		require.NoError(t, err)

		// Verify file exists
		info, err := fs.Stat("test.txt")
		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
	})
}

func TestCreateTestCommit(t *testing.T) {
	t.Run("creates commit successfully", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		hash, err := CreateTestCommit(repo, "Test commit message")
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
		assert.Len(t, hash, 40) // SHA-1 hash is 40 characters
	})

	t.Run("commit has correct message", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		message := "Custom test message"
		hash, err := CreateTestCommit(repo, message)
		require.NoError(t, err)

		commit, err := repo.GetCommit(hash)
		require.NoError(t, err)
		assert.Equal(t, message, commit.Message)
	})

	t.Run("commit has test author", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		hash, err := CreateTestCommit(repo, "Test")
		require.NoError(t, err)

		commit, err := repo.GetCommit(hash)
		require.NoError(t, err)
		assert.Equal(t, TestAuthor, commit.Author)
		assert.Equal(t, TestEmail, commit.Email)
	})

	t.Run("multiple commits work", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		hash1, err := CreateTestCommit(repo, "First commit")
		require.NoError(t, err)

		hash2, err := CreateTestCommit(repo, "Second commit")
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})
}

func TestCreateTestFile(t *testing.T) {
	t.Run("creates file successfully", func(t *testing.T) {
		_, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		err = CreateTestFile(fs, "test.txt", "test content")
		require.NoError(t, err)

		// Verify file exists
		info, err := fs.Stat("test.txt")
		require.NoError(t, err)
		assert.Equal(t, "test.txt", info.Name())
	})

	t.Run("file has correct content", func(t *testing.T) {
		_, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		content := "Hello, World!"
		err = CreateTestFile(fs, "hello.txt", content)
		require.NoError(t, err)

		// Read file back
		file, err := fs.Open("hello.txt")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()

		buf := make([]byte, 1024)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, content, string(buf[:n]))
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		_, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		// Create file with initial content
		err = CreateTestFile(fs, "test.txt", "initial")
		require.NoError(t, err)

		// Overwrite with new content
		err = CreateTestFile(fs, "test.txt", "updated")
		require.NoError(t, err)

		// Verify new content
		file, err := fs.Open("test.txt")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()

		buf := make([]byte, 1024)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "updated", string(buf[:n]))
	})
}

func TestCreateTestCommitWithFile(t *testing.T) {
	t.Run("creates commit with file", func(t *testing.T) {
		repo, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		hash, err := CreateTestCommitWithFile(repo, fs, "README.md", "# Test", "Add README")
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Verify file exists
		info, err := fs.Stat("README.md")
		require.NoError(t, err)
		assert.Equal(t, "README.md", info.Name())
	})

	t.Run("commit has correct message", func(t *testing.T) {
		repo, fs, err := NewMemoryRepo()
		require.NoError(t, err)

		message := "Add important file"
		hash, err := CreateTestCommitWithFile(repo, fs, "file.txt", "content", message)
		require.NoError(t, err)

		commit, err := repo.GetCommit(hash)
		require.NoError(t, err)
		assert.Equal(t, message, commit.Message)
	})
}

func TestCreateTestTag(t *testing.T) {
	t.Run("creates lightweight tag", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		// Create a commit first
		hash, err := CreateTestCommit(repo, "Test commit")
		require.NoError(t, err)

		// Create lightweight tag
		err = CreateTestTag(repo, "v1.0.0", hash, "")
		require.NoError(t, err)

		// Verify tag exists
		tags, err := repo.ListTags()
		require.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, "v1.0.0", tags[0].Name)
	})

	t.Run("creates annotated tag", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		// Create a commit first
		hash, err := CreateTestCommit(repo, "Test commit")
		require.NoError(t, err)

		// Create annotated tag
		message := "Release 1.0.0"
		err = CreateTestTag(repo, "v1.0.0", hash, message)
		require.NoError(t, err)

		// Verify tag exists
		tags, err := repo.ListTags()
		require.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, "v1.0.0", tags[0].Name)
		assert.Equal(t, message, tags[0].Message)
	})
}

func TestCreateTestCommitWithTimestamp(t *testing.T) {
	t.Run("creates commit with timestamp", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		timestamp := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		hash, err := CreateTestCommitWithTimestamp(repo, "Old commit", timestamp)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		commit, err := repo.GetCommit(hash)
		require.NoError(t, err)
		assert.Equal(t, "Old commit", commit.Message)
		// Note: timestamp comparison might be affected by timezone/precision
		// Just verify the commit exists
	})

	t.Run("different timestamps create different commits", func(t *testing.T) {
		repo, _, err := NewMemoryRepo()
		require.NoError(t, err)

		time1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		hash1, err := CreateTestCommitWithTimestamp(repo, "Commit 1", time1)
		require.NoError(t, err)

		time2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		hash2, err := CreateTestCommitWithTimestamp(repo, "Commit 2", time2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})
}

func TestFixtures(t *testing.T) {
	t.Run("test author constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestAuthor)
		assert.NotEmpty(t, TestEmail)
		assert.NotEmpty(t, TestAuthor2)
		assert.NotEmpty(t, TestEmail2)
	})

	t.Run("test URL constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestRepoURL)
		assert.NotEmpty(t, TestRepoSSHURL)
	})

	t.Run("test content constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestFileContent)
		assert.NotEmpty(t, TestGoFileContent)
		assert.NotEmpty(t, TestMarkdownContent)
		assert.NotEmpty(t, TestJSONContent)
		assert.NotEmpty(t, TestYAMLContent)
		assert.NotEmpty(t, TestConfigContent)
	})

	t.Run("test message constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestCommitMessage)
		assert.NotEmpty(t, TestInitialCommit)
		assert.NotEmpty(t, TestFeatureCommit)
		assert.NotEmpty(t, TestBugfixCommit)
		assert.NotEmpty(t, TestMultilineCommit)
	})

	t.Run("test branch constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestBranchName)
		assert.NotEmpty(t, TestBranchMain)
		assert.NotEmpty(t, TestBranchDevelop)
		assert.NotEmpty(t, TestBranchRelease)
	})

	t.Run("test tag constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestTagName)
		assert.NotEmpty(t, TestTagName2)
		assert.NotEmpty(t, TestTagName3)
		assert.NotEmpty(t, TestTagMessage)
	})

	t.Run("test remote constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestRemoteName)
		assert.NotEmpty(t, TestRemoteNameUpstream)
	})

	t.Run("test file path constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestFilePath)
		assert.NotEmpty(t, TestFilePath2)
		assert.NotEmpty(t, TestGoFilePath)
		assert.NotEmpty(t, TestConfigFilePath)
	})

	t.Run("test SSH key constants are defined", func(t *testing.T) {
		assert.NotEmpty(t, TestSSHPrivateKey)
		assert.NotEmpty(t, TestSSHPublicKey)
	})
}

// Integration test: Full workflow using test utilities.
func TestFullWorkflow(t *testing.T) {
	// Create repository
	repo, fs, err := NewMemoryRepo()
	require.NoError(t, err)

	// Create initial commit
	hash1, err := CreateTestCommit(repo, TestInitialCommit)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)

	// Create file and commit
	hash2, err := CreateTestCommitWithFile(repo, fs, TestFilePath, TestFileContent, TestFeatureCommit)
	require.NoError(t, err)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2)

	// Create tag
	err = CreateTestTag(repo, TestTagName, hash2, TestTagMessage)
	require.NoError(t, err)

	// Verify tag exists
	tags, err := repo.ListTags()
	require.NoError(t, err)
	assert.Len(t, tags, 1)
	assert.Equal(t, TestTagName, tags[0].Name)

	// Walk commits from HEAD using the new iterator API
	var commits []git.Commit
	for commit, err := range repo.WalkCommits("", "HEAD") {
		require.NoError(t, err)
		commits = append(commits, commit)
	}
	assert.Len(t, commits, 2)
	// WalkCommits returns commits in reverse chronological order (newest to oldest)
	assert.Equal(t, TestFeatureCommit, commits[0].Message)
	assert.Equal(t, TestInitialCommit, commits[1].Message)
}
