package attributes

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// mockProcessor is a test implementation of the Processor interface.
type mockProcessor struct {
	name          string
	processFunc   func(ctx context.Context, attr Attribute) (cue.Value, error)
	processCalled int
}

func (m *mockProcessor) Name() string {
	return m.name
}

func (m *mockProcessor) Process(ctx context.Context, attr Attribute) (cue.Value, error) {
	m.processCalled++
	if m.processFunc != nil {
		return m.processFunc(ctx, attr)
	}
	cueCtx := cuecontext.New()
	return cueCtx.CompileString(`"processed"`), nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if registry.processors == nil {
		t.Error("Registry processors map is nil")
	}
	if len(registry.processors) != 0 {
		t.Errorf("Expected empty registry, got %d processors", len(registry.processors))
	}
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Registry) // Setup before test
		proc    Processor
		wantErr bool
		errMsg  string
	}{
		{
			name:    "register valid processor",
			setup:   func(_ *Registry) {},
			proc:    &mockProcessor{name: "test"},
			wantErr: false,
		},
		{
			name:    "register nil processor",
			setup:   func(_ *Registry) {},
			proc:    nil,
			wantErr: true,
			errMsg:  "processor cannot be nil",
		},
		{
			name:  "register processor with empty name",
			setup: func(_ *Registry) {},
			proc:  &mockProcessor{name: ""},
			wantErr: true,
			errMsg:  "processor name cannot be empty",
		},
		{
			name: "register duplicate processor",
			setup: func(r *Registry) {
				_ = r.Register(&mockProcessor{name: "duplicate"})
			},
			proc:    &mockProcessor{name: "duplicate"},
			wantErr: true,
			errMsg:  `processor "duplicate" is already registered`,
		},
		{
			name:    "register multiple different processors",
			setup:   func(_ *Registry) {},
			proc:    &mockProcessor{name: "processor1"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if tt.setup != nil {
				tt.setup(registry)
			}

			err := registry.Register(tt.proc)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && err.Error() != tt.errMsg {
				t.Errorf("Register() error message = %q, want %q", err.Error(), tt.errMsg)
			}

			// If registration succeeded, verify processor is stored
			if !tt.wantErr && tt.proc != nil {
				if _, ok := registry.processors[tt.proc.Name()]; !ok {
					t.Errorf("Processor %q not found in registry after registration", tt.proc.Name())
				}
			}
		})
	}
}

func TestRegistry_Register_MultipleProcessors(t *testing.T) {
	registry := NewRegistry()

	// Register multiple processors with different names
	processors := []*mockProcessor{
		{name: "processor1"},
		{name: "processor2"},
		{name: "processor3"},
	}

	for _, p := range processors {
		if err := registry.Register(p); err != nil {
			t.Fatalf("Failed to register processor %q: %v", p.Name(), err)
		}
	}

	// Verify all processors are registered
	if len(registry.processors) != len(processors) {
		t.Errorf("Expected %d processors, got %d", len(processors), len(registry.processors))
	}

	// Verify each processor can be retrieved
	for _, p := range processors {
		retrieved, ok := registry.Get(p.Name())
		if !ok {
			t.Errorf("Processor %q not found", p.Name())
			continue
		}
		if retrieved.Name() != p.Name() {
			t.Errorf("Retrieved processor name = %q, want %q", retrieved.Name(), p.Name())
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Registry) *mockProcessor // Setup and return expected processor
		lookupName string
		wantFound  bool
	}{
		{
			name: "get registered processor",
			setup: func(r *Registry) *mockProcessor {
				p := &mockProcessor{name: "test"}
				_ = r.Register(p)
				return p
			},
			lookupName: "test",
			wantFound:  true,
		},
		{
			name: "get unregistered processor",
			setup: func(_ *Registry) *mockProcessor {
				return nil
			},
			lookupName: "nonexistent",
			wantFound:  false,
		},
		{
			name: "get with empty name",
			setup: func(_ *Registry) *mockProcessor {
				return nil
			},
			lookupName: "",
			wantFound:  false,
		},
		{
			name: "get after multiple registrations",
			setup: func(r *Registry) *mockProcessor {
				_ = r.Register(&mockProcessor{name: "processor1"})
				target := &mockProcessor{name: "processor2"}
				_ = r.Register(target)
				_ = r.Register(&mockProcessor{name: "processor3"})
				return target
			},
			lookupName: "processor2",
			wantFound:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			var expectedProc *mockProcessor
			if tt.setup != nil {
				expectedProc = tt.setup(registry)
			}

			got, found := registry.Get(tt.lookupName)

			if found != tt.wantFound {
				t.Errorf("Get() found = %v, want %v", found, tt.wantFound)
			}

			if !tt.wantFound {
				if got != nil {
					t.Errorf("Get() returned processor %v when found=false, want nil", got)
				}
				return
			}

			// tt.wantFound is true
			if got == nil {
				t.Error("Get() returned nil processor when found=true")
				return
			}
			if got.Name() != tt.lookupName {
				t.Errorf("Get() processor name = %q, want %q", got.Name(), tt.lookupName)
			}
			if expectedProc != nil && got != Processor(expectedProc) {
				t.Error("Get() returned different processor instance")
			}
		})
	}
}

func TestRegistry_ThreadSafety(t *testing.T) {
	registry := NewRegistry()

	// Test concurrent registration
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			p := &mockProcessor{name: string(rune('a' + id))}
			_ = registry.Register(p)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test concurrent reads while writing
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				registry.Get(string(rune('a' + id)))
			}
			done <- true
		}(i)
	}

	// Concurrent writes
	for i := 10; i < 20; i++ {
		go func(id int) {
			p := &mockProcessor{name: string(rune('a' + id))}
			_ = registry.Register(p)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify registry is in consistent state
	if registry.processors == nil {
		t.Error("Registry processors map is nil after concurrent operations")
	}
}

func TestProcessor_Interface(t *testing.T) {
	// Test that mockProcessor implements Processor interface
	var _ Processor = (*mockProcessor)(nil)

	cueCtx := cuecontext.New()
	ctx := context.Background()

	// Test Name method
	proc := &mockProcessor{name: "test-processor"}
	if proc.Name() != "test-processor" {
		t.Errorf("Name() = %q, want %q", proc.Name(), "test-processor")
	}

	// Test Process method with custom function
	expectedValue := cueCtx.CompileString(`"custom-result"`)
	proc.processFunc = func(_ context.Context, _ Attribute) (cue.Value, error) {
		return expectedValue, nil
	}

	attr := Attribute{
		Name: "test",
		Args: map[string]string{"key": "value"},
	}

	result, err := proc.Process(ctx, attr)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Compare values by converting to string
	resultStr, _ := result.String()
	expectedStr, _ := expectedValue.String()
	if resultStr != expectedStr {
		t.Errorf("Process() result = %q, want %q", resultStr, expectedStr)
	}

	if proc.processCalled != 1 {
		t.Errorf("Process() called %d times, want 1", proc.processCalled)
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	registry := NewRegistry()

	// Register a processor
	_ = registry.Register(&mockProcessor{name: "exists"})

	// Try to get processors that don't exist
	testCases := []string{
		"notexist",
		"exists2", // Similar name but not exact match
		"EXISTS",  // Different case
		"",        // Empty string
	}

	for _, name := range testCases {
		t.Run("get_"+name, func(t *testing.T) {
			proc, found := registry.Get(name)
			if found {
				t.Errorf("Get(%q) found = true, want false", name)
			}
			if proc != nil {
				t.Errorf("Get(%q) returned processor %v, want nil", name, proc)
			}
		})
	}
}
