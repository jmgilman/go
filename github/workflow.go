package github

import (
	"context"
	"time"

	"github.com/jmgilman/go/errors"
)

// WorkflowRun represents a GitHub Actions workflow run.
//
// WorkflowRun instances are typically created through a Repository:
//
//	repo := client.Repository("myrepo")
//	run, err := repo.GetWorkflowRun(ctx, 123456789)
//
// Or by listing workflow runs:
//
//	runs, err := repo.ListWorkflowRuns(ctx, github.WithWorkflowStatus("completed"))
//	for _, run := range runs {
//	    fmt.Printf("Run #%d: %s - %s\n", run.RunNumber(), run.Status(), run.Conclusion())
//	}
type WorkflowRun struct {
	client *Client
	owner  string
	repo   string
	data   *WorkflowRunData
}

// Refresh refreshes the workflow run data from GitHub.
func (wr *WorkflowRun) Refresh(ctx context.Context) error {
	data, err := wr.client.provider.GetWorkflowRun(ctx, wr.owner, wr.repo, wr.data.ID)
	if err != nil {
		return WrapHTTPError(err, 0, "failed to refresh workflow run")
	}
	wr.data = data
	return nil
}

// GetJobs retrieves the jobs for this workflow run.
func (wr *WorkflowRun) GetJobs(ctx context.Context) ([]*WorkflowJob, error) {
	jobsData, err := wr.client.provider.GetWorkflowRunJobs(ctx, wr.owner, wr.repo, wr.data.ID)
	if err != nil {
		return nil, WrapHTTPError(err, 0, "failed to get workflow run jobs")
	}

	jobs := make([]*WorkflowJob, len(jobsData))
	for i, data := range jobsData {
		jobs[i] = &WorkflowJob{
			client: wr.client,
			owner:  wr.owner,
			repo:   wr.repo,
			data:   data,
		}
	}

	return jobs, nil
}

// Wait polls the workflow run until it completes or the context is cancelled.
// The pollInterval parameter specifies how often to check the status.
//
// Example:
//
//	// Wait for workflow to complete, checking every 10 seconds
//	err := run.Wait(ctx, 10*time.Second)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if run.IsSuccessful() {
//	    fmt.Println("Workflow succeeded!")
//	}
func (wr *WorkflowRun) Wait(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second // default
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check current status first
	if wr.IsComplete() {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), errors.CodeTimeout, "workflow run wait cancelled or timed out")
		case <-ticker.C:
			if err := wr.Refresh(ctx); err != nil {
				return err
			}
			if wr.IsComplete() {
				return nil
			}
		}
	}
}

// ID returns the unique identifier for the workflow run.
func (wr *WorkflowRun) ID() int64 {
	return wr.data.ID
}

// Name returns the name of the workflow.
func (wr *WorkflowRun) Name() string {
	return wr.data.Name
}

// WorkflowID returns the ID of the workflow definition.
func (wr *WorkflowRun) WorkflowID() int64 {
	return wr.data.WorkflowID
}

// Status returns the run status ("queued", "in_progress", "completed").
func (wr *WorkflowRun) Status() string {
	return wr.data.Status
}

// Conclusion returns the run conclusion ("success", "failure", "cancelled", etc.).
// Only meaningful when Status() is "completed".
func (wr *WorkflowRun) Conclusion() string {
	return wr.data.Conclusion
}

// HeadBranch returns the branch that triggered the workflow.
func (wr *WorkflowRun) HeadBranch() string {
	return wr.data.HeadBranch
}

// HeadSHA returns the commit SHA that triggered the workflow.
func (wr *WorkflowRun) HeadSHA() string {
	return wr.data.HeadSHA
}

// RunNumber returns the sequential run number for this workflow.
func (wr *WorkflowRun) RunNumber() int {
	return wr.data.RunNumber
}

// Event returns the event that triggered the workflow (e.g., "push", "pull_request").
func (wr *WorkflowRun) Event() string {
	return wr.data.Event
}

// HTMLURL returns the URL to view the workflow run on GitHub.
func (wr *WorkflowRun) HTMLURL() string {
	return wr.data.HTMLURL
}

// IsComplete returns true if the workflow run has completed.
func (wr *WorkflowRun) IsComplete() bool {
	return wr.data.Status == WorkflowStatusCompleted
}

// IsSuccessful returns true if the workflow run completed successfully.
func (wr *WorkflowRun) IsSuccessful() bool {
	return wr.IsComplete() && wr.data.Conclusion == WorkflowConclusionSuccess
}

// IsFailed returns true if the workflow run failed.
func (wr *WorkflowRun) IsFailed() bool {
	return wr.IsComplete() && wr.data.Conclusion == WorkflowConclusionFailure
}

// IsCancelled returns true if the workflow run was cancelled.
func (wr *WorkflowRun) IsCancelled() bool {
	return wr.IsComplete() && wr.data.Conclusion == WorkflowConclusionCancelled
}

// IsQueued returns true if the workflow run is queued.
func (wr *WorkflowRun) IsQueued() bool {
	return wr.data.Status == WorkflowStatusQueued
}

// IsInProgress returns true if the workflow run is in progress.
func (wr *WorkflowRun) IsInProgress() bool {
	return wr.data.Status == WorkflowStatusInProgress
}

// Data returns the underlying workflow run data.
// This provides access to all workflow run fields including timestamps.
func (wr *WorkflowRun) Data() *WorkflowRunData {
	return wr.data
}

// WorkflowJob represents a GitHub Actions workflow job.
//
// WorkflowJob instances are typically retrieved through a WorkflowRun:
//
//	run, err := repo.GetWorkflowRun(ctx, 123456789)
//	jobs, err := run.GetJobs(ctx)
//	for _, job := range jobs {
//	    fmt.Printf("Job: %s - %s\n", job.Name(), job.Conclusion())
//	}
type WorkflowJob struct {
	client *Client
	owner  string
	repo   string
	data   *WorkflowJobData
}

// ID returns the unique identifier for the job.
func (wj *WorkflowJob) ID() int64 {
	return wj.data.ID
}

// RunID returns the workflow run ID this job belongs to.
func (wj *WorkflowJob) RunID() int64 {
	return wj.data.RunID
}

// Name returns the name of the job.
func (wj *WorkflowJob) Name() string {
	return wj.data.Name
}

// Status returns the job status ("queued", "in_progress", "completed").
func (wj *WorkflowJob) Status() string {
	return wj.data.Status
}

// Conclusion returns the job conclusion ("success", "failure", "cancelled", etc.).
func (wj *WorkflowJob) Conclusion() string {
	return wj.data.Conclusion
}

// StartedAt returns when the job started (nil if not started).
func (wj *WorkflowJob) StartedAt() *time.Time {
	return wj.data.StartedAt
}

// CompletedAt returns when the job completed (nil if not completed).
func (wj *WorkflowJob) CompletedAt() *time.Time {
	return wj.data.CompletedAt
}

// Steps returns the list of steps in the job.
func (wj *WorkflowJob) Steps() []WorkflowStepData {
	return wj.data.Steps
}

// IsComplete returns true if the job has completed.
func (wj *WorkflowJob) IsComplete() bool {
	return wj.data.Status == WorkflowStatusCompleted
}

// IsSuccessful returns true if the job completed successfully.
func (wj *WorkflowJob) IsSuccessful() bool {
	return wj.IsComplete() && wj.data.Conclusion == WorkflowConclusionSuccess
}

// IsFailed returns true if the job failed.
func (wj *WorkflowJob) IsFailed() bool {
	return wj.IsComplete() && wj.data.Conclusion == WorkflowConclusionFailure
}

// IsCancelled returns true if the job was cancelled.
func (wj *WorkflowJob) IsCancelled() bool {
	return wj.IsComplete() && wj.data.Conclusion == WorkflowConclusionCancelled
}

// Data returns the underlying workflow job data.
// This provides access to all job fields including step details.
func (wj *WorkflowJob) Data() *WorkflowJobData {
	return wj.data
}
