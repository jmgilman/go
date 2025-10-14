package attributes

import (
	"context"
	"fmt"
	"sync"

	"cuelang.org/go/cue"
)

// Processor handles a specific custom attribute type.
// Upstream packages (worker, platform-api) implement this interface.
type Processor interface {
	// Name returns the attribute name (e.g., "artifact").
	Name() string

	// Process resolves the attribute and returns the replacement value.
	// Returns an error if the attribute cannot be resolved.
	Process(ctx context.Context, attr Attribute) (cue.Value, error)
}

// Registry manages registered attribute processors.
// It provides thread-safe registration and lookup of processors.
type Registry struct {
	mu         sync.RWMutex
	processors map[string]Processor
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		processors: make(map[string]Processor),
	}
}

// Register adds a processor to the registry.
// Returns error if a processor with the same name is already registered.
func (r *Registry) Register(p Processor) error {
	if p == nil {
		return fmt.Errorf("processor cannot be nil")
	}

	name := p.Name()
	if name == "" {
		return fmt.Errorf("processor name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.processors[name]; exists {
		return fmt.Errorf("processor %q is already registered", name)
	}

	r.processors[name] = p
	return nil
}

// Get retrieves a processor by name.
// Returns (processor, true) if found, (nil, false) if not found.
func (r *Registry) Get(name string) (Processor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.processors[name]
	return p, ok
}
