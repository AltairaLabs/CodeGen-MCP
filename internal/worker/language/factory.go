package language

import (
	"fmt"
	"sync"
)

// Factory creates language providers.
type Factory interface {
	// CreateProvider creates a new provider instance for the specified language.
	CreateProvider(language string) (Provider, error)
}

// Registry is a factory that maintains registered language providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]func() Provider
}

// NewRegistry creates a new provider registry with default providers.
// Python provider is registered by default.
func NewRegistry() *Registry {
	r := &Registry{
		providers: make(map[string]func() Provider),
	}
	// Note: Python provider registration moved to avoid import cycle
	// Call RegisterDefaultProviders() after creating the registry
	return r
}

// Register adds a provider factory to the registry.
// The factory function is called each time a new provider instance is needed.
func (r *Registry) Register(name string, factory func() Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = factory
}

// CreateProvider creates a new provider instance for the specified language.
func (r *Registry) CreateProvider(language string) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.providers[language]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	return factory(), nil
}

// ListSupported returns the names of all registered language providers.
func (r *Registry) ListSupported() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// IsSupported returns true if the given language has a registered provider.
func (r *Registry) IsSupported(language string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[language]
	return ok
}
