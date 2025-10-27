# exec

A testable, feature-rich interface for executing local commands in Go.

## Features

- **Interface-first design**: Easy to mock for testing
- **Multi-pipe support**: Stream output to stdout/stderr while capturing it
- **Separate output capture**: Access stdout, stderr, and combined output separately
- **Color control**: Built-in support for disabling color output
- **Command wrappers**: Create command-specific executors for frequently used tools
- **Global and local configuration**: Set defaults and override per-execution
- **Context support**: Full support for cancellation and timeouts
- **Structured errors**: Rich error types with exit codes and captured output

## Installation

```bash
go get github.com/jmgilman/go/exec
```

## Quick Start

### Basic Execution

```go
package main

import (
    "fmt"
    "log"

    "github.com/jmgilman/go/exec"
)

func main() {
    executor := exec.New()
    result, err := executor.Run("echo", "hello world")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Stdout) // "hello world\n"
    fmt.Println(result.ExitCode) // 0
}
```

### With Configuration

```go
executor := exec.New()
result, err := executor.
    WithDir("/tmp").
    WithEnv(map[string]string{"MY_VAR": "value"}).
    WithDisableColors().
    Run("some-command", "arg1", "arg2")
```

### Command Wrapper

```go
executor := exec.New()
git := exec.NewWrapper(executor, "git")

// Use it like a git-specific executor
result, err := git.WithDir("/repo").Run("status")
result, err = git.WithDir("/repo").Run("commit", "-m", "message")
```

## Configuration Options

### Global Configuration

Set defaults at creation time that apply to all executions:

```go
executor := exec.New(
    exec.WithEnv(map[string]string{"GLOBAL_VAR": "value"}),
    exec.WithDir("/default/dir"),
    exec.WithDisableColors(),
    exec.WithInheritEnv(),
    exec.WithPassthrough(),
)
```

### Local Configuration

Override global settings per-execution:

```go
result, err := executor.
    WithDir("/other/dir").                    // Override working directory
    WithEnv(map[string]string{"VAR": "val"}). // Add/override environment variables
    WithTimeout("5s").                         // Set execution timeout
    WithContext(ctx).                          // Use custom context
    WithPassthrough().                         // Enable output streaming
    Run("command", "args...")
```

Local settings always override global settings.

## Advanced Features

### Multi-Pipe Support

Stream output in real-time while also capturing it:

```go
executor := exec.New()
result, err := executor.
    WithPassthrough().
    Run("long-running-command")

// User sees output as it happens
// result.Stdout and result.Stderr contain the full captured output
```

### Custom Output Writers

Direct output to custom writers:

```go
var stdout, stderr bytes.Buffer

executor := exec.New()
result, err := executor.
    WithStdout(&stdout).
    WithStderr(&stderr).
    WithPassthrough().
    Run("command")

// Output is written to custom buffers and captured in result
```

### Separate vs Combined Output

Access stdout and stderr separately or combined:

```go
result, err := executor.Run("sh", "-c", "echo stdout && echo stderr >&2")

fmt.Println(result.Stdout)   // "stdout\n"
fmt.Println(result.Stderr)   // "stderr\n"
fmt.Println(result.Combined) // "stdout\nstderr\n" (order preserved)
```

### Timeout Support

Set execution timeouts:

```go
// Using WithTimeout helper
result, err := executor.WithTimeout("5s").Run("command")

// Using context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
result, err := executor.WithContext(ctx).Run("command")
```

### Disable Colors

Automatically disable color output by setting common environment variables:

```go
executor := exec.New()
result, err := executor.
    WithDisableColors().
    Run("some-colorful-command")

// Sets: NO_COLOR=1, TERM=dumb, CLICOLOR=0, etc.
```

### Environment Inheritance

Inherit environment from parent process:

```go
executor := exec.New()
result, err := executor.
    WithInheritEnv().
    WithEnv(map[string]string{"ADDITIONAL_VAR": "value"}).
    Run("command")

// Command runs with parent env + additional variables
```

## Error Handling

Command failures return structured errors with rich context:

```go
result, err := executor.Run("false")
if err != nil {
    execErr := err.(*exec.ExecError)
    fmt.Printf("Exit code: %d\n", execErr.ExitCode)
    fmt.Printf("Command: %v\n", execErr.Command)
    fmt.Printf("Stdout: %s\n", execErr.Stdout)
    fmt.Printf("Stderr: %s\n", execErr.Stderr)
}

// Even on error, result is available with captured output
```

## Testing

The interface-first design makes mocking simple:

```go
type mockExecutor struct{}

func (m *mockExecutor) Run(args ...string) (*exec.Result, error) {
    return &exec.Result{
        Stdout: "mocked output",
        Stderr: "",
        Combined: "mocked output",
        ExitCode: 0,
    }, nil
}

func (m *mockExecutor) WithEnv(env map[string]string) exec.Executor { return m }
func (m *mockExecutor) WithDir(dir string) exec.Executor { return m }
// ... implement other interface methods

// Use in tests
func TestMyFunction(t *testing.T) {
    mock := &mockExecutor{}
    result := myFunction(mock)
    // assertions...
}
```

## Cloning Executors

Create independent copies with the same configuration:

```go
base := exec.New(exec.WithEnv(map[string]string{"GLOBAL_VAR": "value"}))

executor1 := base.Clone()
executor2 := base.Clone()

// executor1 and executor2 are independent but share the same base configuration
```

## Examples

### Running git commands

```go
executor := exec.New(exec.WithDisableColors())
git := exec.NewWrapper(executor, "git")

// Check status
result, err := git.WithDir("/repo").Run("status")

// Commit changes
result, err = git.
    WithDir("/repo").
    WithEnv(map[string]string{"GIT_AUTHOR_NAME": "John Doe"}).
    Run("commit", "-m", "Initial commit")
```

### Running tests with timeout

```go
executor := exec.New()
result, err := executor.
    WithDir("/project").
    WithTimeout("30s").
    WithPassthrough().
    Run("go", "test", "./...")

// User sees test output in real-time
// Full output is captured in result
```

### Building with custom environment

```go
executor := exec.New(
    exec.WithInheritEnv(),
    exec.WithEnv(map[string]string{
        "CGO_ENABLED": "0",
        "GOOS": "linux",
        "GOARCH": "amd64",
    }),
)

result, err := executor.
    WithDir("/project").
    Run("go", "build", "-o", "binary", ".")
```

## License

See repository LICENSE file.
