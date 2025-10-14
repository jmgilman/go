package core_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestErrorVariablesExist verifies all error variables are defined.
func TestErrorVariablesExist(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		isNil bool
	}{
		{"ErrNotExist", core.ErrNotExist, false},
		{"ErrExist", core.ErrExist, false},
		{"ErrPermission", core.ErrPermission, false},
		{"ErrClosed", core.ErrClosed, false},
		{"ErrUnsupported", core.ErrUnsupported, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isNil && tt.err != nil {
				t.Errorf("%s should be nil, got %v", tt.name, tt.err)
			}
			if !tt.isNil && tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
		})
	}
}

// TestReexportedErrorsMatchStdlib verifies re-exported errors match stdlib.
func TestReexportedErrorsMatchStdlib(t *testing.T) {
	tests := []struct {
		name      string
		coreErr   error
		stdlibErr error
	}{
		{"ErrNotExist", core.ErrNotExist, fs.ErrNotExist},
		{"ErrExist", core.ErrExist, fs.ErrExist},
		{"ErrPermission", core.ErrPermission, fs.ErrPermission},
		{"ErrClosed", core.ErrClosed, fs.ErrClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use errors.Is to check they are the same error
			if !errors.Is(tt.coreErr, tt.stdlibErr) || !errors.Is(tt.stdlibErr, tt.coreErr) {
				t.Errorf("%s does not match stdlib: core=%v, stdlib=%v",
					tt.name, tt.coreErr, tt.stdlibErr)
			}
		})
	}
}

// TestErrorsWorkWithIs verifies errors can be used with errors.Is().
func TestErrorsWorkWithIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotExist", core.ErrNotExist},
		{"ErrExist", core.ErrExist},
		{"ErrPermission", core.ErrPermission},
		{"ErrClosed", core.ErrClosed},
		{"ErrUnsupported", core.ErrUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test direct comparison
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("errors.Is(%s, %s) returned false, expected true",
					tt.name, tt.name)
			}

			// Test wrapped error
			wrapped := wrapError(tt.err)
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("errors.Is(wrapped, %s) returned false, expected true",
					tt.name)
			}
		})
	}
}

// TestReexportedErrorsWorkWithStdlibIs verifies re-exported errors work with
// errors.Is() when comparing with stdlib errors.
func TestReexportedErrorsWorkWithStdlibIs(t *testing.T) {
	tests := []struct {
		name      string
		coreErr   error
		stdlibErr error
	}{
		{"ErrNotExist", core.ErrNotExist, fs.ErrNotExist},
		{"ErrExist", core.ErrExist, fs.ErrExist},
		{"ErrPermission", core.ErrPermission, fs.ErrPermission},
		{"ErrClosed", core.ErrClosed, fs.ErrClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Core error should match stdlib error
			if !errors.Is(tt.coreErr, tt.stdlibErr) {
				t.Errorf("errors.Is(%s, stdlib) returned false, expected true",
					tt.name)
			}

			// Stdlib error should match core error
			if !errors.Is(tt.stdlibErr, tt.coreErr) {
				t.Errorf("errors.Is(stdlib, %s) returned false, expected true",
					tt.name)
			}

			// Wrapped core error should match stdlib error
			wrapped := wrapError(tt.coreErr)
			if !errors.Is(wrapped, tt.stdlibErr) {
				t.Errorf("errors.Is(wrapped core, stdlib) returned false, expected true")
			}
		})
	}
}

// TestErrUnsupportedMessage verifies ErrUnsupported has the expected message.
func TestErrUnsupportedMessage(t *testing.T) {
	expected := "operation not supported"
	if core.ErrUnsupported.Error() != expected {
		t.Errorf("ErrUnsupported.Error() = %q, want %q",
			core.ErrUnsupported.Error(), expected)
	}
}

// TestErrorIdentity verifies errors maintain their identity with errors.Is().
func TestErrorIdentity(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotExist", core.ErrNotExist},
		{"ErrExist", core.ErrExist},
		{"ErrPermission", core.ErrPermission},
		{"ErrClosed", core.ErrClosed},
		{"ErrUnsupported", core.ErrUnsupported},
	}

	// Test that error variables maintain identity
	for _, tt := range tests {
		t.Run(tt.name+"_identity", func(t *testing.T) {
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("%s is not equal to itself", tt.name)
			}
		})
	}

	// Test that different errors are distinct
	if errors.Is(core.ErrNotExist, core.ErrExist) {
		t.Error("ErrNotExist should not equal ErrExist")
	}
	if errors.Is(core.ErrUnsupported, core.ErrNotExist) {
		t.Error("ErrUnsupported should not equal ErrNotExist")
	}
}

// wrapError wraps an error to test error unwrapping with errors.Is().
func wrapError(err error) error {
	return &wrappedError{err: err}
}

// wrappedError is a simple error wrapper for testing.
type wrappedError struct {
	err error
}

func (e *wrappedError) Error() string {
	return "wrapped: " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}
