package exec

import (
	"context"
	"io"
	"os"
	osexec "os/exec"
	"time"
)

// Command is the concrete implementation of the Executor interface.
// It provides command execution with configurable settings.
type Command struct {
	config  *config
	ctx     context.Context
	stdout  io.Writer
	stderr  io.Writer
	timeout string
}

// New creates a new Command with the given options.
// Options set global defaults that can be overridden by local settings.
func New(opts ...Option) *Command {
	cmd := &Command{
		config: newConfig(),
		ctx:    context.Background(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	// Apply global options
	for _, opt := range opts {
		opt(cmd)
	}

	return cmd
}

// WithEnv sets environment variables for the command.
func (c *Command) WithEnv(env map[string]string) Executor {
	for k, v := range env {
		c.config.localEnv[k] = v
	}
	return c
}

// WithDir sets the working directory for the command.
func (c *Command) WithDir(dir string) Executor {
	c.config.localDir = dir
	return c
}

// WithContext sets the context for the command.
func (c *Command) WithContext(ctx context.Context) Executor {
	c.ctx = ctx
	return c
}

// WithDisableColors disables color output.
func (c *Command) WithDisableColors() Executor {
	val := true
	c.config.localDisableColors = &val
	return c
}

// WithTimeout sets a timeout for the command.
func (c *Command) WithTimeout(timeout string) Executor {
	c.timeout = timeout
	return c
}

// WithInheritEnv enables environment inheritance.
func (c *Command) WithInheritEnv() Executor {
	val := true
	c.config.localInheritEnv = &val
	return c
}

// WithStdout sets the stdout writer.
func (c *Command) WithStdout(w io.Writer) Executor {
	c.stdout = w
	return c
}

// WithStderr sets the stderr writer.
func (c *Command) WithStderr(w io.Writer) Executor {
	c.stderr = w
	return c
}

// WithPassthrough enables output passthrough.
func (c *Command) WithPassthrough() Executor {
	val := true
	c.config.localPassthrough = &val
	return c
}

// Run executes the command with the given arguments.
func (c *Command) Run(args ...string) (*Result, error) {
	if len(args) == 0 {
		return nil, &ExecError{
			Command:  args,
			ExitCode: -1,
			Err:      osexec.ErrNotFound,
		}
	}

	// Apply timeout if set
	ctx := c.ctx
	if c.timeout != "" {
		duration, err := time.ParseDuration(c.timeout)
		if err != nil {
			return nil, &ExecError{
				Command:  args,
				ExitCode: -1,
				Err:      err,
			}
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	// Create the command
	cmd := osexec.CommandContext(ctx, args[0], args[1:]...)

	// Set working directory
	if dir := c.config.effectiveDir(); dir != "" {
		cmd.Dir = dir
	}

	// Set environment
	if c.config.effectiveInheritEnv() {
		cmd.Env = os.Environ()
	}
	for k, v := range c.config.effectiveEnv() {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Setup output capture
	var stdoutCapture, stderrCapture *outputCapture
	var combined *combinedWriter

	if c.config.effectivePassthrough() {
		stdoutCapture = newOutputCapture(c.stdout)
		stderrCapture = newOutputCapture(c.stderr)
	} else {
		stdoutCapture = newOutputCapture(nil)
		stderrCapture = newOutputCapture(nil)
	}

	// Create combined writer
	combined = newCombinedWriter()

	// Set up multi-writers for combined output
	cmd.Stdout = newMultiWriter(stdoutCapture.Writer(), combined)
	cmd.Stderr = newMultiWriter(stderrCapture.Writer(), combined)

	// Execute the command
	err := cmd.Run()

	// Build result
	result := &Result{
		Stdout:   stdoutCapture.String(),
		Stderr:   stderrCapture.String(),
		Combined: combined.String(),
		ExitCode: cmd.ProcessState.ExitCode(),
	}

	// Reset local configuration for next run
	c.config.resetLocal()
	c.timeout = ""

	// Handle errors
	if err != nil {
		return result, &ExecError{
			Command:  args,
			ExitCode: result.ExitCode,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			Err:      err,
		}
	}

	return result, nil
}

// Clone creates a copy of the executor with the same configuration.
func (c *Command) Clone() Executor {
	return &Command{
		config: c.config.clone(),
		ctx:    c.ctx,
		stdout: c.stdout,
		stderr: c.stderr,
	}
}
