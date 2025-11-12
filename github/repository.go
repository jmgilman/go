package github

import (
	"context"
	"fmt"
)

// Repository represents a GitHub repository and provides repository-scoped operations.
//
// Repository instances are typically created through a Client:
//
//	client := github.NewClient(provider, "myorg")
//	repo := client.Repository("myrepo")
//
// Call Get() to fetch the repository data:
//
//	if err := repo.Get(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Repository:", repo.FullName())
type Repository struct {
	client *Client
	owner  string
	name   string
	data   *RepositoryData
}

// Get fetches the repository data from GitHub.
// This method must be called before accessing repository properties.
// Returns ErrNotFound if the repository doesn't exist.
func (r *Repository) Get(ctx context.Context) error {
	data, err := r.client.provider.GetRepository(ctx, r.owner, r.name)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to get repository")
	}
	r.data = data
	return nil
}

// Refresh refreshes the repository data from GitHub.
// This is an alias for Get() that makes the intent clearer when updating existing data.
func (r *Repository) Refresh(ctx context.Context) error {
	return r.Get(ctx)
}

// Owner returns the repository owner (organization or username).
func (r *Repository) Owner() string {
	return r.owner
}

// Name returns the repository name (without owner).
func (r *Repository) Name() string {
	return r.name
}

// FullName returns the full repository name (owner/name).
// If repository data has been fetched, returns the data's full name.
// Otherwise, constructs it from owner and name.
func (r *Repository) FullName() string {
	if r.data != nil {
		return r.data.FullName
	}
	return fmt.Sprintf("%s/%s", r.owner, r.name)
}

// Description returns the repository description.
// Returns empty string if repository data hasn't been fetched yet.
func (r *Repository) Description() string {
	if r.data == nil {
		return ""
	}
	return r.data.Description
}

// DefaultBranch returns the default branch name.
// Returns empty string if repository data hasn't been fetched yet.
func (r *Repository) DefaultBranch() string {
	if r.data == nil {
		return ""
	}
	return r.data.DefaultBranch
}

// CloneURL returns the HTTPS clone URL.
// Returns empty string if repository data hasn't been fetched yet.
func (r *Repository) CloneURL() string {
	if r.data == nil {
		return ""
	}
	return r.data.CloneURL
}

// SSHURL returns the SSH clone URL.
// Returns empty string if repository data hasn't been fetched yet.
func (r *Repository) SSHURL() string {
	if r.data == nil {
		return ""
	}
	return r.data.SSHURL
}

// HTMLURL returns the URL to view the repository on GitHub.
// Returns empty string if repository data hasn't been fetched yet.
func (r *Repository) HTMLURL() string {
	if r.data == nil {
		return ""
	}
	return r.data.HTMLURL
}

// IsPrivate returns true if the repository is private.
// Returns false if repository data hasn't been fetched yet.
func (r *Repository) IsPrivate() bool {
	if r.data == nil {
		return false
	}
	return r.data.Private
}

// IsFork returns true if the repository is a fork.
// Returns false if repository data hasn't been fetched yet.
func (r *Repository) IsFork() bool {
	if r.data == nil {
		return false
	}
	return r.data.Fork
}

// IsArchived returns true if the repository is archived.
// Returns false if repository data hasn't been fetched yet.
func (r *Repository) IsArchived() bool {
	if r.data == nil {
		return false
	}
	return r.data.Archived
}

// Data returns the underlying repository data.
// Returns nil if repository data hasn't been fetched yet.
// This provides access to all repository fields including timestamps.
func (r *Repository) Data() *RepositoryData {
	return r.data
}

// Issue operations

// CreateIssue creates a new issue in the repository.
//
// Example:
//
//	issue, err := repo.CreateIssue(ctx, "Bug title", "Description",
//	    github.WithLabels("bug", "high-priority"),
//	    github.WithAssignees("user1"),
//	)
func (r *Repository) CreateIssue(ctx context.Context, title, body string, opts ...IssueOption) (*Issue, error) {
	createOpts := CreateIssueOptions{
		Title: title,
		Body:  body,
	}

	for _, opt := range opts {
		opt(&createOpts)
	}

	data, err := r.client.provider.CreateIssue(ctx, r.owner, r.name, createOpts)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to create issue")
	}

	return &Issue{
		client: r.client,
		owner:  r.owner,
		repo:   r.name,
		data:   data,
	}, nil
}

// ListIssues lists issues in the repository with optional filtering.
//
// Example:
//
//	issues, err := repo.ListIssues(ctx,
//	    github.WithState("open"),
//	    github.WithIssueLabels("bug"),
//	)
func (r *Repository) ListIssues(ctx context.Context, opts ...IssueFilterOption) ([]*Issue, error) {
	listOpts := ListIssuesOptions{
		State: StateOpen, // default to open
	}

	for _, opt := range opts {
		opt(&listOpts)
	}

	dataList, err := r.client.provider.ListIssues(ctx, r.owner, r.name, listOpts)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to list issues")
	}

	issues := make([]*Issue, len(dataList))
	for i, data := range dataList {
		issues[i] = &Issue{
			client: r.client,
			owner:  r.owner,
			repo:   r.name,
			data:   data,
		}
	}

	return issues, nil
}

// GetIssue retrieves a specific issue by number.
//
// Example:
//
//	issue, err := repo.GetIssue(ctx, 42)
func (r *Repository) GetIssue(ctx context.Context, number int) (*Issue, error) {
	data, err := r.client.provider.GetIssue(ctx, r.owner, r.name, number)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to get issue")
	}

	return &Issue{
		client: r.client,
		owner:  r.owner,
		repo:   r.name,
		data:   data,
	}, nil
}

// Pull Request operations

// CreatePullRequest creates a new pull request in the repository.
//
// Example:
//
//	pr, err := repo.CreatePullRequest(ctx, github.CreatePullRequestOptions{
//	    Title: "Add new feature",
//	    Body:  "This PR adds...",
//	    Head:  "feature-branch",
//	    Base:  "main",
//	})
func (r *Repository) CreatePullRequest(ctx context.Context, opts CreatePullRequestOptions) (*PullRequest, error) {
	data, err := r.client.provider.CreatePullRequest(ctx, r.owner, r.name, opts)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to create pull request")
	}

	return &PullRequest{
		client: r.client,
		owner:  r.owner,
		repo:   r.name,
		data:   data,
	}, nil
}

// ListPullRequests lists pull requests in the repository with optional filtering.
//
// Example:
//
//	prs, err := repo.ListPullRequests(ctx,
//	    github.WithPRState("open"),
//	    github.WithBase("main"),
//	)
func (r *Repository) ListPullRequests(ctx context.Context, opts ...PRFilterOption) ([]*PullRequest, error) {
	listOpts := ListPullRequestsOptions{
		State: StateOpen, // default to open
	}

	for _, opt := range opts {
		opt(&listOpts)
	}

	dataList, err := r.client.provider.ListPullRequests(ctx, r.owner, r.name, listOpts)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to list pull requests")
	}

	prs := make([]*PullRequest, len(dataList))
	for i, data := range dataList {
		prs[i] = &PullRequest{
			client: r.client,
			owner:  r.owner,
			repo:   r.name,
			data:   data,
		}
	}

	return prs, nil
}

// GetPullRequest retrieves a specific pull request by number.
//
// Example:
//
//	pr, err := repo.GetPullRequest(ctx, 42)
func (r *Repository) GetPullRequest(ctx context.Context, number int) (*PullRequest, error) {
	data, err := r.client.provider.GetPullRequest(ctx, r.owner, r.name, number)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to get pull request")
	}

	return &PullRequest{
		client: r.client,
		owner:  r.owner,
		repo:   r.name,
		data:   data,
	}, nil
}

// Workflow operations

// GetWorkflowRun retrieves a specific workflow run by ID.
//
// Example:
//
//	run, err := repo.GetWorkflowRun(ctx, 123456789)
func (r *Repository) GetWorkflowRun(ctx context.Context, runID int64) (*WorkflowRun, error) {
	data, err := r.client.provider.GetWorkflowRun(ctx, r.owner, r.name, runID)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to get workflow run")
	}

	return &WorkflowRun{
		client: r.client,
		owner:  r.owner,
		repo:   r.name,
		data:   data,
	}, nil
}

// ListWorkflowRuns lists workflow runs in the repository with optional filtering.
//
// Example:
//
//	runs, err := repo.ListWorkflowRuns(ctx,
//	    github.WithStatus("completed"),
//	    github.WithBranch("main"),
//	)
func (r *Repository) ListWorkflowRuns(ctx context.Context, opts ...WorkflowFilterOption) ([]*WorkflowRun, error) {
	listOpts := ListWorkflowRunsOptions{}

	for _, opt := range opts {
		opt(&listOpts)
	}

	dataList, err := r.client.provider.ListWorkflowRuns(ctx, r.owner, r.name, listOpts)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to list workflow runs")
	}

	runs := make([]*WorkflowRun, len(dataList))
	for i, data := range dataList {
		runs[i] = &WorkflowRun{
			client: r.client,
			owner:  r.owner,
			repo:   r.name,
			data:   data,
		}
	}

	return runs, nil
}

// TriggerWorkflow manually triggers a workflow run.
// workflowFileName is the filename of the workflow (e.g., "ci.yml").
// ref is the git ref (branch, tag, or SHA) to run the workflow from.
// inputs contains workflow inputs as key-value pairs (can be nil if workflow has no inputs).
//
// Example:
//
//	err := repo.TriggerWorkflow(ctx, "ci.yml", "main", map[string]interface{}{
//	    "environment": "production",
//	    "debug": true,
//	})
func (r *Repository) TriggerWorkflow(ctx context.Context, workflowFileName, ref string, inputs map[string]interface{}) error {
	err := r.client.provider.TriggerWorkflow(ctx, r.owner, r.name, workflowFileName, ref, inputs)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to trigger workflow")
	}

	return nil
}
