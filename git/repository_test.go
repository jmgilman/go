package git

import (
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_StandardRepository(t *testing.T) {
	fs := memfs.New()

	// Initialize a standard (non-bare) repository
	repo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Verify repository was created
	assert.NotNil(t, repo.Underlying())
	assert.NotNil(t, repo.Filesystem())

	// Verify .git directory exists
	stat, err := fs.Stat("/test-repo/.git")
	require.NoError(t, err)
	assert.True(t, stat.IsDir())

	// Verify we can get the worktree (only works for non-bare repos)
	wt, err := repo.Underlying().Worktree()
	require.NoError(t, err)
	assert.NotNil(t, wt)
}

func TestInit_BareRepository(t *testing.T) {
	fs := memfs.New()

	// Initialize a bare repository
	repo, err := Init( "/bare-repo", WithFilesystem(fs), WithBare())
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Verify repository was created
	assert.NotNil(t, repo.Underlying())
	assert.NotNil(t, repo.Filesystem())

	// Verify bare repository structure (no .git subdirectory)
	// Bare repos have refs, objects, etc. directly in the repo directory
	stat, err := fs.Stat("/bare-repo/refs")
	require.NoError(t, err)
	assert.True(t, stat.IsDir())

	// Verify we cannot get a worktree for bare repos
	_, err = repo.Underlying().Worktree()
	require.Error(t, err)
}

func TestInit_AlreadyExists(t *testing.T) {
	fs := memfs.New()

	// Initialize repository first time
	repo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Try to initialize again at the same path
	_, err = Init( "/test-repo", WithFilesystem(fs))
	require.Error(t, err)
	// The error should indicate the repository already exists
	assert.Contains(t, err.Error(), "already exists")
}

func TestInit_NestedPath(t *testing.T) {
	fs := memfs.New()

	// Initialize repository in nested path
	repo, err := Init( "/parent/child/repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Verify the path was created
	stat, err := fs.Stat("/parent/child/repo/.git")
	require.NoError(t, err)
	assert.True(t, stat.IsDir())
}

func TestOpen_ExistingRepository(t *testing.T) {
	fs := memfs.New()

	// First, create a repository
	repo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Now open the existing repository
	opened, err := Open( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, opened)

	// Verify we got a valid repository
	assert.NotNil(t, opened.Underlying())
	assert.NotNil(t, opened.Filesystem())
}

func TestOpen_BareRepository(t *testing.T) {
	fs := memfs.New()

	// First, create a bare repository
	repo, err := Init( "/bare-repo", WithFilesystem(fs), WithBare())
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Now open the existing bare repository
	opened, err := Open( "/bare-repo", WithFilesystem(fs))
	require.NoError(t, err)
	require.NotNil(t, opened)

	// Verify we got a valid repository
	assert.NotNil(t, opened.Underlying())
	assert.NotNil(t, opened.Filesystem())

	// Verify it's still bare (no worktree)
	_, err = opened.Underlying().Worktree()
	require.Error(t, err)
}

func TestOpen_NonExistentRepository(t *testing.T) {
	fs := memfs.New()

	// Try to open a non-existent repository
	_, err := Open( "/nonexistent", WithFilesystem(fs))
	require.Error(t, err)
	// Error should indicate repository not found or doesn't exist
	assert.True(t, 
		err.Error() == "failed to scope filesystem to path: file does not exist" ||
		err.Error() == "failed to open repository: [NOT_FOUND] repository does not exist",
		"expected error about repository not found, got: %s", err.Error())
}

func TestOpen_InvalidPath(t *testing.T) {
	fs := memfs.New()

	// Create a file instead of a directory
	file, err := fs.Create("/file")
	require.NoError(t, err)
	_ = file.Close()

	// Try to open it as a repository
	_, err = Open( "/file", WithFilesystem(fs))
	require.Error(t, err)
}

func TestUnderlying(t *testing.T) {
	fs := memfs.New()

	// Create a repository
	repo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Get underlying go-git repository
	underlying := repo.Underlying()
	require.NotNil(t, underlying)

	// Verify it's a go-git repository by accessing its methods
	config, err := underlying.Config()
	require.NoError(t, err)
	assert.NotNil(t, config)
}

func TestFilesystem(t *testing.T) {
	fs := memfs.New()

	// Create a repository
	repo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Get filesystem
	repoFs := repo.Filesystem()
	require.NotNil(t, repoFs)

	// Verify we can use the filesystem
	file, err := repoFs.Create("test.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("test content"))
	require.NoError(t, err)
	_ = file.Close()

	// Verify file was created
	stat, err := repoFs.Stat("test.txt")
	require.NoError(t, err)
	assert.Equal(t, "test.txt", stat.Name())
}

func TestRepository_MultipleConcurrentInits(t *testing.T) {

	// Use separate filesystems for true concurrency
	fs1 := memfs.New()
	fs2 := memfs.New()
	fs3 := memfs.New()

	type result struct {
		repo *Repository
		err  error
	}

	results := make(chan result, 3)

	// Initialize three repositories concurrently
	go func() {
		repo, err := Init( "/repo1", WithFilesystem(fs1))
		results <- result{repo, err}
	}()

	go func() {
		repo, err := Init( "/repo2", WithFilesystem(fs2), WithBare())
		results <- result{repo, err}
	}()

	go func() {
		repo, err := Init( "/repo3", WithFilesystem(fs3))
		results <- result{repo, err}
	}()

	// Collect results
	for i := 0; i < 3; i++ {
		res := <-results
		require.NoError(t, res.err)
		require.NotNil(t, res.repo)
	}
}

func TestRepository_InitAndOpen_RoundTrip(t *testing.T) {
	fs := memfs.New()

	// Initialize a repository
	originalRepo, err := Init( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Create a file in the repository using its filesystem
	file, err := originalRepo.Filesystem().Create("README.md")
	require.NoError(t, err)
	_, err = file.Write([]byte("# Test Repository"))
	require.NoError(t, err)
	_ = file.Close()

	// Open the repository
	openedRepo, err := Open( "/test-repo", WithFilesystem(fs))
	require.NoError(t, err)

	// Verify we can read the file through the opened repository
	openedFile, err := openedRepo.Filesystem().Open("README.md")
	require.NoError(t, err)
	content := make([]byte, 17)
	n, err := openedFile.Read(content)
	require.NoError(t, err)
	assert.Equal(t, 17, n)
	assert.Equal(t, "# Test Repository", string(content))
	_ = openedFile.Close()
}

func TestRepository_Path(t *testing.T) {
	fs := memfs.New()

	testCases := []struct {
		name string
		path string
		bare bool
	}{
		{
			name: "standard repository simple path",
			path: "/repo",
			bare: false,
		},
		{
			name: "bare repository simple path",
			path: "/bare",
			bare: true,
		},
		{
			name: "standard repository nested path",
			path: "/parent/child/repo",
			bare: false,
		},
		{
			name: "bare repository nested path",
			path: "/parent/child/bare",
			bare: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []RepositoryOption
			opts = append(opts, WithFilesystem(fs))
			if tc.bare {
				opts = append(opts, WithBare())
			}

			repo, err := Init( tc.path, opts...)
			require.NoError(t, err)
			require.NotNil(t, repo)

			// Verify the repository stores the path correctly
			assert.Equal(t, tc.path, repo.path)

			// Verify we can open it using the same path
			opened, err := Open( tc.path, WithFilesystem(fs))
			require.NoError(t, err)
			assert.Equal(t, tc.path, opened.path)
		})
	}
}
