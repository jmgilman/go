package exec

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	exec := New()
	if exec == nil {
		t.Fatal("New() returned nil")
	}
}

func TestBasicExecution(t *testing.T) {
	exec := New()
	result, err := exec.Run("echo", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "hello world") {
		t.Errorf("expected stdout to contain 'hello world', got: %s", result.Stdout)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got: %d", result.ExitCode)
	}
}

func TestCommandFailure(t *testing.T) {
	exec := New()
	result, err := exec.Run("false")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	execErr, ok := err.(*ExecError)
	if !ok {
		t.Fatalf("expected ExecError, got: %T", err)
	}

	if execErr.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}

	if result == nil {
		t.Fatal("expected result even with error")
	}
}

func TestWithDir(t *testing.T) {
	exec := New()
	result, err := exec.WithDir("/tmp").Run("pwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "/tmp") {
		t.Errorf("expected stdout to contain '/tmp', got: %s", result.Stdout)
	}
}

func TestWithEnv(t *testing.T) {
	exec := New()
	result, err := exec.WithEnv(map[string]string{
		"TEST_VAR": "test_value",
	}).Run("sh", "-c", "echo $TEST_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "test_value") {
		t.Errorf("expected stdout to contain 'test_value', got: %s", result.Stdout)
	}
}

func TestWithDisableColors(t *testing.T) {
	exec := New()
	result, err := exec.WithDisableColors().Run("sh", "-c", "echo $NO_COLOR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "1") {
		t.Errorf("expected NO_COLOR=1, got: %s", result.Stdout)
	}
}

func TestWithTimeout(t *testing.T) {
	exec := New()
	_, err := exec.WithTimeout("100ms").Run("sleep", "1")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	exec := New()
	_, err := exec.WithContext(ctx).Run("sleep", "1")
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestWithPassthrough(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exec := New()
	result, err := exec.WithStdout(&stdout).WithStderr(&stderr).WithPassthrough().Run("echo", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that output was captured
	if !strings.Contains(result.Stdout, "test") {
		t.Errorf("expected captured stdout to contain 'test', got: %s", result.Stdout)
	}

	// Check that output was also written to the custom writer
	if !strings.Contains(stdout.String(), "test") {
		t.Errorf("expected passthrough stdout to contain 'test', got: %s", stdout.String())
	}
}

func TestCombinedOutput(t *testing.T) {
	exec := New()
	result, err := exec.Run("sh", "-c", "echo stdout && echo stderr >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Combined, "stdout") {
		t.Errorf("expected combined output to contain 'stdout', got: %s", result.Combined)
	}

	if !strings.Contains(result.Combined, "stderr") {
		t.Errorf("expected combined output to contain 'stderr', got: %s", result.Combined)
	}
}

func TestSeparateOutput(t *testing.T) {
	exec := New()
	result, err := exec.Run("sh", "-c", "echo stdout && echo stderr >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "stdout") {
		t.Errorf("expected stdout to contain 'stdout', got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stderr, "stderr") {
		t.Errorf("expected stderr to contain 'stderr', got: %s", result.Stderr)
	}
}

func TestGlobalOptions(t *testing.T) {
	exec := New(
		WithEnv(map[string]string{"GLOBAL_VAR": "global"}),
		WithDisableColors(),
	)

	result, err := exec.Run("sh", "-c", "echo $GLOBAL_VAR $NO_COLOR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected global env var to be set, got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stdout, "1") {
		t.Errorf("expected NO_COLOR to be set, got: %s", result.Stdout)
	}
}

func TestLocalOverridesGlobal(t *testing.T) {
	exec := New(
		WithEnv(map[string]string{"TEST_VAR": "global"}),
	)

	result, err := exec.WithEnv(map[string]string{"TEST_VAR": "local"}).Run("sh", "-c", "echo $TEST_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "local") {
		t.Errorf("expected local value to override global, got: %s", result.Stdout)
	}
}

func TestClone(t *testing.T) {
	exec1 := New(WithEnv(map[string]string{"GLOBAL_VAR": "global"}))
	exec2 := exec1.Clone()

	// Modify exec2
	result, err := exec2.WithEnv(map[string]string{"LOCAL_VAR": "local"}).Run("sh", "-c", "echo $GLOBAL_VAR $LOCAL_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected cloned executor to inherit global config, got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stdout, "local") {
		t.Errorf("expected cloned executor to have local config, got: %s", result.Stdout)
	}

	// Verify exec1 is unchanged
	result, err = exec1.Run("sh", "-c", "echo $GLOBAL_VAR $LOCAL_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected original executor to still have global config, got: %s", result.Stdout)
	}

	if strings.Contains(result.Stdout, "local") {
		t.Errorf("expected original executor to not have local config from clone, got: %s", result.Stdout)
	}
}

func TestInheritEnv(t *testing.T) {
	// Set a test environment variable
	t.Setenv("TEST_INHERIT_VAR", "inherited")

	exec := New()
	result, err := exec.WithInheritEnv().Run("sh", "-c", "echo $TEST_INHERIT_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "inherited") {
		t.Errorf("expected to inherit environment variable, got: %s", result.Stdout)
	}
}

func TestEmptyCommand(t *testing.T) {
	exec := New()
	_, err := exec.Run()
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}
