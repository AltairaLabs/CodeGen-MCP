package python

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

func TestNewProvider(t *testing.T) {
	provider := NewProvider()
	if provider == nil {
		t.Fatal("NewProvider returned nil")
	}

	if provider.Name() != "python" {
		t.Errorf("Expected name 'python', got '%s'", provider.Name())
	}

	if provider.initialized {
		t.Error("Provider should not be initialized on creation")
	}
}

func TestProvider_Initialize(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	err := provider.Initialize(ctx, config)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !provider.initialized {
		t.Error("Provider should be initialized after Initialize")
	}

	// Verify venv was created
	venvPath := filepath.Join(tmpDir, ".venv")
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		t.Error("Virtual environment directory not created")
	}

	// Verify python executable exists
	pythonPath := filepath.Join(venvPath, "bin", "python")
	if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
		t.Error("Python executable not found in venv")
	}

	// Verify pip executable exists
	pipPath := filepath.Join(venvPath, "bin", "pip")
	if _, err := os.Stat(pipPath); os.IsNotExist(err) {
		t.Error("Pip executable not found in venv")
	}
}

func TestProvider_Execute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tests := []struct {
		name     string
		code     string
		wantExit int
		checkOut func(t *testing.T, result *language.ExecuteResult)
	}{
		{
			name:     "simple print",
			code:     "print('hello world')",
			wantExit: 0,
			checkOut: func(t *testing.T, result *language.ExecuteResult) {
				if !strings.Contains(result.Stdout, "hello world") {
					t.Errorf("Expected 'hello world' in stdout, got: %s", result.Stdout)
				}
			},
		},
		{
			name:     "arithmetic",
			code:     "print(2 + 2)",
			wantExit: 0,
			checkOut: func(t *testing.T, result *language.ExecuteResult) {
				if !strings.Contains(result.Stdout, "4") {
					t.Errorf("Expected '4' in stdout, got: %s", result.Stdout)
				}
			},
		},
		{
			name:     "import sys",
			code:     "import sys; print(sys.version_info.major >= 3)",
			wantExit: 0,
			checkOut: func(t *testing.T, result *language.ExecuteResult) {
				if !strings.Contains(result.Stdout, "True") {
					t.Errorf("Expected Python 3+, got stdout: %s", result.Stdout)
				}
			},
		},
		{
			name:     "syntax error",
			code:     "print('unclosed string",
			wantExit: 1,
			checkOut: func(t *testing.T, result *language.ExecuteResult) {
				if result.Error == nil {
					t.Error("Expected error for syntax error")
				}
			},
		},
		{
			name:     "runtime error",
			code:     "1/0",
			wantExit: 1,
			checkOut: func(t *testing.T, result *language.ExecuteResult) {
				if result.Error == nil {
					t.Error("Expected error for division by zero")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &language.ExecuteRequest{
				Code: tt.code,
			}

			result, err := provider.Execute(ctx, req)
			if err != nil && tt.wantExit == 0 {
				t.Fatalf("Execute failed: %v", err)
			}

			if result.ExitCode != tt.wantExit {
				t.Errorf("Expected exit code %d, got %d", tt.wantExit, result.ExitCode)
			}

			if tt.checkOut != nil {
				tt.checkOut(t, result)
			}

			if result.Duration == 0 {
				t.Error("Expected non-zero duration")
			}
		})
	}
}

func TestProvider_Execute_NotInitialized(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider()

	req := &language.ExecuteRequest{
		Code: "print('test')",
	}

	_, err := provider.Execute(ctx, req)
	if err != errNotInitialized {
		t.Errorf("Expected errNotInitialized, got: %v", err)
	}
}

func TestProvider_InstallPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Install a simple package
	packages := []string{"six"}
	err := provider.InstallPackages(ctx, packages)
	if err != nil {
		t.Fatalf("InstallPackages failed: %v", err)
	}

	// Verify package is installed by importing it
	req := &language.ExecuteRequest{
		Code: "import six; print(six.__version__)",
	}

	result, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected successful import, got exit code %d", result.ExitCode)
	}

	if len(result.Stdout) == 0 {
		t.Error("Expected version output from six package")
	}
}

func TestProvider_InstallPackages_NotInitialized(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider()

	err := provider.InstallPackages(ctx, []string{"requests"})
	if err != errNotInitialized {
		t.Errorf("Expected errNotInitialized, got: %v", err)
	}
}

func TestProvider_InstallPackages_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Empty package list should be no-op
	err := provider.InstallPackages(ctx, []string{})
	if err != nil {
		t.Errorf("InstallPackages with empty list should not error, got: %v", err)
	}
}

func TestProvider_ListPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// List packages (should have at least pip and setuptools)
	packages, err := provider.ListPackages(ctx)
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}

	if len(packages) == 0 {
		t.Error("Expected at least some packages (pip, setuptools)")
	}

	// Check that packages have names and versions
	for _, pkg := range packages {
		if pkg.Name == "" {
			t.Error("Package has empty name")
		}
		if pkg.Version == "" {
			t.Error("Package has empty version")
		}
	}
}

func TestProvider_ListPackages_NotInitialized(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider()

	_, err := provider.ListPackages(ctx)
	if err != errNotInitialized {
		t.Errorf("Expected errNotInitialized, got: %v", err)
	}
}

func TestProvider_Cleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	venvPath := filepath.Join(tmpDir, ".venv")

	// Verify venv exists
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		t.Fatal("Venv should exist before cleanup")
	}

	// Cleanup
	err := provider.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify venv is removed
	if _, err := os.Stat(venvPath); !os.IsNotExist(err) {
		t.Error("Venv should be removed after cleanup")
	}
}

func TestProvider_Cleanup_SafetyCheck(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider()

	// Try to cleanup a non-venv path
	provider.venvPath = "/tmp/not-a-venv"

	err := provider.Cleanup(ctx)
	if err == nil {
		t.Error("Expected error when cleaning up non-venv path")
	}

	if !strings.Contains(err.Error(), "refusing to remove") {
		t.Errorf("Expected 'refusing to remove' error, got: %v", err)
	}
}

func TestProvider_Cleanup_EmptyPath(t *testing.T) {
	ctx := context.Background()
	provider := NewProvider()

	// Empty venvPath should be no-op
	err := provider.Cleanup(ctx)
	if err != nil {
		t.Errorf("Cleanup with empty path should not error, got: %v", err)
	}
}

func TestProvider_Supports(t *testing.T) {
	provider := NewProvider()

	tests := []struct {
		operation string
		want      bool
	}{
		{"run.python", true},
		{"pip.install", true},
		{"pip.list", true},
		{"unknown.operation", false},
		{"run.node", false},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			got := provider.Supports(tt.operation)
			if got != tt.want {
				t.Errorf("Supports(%s) = %v, want %v", tt.operation, got, tt.want)
			}
		})
	}
}

func TestProvider_Name(t *testing.T) {
	provider := NewProvider()
	if provider.Name() != "python" {
		t.Errorf("Expected name 'python', got '%s'", provider.Name())
	}
}
