package exec

import (
	"context"
	"io"
)

// CommandWrapper wraps a Command to provide a command-specific interface.
// It prepends a command name to all Run() calls, making it convenient for
// tools that are called frequently with different arguments (e.g., git, docker).
type CommandWrapper struct {
	command *Command
	cmd     string
}

// NewWrapper creates a new CommandWrapper that prepends the given command to all Run() calls.
func NewWrapper(command *Command, cmd string) *CommandWrapper {
	return &CommandWrapper{
		command: command,
		cmd:     cmd,
	}
}

// WithEnv sets environment variables for the command.
func (w *CommandWrapper) WithEnv(env map[string]string) *CommandWrapper {
	w.command = w.command.WithEnv(env)
	return w
}

// WithDir sets the working directory for the command.
func (w *CommandWrapper) WithDir(dir string) *CommandWrapper {
	w.command = w.command.WithDir(dir)
	return w
}

// WithContext sets the context for the command.
func (w *CommandWrapper) WithContext(ctx context.Context) *CommandWrapper {
	w.command = w.command.WithContext(ctx)
	return w
}

// WithDisableColors disables color output.
func (w *CommandWrapper) WithDisableColors() *CommandWrapper {
	w.command = w.command.WithDisableColors()
	return w
}

// WithTimeout sets a timeout for the command.
func (w *CommandWrapper) WithTimeout(timeout string) *CommandWrapper {
	w.command = w.command.WithTimeout(timeout)
	return w
}

// WithInheritEnv enables environment inheritance.
func (w *CommandWrapper) WithInheritEnv() *CommandWrapper {
	w.command = w.command.WithInheritEnv()
	return w
}

// WithStdout sets the stdout writer.
func (w *CommandWrapper) WithStdout(w2 io.Writer) *CommandWrapper {
	w.command = w.command.WithStdout(w2)
	return w
}

// WithStderr sets the stderr writer.
func (w *CommandWrapper) WithStderr(w2 io.Writer) *CommandWrapper {
	w.command = w.command.WithStderr(w2)
	return w
}

// WithPassthrough enables output passthrough.
func (w *CommandWrapper) WithPassthrough() *CommandWrapper {
	w.command = w.command.WithPassthrough()
	return w
}

// Run executes the wrapped command with the given arguments.
// The command name is prepended to the arguments.
func (w *CommandWrapper) Run(args ...string) (*Result, error) {
	fullArgs := append([]string{w.cmd}, args...)
	return w.command.Run(fullArgs...)
}

// Clone creates a copy of the wrapper with the same configuration.
func (w *CommandWrapper) Clone() *CommandWrapper {
	return &CommandWrapper{
		command: w.command.Clone(),
		cmd:     w.cmd,
	}
}
