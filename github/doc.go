// Package github provides a clean, idiomatic wrapper around GitHub operations.
//
// This library provides two pluggable backend implementations: one using the official
// Go GitHub SDK (go-github) and another using the gh CLI tool. The provider abstraction
// allows users to choose the implementation that best fits their needs while providing
// a consistent, high-level API for GitHub operations.
//
// # Architecture
//
// The library is built on several key principles:
//
//  1. Provider abstraction through the Provider interface
//  2. Two concrete provider implementations (SDK and CLI)
//  3. High-level types (Client, Repository, Issue, PullRequest, WorkflowRun)
//  4. Escape hatches for accessing underlying implementations
//  5. Consistent error handling using workspace errors library
//  6. Context support for cancellation and timeouts
//
// # Core Types
//
// Client is the main entry point and provides access to GitHub resources.
//
// Repository represents a GitHub repository with methods for repository operations.
//
// Provider interface abstracts the underlying implementation (SDK or CLI).
//
// # Provider Implementations
//
// ## SDK Provider
//
// Uses the official google/go-github SDK for GitHub API operations.
// Best for applications that need fine-grained control or are already using go-github.
//
// Authentication options:
//   - Personal access token
//   - Custom GitHub client (for advanced auth like GitHub Apps)
//
// ## CLI Provider
//
// Uses the gh CLI tool for GitHub operations.
// Best for scripts, automation, or environments where gh CLI is already configured.
// Inherits authentication from gh CLI configuration.
//
// # Usage Examples
//
// ## Example 1: Using SDK Provider with Token
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "log"
//
//	    "github.com/jmgilman/go/github"
//	    "github.com/jmgilman/go/github/providers/sdk"
//	)
//
//	func main() {
//	    // Create SDK provider with personal access token
//	    provider, err := sdk.NewSDKProvider(
//	        sdk.WithToken("ghp_xxxxxxxxxxxx"),
//	    )
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Create client for organization
//	    client := github.NewClient(provider, "myorg")
//
//	    // Access repository
//	    repo := client.Repository("myrepo")
//
//	    // Fetch repository data
//	    ctx := context.Background()
//	    if err := repo.Get(ctx); err != nil {
//	        log.Fatal(err)
//	    }
//
//	    fmt.Printf("Repository: %s\n", repo.FullName())
//	    fmt.Printf("Description: %s\n", repo.Description())
//	    fmt.Printf("Default Branch: %s\n", repo.DefaultBranch())
//	    fmt.Printf("Clone URL: %s\n", repo.CloneURL())
//	}
//
// ## Example 2: Using CLI Provider
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "log"
//
//	    "github.com/jmgilman/go/github"
//	    "github.com/jmgilman/go/github/providers/cli"
//	)
//
//	func main() {
//	    // Create CLI provider (uses gh CLI authentication)
//	    provider, err := cli.NewCLIProvider()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Create GitHub client
//	    ghClient := github.NewClient(provider, "myorg")
//
//	    // List repositories
//	    ctx := context.Background()
//	    repos, err := client.Provider().ListRepositories(ctx, "myorg", github.ListOptions{
//	        PerPage: 10,
//	    })
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    for _, repo := range repos {
//	        fmt.Printf("- %s\n", repo.FullName)
//	    }
//	}
//
// ## Example 3: Creating and Managing Issues (Phase 2)
//
//	func manageIssues(ctx context.Context, client *github.Client) error {
//	    repo := client.Repository("myrepo")
//
//	    // Create issue with labels
//	    issue, err := repo.CreateIssue(ctx,
//	        "Bug: Login form validation",
//	        "The login form doesn't validate email addresses properly.",
//	        github.WithLabels("bug", "high-priority"),
//	        github.WithAssignees("developer1"),
//	    )
//	    if err != nil {
//	        return err
//	    }
//
//	    fmt.Printf("Created issue #%d: %s\n", issue.Number(), issue.HTMLURL())
//
//	    // List open issues with specific labels
//	    issues, err := repo.ListIssues(ctx,
//	        github.WithState("open"),
//	        github.WithIssueLabels("bug"),
//	    )
//	    if err != nil {
//	        return err
//	    }
//
//	    for _, issue := range issues {
//	        fmt.Printf("#%d: %s\n", issue.Number(), issue.Title())
//	    }
//
//	    return nil
//	}
//
// ## Example 4: Managing Pull Requests (Phase 2)
//
//	func managePullRequests(ctx context.Context, client *github.Client) error {
//	    repo := client.Repository("myrepo")
//
//	    // Create pull request
//	    pr, err := repo.CreatePullRequest(ctx, github.CreatePullRequestOptions{
//	        Title: "Add new feature",
//	        Body:  "This PR adds a new feature that...",
//	        Head:  "feature-branch",
//	        Base:  "main",
//	        Draft: false,
//	    })
//	    if err != nil {
//	        return err
//	    }
//
//	    fmt.Printf("Created PR #%d: %s\n", pr.Number(), pr.HTMLURL())
//
//	    // Wait for checks to complete, then merge
//	    if err := pr.Refresh(ctx); err != nil {
//	        return err
//	    }
//
//	    if pr.IsMerged() {
//	        fmt.Println("Already merged")
//	        return nil
//	    }
//
//	    // Merge with squash
//	    err = pr.Merge(ctx,
//	        github.WithMergeMethod("squash"),
//	        github.WithCommitMessage("Merge feature branch"),
//	    )
//	    if err != nil {
//	        return err
//	    }
//
//	    fmt.Println("Pull request merged successfully")
//	    return nil
//	}
//
// ## Example 5: Monitoring Workflows (Phase 3)
//
//	func monitorWorkflow(ctx context.Context, repo *github.Repository, runID int64) error {
//	    // Get workflow run
//	    run, err := repo.GetWorkflowRun(ctx, runID)
//	    if err != nil {
//	        return err
//	    }
//
//	    fmt.Printf("Workflow: %s\n", run.Name())
//	    fmt.Printf("Status: %s\n", run.Status())
//
//	    // Poll until complete
//	    if err := run.Wait(ctx, 10*time.Second); err != nil {
//	        return err
//	    }
//
//	    // Check result
//	    if !run.IsSuccessful() {
//	        // Get job details
//	        jobs, err := run.GetJobs(ctx)
//	        if err != nil {
//	            return err
//	        }
//
//	        fmt.Println("Failed jobs:")
//	        for _, job := range jobs {
//	            if job.Conclusion != github.WorkflowConclusionSuccess {
//	                fmt.Printf("  - %s: %s\n", job.Name, job.Conclusion)
//	            }
//	        }
//
//	        return fmt.Errorf("workflow failed: %s", run.Conclusion())
//	    }
//
//	    fmt.Println("Workflow completed successfully")
//	    return nil
//	}
//
// # Error Handling
//
// The library uses the workspace errors library for consistent error handling.
// All errors are wrapped with appropriate error codes:
//
//   - ErrCodeNotFound: Repository, issue, PR, or workflow run not found
//   - ErrCodeAuthenticationFailed: Invalid or missing authentication
//   - ErrCodePermissionDenied: Insufficient permissions
//   - ErrCodeRateLimited: API rate limit exceeded
//   - ErrCodeInvalidInput: Invalid parameters
//   - ErrCodeConflict: Resource conflict (already exists, merge conflict, etc.)
//   - ErrCodeNetwork: Network or connectivity issues
//   - ErrCodeInternal: Internal errors or unexpected responses
//
// Example error handling:
//
//	repo := client.Repository("myrepo")
//	if err := repo.Get(ctx); err != nil {
//	    var platformErr errors.PlatformError
//	    if errors.As(err, &platformErr) {
//	        switch platformErr.Code() {
//	        case errors.CodeNotFound:
//	            fmt.Println("Repository not found")
//	        case errors.CodeUnauthorized:
//	            fmt.Println("Authentication failed")
//	        case errors.CodeForbidden:
//	            fmt.Println("Access denied")
//	        default:
//	            fmt.Printf("Error: %v\n", err)
//	        }
//	    }
//	    return err
//	}
//
// # Context and Cancellation
//
// All operations that interact with GitHub accept a context.Context parameter
// for cancellation and timeout control:
//
//	// With timeout
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	repo := client.Repository("myrepo")
//	if err := repo.Get(ctx); err != nil {
//	    // Handle timeout or other errors
//	    return err
//	}
//
//	// With cancellation
//	ctx, cancel := context.WithCancel(context.Background())
//	go func() {
//	    // Cancel after some condition
//	    <-someChannel
//	    cancel()
//	}()
//
//	repos, err := client.Provider().ListRepositories(ctx, "myorg", github.ListOptions{})
//
// # Testing
//
// The library provides testing utilities in the testutil sub-package:
//
//	import "github.com/jmgilman/go/github/testutil"
//
//	// Mock provider for testing
//	mockProvider := testutil.NewMockProvider()
//	mockProvider.OnGetRepository = func(ctx context.Context, owner, repo string) (*github.RepositoryData, error) {
//	    return &github.RepositoryData{
//	        ID:   123,
//	        Name: repo,
//	        Owner: owner,
//	    }, nil
//	}
//
//	client := github.NewClient(mockProvider, "testorg")
//	// Test your code with the mock provider
//
// For testing CLI provider without gh CLI installed:
//
//	mockExecutor := testutil.NewMockExecutor()
//	mockExecutor.OnRun = func(args ...string) (*exec.Result, error) {
//	    return &exec.Result{
//	        Stdout: `{"id": 123, "name": "test"}`,
//	    }, nil
//	}
//
//	provider, err := cli.NewCLIProvider(cli.WithExecutor(mockExecutor))
//
// # Provider Interface
//
// The Provider interface allows direct access to lower-level operations:
//
//	// Access provider directly for operations not wrapped by high-level types
//	provider := client.Provider()
//
//	// Direct provider calls
//	repos, err := provider.ListRepositories(ctx, "myorg", github.ListOptions{
//	    Page: 1,
//	    PerPage: 50,
//	})
//
// # Implementation Status
//
// Phase 1 (Foundation) - Complete:
//   - Provider interface
//   - SDKProvider with repository operations
//   - Client and Repository types
//   - Error handling and options patterns
//
// Phase 2 (Core Resources) - Planned:
//   - Issue type and operations
//   - PullRequest type and operations
//
// Phase 3 (Workflows) - Planned:
//   - WorkflowRun and WorkflowJob types
//   - Workflow triggering and monitoring
//
// Phase 4 (CLI Provider) - Planned:
//   - CLIProvider implementation
//   - CLI-based operations for all resource types
//
// # Dependencies
//
// This library depends on:
//   - github.com/google/go-github/v67 - Official GitHub SDK (for SDKProvider)
//   - github.com/jmgilman/go/errors - Workspace errors library
//   - github.com/jmgilman/go/exec - Workspace exec library (for CLIProvider)
//
// # References
//
// For more information:
//   - GitHub REST API: https://docs.github.com/en/rest
//   - go-github SDK: https://pkg.go.dev/github.com/google/go-github/v67
//   - gh CLI: https://cli.github.com/
package github
