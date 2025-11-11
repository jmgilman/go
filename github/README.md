# GitHub

Idiomatic Go wrapper for GitHub operations with pluggable backend implementations (SDK and CLI providers).

## Quick start

```bash
# Prerequisites: Go 1.25.3+
go get github.com/jmgilman/go/github
```

Hello world with SDK provider:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/jmgilman/go/github"
    "github.com/jmgilman/go/github/providers/sdk"
)

func main() {
    // Create SDK provider with personal access token
    provider, err := sdk.NewSDKProvider(
        sdk.WithToken("ghp_xxxxxxxxxxxx"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create client and fetch repository
    client := github.NewClient(provider, "myorg")
    repo := client.Repository("myrepo")

    ctx := context.Background()
    if err := repo.Get(ctx); err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Repository: %s\n", repo.FullName())
    fmt.Printf("Default Branch: %s\n", repo.DefaultBranch())
}
```

## Usage

Common tasks:

```go
// 1) Create and manage issues
issue, err := repo.CreateIssue(ctx, "Bug: Login validation",
    "The login form doesn't validate email addresses properly.",
    github.WithLabels("bug", "high-priority"),
    github.WithAssignees("developer1"),
)

// List open issues with specific labels
issues, err := repo.ListIssues(ctx,
    github.WithState("open"),
    github.WithIssueLabels("bug"),
)

// 2) Create and merge pull requests
pr, err := repo.CreatePullRequest(ctx, github.CreatePullRequestOptions{
    Title: "Add new feature",
    Body:  "This PR adds a new feature that...",
    Head:  "feature-branch",
    Base:  "main",
})

// Merge with squash
err = pr.Merge(ctx,
    github.WithMergeMethod("squash"),
    github.WithCommitMessage("Merge feature branch"),
)

// 3) Monitor workflows
run, err := repo.GetWorkflowRun(ctx, runID)
if err := run.Wait(ctx, 10*time.Second); err != nil {
    log.Fatal(err)
}

if !run.IsSuccessful() {
    jobs, _ := run.GetJobs(ctx)
    // Handle failed jobs
}

// 4) Use CLI provider (inherits gh CLI auth)
provider, err := cli.NewCLIProvider()
client := github.NewClient(provider, "myorg")
```

## Configuration

### Provider Selection

| Provider | Use Case | Authentication |
| -------- | -------- | -------------- |
| SDK | Fine-grained control, existing go-github usage | Personal access token or custom client |
| CLI | Scripts, automation, environments with gh CLI | Inherits from gh CLI config |

### SDK Provider Options

```go
// With personal access token
provider, err := sdk.NewSDKProvider(
    sdk.WithToken("ghp_xxxxxxxxxxxx"),
)

// With custom GitHub client (for GitHub Apps, etc.)
provider, err := sdk.NewSDKProvider(
    sdk.WithClient(customGitHubClient),
)
```

### CLI Provider Options

```go
// Default (uses gh CLI authentication)
provider, err := cli.NewCLIProvider()

// With custom executor (for testing)
provider, err := cli.NewCLIProvider(
    cli.WithExecutor(mockExecutor),
)
```

## Troubleshooting

* **Authentication failed**: Verify token has required scopes (repo, workflow, etc.) for SDK provider. For CLI provider, run `gh auth status` to check gh CLI authentication.
* **Repository not found**: Check repository name format (use "myrepo" not "owner/myrepo") and verify access permissions.
* **Rate limit exceeded**: SDK provider respects GitHub API rate limits. Implement exponential backoff or use authenticated requests for higher limits.

## Error Handling

All errors use the workspace errors library with consistent error codes:

```go
repo := client.Repository("myrepo")
if err := repo.Get(ctx); err != nil {
    var platformErr errors.PlatformError
    if errors.As(err, &platformErr) {
        switch platformErr.Code() {
        case errors.CodeNotFound:
            fmt.Println("Repository not found")
        case errors.CodeUnauthorized:
            fmt.Println("Authentication failed")
        case errors.CodeForbidden:
            fmt.Println("Access denied")
        }
    }
}
```

## Architecture

The library follows a layered architecture:

1. **Provider interface** - Abstracts GitHub API implementation
2. **Two concrete providers** - SDK (go-github) and CLI (gh CLI)
3. **High-level types** - Client, Repository, Issue, PullRequest, WorkflowRun
4. **Escape hatches** - Direct provider access for advanced use cases

## Contributing

Submit issues and PRs to the workspace repository. Follow Go conventions and include tests for new features.

### Testing

Use mock providers for testing:

```go
import "github.com/jmgilman/go/github/testutil"

mockProvider := testutil.NewMockProvider()
mockProvider.OnGetRepository = func(ctx context.Context, owner, repo string) (*github.RepositoryData, error) {
    return &github.RepositoryData{
        ID:   123,
        Name: repo,
        Owner: owner,
    }, nil
}

client := github.NewClient(mockProvider, "testorg")
```

## License

See workspace LICENSE file.

## Links

* Package documentation: [pkg.go.dev/github.com/jmgilman/go/github](https://pkg.go.dev/github.com/jmgilman/go/github)
* GitHub REST API: [docs.github.com/en/rest](https://docs.github.com/en/rest)
* go-github SDK: [pkg.go.dev/github.com/google/go-github/v67](https://pkg.go.dev/github.com/google/go-github/v67)
* gh CLI: [cli.github.com](https://cli.github.com/)
