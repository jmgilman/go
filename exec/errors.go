package exec

import "fmt"

// ExecError represents an error that occurred during command execution.
// It includes the exit code, the command that was run, and any captured output.
type ExecError struct {
	// Command is the full command that was executed (including arguments)
	Command []string

	// ExitCode is the exit code returned by the command
	ExitCode int

	// Stdout is the captured standard output
	Stdout string

	// Stderr is the captured standard error
	Stderr string

	// Err is the underlying error from the execution
	Err error
}

// Error implements the error interface.
func (e *ExecError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("command %v failed with exit code %d: %v", e.Command, e.ExitCode, e.Err)
	}
	return fmt.Sprintf("command %v failed with exit code %d", e.Command, e.ExitCode)
}

// Unwrap returns the underlying error.
func (e *ExecError) Unwrap() error {
	return e.Err
}
