//nolint:contextcheck // Context is properly passed via CommandWrapper.WithContext() but linter cannot verify
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmgilman/go/errors"
	"github.com/jmgilman/go/exec"
	github "github.com/jmgilman/go/github"
)

// Option configures the CLI provider.
type Option func(*CLIProvider) error

// CLIProvider implements GitHubProvider using the gh CLI.
type CLIProvider struct {
	wrapper *exec.CommandWrapper
}

// NewCLIProvider creates a provider using the gh CLI.
// Inherits authentication from gh CLI configuration.
// Uses the workspace exec module for command execution.
//
// Example:
//
//	provider, err := github.NewCLIProvider()
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewCLIProvider(opts ...Option) (*CLIProvider, error) {
	// Default executor
	executor := exec.New(exec.WithInheritEnv())

	provider := &CLIProvider{
		wrapper: exec.NewWrapper(executor, "gh"),
	}

	// Apply options (can override the wrapper)
	for _, opt := range opts {
		if err := opt(provider); err != nil {
			return nil, err
		}
	}

	// Verify gh is installed and authenticated
	result, err := provider.wrapper.Run("auth", "status")
	if err != nil {
		return nil, wrapAuthError(err, result)
	}

	return provider, nil
}

// AddLabels adds labels to an issue.
func (c *CLIProvider) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	args := []string{"issue", "edit", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo)}
	for _, label := range labels {
		args = append(args, "--add-label", label)
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return c.wrapCLIError(err, result, "failed to add labels")
	}

	return nil
}

// CloseIssue closes an issue.
func (c *CLIProvider) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("issue", "close", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo))

	if err != nil {
		return c.wrapCLIError(err, result, "failed to close issue")
	}

	return nil
}

// CreateIssue creates a new issue.
func (c *CLIProvider) CreateIssue(ctx context.Context, owner, repo string, opts github.CreateIssueOptions) (*github.IssueData, error) {
	args := []string{"issue", "create", "--repo", fmt.Sprintf("%s/%s", owner, repo), "--title", opts.Title}

	if opts.Body != "" {
		args = append(args, "--body", opts.Body)
	}
	if len(opts.Labels) > 0 {
		for _, label := range opts.Labels {
			args = append(args, "--label", label)
		}
	}
	if len(opts.Assignees) > 0 {
		for _, assignee := range opts.Assignees {
			args = append(args, "--assignee", assignee)
		}
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to create issue")
	}

	// Extract issue number from output (gh issue create returns the URL)
	// Example: https://github.com/owner/repo/issues/123
	issueURL := strings.TrimSpace(result.Stdout)
	parts := strings.Split(issueURL, "/")
	if len(parts) < 5 || parts[len(parts)-1] == "" {
		return nil, errors.New(errors.CodeInvalidInput, "failed to parse issue number from output")
	}
	numberStr := parts[len(parts)-1]
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidInput, "failed to parse issue number")
	}

	// Fetch the created issue to get full data
	return c.GetIssue(ctx, owner, repo, number)
}

// CreatePullRequest creates a new pull request.
func (c *CLIProvider) CreatePullRequest(ctx context.Context, owner, repo string, opts github.CreatePullRequestOptions) (*github.PullRequestData, error) {
	args := []string{"pr", "create", "--repo", fmt.Sprintf("%s/%s", owner, repo), "--title", opts.Title, "--head", opts.Head, "--base", opts.Base}

	if opts.Body != "" {
		args = append(args, "--body", opts.Body)
	}
	if opts.Draft {
		args = append(args, "--draft")
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)
	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to create pull request")
	}

	// Extract PR number from output
	prURL := strings.TrimSpace(result.Stdout)
	parts := strings.Split(prURL, "/")
	if len(parts) < 5 || parts[len(parts)-1] == "" {
		return nil, errors.New(errors.CodeInvalidInput, "failed to parse PR number from output")
	}
	numberStr := parts[len(parts)-1]
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidInput, "failed to parse PR number")
	}

	// Fetch the created PR to get full data
	return c.GetPullRequest(ctx, owner, repo, number)
}

// CreateRepository creates a new repository.
func (c *CLIProvider) CreateRepository(ctx context.Context, owner string, opts github.CreateRepositoryOptions) (*github.RepositoryData, error) {
	// Build request body
	reqBody := map[string]interface{}{
		"name":        opts.Name,
		"description": opts.Description,
		"private":     opts.Private,
		"auto_init":   opts.AutoInit,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidInput, "failed to marshal request")
	}

	// Try to create as org repository first
	result, err := c.wrapper.Clone().WithContext(ctx).Run("api", fmt.Sprintf("orgs/%s/repos", owner), "--input", "-", "--method", "POST", "--field", string(reqJSON))

	if err != nil {
		// Try as user repository
		result, err = c.wrapper.Clone().WithContext(ctx).Run("api", "user/repos", "--input", "-", "--method", "POST", "--field", string(reqJSON))
		if err != nil {
			return nil, c.wrapCLIError(err, result, "failed to create repository")
		}
	}

	var apiResp struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		Fork          bool   `json:"fork"`
		Archived      bool   `json:"archived"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		HTMLURL       string `json:"html_url"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}

	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	data := &github.RepositoryData{
		ID:            apiResp.ID,
		Owner:         apiResp.Owner.Login,
		Name:          apiResp.Name,
		FullName:      apiResp.FullName,
		Description:   apiResp.Description,
		DefaultBranch: apiResp.DefaultBranch,
		Private:       apiResp.Private,
		Fork:          apiResp.Fork,
		Archived:      apiResp.Archived,
		CloneURL:      apiResp.CloneURL,
		SSHURL:        apiResp.SSHURL,
		HTMLURL:       apiResp.HTMLURL,
	}

	if t, err := github.ParseGitHubTime(apiResp.CreatedAt); err == nil {
		data.CreatedAt = t
	}
	if t, err := github.ParseGitHubTime(apiResp.UpdatedAt); err == nil {
		data.UpdatedAt = t
	}

	return data, nil
}

// GetIssue retrieves a specific issue by number.
func (c *CLIProvider) GetIssue(ctx context.Context, owner, repo string, number int) (*github.IssueData, error) {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("issue", "view", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "number,title,body,state,author,labels,assignees,milestone,createdAt,updatedAt,closedAt,url")

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to get issue")
	}

	return c.parseIssueFromJSON(result)
}

// GetPullRequest retrieves a specific pull request by number.
func (c *CLIProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequestData, error) {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("pr", "view", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "number,title,body,state,author,headRefName,baseRefName,headRefOid,labels,isDraft,mergeable,mergedAt,createdAt,updatedAt,closedAt,url")
	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to get pull request")
	}

	return c.parsePRFromJSON(result)
}

// GetRepository retrieves repository information.
func (c *CLIProvider) GetRepository(ctx context.Context, owner, repo string) (*github.RepositoryData, error) {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("api", fmt.Sprintf("repos/%s/%s", owner, repo))
	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to get repository")
	}

	// Parse gh API response
	var apiResp struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		Fork          bool   `json:"fork"`
		Archived      bool   `json:"archived"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		HTMLURL       string `json:"html_url"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}

	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	data := &github.RepositoryData{
		ID:            apiResp.ID,
		Owner:         apiResp.Owner.Login,
		Name:          apiResp.Name,
		FullName:      apiResp.FullName,
		Description:   apiResp.Description,
		DefaultBranch: apiResp.DefaultBranch,
		Private:       apiResp.Private,
		Fork:          apiResp.Fork,
		Archived:      apiResp.Archived,
		CloneURL:      apiResp.CloneURL,
		SSHURL:        apiResp.SSHURL,
		HTMLURL:       apiResp.HTMLURL,
	}

	// Parse timestamps
	if t, err := github.ParseGitHubTime(apiResp.CreatedAt); err == nil {
		data.CreatedAt = t
	}
	if t, err := github.ParseGitHubTime(apiResp.UpdatedAt); err == nil {
		data.UpdatedAt = t
	}

	return data, nil
}

// GetWorkflowRun retrieves a specific workflow run by ID.
func (c *CLIProvider) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*github.WorkflowRunData, error) {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("run", "view", strconv.FormatInt(runID, 10), "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "databaseId,name,workflowDatabaseId,status,conclusion,headBranch,headSha,number,event,createdAt,updatedAt,url")

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to get workflow run")
	}

	return c.parseWorkflowRunFromJSON(result)
}

// GetWorkflowRunJobs retrieves the jobs for a specific workflow run.
func (c *CLIProvider) GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64) ([]*github.WorkflowJobData, error) {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("run", "view", strconv.FormatInt(runID, 10), "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "jobs")

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to get workflow run jobs")
	}

	var apiResp struct {
		Jobs []map[string]interface{} `json:"jobs"`
	}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	jobs := make([]*github.WorkflowJobData, 0, len(apiResp.Jobs))
	for _, item := range apiResp.Jobs {
		job := c.convertWorkflowJobFromMap(item, runID)
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// ListIssues lists issues for a repository with optional filtering.
func (c *CLIProvider) ListIssues(ctx context.Context, owner, repo string, opts github.ListIssuesOptions) ([]*github.IssueData, error) {
	args := []string{"issue", "list", "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "number,title,body,state,author,labels,assignees,milestone,createdAt,updatedAt,closedAt,url"}

	if opts.State != "" {
		args = append(args, "--state", opts.State)
	}
	if len(opts.Labels) > 0 {
		args = append(args, "--label", strings.Join(opts.Labels, ","))
	}
	if opts.Assignee != "" {
		args = append(args, "--assignee", opts.Assignee)
	}
	if opts.PerPage > 0 {
		args = append(args, "--limit", strconv.Itoa(opts.PerPage))
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to list issues")
	}

	var apiResp []map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	issues := make([]*github.IssueData, 0, len(apiResp))
	for _, item := range apiResp {
		issue := c.convertIssueFromMap(item)
		issues = append(issues, issue)
	}

	return issues, nil
}

// ListPullRequests lists pull requests for a repository with optional filtering.
func (c *CLIProvider) ListPullRequests(ctx context.Context, owner, repo string, opts github.ListPullRequestsOptions) ([]*github.PullRequestData, error) {
	args := []string{"pr", "list", "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "number,title,body,state,author,headRefName,baseRefName,headRefOid,labels,isDraft,mergeable,mergedAt,createdAt,updatedAt,closedAt,url"}

	if opts.State != "" {
		args = append(args, "--state", opts.State)
	}
	if opts.Head != "" {
		args = append(args, "--head", opts.Head)
	}
	if opts.Base != "" {
		args = append(args, "--base", opts.Base)
	}
	if opts.PerPage > 0 {
		args = append(args, "--limit", strconv.Itoa(opts.PerPage))
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to list pull requests")
	}

	var apiResp []map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	prs := make([]*github.PullRequestData, 0, len(apiResp))
	for _, item := range apiResp {
		pr := c.convertPRFromMap(item)
		prs = append(prs, pr)
	}

	return prs, nil
}

// ListRepositories lists repositories for the given owner.
func (c *CLIProvider) ListRepositories(ctx context.Context, owner string, opts github.ListOptions) ([]*github.RepositoryData, error) {
	args := []string{"api", fmt.Sprintf("users/%s/repos", owner)}
	if opts.PerPage > 0 {
		args = append(args, "--paginate")
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		// Try as organization if user fails
		result, err = c.wrapper.Clone().WithContext(ctx).Run("api", fmt.Sprintf("orgs/%s/repos", owner))
		if err != nil {
			return nil, c.wrapCLIError(err, result, "failed to list repositories")
		}
	}

	var apiResp []struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		Fork          bool   `json:"fork"`
		Archived      bool   `json:"archived"`
		CloneURL      string `json:"clone_url"`
		SSHURL        string `json:"ssh_url"`
		HTMLURL       string `json:"html_url"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}

	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	repos := make([]*github.RepositoryData, len(apiResp))
	for i, r := range apiResp {
		repos[i] = &github.RepositoryData{
			ID:            r.ID,
			Owner:         r.Owner.Login,
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			Fork:          r.Fork,
			Archived:      r.Archived,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			HTMLURL:       r.HTMLURL,
		}

		if t, err := github.ParseGitHubTime(r.CreatedAt); err == nil {
			repos[i].CreatedAt = t
		}
		if t, err := github.ParseGitHubTime(r.UpdatedAt); err == nil {
			repos[i].UpdatedAt = t
		}
	}

	return repos, nil
}

// ListWorkflowRuns lists workflow runs for a repository with optional filtering.
func (c *CLIProvider) ListWorkflowRuns(ctx context.Context, owner, repo string, opts github.ListWorkflowRunsOptions) ([]*github.WorkflowRunData, error) {
	args := []string{"run", "list", "--repo", fmt.Sprintf("%s/%s", owner, repo), "--json", "databaseId,name,workflowDatabaseId,status,conclusion,headBranch,headSha,number,event,createdAt,updatedAt,url"}

	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}
	if opts.Status != "" {
		// gh run list uses different status names
		// Map our status constants to gh CLI status
		switch opts.Status {
		case github.WorkflowStatusCompleted:
			args = append(args, "--status", "completed")
		case github.WorkflowStatusInProgress:
			args = append(args, "--status", "in_progress")
		case github.WorkflowStatusQueued:
			args = append(args, "--status", "queued")
		}
	}
	if opts.PerPage > 0 {
		args = append(args, "--limit", strconv.Itoa(opts.PerPage))
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to list workflow runs")
	}

	var apiResp []map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	runs := make([]*github.WorkflowRunData, 0, len(apiResp))
	for _, item := range apiResp {
		run := c.convertWorkflowRunFromMap(item)
		runs = append(runs, run)
	}

	return runs, nil
}

// MergePullRequest merges a pull request.
func (c *CLIProvider) MergePullRequest(ctx context.Context, owner, repo string, number int, opts github.MergePullRequestOptions) error {
	args := []string{"pr", "merge", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo)}

	// Map merge method
	switch opts.MergeMethod {
	case github.MergeMethodSquash:
		args = append(args, "--squash")
	case github.MergeMethodRebase:
		args = append(args, "--rebase")
	case github.MergeMethodMerge, "":
		args = append(args, "--merge")
	}

	// gh pr merge doesn't support custom commit messages in the same way
	// We'll use auto-merge behavior

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return c.wrapCLIError(err, result, "failed to merge pull request")
	}

	return nil
}

// RemoveLabel removes a label from an issue.
func (c *CLIProvider) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	result, err := c.wrapper.Clone().WithContext(ctx).Run("issue", "edit", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo), "--remove-label", label)

	if err != nil {
		return c.wrapCLIError(err, result, "failed to remove label")
	}

	return nil
}

// TriggerWorkflow manually triggers a workflow run.
func (c *CLIProvider) TriggerWorkflow(ctx context.Context, owner, repo, workflowFileName string, ref string, inputs map[string]interface{}) error {
	args := []string{"workflow", "run", workflowFileName, "--repo", fmt.Sprintf("%s/%s", owner, repo), "--ref", ref}

	// Add inputs as field arguments
	for key, value := range inputs {
		args = append(args, "-f", fmt.Sprintf("%s=%v", key, value))
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return c.wrapCLIError(err, result, "failed to trigger workflow")
	}

	return nil
}

// UpdateIssue updates an existing issue.
func (c *CLIProvider) UpdateIssue(ctx context.Context, owner, repo string, number int, opts github.UpdateIssueOptions) (*github.IssueData, error) {
	args := []string{"issue", "edit", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo)}

	if opts.Title != nil {
		args = append(args, "--title", *opts.Title)
	}
	if opts.Body != nil {
		args = append(args, "--body", *opts.Body)
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to update issue")
	}

	// Fetch updated issue
	return c.GetIssue(ctx, owner, repo, number)
}

// UpdatePullRequest updates an existing pull request.
func (c *CLIProvider) UpdatePullRequest(ctx context.Context, owner, repo string, number int, opts github.UpdatePullRequestOptions) (*github.PullRequestData, error) {
	args := []string{"pr", "edit", strconv.Itoa(number), "--repo", fmt.Sprintf("%s/%s", owner, repo)}

	if opts.Title != nil {
		args = append(args, "--title", *opts.Title)
	}
	if opts.Body != nil {
		args = append(args, "--body", *opts.Body)
	}
	if opts.Base != nil {
		args = append(args, "--base", *opts.Base)
	}

	result, err := c.wrapper.Clone().WithContext(ctx).Run(args...)

	if err != nil {
		return nil, c.wrapCLIError(err, result, "failed to update pull request")
	}

	// Fetch updated PR
	return c.GetPullRequest(ctx, owner, repo, number)
}

// convertIssueFromMap converts a map from gh CLI JSON to IssueData.
func (c *CLIProvider) convertIssueFromMap(data map[string]interface{}) *github.IssueData {
	issue := &github.IssueData{}

	if v, ok := data["number"].(float64); ok {
		issue.Number = int(v)
	}
	if v, ok := data["title"].(string); ok {
		issue.Title = v
	}
	if v, ok := data["body"].(string); ok {
		issue.Body = v
	}
	if v, ok := data["state"].(string); ok {
		issue.State = strings.ToLower(v)
	}
	if v, ok := data["url"].(string); ok {
		issue.HTMLURL = v
	}

	// Parse author
	if author, ok := data["author"].(map[string]interface{}); ok {
		if login, ok := author["login"].(string); ok {
			issue.Author = login
		}
	}

	// Parse labels
	if labels, ok := data["labels"].([]interface{}); ok {
		issue.Labels = make([]string, 0, len(labels))
		for _, l := range labels {
			if labelMap, ok := l.(map[string]interface{}); ok {
				if name, ok := labelMap["name"].(string); ok {
					issue.Labels = append(issue.Labels, name)
				}
			}
		}
	}

	// Parse assignees
	if assignees, ok := data["assignees"].([]interface{}); ok {
		issue.Assignees = make([]string, 0, len(assignees))
		for _, a := range assignees {
			if assigneeMap, ok := a.(map[string]interface{}); ok {
				if login, ok := assigneeMap["login"].(string); ok {
					issue.Assignees = append(issue.Assignees, login)
				}
			}
		}
	}

	// Parse milestone
	if milestone, ok := data["milestone"].(map[string]interface{}); ok {
		if title, ok := milestone["title"].(string); ok {
			issue.Milestone = title
		}
	}

	// Parse timestamps
	if v, ok := data["createdAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			issue.CreatedAt = t
		}
	}
	if v, ok := data["updatedAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			issue.UpdatedAt = t
		}
	}
	if v, ok := data["closedAt"].(string); ok && v != "" {
		if t, err := github.ParseGitHubTime(v); err == nil {
			issue.ClosedAt = &t
		}
	}

	return issue
}

// convertPRFromMap converts a map from gh CLI JSON to PullRequestData.
func (c *CLIProvider) convertPRFromMap(data map[string]interface{}) *github.PullRequestData {
	pr := &github.PullRequestData{}

	if v, ok := data["number"].(float64); ok {
		pr.Number = int(v)
	}
	if v, ok := data["title"].(string); ok {
		pr.Title = v
	}
	if v, ok := data["body"].(string); ok {
		pr.Body = v
	}
	if v, ok := data["state"].(string); ok {
		// gh CLI returns "MERGED" as a state, but our library uses "closed" with Merged=true
		state := strings.ToLower(v)
		if state == "merged" {
			state = "closed"
		}
		pr.State = state
	}
	if v, ok := data["headRefName"].(string); ok {
		pr.HeadRef = v
	}
	if v, ok := data["baseRefName"].(string); ok {
		pr.BaseRef = v
	}
	if v, ok := data["headRefOid"].(string); ok {
		pr.HeadSHA = v
	}
	if v, ok := data["isDraft"].(bool); ok {
		pr.Draft = v
	}
	if v, ok := data["url"].(string); ok {
		pr.HTMLURL = v
	}

	// Parse author
	if author, ok := data["author"].(map[string]interface{}); ok {
		if login, ok := author["login"].(string); ok {
			pr.Author = login
		}
	}

	// Parse labels
	if labels, ok := data["labels"].([]interface{}); ok {
		pr.Labels = make([]string, 0, len(labels))
		for _, l := range labels {
			if labelMap, ok := l.(map[string]interface{}); ok {
				if name, ok := labelMap["name"].(string); ok {
					pr.Labels = append(pr.Labels, name)
				}
			}
		}
	}

	// Parse mergeable
	if v, ok := data["mergeable"].(string); ok {
		mergeable := v == "MERGEABLE"
		pr.Mergeable = &mergeable
	}

	// Parse timestamps
	if v, ok := data["createdAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			pr.CreatedAt = t
		}
	}
	if v, ok := data["updatedAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			pr.UpdatedAt = t
		}
	}
	if v, ok := data["closedAt"].(string); ok && v != "" {
		if t, err := github.ParseGitHubTime(v); err == nil {
			pr.ClosedAt = &t
		}
	}
	if v, ok := data["mergedAt"].(string); ok && v != "" {
		if t, err := github.ParseGitHubTime(v); err == nil {
			pr.MergedAt = &t
			// If mergedAt is present, the PR is merged
			pr.Merged = true
		}
	}

	return pr
}

// convertWorkflowJobFromMap converts a map from gh CLI JSON to WorkflowJobData.
func (c *CLIProvider) convertWorkflowJobFromMap(data map[string]interface{}, runID int64) *github.WorkflowJobData {
	job := &github.WorkflowJobData{
		RunID: runID,
	}

	if v, ok := data["databaseId"].(float64); ok {
		job.ID = int64(v)
	}
	if v, ok := data["name"].(string); ok {
		job.Name = v
	}
	if v, ok := data["status"].(string); ok {
		job.Status = strings.ToLower(v)
	}
	if v, ok := data["conclusion"].(string); ok {
		job.Conclusion = strings.ToLower(v)
	}

	// Parse timestamps
	if v, ok := data["startedAt"].(string); ok && v != "" {
		if t, err := github.ParseGitHubTime(v); err == nil {
			job.StartedAt = &t
		}
	}
	if v, ok := data["completedAt"].(string); ok && v != "" {
		if t, err := github.ParseGitHubTime(v); err == nil {
			job.CompletedAt = &t
		}
	}

	// Parse steps
	if steps, ok := data["steps"].([]interface{}); ok {
		job.Steps = c.parseWorkflowSteps(steps)
	}

	return job
}

// convertWorkflowRunFromMap converts a map from gh CLI JSON to WorkflowRunData.
func (c *CLIProvider) convertWorkflowRunFromMap(data map[string]interface{}) *github.WorkflowRunData {
	run := &github.WorkflowRunData{}

	if v, ok := data["databaseId"].(float64); ok {
		run.ID = int64(v)
	}
	if v, ok := data["name"].(string); ok {
		run.Name = v
	}
	if v, ok := data["workflowDatabaseId"].(float64); ok {
		run.WorkflowID = int64(v)
	}
	if v, ok := data["status"].(string); ok {
		run.Status = strings.ToLower(v)
	}
	if v, ok := data["conclusion"].(string); ok {
		run.Conclusion = strings.ToLower(v)
	}
	if v, ok := data["headBranch"].(string); ok {
		run.HeadBranch = v
	}
	if v, ok := data["headSha"].(string); ok {
		run.HeadSHA = v
	}
	if v, ok := data["number"].(float64); ok {
		run.RunNumber = int(v)
	}
	if v, ok := data["event"].(string); ok {
		run.Event = v
	}
	if v, ok := data["url"].(string); ok {
		run.HTMLURL = v
	}

	// Parse timestamps
	if v, ok := data["createdAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			run.CreatedAt = t
		}
	}
	if v, ok := data["updatedAt"].(string); ok {
		if t, err := github.ParseGitHubTime(v); err == nil {
			run.UpdatedAt = t
		}
	}

	return run
}

// getErrorCodeFromResult determines the error code based on the result.
func (c *CLIProvider) getErrorCodeFromResult(result *exec.Result) errors.ErrorCode {
	switch result.ExitCode {
	case 2:
		return errors.CodeUnauthorized
	case 4:
		return errors.CodeNotFound
	case 1:
		// Check stderr for specific error patterns
		stderr := strings.ToLower(result.Stderr)
		if strings.Contains(stderr, "not found") || strings.Contains(stderr, "could not resolve") {
			return errors.CodeNotFound
		}
		if strings.Contains(stderr, "authentication") || strings.Contains(stderr, "unauthorized") {
			return errors.CodeUnauthorized
		}
		if strings.Contains(stderr, "forbidden") || strings.Contains(stderr, "permission denied") {
			return errors.CodeForbidden
		}
		if strings.Contains(stderr, "rate limit") {
			return errors.CodeRateLimit
		}
	}
	return errors.CodeExecutionFailed
}

// parseIssueFromJSON parses issue data from gh CLI JSON output.
func (c *CLIProvider) parseIssueFromJSON(result *exec.Result) (*github.IssueData, error) {
	var apiResp map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	return c.convertIssueFromMap(apiResp), nil
}

// parseJSON unmarshals JSON from result stdout into the target.
func (c *CLIProvider) parseJSON(result *exec.Result, target interface{}) error {
	if err := json.Unmarshal([]byte(result.Stdout), target); err != nil {
		wrappedErr := errors.Wrap(err, errors.CodeInvalidInput, "failed to parse JSON response")
		wrappedErr = errors.WithContext(wrappedErr, "stdout", result.Stdout)
		return wrappedErr
	}
	return nil
}

// parsePRFromJSON parses PR data from gh CLI JSON output.
func (c *CLIProvider) parsePRFromJSON(result *exec.Result) (*github.PullRequestData, error) {
	var apiResp map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	return c.convertPRFromMap(apiResp), nil
}

// parseWorkflowRunFromJSON parses workflow run data from gh CLI JSON output.
func (c *CLIProvider) parseWorkflowRunFromJSON(result *exec.Result) (*github.WorkflowRunData, error) {
	var apiResp map[string]interface{}
	if err := c.parseJSON(result, &apiResp); err != nil {
		return nil, err
	}

	return c.convertWorkflowRunFromMap(apiResp), nil
}

// parseWorkflowSteps parses workflow step data from the steps array.
func (c *CLIProvider) parseWorkflowSteps(steps []interface{}) []github.WorkflowStepData {
	result := make([]github.WorkflowStepData, 0, len(steps))
	for i, s := range steps {
		stepMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		step := github.WorkflowStepData{
			Number: i + 1,
		}

		if v, ok := stepMap["name"].(string); ok {
			step.Name = v
		}
		if v, ok := stepMap["status"].(string); ok {
			step.Status = strings.ToLower(v)
		}
		if v, ok := stepMap["conclusion"].(string); ok {
			step.Conclusion = strings.ToLower(v)
		}
		if v, ok := stepMap["startedAt"].(string); ok && v != "" {
			if t, err := github.ParseGitHubTime(v); err == nil {
				step.StartedAt = &t
			}
		}
		if v, ok := stepMap["completedAt"].(string); ok && v != "" {
			if t, err := github.ParseGitHubTime(v); err == nil {
				step.CompletedAt = &t
			}
		}

		result = append(result, step)
	}
	return result
}

// wrapCLIError wraps CLI execution errors with appropriate error types.
func (c *CLIProvider) wrapCLIError(err error, result *exec.Result, message string) error {
	if err == nil {
		return nil
	}

	// Default to execution failed
	code := errors.CodeExecutionFailed

	// Map exit codes to error types
	if result != nil {
		code = c.getErrorCodeFromResult(result)
	}

	wrappedErr := errors.Wrap(err, code, message)

	// Include stderr in error details if available
	if result != nil && result.Stderr != "" {
		wrappedErr = errors.WithContext(wrappedErr, "stderr", result.Stderr)
		wrappedErr = errors.WithContext(wrappedErr, "exit_code", result.ExitCode)
	}

	return wrappedErr
}

// WithExecutor sets a custom executor for the CLI provider.
// This is primarily useful for testing with a mock executor.
func WithExecutor(executor exec.Executor) Option {
	return func(p *CLIProvider) error {
		if executor == nil {
			err := errors.New(errors.CodeInvalidInput, "executor cannot be nil")
			return errors.WithContext(err, "field", "executor")
		}
		p.wrapper = exec.NewWrapper(executor, "gh")
		return nil
	}
}

// wrapAuthError wraps authentication errors from gh CLI.
func wrapAuthError(err error, result *exec.Result) error {
	authErr := errors.Wrap(err, errors.CodeUnauthorized, "gh CLI not authenticated")
	authErr = errors.WithContext(authErr, "hint", "Run 'gh auth login' to authenticate")
	if result != nil && result.Stderr != "" {
		authErr = errors.WithContext(authErr, "stderr", result.Stderr)
	}
	return authErr
}
