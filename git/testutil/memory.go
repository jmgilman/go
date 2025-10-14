// Package testutil provides in-memory testing utilities for the git package.
// It includes helpers for creating in-memory repositories and test data,
// enabling tests to run quickly without external dependencies.
package testutil

import (
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jmgilman/go/git"
)

// NewMemoryRepo creates a new in-memory Git repository for testing.
// It uses billy's memory filesystem (memfs) to provide a fully functional
// repository without touching the actual filesystem.
//
// Returns:
//   - *git.Repository: The initialized in-memory repository
//   - billy.Filesystem: The memory filesystem for file operations
//   - error: Any error encountered during repository initialization
//
// The returned filesystem can be used to create files and directories
// within the repository's working tree. All operations are in-memory
// and will not persist after the test completes.
//
// Example:
//
//	repo, fs, err := testutil.NewMemoryRepo()
//	if err != nil {
//	    t.Fatal(err)
//	}
//	// Create files using fs
//	// Perform git operations using repo
func NewMemoryRepo() (*git.Repository, billy.Filesystem, error) {
	fs := memfs.New()

	// Initialize repository with memory filesystem
	repo, err := git.Init("/", git.WithFilesystem(fs))
	if err != nil {
		//nolint:wrapcheck // Test utility - errors from git package are already wrapped
		return nil, nil, err
	}

	return repo, fs, nil
}

// CreateTestCommit creates a commit in the given repository with test data.
// This is a convenience helper that creates an empty commit with standard
// test author information and the provided commit message.
//
// Parameters:
//   - repo: The repository to create the commit in
//   - message: The commit message to use
//
// Returns:
//   - string: The hash of the created commit
//   - error: Any error encountered during commit creation
//
// The commit is created with:
//   - Author: "Test User"
//   - Email: "test@example.com"
//   - AllowEmpty: true (no file changes required)
//
// Example:
//
//	hash, err := testutil.CreateTestCommit(repo, "Initial commit")
//	if err != nil {
//	    t.Fatal(err)
//	}
//	fmt.Println("Created commit:", hash)
func CreateTestCommit(repo *git.Repository, message string) (string, error) {
	//nolint:wrapcheck // Test utility - errors from git package are already wrapped
	return repo.CreateCommit(git.CommitOptions{
		Author:     TestAuthor,
		Email:      TestEmail,
		Message:    message,
		AllowEmpty: true,
	})
}

// CreateTestFile creates a file with the specified content in the given filesystem.
// This helper simplifies file creation for testing by handling directory creation
// and file writing in one call.
//
// Parameters:
//   - fs: The billy filesystem to create the file in
//   - path: The path of the file to create (relative to filesystem root)
//   - content: The content to write to the file
//
// Returns:
//   - error: Any error encountered during file creation or writing
//
// If the parent directory does not exist, it will be created automatically.
// If the file already exists, it will be truncated and overwritten.
//
// Example:
//
//	err := testutil.CreateTestFile(fs, "README.md", "# Test Repository")
//	if err != nil {
//	    t.Fatal(err)
//	}
func CreateTestFile(fs billy.Filesystem, path, content string) error {
	file, err := fs.Create(path)
	if err != nil {
		//nolint:wrapcheck // Test utility - simple file operation error
		return err
	}
	defer func() {
		_ = file.Close() // Ignore close error in test utility
	}()

	_, err = file.Write([]byte(content))
	//nolint:wrapcheck // Test utility - simple file operation error
	return err
}

// CreateTestCommitWithFile creates a commit that includes a file change.
// This is a convenience helper that combines file creation with commit creation.
//
// Parameters:
//   - repo: The repository to create the commit in
//   - fs: The filesystem to create the file in
//   - path: The path of the file to create
//   - content: The content to write to the file
//   - message: The commit message to use
//
// Returns:
//   - string: The hash of the created commit
//   - error: Any error encountered during file creation or commit
//
// Example:
//
//	hash, err := testutil.CreateTestCommitWithFile(
//	    repo, fs, "README.md", "# Test", "Add README")
//	if err != nil {
//	    t.Fatal(err)
//	}
func CreateTestCommitWithFile(repo *git.Repository, fs billy.Filesystem, path, content, message string) (string, error) {
	// Create the file
	if err := CreateTestFile(fs, path, content); err != nil {
		return "", err
	}

	// Get the worktree
	wt, err := repo.Underlying().Worktree()
	if err != nil {
		//nolint:wrapcheck // Test utility - errors from go-git are transparent
		return "", err // Bare repository, cannot add files
	}

	// Add the file to the worktree
	if _, err := wt.Add(path); err != nil {
		//nolint:wrapcheck // Test utility - errors from go-git are transparent
		return "", err
	}

	// Create commit
	//nolint:wrapcheck // Test utility - errors from git package are already wrapped
	return repo.CreateCommit(git.CommitOptions{
		Author:     TestAuthor,
		Email:      TestEmail,
		Message:    message,
		AllowEmpty: false,
	})
}

// CreateTestTag creates a test tag pointing to the specified commit.
// This is a convenience helper for creating tags during testing.
//
// Parameters:
//   - repo: The repository to create the tag in
//   - name: The name of the tag
//   - commitHash: The hash of the commit to tag
//   - message: The tag message (empty for lightweight tag)
//
// Returns:
//   - error: Any error encountered during tag creation
//
// Example:
//
//	err := testutil.CreateTestTag(repo, "v1.0.0", commitHash, "Release 1.0.0")
//	if err != nil {
//	    t.Fatal(err)
//	}
func CreateTestTag(repo *git.Repository, name, commitHash, message string) error {
	if message == "" {
		//nolint:wrapcheck // Test utility - errors from git package are already wrapped
		return repo.CreateLightweightTag(name, commitHash)
	}
	//nolint:wrapcheck // Test utility - errors from git package are already wrapped
	return repo.CreateTag(name, commitHash, message)
}

// CreateTestCommitWithTimestamp creates a commit with a specific timestamp.
// This is useful for testing time-based operations like commit walking.
//
// Parameters:
//   - repo: The repository to create the commit in
//   - message: The commit message to use
//   - timestamp: The timestamp to use for the commit
//
// Returns:
//   - string: The hash of the created commit
//   - error: Any error encountered during commit creation
//
// Example:
//
//	timestamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
//	hash, err := testutil.CreateTestCommitWithTimestamp(repo, "Old commit", timestamp)
//	if err != nil {
//	    t.Fatal(err)
//	}
func CreateTestCommitWithTimestamp(repo *git.Repository, message string, timestamp time.Time) (string, error) {
	// Get the worktree to create commit
	wt, err := repo.Underlying().Worktree()
	if err != nil {
		// For bare repos, we need to create commit differently
		// For now, just create a normal commit (timestamp will be current)
		//nolint:wrapcheck // Test utility - errors from git package are already wrapped
		return repo.CreateCommit(git.CommitOptions{
			Author:     TestAuthor,
			Email:      TestEmail,
			Message:    message,
			AllowEmpty: true,
		})
	}

	// Create commit with specific timestamp
	commitHash, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  TestAuthor,
			Email: TestEmail,
			When:  timestamp,
		},
		AllowEmptyCommits: true,
	})
	if err != nil {
		//nolint:wrapcheck // Test utility - errors from go-git are transparent
		return "", err
	}

	return commitHash.String(), nil
}
