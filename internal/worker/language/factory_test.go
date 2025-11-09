package language

import (
	"context"
	"testing"
)

// mockProvider is a simple test provider
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string                                            { return m.name }
func (m *mockProvider) Initialize(ctx context.Context, config InitConfig) error { return nil }
func (m *mockProvider) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	return &ExecuteResult{}, nil
}
func (m *mockProvider) InstallPackages(ctx context.Context, packages []string) error { return nil }
func (m *mockProvider) ListPackages(ctx context.Context) ([]PackageInfo, error)      { return nil, nil }
func (m *mockProvider) Cleanup(ctx context.Context) error                            { return nil }
func (m *mockProvider) Supports(operation string) bool                               { return true }

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}

	if registry.providers == nil {
		t.Fatal("Registry providers map not initialized")
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	// Register a test provider
	registry.Register("test", func() Provider {
		return &mockProvider{name: "test"}
	})

	// Verify it's registered
	if !registry.IsSupported("test") {
		t.Error("Provider 'test' should be registered")
	}

	// Verify we can create it
	provider, err := registry.CreateProvider("test")
	if err != nil {
		t.Fatalf("Failed to create registered provider: %v", err)
	}

	if provider.Name() != "test" {
		t.Errorf("Expected provider name 'test', got '%s'", provider.Name())
	}
}

func TestRegistry_CreateProvider(t *testing.T) {
	registry := NewRegistry()

	// Register multiple providers
	registry.Register("python", func() Provider {
		return &mockProvider{name: "python"}
	})
	registry.Register("node", func() Provider {
		return &mockProvider{name: "node"}
	})

	tests := []struct {
		name     string
		language string
		wantErr  bool
		wantName string
	}{
		{
			name:     "create python provider",
			language: "python",
			wantErr:  false,
			wantName: "python",
		},
		{
			name:     "create node provider",
			language: "node",
			wantErr:  false,
			wantName: "node",
		},
		{
			name:     "unsupported language",
			language: "ruby",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := registry.CreateProvider(tt.language)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error for unsupported language")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if provider.Name() != tt.wantName {
				t.Errorf("Expected name '%s', got '%s'", tt.wantName, provider.Name())
			}
		})
	}
}

func TestRegistry_ListSupported(t *testing.T) {
	registry := NewRegistry()

	// Initially empty
	supported := registry.ListSupported()
	if len(supported) != 0 {
		t.Errorf("Expected empty list, got %d items", len(supported))
	}

	// Register some providers
	registry.Register("python", func() Provider {
		return &mockProvider{name: "python"}
	})
	registry.Register("node", func() Provider {
		return &mockProvider{name: "node"}
	})
	registry.Register("go", func() Provider {
		return &mockProvider{name: "go"}
	})

	supported = registry.ListSupported()
	if len(supported) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(supported))
	}

	// Verify all registered names are present
	expectedNames := map[string]bool{
		"python": false,
		"node":   false,
		"go":     false,
	}

	for _, name := range supported {
		if _, exists := expectedNames[name]; !exists {
			t.Errorf("Unexpected provider name: %s", name)
		}
		expectedNames[name] = true
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected provider '%s' not in list", name)
		}
	}
}

func TestRegistry_IsSupported(t *testing.T) {
	registry := NewRegistry()

	// Not supported initially
	if registry.IsSupported("python") {
		t.Error("Python should not be supported before registration")
	}

	// Register python
	registry.Register("python", func() Provider {
		return &mockProvider{name: "python"}
	})

	// Now supported
	if !registry.IsSupported("python") {
		t.Error("Python should be supported after registration")
	}

	// Other languages still not supported
	if registry.IsSupported("node") {
		t.Error("Node should not be supported")
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	registry := NewRegistry()

	// Register initial provider
	registry.Register("python", func() Provider {
		return &mockProvider{name: "python"}
	})

	// Concurrent operations
	done := make(chan bool)
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		// Register
		go func(n int) {
			registry.Register("test", func() Provider {
				return &mockProvider{name: "test"}
			})
			done <- true
		}(i)

		// Create
		go func() {
			_, _ = registry.CreateProvider("python")
			done <- true
		}()

		// List
		go func() {
			_ = registry.ListSupported()
			done <- true
		}()

		// Check support
		go func() {
			_ = registry.IsSupported("python")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < concurrency*4; i++ {
		<-done
	}
}
