package github

import "time"

// RepositoryData contains repository information from the provider.
type RepositoryData struct {
	// Identification
	ID       int64  `json:"id"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`

	// Metadata
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Archived      bool   `json:"archived"`

	// URLs
	CloneURL string `json:"clone_url"`
	SSHURL   string `json:"ssh_url"`
	HTMLURL  string `json:"html_url"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IssueData contains issue information from the provider.
type IssueData struct {
	// Identification
	Number int `json:"number"`

	// Content
	Title string `json:"title"`
	Body  string `json:"body"`

	// State and metadata
	State     string   `json:"state"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
	Milestone string   `json:"milestone"`

	// URL
	HTMLURL string `json:"html_url"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// PullRequestData contains pull request information from the provider.
type PullRequestData struct {
	// Identification
	Number int `json:"number"`

	// Content
	Title string `json:"title"`
	Body  string `json:"body"`

	// Branch information
	HeadRef string `json:"head_ref"`
	BaseRef string `json:"base_ref"`
	HeadSHA string `json:"head_sha"`

	// State and metadata
	State     string   `json:"state"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	Draft     bool     `json:"draft"`
	Mergeable *bool    `json:"mergeable,omitempty"`
	Merged    bool     `json:"merged"`

	// URL
	HTMLURL string `json:"html_url"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	MergedAt  *time.Time `json:"merged_at,omitempty"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// WorkflowRunData contains workflow run information.
type WorkflowRunData struct {
	// Identification
	ID         int64 `json:"id"`
	WorkflowID int64 `json:"workflow_id"`
	RunNumber  int   `json:"run_number"`

	// Content
	Name string `json:"name"`

	// Status and conclusion
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"` // Only set when Status is "completed"

	// Trigger information
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Event      string `json:"event"` // e.g., "push", "pull_request"

	// URL
	HTMLURL string `json:"html_url"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkflowJobData contains individual job information.
type WorkflowJobData struct {
	// Identification
	ID    int64 `json:"id"`
	RunID int64 `json:"run_id"`

	// Content
	Name string `json:"name"`

	// Status and conclusion
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"`

	// Steps
	Steps []WorkflowStepData `json:"steps"`

	// Timestamps
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// WorkflowStepData contains step information.
type WorkflowStepData struct {
	// Identification
	Number int `json:"number"`

	// Content
	Name string `json:"name"`

	// Status and conclusion
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"`

	// Timestamps
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// State constants for issues and pull requests.
const (
	// StateOpen indicates an issue or pull request is open.
	StateOpen = "open"

	// StateClosed indicates an issue or pull request is closed.
	StateClosed = "closed"

	// StateAll is used for filtering to include all states.
	StateAll = "all"
)

// Workflow run statuses.
const (
	// WorkflowStatusQueued indicates a workflow run is queued.
	WorkflowStatusQueued = "queued"

	// WorkflowStatusInProgress indicates a workflow run is in progress.
	WorkflowStatusInProgress = "in_progress"

	// WorkflowStatusCompleted indicates a workflow run is completed.
	WorkflowStatusCompleted = "completed"
)

// Workflow run conclusions.
const (
	// WorkflowConclusionSuccess indicates successful completion.
	WorkflowConclusionSuccess = "success"

	// WorkflowConclusionFailure indicates the workflow failed.
	WorkflowConclusionFailure = "failure"

	// WorkflowConclusionCancelled indicates the workflow was cancelled.
	WorkflowConclusionCancelled = "cancelled"

	// WorkflowConclusionSkipped indicates the workflow was skipped.
	WorkflowConclusionSkipped = "skipped"

	// WorkflowConclusionTimedOut indicates the workflow timed out.
	WorkflowConclusionTimedOut = "timed_out"

	// WorkflowConclusionActionRequired indicates action is required.
	WorkflowConclusionActionRequired = "action_required"

	// WorkflowConclusionNeutral indicates a neutral conclusion.
	WorkflowConclusionNeutral = "neutral"
)

// Merge methods for pull requests.
const (
	// MergeMethodMerge creates a merge commit.
	MergeMethodMerge = "merge"

	// MergeMethodSquash squashes all commits into one.
	MergeMethodSquash = "squash"

	// MergeMethodRebase rebases and merges commits.
	MergeMethodRebase = "rebase"
)

// ListOptions contains options for list operations.
type ListOptions struct {
	// Page is the page number for pagination (1-indexed)
	Page int

	// PerPage is the number of items per page
	PerPage int
}

// CreateRepositoryOptions contains options for creating a repository.
type CreateRepositoryOptions struct {
	// Name is the repository name (required)
	Name string

	// Description is the repository description
	Description string

	// Private indicates whether the repository should be private
	Private bool

	// AutoInit indicates whether to initialize with README
	AutoInit bool
}

// ListIssuesOptions contains options for listing issues.
type ListIssuesOptions struct {
	// State filters by issue state ("open", "closed", "all")
	State string

	// Labels filters by labels (all must match)
	Labels []string

	// Assignee filters by assignee username
	Assignee string

	// Since filters issues updated after this time
	Since *time.Time

	// ListOptions for pagination
	ListOptions
}

// CreateIssueOptions contains options for creating an issue.
type CreateIssueOptions struct {
	// Title is the issue title (required)
	Title string

	// Body is the issue description
	Body string

	// Labels is the list of labels to apply
	Labels []string

	// Assignees is the list of usernames to assign
	Assignees []string

	// Milestone is the milestone name to assign
	Milestone string
}

// UpdateIssueOptions contains options for updating an issue.
type UpdateIssueOptions struct {
	// Title is the new issue title
	Title *string

	// Body is the new issue body
	Body *string

	// State is the new issue state ("open" or "closed")
	State *string

	// Labels is the new list of labels (replaces existing)
	Labels []string

	// Assignees is the new list of assignees (replaces existing)
	Assignees []string
}

// ListPullRequestsOptions contains options for listing pull requests.
type ListPullRequestsOptions struct {
	// State filters by pull request state ("open", "closed", "all")
	State string

	// Head filters by head branch (format: "user:ref-name" or "ref-name")
	Head string

	// Base filters by base branch
	Base string

	// ListOptions for pagination
	ListOptions
}

// CreatePullRequestOptions contains options for creating a pull request.
type CreatePullRequestOptions struct {
	// Title is the pull request title (required)
	Title string

	// Body is the pull request description
	Body string

	// Head is the name of the branch where changes are implemented (required)
	Head string

	// Base is the name of the branch to merge into (required)
	Base string

	// Draft indicates whether to create as a draft pull request
	Draft bool

	// MaintainerCanModify indicates whether maintainers can modify the pull request
	MaintainerCanModify bool
}

// UpdatePullRequestOptions contains options for updating a pull request.
type UpdatePullRequestOptions struct {
	// Title is the new pull request title
	Title *string

	// Body is the new pull request body
	Body *string

	// State is the new pull request state ("open" or "closed")
	State *string

	// Base is the new base branch
	Base *string
}

// MergePullRequestOptions contains options for merging a pull request.
type MergePullRequestOptions struct {
	// MergeMethod is the merge method to use ("merge", "squash", "rebase")
	MergeMethod string

	// CommitTitle is the title for the merge commit
	CommitTitle string

	// CommitMessage is the message for the merge commit
	CommitMessage string
}

// ListWorkflowRunsOptions contains options for listing workflow runs.
type ListWorkflowRunsOptions struct {
	// Branch filters by branch name
	Branch string

	// Event filters by event type (e.g., "push", "pull_request")
	Event string

	// Status filters by status ("queued", "in_progress", "completed")
	Status string

	// ListOptions for pagination
	ListOptions
}
