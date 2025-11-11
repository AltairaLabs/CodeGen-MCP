package worker

import (
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language/mock"
)

// mockProviderFactory creates mock providers for testing
type mockProviderFactory struct {
	provider *mock.Provider
}

func (f *mockProviderFactory) CreateProvider(lang string) (language.Provider, error) {
	if f.provider != nil {
		return f.provider, nil
	}
	return mock.NewProvider(lang), nil
}

// newTestSessionPool creates a session pool with mock providers for fast testing.
// Use this for unit tests that don't need real language runtimes.
func newTestSessionPool(workerID string, maxSessions int32, baseWorkspace string) *SessionPool {
	factory := &mockProviderFactory{
		provider: mock.NewProvider("python"),
	}
	return NewSessionPoolWithFactory(workerID, maxSessions, baseWorkspace, factory)
}
