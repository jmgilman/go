// Package sdk provides a GitHub provider implementation using the go-github SDK.
//
// This package implements the GitHubProvider interface by wrapping the
// github.com/google/go-github/v67 SDK, providing a consistent API for
// interacting with GitHub repositories, issues, pull requests, and workflows.
package sdk

import (
	"context"
	"net/http"

	"github.com/google/go-github/v67/github"
	"github.com/jmgilman/go/errors"
	gh "github.com/jmgilman/go/github"
)

// SDKProvider implements GitHubProvider using the go-github SDK.
type SDKProvider struct {
	client *github.Client
}

// NewSDKProvider creates a provider using the GitHub SDK.
//
// Example with token authentication:
//
//	provider, err := sdk.NewSDKProvider(sdk.WithToken("ghp_..."))
//
// Example with custom client:
//
//	httpClient := &http.Client{Timeout: 30 * time.Second}
//	ghClient := github.NewClient(httpClient)
//	provider, err := sdk.NewSDKProvider(sdk.WithClient(ghClient))
func NewSDKProvider(opts ...Option) (*SDKProvider, error) {
	cfg := &config{}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	// If no client was provided, create a default one
	if cfg.client == nil {
		if cfg.token == "" {
			err := errors.New(errors.CodeInvalidInput, "either token or client must be provided")
			return nil, errors.WithContext(err, "field", "token or client")
		}
		cfg.client = github.NewClient(nil).WithAuthToken(cfg.token)
	}

	return &SDKProvider{
		client: cfg.client,
	}, nil
}

// config holds configuration for SDKProvider.
type config struct {
	client *github.Client
	token  string
}

// Option configures the SDK provider.
type Option func(*config) error

// WithToken sets the authentication token for the SDK provider.
func WithToken(token string) Option {
	return func(cfg *config) error {
		if token == "" {
			err := errors.New(errors.CodeInvalidInput, "token cannot be empty")
			return errors.WithContext(err, "field", "token")
		}
		cfg.token = token
		return nil
	}
}

// WithClient sets a custom GitHub client for the SDK provider.
// This allows full control over the HTTP client configuration,
// authentication, and other advanced settings.
func WithClient(client *github.Client) Option {
	return func(cfg *config) error {
		if client == nil {
			err := errors.New(errors.CodeInvalidInput, "client cannot be nil")
			return errors.WithContext(err, "field", "client")
		}
		cfg.client = client
		return nil
	}
}

// CreateRepository creates a new repository.
func (s *SDKProvider) CreateRepository(ctx context.Context, owner string, opts gh.CreateRepositoryOptions) (*gh.RepositoryData, error) {
	ghRepo := &github.Repository{
		Name:        github.String(opts.Name),
		Description: github.String(opts.Description),
		Private:     github.Bool(opts.Private),
		AutoInit:    github.Bool(opts.AutoInit),
	}

	// Try to create as organization repository first
	repo, resp, err := s.client.Repositories.Create(ctx, owner, ghRepo)
	if err != nil {
		// If owner is not an org or doesn't match authenticated user, try user repository
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && (ghErr.Response.StatusCode == http.StatusNotFound || ghErr.Response.StatusCode == http.StatusUnprocessableEntity) {
			// Create for authenticated user
			repo, resp, err = s.client.Repositories.Create(ctx, "", ghRepo)
			if err != nil {
				return nil, s.wrapError(err, resp, "failed to create repository")
			}
		} else {
			return nil, s.wrapError(err, resp, "failed to create repository")
		}
	}

	return s.convertRepository(repo), nil
}

// GetRepository retrieves repository information.
func (s *SDKProvider) GetRepository(ctx context.Context, owner, repo string) (*gh.RepositoryData, error) {
	ghRepo, resp, err := s.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to get repository")
	}

	return s.convertRepository(ghRepo), nil
}

// ListRepositories lists repositories for the given owner.
func (s *SDKProvider) ListRepositories(ctx context.Context, owner string, opts gh.ListOptions) ([]*gh.RepositoryData, error) {
	listOpts := github.ListOptions{
		Page:    opts.Page,
		PerPage: opts.PerPage,
	}

	// Determine if owner is an organization or user
	// Try organization first, fall back to user if not found
	orgOpts := &github.RepositoryListByOrgOptions{
		ListOptions: listOpts,
	}
	repos, resp, err := s.client.Repositories.ListByOrg(ctx, owner, orgOpts)
	if err != nil {
		// If organization not found, try as user
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound {
			userOpts := &github.RepositoryListByUserOptions{
				ListOptions: listOpts,
			}
			repos, resp, err = s.client.Repositories.ListByUser(ctx, owner, userOpts)
			if err != nil {
				return nil, s.wrapError(err, resp, "failed to list repositories")
			}
		} else {
			return nil, s.wrapError(err, resp, "failed to list repositories")
		}
	}

	result := make([]*gh.RepositoryData, len(repos))
	for i, repo := range repos {
		result[i] = s.convertRepository(repo)
	}

	return result, nil
}

// convertRepository converts a go-github Repository to RepositoryData.
func (s *SDKProvider) convertRepository(repo *github.Repository) *gh.RepositoryData {
	if repo == nil {
		return nil
	}

	data := &gh.RepositoryData{
		ID:            repo.GetID(),
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		Description:   repo.GetDescription(),
		DefaultBranch: repo.GetDefaultBranch(),
		Private:       repo.GetPrivate(),
		Fork:          repo.GetFork(),
		Archived:      repo.GetArchived(),
		CloneURL:      repo.GetCloneURL(),
		SSHURL:        repo.GetSSHURL(),
		HTMLURL:       repo.GetHTMLURL(),
	}

	// Extract owner from repository
	if owner := repo.GetOwner(); owner != nil {
		data.Owner = owner.GetLogin()
	}

	// Set timestamps
	if createdAt := repo.GetCreatedAt(); !createdAt.IsZero() {
		data.CreatedAt = createdAt.Time
	}
	if updatedAt := repo.GetUpdatedAt(); !updatedAt.IsZero() {
		data.UpdatedAt = updatedAt.Time
	}

	return data
}

// wrapError wraps go-github errors with appropriate error codes.
func (s *SDKProvider) wrapError(err error, resp *github.Response, message string) error {
	if err == nil {
		return nil
	}

	// Extract status code from response
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	// Try to get status code from ErrorResponse
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil {
		statusCode = ghErr.Response.StatusCode
	}

	if statusCode != 0 {
		return gh.WrapHTTPError(err, statusCode, message)
	}

	// Fallback to network error for unknown errors
	return errors.Wrap(err, errors.CodeNetwork, message)
}

// Issue operations

// AddLabels adds labels to an issue.
func (s *SDKProvider) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	_, resp, err := s.client.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	if err != nil {
		return s.wrapError(err, resp, "failed to add labels")
	}

	return nil
}

// CloseIssue closes an issue.
func (s *SDKProvider) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	state := "closed"
	req := &github.IssueRequest{
		State: &state,
	}

	_, resp, err := s.client.Issues.Edit(ctx, owner, repo, number, req)
	if err != nil {
		return s.wrapError(err, resp, "failed to close issue")
	}

	return nil
}

// CreateIssue creates a new issue.
func (s *SDKProvider) CreateIssue(ctx context.Context, owner, repo string, opts gh.CreateIssueOptions) (*gh.IssueData, error) {
	req := &github.IssueRequest{
		Title: github.String(opts.Title),
		Body:  github.String(opts.Body),
	}

	// Only set labels and assignees if they're non-empty
	// GitHub API rejects nil pointers to empty slices
	if len(opts.Labels) > 0 {
		req.Labels = &opts.Labels
	}
	if len(opts.Assignees) > 0 {
		req.Assignees = &opts.Assignees
	}

	// Note: Milestone in the request expects a milestone number, not name.
	// For now, we skip milestone support in create; it can be added via Update.
	// This matches the design where milestone is a string in our API.

	issue, resp, err := s.client.Issues.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to create issue")
	}

	return s.convertIssue(issue), nil
}

// GetIssue retrieves a specific issue by number.
func (s *SDKProvider) GetIssue(ctx context.Context, owner, repo string, number int) (*gh.IssueData, error) {
	issue, resp, err := s.client.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to get issue")
	}

	return s.convertIssue(issue), nil
}

// ListIssues lists issues for a repository with optional filtering.
func (s *SDKProvider) ListIssues(ctx context.Context, owner, repo string, opts gh.ListIssuesOptions) ([]*gh.IssueData, error) {
	ghOpts := &github.IssueListByRepoOptions{
		State:    opts.State,
		Labels:   opts.Labels,
		Assignee: opts.Assignee,
		ListOptions: github.ListOptions{
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	}

	if opts.Since != nil {
		ghOpts.Since = *opts.Since
	}

	issues, resp, err := s.client.Issues.ListByRepo(ctx, owner, repo, ghOpts)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to list issues")
	}

	// Filter out pull requests (GitHub API returns both issues and PRs)
	result := make([]*gh.IssueData, 0, len(issues))
	for _, issue := range issues {
		if !issue.IsPullRequest() {
			result = append(result, s.convertIssue(issue))
		}
	}

	return result, nil
}

// RemoveLabel removes a label from an issue.
func (s *SDKProvider) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	resp, err := s.client.Issues.RemoveLabelForIssue(ctx, owner, repo, number, label)
	if err != nil {
		return s.wrapError(err, resp, "failed to remove label")
	}

	return nil
}

// UpdateIssue updates an existing issue.
func (s *SDKProvider) UpdateIssue(ctx context.Context, owner, repo string, number int, opts gh.UpdateIssueOptions) (*gh.IssueData, error) {
	req := &github.IssueRequest{}

	if opts.Title != nil {
		req.Title = opts.Title
	}
	if opts.Body != nil {
		req.Body = opts.Body
	}
	if opts.State != nil {
		req.State = opts.State
	}
	if opts.Labels != nil {
		req.Labels = &opts.Labels
	}
	if opts.Assignees != nil {
		req.Assignees = &opts.Assignees
	}

	issue, resp, err := s.client.Issues.Edit(ctx, owner, repo, number, req)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to update issue")
	}

	return s.convertIssue(issue), nil
}

// convertIssue converts a go-github Issue to IssueData.
func (s *SDKProvider) convertIssue(issue *github.Issue) *gh.IssueData {
	if issue == nil {
		return nil
	}

	data := &gh.IssueData{
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		State:     issue.GetState(),
		HTMLURL:   issue.GetHTMLURL(),
		CreatedAt: issue.GetCreatedAt().Time,
		UpdatedAt: issue.GetUpdatedAt().Time,
	}

	// Extract author
	if user := issue.GetUser(); user != nil {
		data.Author = user.GetLogin()
	}

	// Extract labels
	data.Labels = make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		data.Labels = append(data.Labels, label.GetName())
	}

	// Extract assignees
	data.Assignees = make([]string, 0, len(issue.Assignees))
	for _, assignee := range issue.Assignees {
		data.Assignees = append(data.Assignees, assignee.GetLogin())
	}

	// Extract milestone
	if milestone := issue.GetMilestone(); milestone != nil {
		data.Milestone = milestone.GetTitle()
	}

	// Extract closed time
	if closedAt := issue.GetClosedAt(); !closedAt.IsZero() {
		t := closedAt.Time
		data.ClosedAt = &t
	}

	return data
}

// Pull Request operations

// CreatePullRequest creates a new pull request.
func (s *SDKProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts gh.CreatePullRequestOptions) (*gh.PullRequestData, error) {
	req := &github.NewPullRequest{
		Title:               github.String(opts.Title),
		Body:                github.String(opts.Body),
		Head:                github.String(opts.Head),
		Base:                github.String(opts.Base),
		MaintainerCanModify: github.Bool(opts.MaintainerCanModify),
		Draft:               github.Bool(opts.Draft),
	}

	pr, resp, err := s.client.PullRequests.Create(ctx, owner, repo, req)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to create pull request")
	}

	return s.convertPullRequest(pr), nil
}

// GetPullRequest retrieves a specific pull request by number.
func (s *SDKProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*gh.PullRequestData, error) {
	pr, resp, err := s.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to get pull request")
	}

	return s.convertPullRequest(pr), nil
}

// ListPullRequests lists pull requests for a repository with optional filtering.
func (s *SDKProvider) ListPullRequests(ctx context.Context, owner, repo string, opts gh.ListPullRequestsOptions) ([]*gh.PullRequestData, error) {
	ghOpts := &github.PullRequestListOptions{
		State: opts.State,
		Head:  opts.Head,
		Base:  opts.Base,
		ListOptions: github.ListOptions{
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	}

	prs, resp, err := s.client.PullRequests.List(ctx, owner, repo, ghOpts)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to list pull requests")
	}

	result := make([]*gh.PullRequestData, len(prs))
	for i, pr := range prs {
		result[i] = s.convertPullRequest(pr)
	}

	return result, nil
}

// MergePullRequest merges a pull request.
func (s *SDKProvider) MergePullRequest(ctx context.Context, owner, repo string, number int, opts gh.MergePullRequestOptions) error {
	mergeOpts := &github.PullRequestOptions{
		MergeMethod: opts.MergeMethod,
		CommitTitle: opts.CommitTitle,
	}

	_, resp, err := s.client.PullRequests.Merge(ctx, owner, repo, number, opts.CommitMessage, mergeOpts)
	if err != nil {
		return s.wrapError(err, resp, "failed to merge pull request")
	}

	return nil
}

// UpdatePullRequest updates an existing pull request.
func (s *SDKProvider) UpdatePullRequest(ctx context.Context, owner, repo string, number int, opts gh.UpdatePullRequestOptions) (*gh.PullRequestData, error) {
	req := &github.PullRequest{}

	if opts.Title != nil {
		req.Title = opts.Title
	}
	if opts.Body != nil {
		req.Body = opts.Body
	}
	if opts.State != nil {
		req.State = opts.State
	}
	if opts.Base != nil {
		req.Base = &github.PullRequestBranch{
			Ref: opts.Base,
		}
	}

	pr, resp, err := s.client.PullRequests.Edit(ctx, owner, repo, number, req)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to update pull request")
	}

	return s.convertPullRequest(pr), nil
}

// convertPullRequest converts a go-github PullRequest to PullRequestData.
func (s *SDKProvider) convertPullRequest(pr *github.PullRequest) *gh.PullRequestData {
	if pr == nil {
		return nil
	}

	data := &gh.PullRequestData{
		Number:    pr.GetNumber(),
		Title:     pr.GetTitle(),
		Body:      pr.GetBody(),
		State:     pr.GetState(),
		Draft:     pr.GetDraft(),
		Merged:    pr.GetMerged(),
		HTMLURL:   pr.GetHTMLURL(),
		CreatedAt: pr.GetCreatedAt().Time,
		UpdatedAt: pr.GetUpdatedAt().Time,
	}

	// Extract author
	if user := pr.GetUser(); user != nil {
		data.Author = user.GetLogin()
	}

	// Extract head and base refs
	if head := pr.GetHead(); head != nil {
		data.HeadRef = head.GetRef()
		data.HeadSHA = head.GetSHA()
	}
	if base := pr.GetBase(); base != nil {
		data.BaseRef = base.GetRef()
	}

	// Extract labels
	data.Labels = make([]string, 0, len(pr.Labels))
	for _, label := range pr.Labels {
		data.Labels = append(data.Labels, label.GetName())
	}

	// Extract mergeable status
	if mergeable := pr.Mergeable; mergeable != nil {
		data.Mergeable = mergeable
	}

	// Extract merged time
	if mergedAt := pr.GetMergedAt(); !mergedAt.IsZero() {
		t := mergedAt.Time
		data.MergedAt = &t
	}

	// Extract closed time
	if closedAt := pr.GetClosedAt(); !closedAt.IsZero() {
		t := closedAt.Time
		data.ClosedAt = &t
	}

	return data
}

// Workflow operations

// GetWorkflowRun retrieves a specific workflow run by ID.
func (s *SDKProvider) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*gh.WorkflowRunData, error) {
	run, resp, err := s.client.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to get workflow run")
	}

	return s.convertWorkflowRun(run), nil
}

// GetWorkflowRunJobs retrieves the jobs for a specific workflow run.
func (s *SDKProvider) GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64) ([]*gh.WorkflowJobData, error) {
	jobs, resp, err := s.client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, nil)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to get workflow run jobs")
	}

	result := make([]*gh.WorkflowJobData, len(jobs.Jobs))
	for i, job := range jobs.Jobs {
		result[i] = s.convertWorkflowJob(job)
	}

	return result, nil
}

// ListWorkflowRuns lists workflow runs for a repository with optional filtering.
func (s *SDKProvider) ListWorkflowRuns(ctx context.Context, owner, repo string, opts gh.ListWorkflowRunsOptions) ([]*gh.WorkflowRunData, error) {
	ghOpts := &github.ListWorkflowRunsOptions{
		Branch: opts.Branch,
		Event:  opts.Event,
		Status: opts.Status,
		ListOptions: github.ListOptions{
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	}

	runs, resp, err := s.client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, ghOpts)
	if err != nil {
		return nil, s.wrapError(err, resp, "failed to list workflow runs")
	}

	result := make([]*gh.WorkflowRunData, len(runs.WorkflowRuns))
	for i, run := range runs.WorkflowRuns {
		result[i] = s.convertWorkflowRun(run)
	}

	return result, nil
}

// TriggerWorkflow manually triggers a workflow run.
func (s *SDKProvider) TriggerWorkflow(ctx context.Context, owner, repo, workflowFileName string, ref string, inputs map[string]interface{}) error {
	event := github.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: inputs,
	}

	resp, err := s.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, workflowFileName, event)
	if err != nil {
		return s.wrapError(err, resp, "failed to trigger workflow")
	}

	return nil
}

// convertWorkflowJob converts a go-github WorkflowJob to WorkflowJobData.
func (s *SDKProvider) convertWorkflowJob(job *github.WorkflowJob) *gh.WorkflowJobData {
	if job == nil {
		return nil
	}

	data := &gh.WorkflowJobData{
		ID:         job.GetID(),
		RunID:      job.GetRunID(),
		Name:       job.GetName(),
		Status:     job.GetStatus(),
		Conclusion: job.GetConclusion(),
	}

	// Set timestamps
	if startedAt := job.GetStartedAt(); !startedAt.IsZero() {
		t := startedAt.Time
		data.StartedAt = &t
	}
	if completedAt := job.GetCompletedAt(); !completedAt.IsZero() {
		t := completedAt.Time
		data.CompletedAt = &t
	}

	// Convert steps
	data.Steps = make([]gh.WorkflowStepData, len(job.Steps))
	for i, step := range job.Steps {
		stepData := gh.WorkflowStepData{
			Name:       step.GetName(),
			Number:     int(step.GetNumber()),
			Status:     step.GetStatus(),
			Conclusion: step.GetConclusion(),
		}

		if startedAt := step.GetStartedAt(); !startedAt.IsZero() {
			t := startedAt.Time
			stepData.StartedAt = &t
		}
		if completedAt := step.GetCompletedAt(); !completedAt.IsZero() {
			t := completedAt.Time
			stepData.CompletedAt = &t
		}

		data.Steps[i] = stepData
	}

	return data
}

// convertWorkflowRun converts a go-github WorkflowRun to WorkflowRunData.
func (s *SDKProvider) convertWorkflowRun(run *github.WorkflowRun) *gh.WorkflowRunData {
	if run == nil {
		return nil
	}

	data := &gh.WorkflowRunData{
		ID:         run.GetID(),
		Name:       run.GetName(),
		WorkflowID: run.GetWorkflowID(),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
		HeadBranch: run.GetHeadBranch(),
		HeadSHA:    run.GetHeadSHA(),
		RunNumber:  run.GetRunNumber(),
		Event:      run.GetEvent(),
		HTMLURL:    run.GetHTMLURL(),
	}

	// Set timestamps
	if createdAt := run.GetCreatedAt(); !createdAt.IsZero() {
		data.CreatedAt = createdAt.Time
	}
	if updatedAt := run.GetUpdatedAt(); !updatedAt.IsZero() {
		data.UpdatedAt = updatedAt.Time
	}

	return data
}
