package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

func TestNewProvider(t *testing.T) {
	provider := NewProvider("test-lang")

	if provider == nil {
		t.Fatal("NewProvider returned nil")
	}

	if provider.Name() != "test-lang" {
		t.Errorf("Expected name 'test-lang', got '%s'", provider.Name())
	}

	if provider.initCalled {
		t.Error("initCalled should be false initially")
	}

	if provider.cleanupCalled {
		t.Error("cleanupCalled should be false initially")
	}

	if provider.executeCallCount != 0 {
		t.Errorf("Expected executeCallCount 0, got %d", provider.executeCallCount)
	}
}

func TestProvider_Initialize(t *testing.T) {
	ctx := context.Background()

	t.Run("successful initialization", func(t *testing.T) {
		provider := NewProvider("python")
		config := language.InitConfig{
			WorkspacePath: "/tmp/test",
		}

		err := provider.Initialize(ctx, config)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if !provider.InitCalled() {
			t.Error("InitCalled should return true after Initialize")
		}
	})

	t.Run("initialization with error", func(t *testing.T) {
		provider := NewProvider("python")
		expectedErr := errors.New("init failed")
		provider.SetInitError(expectedErr)

		config := language.InitConfig{
			WorkspacePath: "/tmp/test",
		}

		err := provider.Initialize(ctx, config)
		if err != expectedErr {
			t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
		}

		if !provider.InitCalled() {
			t.Error("InitCalled should return true even on error")
		}
	})
}

func TestProvider_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("default execution", func(t *testing.T) {
		provider := NewProvider("python")
		req := &language.ExecuteRequest{
			Code: "print('hello')",
		}

		result, err := provider.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if result == nil {
			t.Fatal("Execute returned nil result")
		}

		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", result.ExitCode)
		}

		if !contains(result.Stdout, "print('hello')") {
			t.Errorf("Expected stdout to contain code, got: %s", result.Stdout)
		}

		if provider.ExecuteCallCount() != 1 {
			t.Errorf("Expected 1 execute call, got %d", provider.ExecuteCallCount())
		}
	})

	t.Run("queued results", func(t *testing.T) {
		provider := NewProvider("python")

		// Queue multiple results
		result1 := &language.ExecuteResult{
			Stdout:   "output 1",
			ExitCode: 0,
		}
		result2 := &language.ExecuteResult{
			Stdout:   "output 2",
			ExitCode: 1,
		}

		provider.QueueExecuteResult(result1, nil)
		provider.QueueExecuteResult(result2, errors.New("execution failed"))

		// First execution
		req := &language.ExecuteRequest{Code: "test1"}
		got, err := provider.Execute(ctx, req)
		if err != nil {
			t.Errorf("First execute should not error, got: %v", err)
		}
		if got.Stdout != "output 1" {
			t.Errorf("Expected 'output 1', got '%s'", got.Stdout)
		}

		// Second execution
		req = &language.ExecuteRequest{Code: "test2"}
		got, err = provider.Execute(ctx, req)
		if err == nil {
			t.Error("Second execute should error")
		}
		if got.Stdout != "output 2" {
			t.Errorf("Expected 'output 2', got '%s'", got.Stdout)
		}

		// Third execution (no more queued results, use default)
		req = &language.ExecuteRequest{Code: "test3"}
		got, err = provider.Execute(ctx, req)
		if err != nil {
			t.Errorf("Third execute should not error, got: %v", err)
		}
		if !contains(got.Stdout, "test3") {
			t.Errorf("Expected default output to contain code")
		}

		if provider.ExecuteCallCount() != 3 {
			t.Errorf("Expected 3 execute calls, got %d", provider.ExecuteCallCount())
		}
	})
}

func TestProvider_InstallPackages(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider("python")

	packages := []string{"requests", "flask", "numpy"}
	err := provider.InstallPackages(ctx, packages)
	if err != nil {
		t.Fatalf("InstallPackages failed: %v", err)
	}

	installed := provider.InstalledPackages()
	if len(installed) != 3 {
		t.Errorf("Expected 3 packages, got %d", len(installed))
	}

	for _, pkg := range packages {
		if !containsString(installed, pkg) {
			t.Errorf("Package '%s' not found in installed packages", pkg)
		}
	}

	// Install more packages
	more := []string{"pandas"}
	err = provider.InstallPackages(ctx, more)
	if err != nil {
		t.Fatalf("Second InstallPackages failed: %v", err)
	}

	installed = provider.InstalledPackages()
	if len(installed) != 4 {
		t.Errorf("Expected 4 packages after second install, got %d", len(installed))
	}
}

func TestProvider_ListPackages(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider("python")

	// Install some packages first
	packages := []string{"requests", "flask"}
	err := provider.InstallPackages(ctx, packages)
	if err != nil {
		t.Fatalf("InstallPackages failed: %v", err)
	}

	// List packages
	pkgInfo, err := provider.ListPackages(ctx)
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}

	if len(pkgInfo) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(pkgInfo))
	}

	for _, info := range pkgInfo {
		if !containsString(packages, info.Name) {
			t.Errorf("Unexpected package: %s", info.Name)
		}
		if info.Version != "1.0.0" {
			t.Errorf("Expected version 1.0.0, got %s", info.Version)
		}
	}
}

func TestProvider_Cleanup(t *testing.T) {
	ctx := context.Background()

	t.Run("successful cleanup", func(t *testing.T) {
		provider := NewProvider("python")

		err := provider.Cleanup(ctx)
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		if !provider.CleanupCalled() {
			t.Error("CleanupCalled should return true after Cleanup")
		}
	})

	t.Run("cleanup with error", func(t *testing.T) {
		provider := NewProvider("python")
		expectedErr := errors.New("cleanup failed")
		provider.SetCleanupError(expectedErr)

		err := provider.Cleanup(ctx)
		if err != expectedErr {
			t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
		}

		if !provider.CleanupCalled() {
			t.Error("CleanupCalled should return true even on error")
		}
	})
}

func TestProvider_Supports(t *testing.T) {
	t.Run("default supports all", func(t *testing.T) {
		provider := NewProvider("python")

		operations := []string{"run.python", "pip.install", "custom.op"}
		for _, op := range operations {
			if !provider.Supports(op) {
				t.Errorf("Provider should support '%s' by default", op)
			}
		}
	})

	t.Run("explicit support configuration", func(t *testing.T) {
		provider := NewProvider("python")

		// Configure specific operations
		provider.SetSupports("run.python", true)
		provider.SetSupports("pip.install", true)
		provider.SetSupports("custom.op", false)

		if !provider.Supports("run.python") {
			t.Error("Should support run.python")
		}

		if !provider.Supports("pip.install") {
			t.Error("Should support pip.install")
		}

		if provider.Supports("custom.op") {
			t.Error("Should not support custom.op")
		}

		if provider.Supports("unknown.op") {
			t.Error("Should not support unknown.op (not explicitly set)")
		}
	})
}

func TestProvider_ExecuteResult_Duration(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider("python")

	req := &language.ExecuteRequest{
		Code: "test",
	}

	result, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	// Should be ~1ms (mock execution time)
	if result.Duration > 10*time.Millisecond {
		t.Errorf("Duration too long for mock: %v", result.Duration)
	}
}

func TestProvider_Name(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
	}{
		{"python", "python"},
		{"node", "node"},
		{"go", "go"},
		{"custom", "my-custom-lang"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.providerName)
			if provider.Name() != tt.providerName {
				t.Errorf("Expected name '%s', got '%s'", tt.providerName, provider.Name())
			}
		})
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) > 0 && (s == substr || len(substr) > 0 && len(s) >= len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr) && containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
