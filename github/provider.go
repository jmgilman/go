package github

import "context"

//go:generate go run github.com/matryer/moq@latest -out mocks/provider.go -pkg mocks . Provider

// Provider defines the interface for interacting with GitHub.
// Implementations include SDKProvider (using go-github) and CLIProvider (using gh CLI).
//
// The provider interface abstracts the underlying GitHub API implementation,
// allowing users to choose between the official SDK or the gh CLI tool based
// on their needs. This design enables easy testing through mock implementations
// and provides flexibility in authentication and deployment scenarios.
//
// All methods accept a context.Context as the first parameter for cancellation
// and timeout control. Methods return structured data types (e.g., RepositoryData,
// IssueData) that are independent of the underlying implementation.
//
// Example using SDK provider:
//
//	provider, err := github.NewSDKProvider(github.SDKWithToken("ghp_..."))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	repo, err := provider.GetRepository(ctx, "owner", "repo")
//
// Example using CLI provider:
//
//	provider, err := github.NewCLIProvider()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	issues, err := provider.ListIssues(ctx, "owner", "repo", ListIssuesOptions{State: "open"})
type Provider interface {
	// Repository operations

	// GetRepository retrieves repository information.
	// Returns ErrNotFound if the repository doesn't exist.
	// Returns ErrAuthenticationFailed if authentication is invalid.
	// Returns ErrPermissionDenied if the user lacks access to the repository.
	GetRepository(ctx context.Context, owner, repo string) (*RepositoryData, error)

	// ListRepositories lists repositories for the given owner.
	// For organizations, this lists organization repositories.
	// For users, this lists user repositories.
	// Returns an empty slice if no repositories are found.
	ListRepositories(ctx context.Context, owner string, opts ListOptions) ([]*RepositoryData, error)

	// CreateRepository creates a new repository.
	// For organizations, creates an organization repository.
	// For users, creates a user repository.
	// Returns ErrConflict if a repository with the same name already exists.
	// Returns ErrInvalidInput if the repository name is invalid.
	CreateRepository(ctx context.Context, owner string, opts CreateRepositoryOptions) (*RepositoryData, error)

	// Issue operations

	// GetIssue retrieves a specific issue by number.
	// Returns ErrNotFound if the issue doesn't exist.
	GetIssue(ctx context.Context, owner, repo string, number int) (*IssueData, error)

	// ListIssues lists issues for a repository with optional filtering.
	// Returns an empty slice if no issues match the criteria.
	ListIssues(ctx context.Context, owner, repo string, opts ListIssuesOptions) ([]*IssueData, error)

	// CreateIssue creates a new issue.
	// Returns ErrInvalidInput if required fields are missing or invalid.
	CreateIssue(ctx context.Context, owner, repo string, opts CreateIssueOptions) (*IssueData, error)

	// UpdateIssue updates an existing issue.
	// Only non-nil fields in opts are updated.
	// Returns ErrNotFound if the issue doesn't exist.
	UpdateIssue(ctx context.Context, owner, repo string, number int, opts UpdateIssueOptions) (*IssueData, error)

	// CloseIssue closes an issue.
	// Returns ErrNotFound if the issue doesn't exist.
	CloseIssue(ctx context.Context, owner, repo string, number int) error

	// AddLabels adds labels to an issue.
	// Labels that don't exist will be created.
	// Returns ErrNotFound if the issue doesn't exist.
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error

	// RemoveLabel removes a label from an issue.
	// No error if the label wasn't applied to the issue.
	// Returns ErrNotFound if the issue doesn't exist.
	RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error

	// Pull Request operations

	// GetPullRequest retrieves a specific pull request by number.
	// Returns ErrNotFound if the pull request doesn't exist.
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequestData, error)

	// ListPullRequests lists pull requests for a repository with optional filtering.
	// Returns an empty slice if no pull requests match the criteria.
	ListPullRequests(ctx context.Context, owner, repo string, opts ListPullRequestsOptions) ([]*PullRequestData, error)

	// CreatePullRequest creates a new pull request.
	// Returns ErrInvalidInput if required fields are missing or invalid.
	// Returns ErrConflict if a pull request already exists for the branch.
	CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePullRequestOptions) (*PullRequestData, error)

	// UpdatePullRequest updates an existing pull request.
	// Only non-nil fields in opts are updated.
	// Returns ErrNotFound if the pull request doesn't exist.
	UpdatePullRequest(ctx context.Context, owner, repo string, number int, opts UpdatePullRequestOptions) (*PullRequestData, error)

	// MergePullRequest merges a pull request.
	// Returns ErrNotFound if the pull request doesn't exist.
	// Returns ErrConflict if the pull request cannot be merged (conflicts, checks failing, etc.).
	MergePullRequest(ctx context.Context, owner, repo string, number int, opts MergePullRequestOptions) error

	// Workflow operations

	// GetWorkflowRun retrieves a specific workflow run by ID.
	// Returns ErrNotFound if the workflow run doesn't exist.
	GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRunData, error)

	// ListWorkflowRuns lists workflow runs for a repository with optional filtering.
	// Returns an empty slice if no workflow runs match the criteria.
	ListWorkflowRuns(ctx context.Context, owner, repo string, opts ListWorkflowRunsOptions) ([]*WorkflowRunData, error)

	// GetWorkflowRunJobs retrieves the jobs for a specific workflow run.
	// Returns an empty slice if the workflow run has no jobs yet.
	// Returns ErrNotFound if the workflow run doesn't exist.
	GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64) ([]*WorkflowJobData, error)

	// TriggerWorkflow manually triggers a workflow run.
	// workflowFileName is the filename of the workflow (e.g., "ci.yml").
	// ref is the git ref (branch, tag, or SHA) to run the workflow from.
	// inputs contains workflow inputs as key-value pairs (can be nil if workflow has no inputs).
	// Returns ErrNotFound if the workflow doesn't exist.
	// Returns ErrInvalidInput if required inputs are missing or invalid.
	TriggerWorkflow(ctx context.Context, owner, repo, workflowFileName string, ref string, inputs map[string]interface{}) error
}
