package github

// This file contains option types and helper functions for the options pattern
// used throughout the library. Options provide a flexible way to configure
// operations without requiring large parameter lists.

// Note: Most option types are defined in types.go alongside the data types they configure.
// This file contains additional helper functions and patterns.

// IssueOption configures issue creation.
type IssueOption func(*CreateIssueOptions)

// WithLabels sets labels for an issue.
func WithLabels(labels ...string) IssueOption {
	return func(opts *CreateIssueOptions) {
		opts.Labels = labels
	}
}

// WithAssignees sets assignees for an issue.
func WithAssignees(assignees ...string) IssueOption {
	return func(opts *CreateIssueOptions) {
		opts.Assignees = assignees
	}
}

// WithMilestone sets the milestone for an issue.
func WithMilestone(milestone string) IssueOption {
	return func(opts *CreateIssueOptions) {
		opts.Milestone = milestone
	}
}

// IssueFilterOption configures issue filtering.
type IssueFilterOption func(*ListIssuesOptions)

// WithState filters issues by state ("open", "closed", "all").
func WithState(state string) IssueFilterOption {
	return func(opts *ListIssuesOptions) {
		opts.State = state
	}
}

// WithIssueLabels filters issues by labels (all must match).
func WithIssueLabels(labels ...string) IssueFilterOption {
	return func(opts *ListIssuesOptions) {
		opts.Labels = labels
	}
}

// WithAssignee filters issues by assignee username.
func WithAssignee(assignee string) IssueFilterOption {
	return func(opts *ListIssuesOptions) {
		opts.Assignee = assignee
	}
}

// IssueUpdateOption configures issue updates.
type IssueUpdateOption func(*UpdateIssueOptions)

// WithTitle sets a new title for an issue.
func WithTitle(title string) IssueUpdateOption {
	return func(opts *UpdateIssueOptions) {
		opts.Title = &title
	}
}

// WithBody sets a new body for an issue.
func WithBody(body string) IssueUpdateOption {
	return func(opts *UpdateIssueOptions) {
		opts.Body = &body
	}
}

// WithIssueState sets a new state for an issue.
func WithIssueState(state string) IssueUpdateOption {
	return func(opts *UpdateIssueOptions) {
		opts.State = &state
	}
}

// PRFilterOption configures pull request filtering.
type PRFilterOption func(*ListPullRequestsOptions)

// WithPRState filters pull requests by state ("open", "closed", "all").
func WithPRState(state string) PRFilterOption {
	return func(opts *ListPullRequestsOptions) {
		opts.State = state
	}
}

// WithHead filters pull requests by head branch.
func WithHead(head string) PRFilterOption {
	return func(opts *ListPullRequestsOptions) {
		opts.Head = head
	}
}

// WithBase filters pull requests by base branch.
func WithBase(base string) PRFilterOption {
	return func(opts *ListPullRequestsOptions) {
		opts.Base = base
	}
}

// PRUpdateOption configures pull request updates.
type PRUpdateOption func(*UpdatePullRequestOptions)

// WithPRTitle sets a new title for a pull request.
func WithPRTitle(title string) PRUpdateOption {
	return func(opts *UpdatePullRequestOptions) {
		opts.Title = &title
	}
}

// WithPRBody sets a new body for a pull request.
func WithPRBody(body string) PRUpdateOption {
	return func(opts *UpdatePullRequestOptions) {
		opts.Body = &body
	}
}

// WithPRUpdateState sets a new state for a pull request.
func WithPRUpdateState(state string) PRUpdateOption {
	return func(opts *UpdatePullRequestOptions) {
		opts.State = &state
	}
}

// WithPRBase sets a new base branch for a pull request.
func WithPRBase(base string) PRUpdateOption {
	return func(opts *UpdatePullRequestOptions) {
		opts.Base = &base
	}
}

// MergeOption configures pull request merging.
type MergeOption func(*MergePullRequestOptions)

// WithMergeMethod sets the merge method ("merge", "squash", "rebase").
func WithMergeMethod(method string) MergeOption {
	return func(opts *MergePullRequestOptions) {
		opts.MergeMethod = method
	}
}

// WithCommitTitle sets the title for the merge commit.
func WithCommitTitle(title string) MergeOption {
	return func(opts *MergePullRequestOptions) {
		opts.CommitTitle = title
	}
}

// WithCommitMessage sets the message for the merge commit.
func WithCommitMessage(message string) MergeOption {
	return func(opts *MergePullRequestOptions) {
		opts.CommitMessage = message
	}
}

// WorkflowFilterOption configures workflow run filtering.
type WorkflowFilterOption func(*ListWorkflowRunsOptions)

// WithBranch filters workflow runs by branch name.
func WithBranch(branch string) WorkflowFilterOption {
	return func(opts *ListWorkflowRunsOptions) {
		opts.Branch = branch
	}
}

// WithEvent filters workflow runs by event type.
func WithEvent(event string) WorkflowFilterOption {
	return func(opts *ListWorkflowRunsOptions) {
		opts.Event = event
	}
}

// WithStatus filters workflow runs by status.
func WithStatus(status string) WorkflowFilterOption {
	return func(opts *ListWorkflowRunsOptions) {
		opts.Status = status
	}
}
