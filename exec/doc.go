// Package exec provides a testable interface for executing local commands with enhanced features.
//
// This package wraps the standard library's os/exec, providing the Command struct
// that implements the Executor interface. Following Go best practices, the package
// returns concrete types (Command, CommandWrapper) while accepting interfaces in
// function parameters, making it easy to mock command execution in tests. Additional
// functionality includes multi-pipe support (simultaneous passthrough and capture),
// color disabling, and command wrappers.
//
// # Basic Usage
//
// Create an executor and run a command:
//
//	exec := exec.New()
//	result, err := exec.Run("echo", "hello world")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(result.Stdout) // "hello world\n"
//
// # Configuration
//
// The package supports both global configuration (set at creation time) and
// local configuration (set per-execution). Local settings always override global settings:
//
//	// Global configuration
//	exec := exec.New(
//		exec.WithEnv(map[string]string{"GLOBAL_VAR": "value"}),
//		exec.WithDisableColors(),
//		exec.WithInheritEnv(),
//	)
//
//	// Local configuration (overrides global)
//	result, err := exec.
//		WithDir("/tmp").
//		WithEnv(map[string]string{"LOCAL_VAR": "value"}).
//		WithTimeout("5s").
//		Run("some-command")
//
// # Command Wrappers
//
// For commands that are executed frequently, create a wrapper that automatically
// prepends the command name:
//
//	exec := exec.New()
//	git := exec.NewWrapper(exec, "git")
//
//	result, err := git.WithDir("/repo").Run("status")
//	// Equivalent to: exec.WithDir("/repo").Run("git", "status")
//
// # Multi-Pipe Support
//
// Enable passthrough to stream output to stdout/stderr while simultaneously
// capturing it for later use:
//
//	exec := exec.New()
//	result, err := exec.
//		WithPassthrough().
//		Run("long-running-command")
//
//	// User sees output in real-time
//	// result.Stdout and result.Stderr contain the full captured output
//
// # Output Capture
//
// The package captures stdout and stderr separately, as well as combined:
//
//	result, err := exec.Run("sh", "-c", "echo stdout && echo stderr >&2")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	fmt.Println(result.Stdout)   // "stdout\n"
//	fmt.Println(result.Stderr)   // "stderr\n"
//	fmt.Println(result.Combined) // "stdout\nstderr\n" (order preserved)
//
// # Error Handling
//
// Command failures return a structured error that includes the exit code,
// command, and captured output:
//
//	result, err := exec.Run("false")
//	if err != nil {
//		execErr := err.(*exec.ExecError)
//		fmt.Printf("Exit code: %d\n", execErr.ExitCode)
//		fmt.Printf("Command: %v\n", execErr.Command)
//		fmt.Printf("Stderr: %s\n", execErr.Stderr)
//	}
//
// # Context Support
//
// Commands respect context cancellation and timeouts:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	exec := exec.New()
//	result, err := exec.WithContext(ctx).Run("long-running-command")
//
//	// Or use the timeout helper
//	result, err := exec.WithTimeout("5s").Run("long-running-command")
//
// # Testing
//
// The package follows the Go idiom "accept interfaces, return structs". Production
// code uses the concrete *Command type, but test code can provide mock implementations
// of the Executor interface:
//
//	type mockExecutor struct{}
//
//	func (m *mockExecutor) Run(args ...string) (*exec.Result, error) {
//		return &exec.Result{Stdout: "mocked output"}, nil
//	}
//	func (m *mockExecutor) WithEnv(env map[string]string) exec.Executor { return m }
//	// ... implement other Executor interface methods
//
// Then your functions can accept the Executor interface for easy testing:
//
//	func DeployApp(executor exec.Executor) error {
//		result, err := executor.WithDir("/app").Run("deploy.sh")
//		// ...
//	}
//
// This allows you to pass either exec.New() in production or a mock in tests.
package exec
