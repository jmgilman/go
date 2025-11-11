package exec

import (
	"context"
	"io"
)

// CommandWrapper wraps an Executor to provide a command-specific interface.
// It prepends a command name to all Run() calls, making it convenient for
// tools that are called frequently with different arguments (e.g., git, docker).
// CommandWrapper implements the Executor interface, allowing it to be used
// anywhere an Executor is expected.
type CommandWrapper struct {
	executor Executor
	cmd      string
}

// NewWrapper creates a new CommandWrapper that prepends the given command to all Run() calls.
// The executor parameter can be any implementation of the Executor interface, including
// mock executors for testing.
func NewWrapper(executor Executor, cmd string) *CommandWrapper {
	return &CommandWrapper{
		executor: executor,
		cmd:      cmd,
	}
}

// WithEnv sets environment variables for the command.
func (w *CommandWrapper) WithEnv(env map[string]string) Executor {
	w.executor = w.executor.WithEnv(env)
	return w
}

// WithDir sets the working directory for the command.
func (w *CommandWrapper) WithDir(dir string) Executor {
	w.executor = w.executor.WithDir(dir)
	return w
}

// WithContext sets the context for the command.
func (w *CommandWrapper) WithContext(ctx context.Context) Executor {
	w.executor = w.executor.WithContext(ctx)
	return w
}

// WithDisableColors disables color output.
func (w *CommandWrapper) WithDisableColors() Executor {
	w.executor = w.executor.WithDisableColors()
	return w
}

// WithTimeout sets a timeout for the command.
func (w *CommandWrapper) WithTimeout(timeout string) Executor {
	w.executor = w.executor.WithTimeout(timeout)
	return w
}

// WithInheritEnv enables environment inheritance.
func (w *CommandWrapper) WithInheritEnv() Executor {
	w.executor = w.executor.WithInheritEnv()
	return w
}

// WithStdout sets the stdout writer.
func (w *CommandWrapper) WithStdout(w2 io.Writer) Executor {
	w.executor = w.executor.WithStdout(w2)
	return w
}

// WithStderr sets the stderr writer.
func (w *CommandWrapper) WithStderr(w2 io.Writer) Executor {
	w.executor = w.executor.WithStderr(w2)
	return w
}

// WithPassthrough enables output passthrough.
func (w *CommandWrapper) WithPassthrough() Executor {
	w.executor = w.executor.WithPassthrough()
	return w
}

// Run executes the wrapped command with the given arguments.
// The command name is prepended to the arguments.
func (w *CommandWrapper) Run(args ...string) (*Result, error) {
	fullArgs := append([]string{w.cmd}, args...)
	return w.executor.Run(fullArgs...)
}

// Clone creates a copy of the wrapper with the same configuration.
func (w *CommandWrapper) Clone() Executor {
	return &CommandWrapper{
		executor: w.executor.Clone(),
		cmd:      w.cmd,
	}
}
