package exec_test

import (
	"context"
	"io"
	"testing"

	"github.com/jmgilman/go/exec"
	"github.com/jmgilman/go/exec/mocks"
)

func TestWrapperWithMock(t *testing.T) {
	// Create a mock executor
	var mockExec *mocks.ExecutorMock
	mockExec = &mocks.ExecutorMock{
		WithEnvFunc: func(env map[string]string) exec.Executor {
			// Verify the expected env is passed
			if env["TEST_VAR"] != "test_value" {
				t.Errorf("expected TEST_VAR=test_value, got: %v", env)
			}
			return mockExec // Return self for chaining
		},
		WithDirFunc: func(dir string) exec.Executor {
			// Verify the expected dir is passed
			if dir != "/test/dir" {
				t.Errorf("expected dir=/test/dir, got: %s", dir)
			}
			return mockExec // Return self for chaining
		},
		WithContextFunc: func(ctx context.Context) exec.Executor {
			return mockExec // Return self for chaining
		},
		WithDisableColorsFunc: func() exec.Executor {
			return mockExec // Return self for chaining
		},
		WithTimeoutFunc: func(timeout string) exec.Executor {
			return mockExec // Return self for chaining
		},
		WithInheritEnvFunc: func() exec.Executor {
			return mockExec // Return self for chaining
		},
		WithStdoutFunc: func(w io.Writer) exec.Executor {
			return mockExec // Return self for chaining
		},
		WithStderrFunc: func(w io.Writer) exec.Executor {
			return mockExec // Return self for chaining
		},
		WithPassthroughFunc: func() exec.Executor {
			return mockExec // Return self for chaining
		},
		RunFunc: func(args ...string) (*exec.Result, error) {
			// Verify that the wrapper prepends the command name
			if len(args) < 1 || args[0] != "git" {
				t.Errorf("expected first arg to be 'git', got: %v", args)
			}
			if len(args) < 2 || args[1] != "status" {
				t.Errorf("expected second arg to be 'status', got: %v", args)
			}
			return &exec.Result{
				Stdout:   "mock output",
				Stderr:   "",
				Combined: "mock output",
				ExitCode: 0,
			}, nil
		},
		CloneFunc: func() exec.Executor {
			return mockExec // Return self for simplicity in this test
		},
	}

	// Create a wrapper with the mock executor
	wrapper := exec.NewWrapper(mockExec, "git")

	// Test that we can configure the wrapper with fluent API
	result, err := wrapper.
		WithEnv(map[string]string{"TEST_VAR": "test_value"}).
		WithDir("/test/dir").
		Run("status")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Stdout != "mock output" {
		t.Errorf("expected mock output, got: %s", result.Stdout)
	}

	// Verify that the mock methods were called
	if len(mockExec.WithEnvCalls()) != 1 {
		t.Errorf("expected WithEnv to be called once, got: %d", len(mockExec.WithEnvCalls()))
	}

	if len(mockExec.WithDirCalls()) != 1 {
		t.Errorf("expected WithDir to be called once, got: %d", len(mockExec.WithDirCalls()))
	}

	if len(mockExec.RunCalls()) != 1 {
		t.Errorf("expected Run to be called once, got: %d", len(mockExec.RunCalls()))
	}

	// Verify the arguments passed to Run
	runCalls := mockExec.RunCalls()
	if len(runCalls) > 0 && len(runCalls[0].Args) >= 2 {
		if runCalls[0].Args[0] != "git" {
			t.Errorf("expected first arg 'git', got: %s", runCalls[0].Args[0])
		}
		if runCalls[0].Args[1] != "status" {
			t.Errorf("expected second arg 'status', got: %s", runCalls[0].Args[1])
		}
	}
}
