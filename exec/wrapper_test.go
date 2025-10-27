package exec

import (
	"strings"
	"testing"
)

func TestNewWrapper(t *testing.T) {
	exec := New()
	wrapper := NewWrapper(exec, "echo")
	if wrapper == nil {
		t.Fatal("NewWrapper() returned nil")
	}
}

func TestWrapperBasicExecution(t *testing.T) {
	exec := New()
	echo := NewWrapper(exec, "echo")

	result, err := echo.Run("hello", "world")
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

func TestWrapperWithDir(t *testing.T) {
	exec := New()
	pwd := NewWrapper(exec, "pwd")

	result, err := pwd.WithDir("/tmp").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "/tmp") {
		t.Errorf("expected stdout to contain '/tmp', got: %s", result.Stdout)
	}
}

func TestWrapperWithEnv(t *testing.T) {
	exec := New()
	sh := NewWrapper(exec, "sh")

	result, err := sh.WithEnv(map[string]string{
		"WRAPPER_VAR": "wrapper_value",
	}).Run("-c", "echo $WRAPPER_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "wrapper_value") {
		t.Errorf("expected stdout to contain 'wrapper_value', got: %s", result.Stdout)
	}
}

func TestWrapperChaining(t *testing.T) {
	exec := New()
	sh := NewWrapper(exec, "sh")

	result, err := sh.
		WithEnv(map[string]string{"VAR1": "value1"}).
		WithEnv(map[string]string{"VAR2": "value2"}).
		WithDir("/tmp").
		Run("-c", "echo $VAR1 $VAR2 && pwd")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "value1 value2") {
		t.Errorf("expected both env vars to be set, got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stdout, "/tmp") {
		t.Errorf("expected working directory to be /tmp, got: %s", result.Stdout)
	}
}

func TestWrapperWithGlobalOptions(t *testing.T) {
	exec := New(
		WithEnv(map[string]string{"GLOBAL_VAR": "global"}),
		WithDisableColors(),
	)
	sh := NewWrapper(exec, "sh")

	result, err := sh.Run("-c", "echo $GLOBAL_VAR $NO_COLOR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected global env var to be inherited, got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stdout, "1") {
		t.Errorf("expected NO_COLOR to be set, got: %s", result.Stdout)
	}
}

func TestWrapperLocalOverridesGlobal(t *testing.T) {
	exec := New(
		WithEnv(map[string]string{"TEST_VAR": "global"}),
	)
	sh := NewWrapper(exec, "sh")

	result, err := sh.WithEnv(map[string]string{"TEST_VAR": "local"}).Run("-c", "echo $TEST_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "local") {
		t.Errorf("expected local value to override global, got: %s", result.Stdout)
	}
}

func TestWrapperClone(t *testing.T) {
	exec := New(WithEnv(map[string]string{"GLOBAL_VAR": "global"}))
	sh1 := NewWrapper(exec, "sh")
	sh2 := sh1.Clone()

	// Modify sh2
	result, err := sh2.WithEnv(map[string]string{"LOCAL_VAR": "local"}).Run("-c", "echo $GLOBAL_VAR $LOCAL_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected cloned wrapper to inherit global config, got: %s", result.Stdout)
	}

	if !strings.Contains(result.Stdout, "local") {
		t.Errorf("expected cloned wrapper to have local config, got: %s", result.Stdout)
	}

	// Verify sh1 is unchanged
	result, err = sh1.Run("-c", "echo $GLOBAL_VAR $LOCAL_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "global") {
		t.Errorf("expected original wrapper to still have global config, got: %s", result.Stdout)
	}

	if strings.Contains(result.Stdout, "local") {
		t.Errorf("expected original wrapper to not have local config from clone, got: %s", result.Stdout)
	}
}

func TestWrapperMultipleWrappers(t *testing.T) {
	exec := New()
	echo := NewWrapper(exec, "echo")
	pwd := NewWrapper(exec, "pwd")

	// Test echo
	result1, err := echo.Run("test")
	if err != nil {
		t.Fatalf("unexpected error from echo: %v", err)
	}

	if !strings.Contains(result1.Stdout, "test") {
		t.Errorf("expected echo output to contain 'test', got: %s", result1.Stdout)
	}

	// Test pwd
	result2, err := pwd.WithDir("/tmp").Run()
	if err != nil {
		t.Fatalf("unexpected error from pwd: %v", err)
	}

	if !strings.Contains(result2.Stdout, "/tmp") {
		t.Errorf("expected pwd output to contain '/tmp', got: %s", result2.Stdout)
	}
}

func TestWrapperCommandFailure(t *testing.T) {
	exec := New()
	wrapper := NewWrapper(exec, "false")

	result, err := wrapper.Run()
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

func TestWrapperWithPassthrough(t *testing.T) {
	exec := New()
	echo := NewWrapper(exec, "echo")

	result, err := echo.WithPassthrough().Run("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Stdout, "test") {
		t.Errorf("expected captured output to contain 'test', got: %s", result.Stdout)
	}
}

func TestWrapperWithTimeout(t *testing.T) {
	exec := New()
	sleep := NewWrapper(exec, "sleep")

	_, err := sleep.WithTimeout("100ms").Run("1")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}
