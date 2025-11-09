package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

// Provider is a mock language provider for testing.
// It provides instant, no-op implementations that don't require actual language runtimes.
type Provider struct {
	name             string
	initCalled       bool
	initError        error
	installedPkgs    []string
	executeCallCount int
	executeResults   []*language.ExecuteResult
	executeErrors    []error
	cleanupCalled    bool
	cleanupError     error
	supportedOps     map[string]bool
}

// NewProvider creates a new mock provider with the given language name.
func NewProvider(name string) *Provider {
	return &Provider{
		name:           name,
		installedPkgs:  []string{},
		executeResults: []*language.ExecuteResult{},
		executeErrors:  []error{},
		supportedOps:   make(map[string]bool),
	}
}

// Name returns the mock language name.
func (p *Provider) Name() string {
	return p.name
}

// Initialize simulates environment initialization (instant, no actual setup).
func (p *Provider) Initialize(ctx context.Context, config language.InitConfig) error {
	p.initCalled = true
	return p.initError
}

// Execute simulates code execution.
// Returns queued results (if any) or a default success result.
func (p *Provider) Execute(ctx context.Context, req *language.ExecuteRequest) (*language.ExecuteResult, error) {
	defer func() { p.executeCallCount++ }()

	if len(p.executeResults) > 0 {
		result := p.executeResults[0]
		p.executeResults = p.executeResults[1:]

		var err error
		if len(p.executeErrors) > 0 {
			err = p.executeErrors[0]
			p.executeErrors = p.executeErrors[1:]
		}

		return result, err
	}

	// Default success result
	return &language.ExecuteResult{
		Stdout:   fmt.Sprintf("mock output for: %s", req.Code),
		Stderr:   "",
		ExitCode: 0,
		Duration: 1 * time.Millisecond,
		Error:    nil,
	}, nil
}

// InstallPackages simulates package installation (instant, no actual installation).
func (p *Provider) InstallPackages(ctx context.Context, packages []string) error {
	p.installedPkgs = append(p.installedPkgs, packages...)
	return nil
}

// ListPackages returns the list of "installed" packages from InstallPackages calls.
func (p *Provider) ListPackages(ctx context.Context) ([]language.PackageInfo, error) {
	var result []language.PackageInfo
	for _, pkg := range p.installedPkgs {
		result = append(result, language.PackageInfo{
			Name:    pkg,
			Version: "1.0.0",
		})
	}
	return result, nil
}

// Cleanup simulates environment cleanup (instant, no actual cleanup).
func (p *Provider) Cleanup(ctx context.Context) error {
	p.cleanupCalled = true
	return p.cleanupError
}

// Supports returns true if the operation was marked as supported via SetSupports.
// By default, returns true for all operations.
func (p *Provider) Supports(operation string) bool {
	if len(p.supportedOps) == 0 {
		return true // Support everything by default
	}
	return p.supportedOps[operation]
}

// Test helper methods

// SetInitError configures the provider to return an error on Initialize.
func (p *Provider) SetInitError(err error) {
	p.initError = err
}

// SetCleanupError configures the provider to return an error on Cleanup.
func (p *Provider) SetCleanupError(err error) {
	p.cleanupError = err
}

// QueueExecuteResult queues a result to be returned by the next Execute call.
// Multiple results can be queued for multiple Execute calls.
func (p *Provider) QueueExecuteResult(result *language.ExecuteResult, err error) {
	p.executeResults = append(p.executeResults, result)
	p.executeErrors = append(p.executeErrors, err)
}

// SetSupports configures which operations this provider supports.
// If never called, the provider supports all operations by default.
func (p *Provider) SetSupports(operation string, supported bool) {
	p.supportedOps[operation] = supported
}

// Inspection methods for tests

// InitCalled returns true if Initialize was called.
func (p *Provider) InitCalled() bool {
	return p.initCalled
}

// ExecuteCallCount returns the number of times Execute was called.
func (p *Provider) ExecuteCallCount() int {
	return p.executeCallCount
}

// InstalledPackages returns the list of packages passed to InstallPackages.
func (p *Provider) InstalledPackages() []string {
	return p.installedPkgs
}

// CleanupCalled returns true if Cleanup was called.
func (p *Provider) CleanupCalled() bool {
	return p.cleanupCalled
}
