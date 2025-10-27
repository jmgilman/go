package core_test

import (
	"testing"

	"github.com/jmgilman/go/fs/core"
)

// TestFSType_String verifies FSType.String() returns correct string representations.
func TestFSType_String(t *testing.T) {
	tests := []struct {
		name     string
		fsType   core.FSType
		expected string
	}{
		{
			name:     "Unknown",
			fsType:   core.FSTypeUnknown,
			expected: "unknown",
		},
		{
			name:     "Local",
			fsType:   core.FSTypeLocal,
			expected: "local",
		},
		{
			name:     "Memory",
			fsType:   core.FSTypeMemory,
			expected: "memory",
		},
		{
			name:     "Remote",
			fsType:   core.FSTypeRemote,
			expected: "remote",
		},
		{
			name:     "Invalid",
			fsType:   core.FSType(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fsType.String()
			if result != tt.expected {
				t.Errorf("FSType(%d).String() = %q, want %q", tt.fsType, result, tt.expected)
			}
		})
	}
}

// TestFSType_Constants verifies FSType constants have expected values.
func TestFSType_Constants(t *testing.T) {
	// Verify that FSTypeUnknown is 0 (the zero value)
	if core.FSTypeUnknown != 0 {
		t.Errorf("FSTypeUnknown = %d, want 0 (zero value)", core.FSTypeUnknown)
	}

	// Verify other constants are non-zero and distinct
	types := []core.FSType{
		core.FSTypeLocal,
		core.FSTypeMemory,
		core.FSTypeRemote,
	}

	seen := make(map[core.FSType]bool)
	for _, fsType := range types {
		if fsType == 0 {
			t.Errorf("FSType %s has zero value", fsType.String())
		}
		if seen[fsType] {
			t.Errorf("Duplicate FSType value: %d", fsType)
		}
		seen[fsType] = true
	}
}
