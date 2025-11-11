package github

import "context"

// PullRequest represents a GitHub pull request.
//
// PullRequest instances are typically created through a Repository:
//
//	repo := client.Repository("myrepo")
//	pr, err := repo.CreatePullRequest(ctx, github.CreatePullRequestOptions{
//	    Title: "Add new feature",
//	    Body:  "Description of changes",
//	    Head:  "feature-branch",
//	    Base:  "main",
//	})
//
// Or by retrieving existing pull requests:
//
//	prs, err := repo.ListPullRequests(ctx, github.WithPRState("open"))
//	for _, pr := range prs {
//	    fmt.Println(pr.Title())
//	}
type PullRequest struct {
	client *Client
	owner  string
	repo   string
	data   *PullRequestData
}

// Refresh refreshes the pull request data from GitHub.
func (pr *PullRequest) Refresh(ctx context.Context) error {
	data, err := pr.client.provider.GetPullRequest(ctx, pr.owner, pr.repo, pr.data.Number)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to refresh pull request")
	}
	pr.data = data
	return nil
}

// Update updates the pull request with the provided options.
// Only non-nil fields in the options are updated.
//
// Example:
//
//	err := pr.Update(ctx,
//	    github.WithPRTitle("Updated title"),
//	    github.WithPRBody("Updated description"),
//	)
func (pr *PullRequest) Update(ctx context.Context, opts ...PRUpdateOption) error {
	updateOpts := &UpdatePullRequestOptions{}
	for _, opt := range opts {
		opt(updateOpts)
	}

	data, err := pr.client.provider.UpdatePullRequest(ctx, pr.owner, pr.repo, pr.data.Number, *updateOpts)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to update pull request")
	}
	pr.data = data
	return nil
}

// Merge merges the pull request with the provided options.
//
// Example:
//
//	err := pr.Merge(ctx,
//	    github.WithMergeMethod("squash"),
//	    github.WithCommitMessage("Merge feature branch"),
//	)
func (pr *PullRequest) Merge(ctx context.Context, opts ...MergeOption) error {
	mergeOpts := &MergePullRequestOptions{
		MergeMethod: MergeMethodMerge, // default
	}
	for _, opt := range opts {
		opt(mergeOpts)
	}

	if err := pr.client.provider.MergePullRequest(ctx, pr.owner, pr.repo, pr.data.Number, *mergeOpts); err != nil {
		return WrapHTTPError(err, 0, "failed to merge pull request")
	}

	// Update local state
	pr.data.Merged = true
	pr.data.State = "closed"

	return nil
}

// AddLabels adds labels to the pull request.
// Labels that don't exist will be created.
func (pr *PullRequest) AddLabels(ctx context.Context, labels ...string) error {
	// PRs use the same label API as issues
	if err := pr.client.provider.AddLabels(ctx, pr.owner, pr.repo, pr.data.Number, labels); err != nil {
		return WrapHTTPError(err, 0, "failed to add labels to pull request")
	}

	// Update local labels
	labelSet := make(map[string]bool)
	for _, label := range pr.data.Labels {
		labelSet[label] = true
	}
	for _, label := range labels {
		labelSet[label] = true
	}

	newLabels := make([]string, 0, len(labelSet))
	for label := range labelSet {
		newLabels = append(newLabels, label)
	}
	pr.data.Labels = newLabels

	return nil
}

// RemoveLabel removes a label from the pull request.
// No error if the label wasn't applied to the pull request.
func (pr *PullRequest) RemoveLabel(ctx context.Context, label string) error {
	if err := pr.client.provider.RemoveLabel(ctx, pr.owner, pr.repo, pr.data.Number, label); err != nil {
		return WrapHTTPError(err, 0, "failed to remove label from pull request")
	}

	// Update local labels
	newLabels := make([]string, 0, len(pr.data.Labels))
	for _, l := range pr.data.Labels {
		if l != label {
			newLabels = append(newLabels, l)
		}
	}
	pr.data.Labels = newLabels

	return nil
}

// Number returns the pull request number.
func (pr *PullRequest) Number() int {
	return pr.data.Number
}

// Title returns the pull request title.
func (pr *PullRequest) Title() string {
	return pr.data.Title
}

// Body returns the pull request body/description.
func (pr *PullRequest) Body() string {
	return pr.data.Body
}

// State returns the pull request state ("open", "closed", or "merged").
func (pr *PullRequest) State() string {
	return pr.data.State
}

// Author returns the username of the pull request creator.
func (pr *PullRequest) Author() string {
	return pr.data.Author
}

// HeadRef returns the head branch name.
func (pr *PullRequest) HeadRef() string {
	return pr.data.HeadRef
}

// BaseRef returns the base branch name.
func (pr *PullRequest) BaseRef() string {
	return pr.data.BaseRef
}

// HeadSHA returns the commit SHA of the head branch.
func (pr *PullRequest) HeadSHA() string {
	return pr.data.HeadSHA
}

// Labels returns the list of labels applied to the pull request.
func (pr *PullRequest) Labels() []string {
	return pr.data.Labels
}

// IsDraft returns true if the pull request is a draft.
func (pr *PullRequest) IsDraft() bool {
	return pr.data.Draft
}

// IsMergeable returns true if the pull request can be merged.
// Returns false if mergeable status is unknown or if it can't be merged.
func (pr *PullRequest) IsMergeable() bool {
	return pr.data.Mergeable != nil && *pr.data.Mergeable
}

// IsMerged returns true if the pull request has been merged.
func (pr *PullRequest) IsMerged() bool {
	return pr.data.Merged
}

// IsClosed returns true if the pull request is closed (either merged or closed without merging).
func (pr *PullRequest) IsClosed() bool {
	return pr.data.State == "closed" || pr.data.Merged
}

// IsOpen returns true if the pull request is open.
func (pr *PullRequest) IsOpen() bool {
	return pr.data.State == "open"
}

// HTMLURL returns the URL to view the pull request on GitHub.
func (pr *PullRequest) HTMLURL() string {
	return pr.data.HTMLURL
}

// Data returns the underlying pull request data.
// This provides access to all pull request fields including timestamps.
func (pr *PullRequest) Data() *PullRequestData {
	return pr.data
}
