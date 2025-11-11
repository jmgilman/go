package exec

import (
	"context"
	"io"
)

//go:generate go run github.com/matryer/moq@latest -out mocks/executor.go -pkg mocks . Executor

// Executor is the main interface for executing commands.
// It provides a fluent API for configuring and running commands.
type Executor interface {
	// WithEnv sets environment variables for the command.
	// These are local settings that override any global environment variables.
	WithEnv(env map[string]string) Executor

	// WithDir sets the working directory for the command.
	// This is a local setting that overrides any global working directory.
	WithDir(dir string) Executor

	// WithContext sets the context for the command.
	// The command will be canceled if the context is canceled.
	WithContext(ctx context.Context) Executor

	// WithDisableColors disables color output by setting common environment variables.
	// This sets NO_COLOR=1, TERM=dumb, and other common color-disabling variables.
	WithDisableColors() Executor

	// WithTimeout sets a timeout for the command execution.
	// This is a convenience method that creates a context with timeout.
	WithTimeout(timeout string) Executor

	// WithInheritEnv inherits environment variables from the parent process.
	WithInheritEnv() Executor

	// WithStdout sets a custom writer for stdout.
	// If passthrough is enabled, output will be written here in addition to being captured.
	WithStdout(w io.Writer) Executor

	// WithStderr sets a custom writer for stderr.
	// If passthrough is enabled, output will be written here in addition to being captured.
	WithStderr(w io.Writer) Executor

	// WithPassthrough enables streaming output to stdout/stderr while also capturing it.
	// The output will be written to the writers set by WithStdout/WithStderr (or os.Stdout/os.Stderr by default).
	WithPassthrough() Executor

	// Run executes the command with the given arguments.
	// It returns a Result containing the captured output and exit code.
	Run(args ...string) (*Result, error)

	// Clone creates a copy of the executor with the same configuration.
	// This is useful for creating multiple executors with the same base configuration.
	Clone() Executor
}

// Result represents the result of a command execution.
type Result struct {
	// Stdout is the captured standard output
	Stdout string

	// Stderr is the captured standard error
	Stderr string

	// Combined is the combined stdout and stderr output
	Combined string

	// ExitCode is the exit code returned by the command
	ExitCode int
}

// Option is a function that configures a Command with global settings.
// These settings are applied at creation time and can be overridden by local settings.
type Option func(*Command)

// WithEnv returns an Option that sets global environment variables.
func WithEnv(env map[string]string) Option {
	return func(c *Command) {
		c.WithEnv(env)
	}
}

// WithDir returns an Option that sets the global working directory.
func WithDir(dir string) Option {
	return func(c *Command) {
		c.WithDir(dir)
	}
}

// WithContext returns an Option that sets the global context.
func WithContext(ctx context.Context) Option {
	return func(c *Command) {
		c.WithContext(ctx)
	}
}

// WithDisableColors returns an Option that globally disables color output.
func WithDisableColors() Option {
	return func(c *Command) {
		c.WithDisableColors()
	}
}

// WithTimeout returns an Option that sets a global timeout.
func WithTimeout(timeout string) Option {
	return func(c *Command) {
		c.WithTimeout(timeout)
	}
}

// WithInheritEnv returns an Option that globally enables environment inheritance.
func WithInheritEnv() Option {
	return func(c *Command) {
		c.WithInheritEnv()
	}
}

// WithStdout returns an Option that sets the global stdout writer.
func WithStdout(w io.Writer) Option {
	return func(c *Command) {
		c.WithStdout(w)
	}
}

// WithStderr returns an Option that sets the global stderr writer.
func WithStderr(w io.Writer) Option {
	return func(c *Command) {
		c.WithStderr(w)
	}
}

// WithPassthrough returns an Option that globally enables output passthrough.
func WithPassthrough() Option {
	return func(c *Command) {
		c.WithPassthrough()
	}
}
