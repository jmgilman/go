package github

import "context"

// Issue represents a GitHub issue.
//
// Issue instances are typically created through a Repository:
//
//	repo := client.Repository("myrepo")
//	issue, err := repo.CreateIssue(ctx, "Bug title", "Description",
//	    github.WithLabels("bug"),
//	)
//
// Or by retrieving an existing issue:
//
//	issues, err := repo.ListIssues(ctx, github.WithState("open"))
//	for _, issue := range issues {
//	    fmt.Println(issue.Title())
//	}
type Issue struct {
	client *Client
	owner  string
	repo   string
	data   *IssueData
}

// Refresh refreshes the issue data from GitHub.
func (i *Issue) Refresh(ctx context.Context) error {
	data, err := i.client.provider.GetIssue(ctx, i.owner, i.repo, i.data.Number)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to refresh issue")
	}
	i.data = data
	return nil
}

// Update updates the issue with the provided options.
// Only non-nil fields in the options are updated.
//
// Example:
//
//	err := issue.Update(ctx,
//	    github.WithTitle("New title"),
//	    github.WithIssueState("closed"),
//	)
func (i *Issue) Update(ctx context.Context, opts ...IssueUpdateOption) error {
	updateOpts := &UpdateIssueOptions{}
	for _, opt := range opts {
		opt(updateOpts)
	}

	data, err := i.client.provider.UpdateIssue(ctx, i.owner, i.repo, i.data.Number, *updateOpts)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to update issue")
	}
	i.data = data
	return nil
}

// Close closes the issue.
func (i *Issue) Close(ctx context.Context) error {
	if err := i.client.provider.CloseIssue(ctx, i.owner, i.repo, i.data.Number); err != nil {
		return WrapHTTPError(err, 0, "failed to close issue")
	}
	// Update local state
	i.data.State = "closed"
	return nil
}

// AddLabels adds labels to the issue.
// Labels that don't exist will be created.
func (i *Issue) AddLabels(ctx context.Context, labels ...string) error {
	if err := i.client.provider.AddLabels(ctx, i.owner, i.repo, i.data.Number, labels); err != nil {
		return WrapHTTPError(err, 0, "failed to add labels to issue")
	}

	// Update local labels
	labelSet := make(map[string]bool)
	for _, label := range i.data.Labels {
		labelSet[label] = true
	}
	for _, label := range labels {
		labelSet[label] = true
	}

	newLabels := make([]string, 0, len(labelSet))
	for label := range labelSet {
		newLabels = append(newLabels, label)
	}
	i.data.Labels = newLabels

	return nil
}

// RemoveLabel removes a label from the issue.
// No error if the label wasn't applied to the issue.
func (i *Issue) RemoveLabel(ctx context.Context, label string) error {
	if err := i.client.provider.RemoveLabel(ctx, i.owner, i.repo, i.data.Number, label); err != nil {
		return WrapHTTPError(err, 0, "failed to remove label from issue")
	}

	// Update local labels
	newLabels := make([]string, 0, len(i.data.Labels))
	for _, l := range i.data.Labels {
		if l != label {
			newLabels = append(newLabels, l)
		}
	}
	i.data.Labels = newLabels

	return nil
}

// Number returns the issue number.
func (i *Issue) Number() int {
	return i.data.Number
}

// Title returns the issue title.
func (i *Issue) Title() string {
	return i.data.Title
}

// Body returns the issue body/description.
func (i *Issue) Body() string {
	return i.data.Body
}

// State returns the issue state ("open" or "closed").
func (i *Issue) State() string {
	return i.data.State
}

// Author returns the username of the issue creator.
func (i *Issue) Author() string {
	return i.data.Author
}

// Labels returns the list of labels applied to the issue.
func (i *Issue) Labels() []string {
	return i.data.Labels
}

// Assignees returns the list of usernames assigned to the issue.
func (i *Issue) Assignees() []string {
	return i.data.Assignees
}

// Milestone returns the milestone name (empty string if no milestone).
func (i *Issue) Milestone() string {
	return i.data.Milestone
}

// HTMLURL returns the URL to view the issue on GitHub.
func (i *Issue) HTMLURL() string {
	return i.data.HTMLURL
}

// IsClosed returns true if the issue is closed.
func (i *Issue) IsClosed() bool {
	return i.data.State == "closed"
}

// IsOpen returns true if the issue is open.
func (i *Issue) IsOpen() bool {
	return i.data.State == "open"
}

// Data returns the underlying issue data.
// This provides access to all issue fields including timestamps.
func (i *Issue) Data() *IssueData {
	return i.data
}
