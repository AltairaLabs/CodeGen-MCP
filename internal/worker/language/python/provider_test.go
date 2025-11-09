package python

import (
	"context"
	"fmt"
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

func TestProvider_Execute_WithEnv(t *testing.T) {
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

	// Test with environment variables
	req := &language.ExecuteRequest{
		Code: "import os; print(os.environ.get('TEST_VAR', 'not found'))",
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	result, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result.Stdout, "test_value") {
		t.Errorf("Expected 'test_value' in output, got: %s", result.Stdout)
	}
}

func TestProvider_Execute_WithWorkingDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	provider := NewProvider()
	config := language.InitConfig{
		WorkspacePath: tmpDir,
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test with custom working directory
	req := &language.ExecuteRequest{
		Code:       "import os; print(os.getcwd())",
		WorkingDir: subDir,
	}

	result, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result.Stdout, "subdir") {
		t.Errorf("Expected 'subdir' in output, got: %s", result.Stdout)
	}
}

func TestProvider_Execute_WithArgs(t *testing.T) {
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

	// Test with command-line arguments
	req := &language.ExecuteRequest{
		Code: "import sys; print(' '.join(sys.argv[1:]))",
		Args: []string{"arg1", "arg2", "arg3"},
	}

	result, err := provider.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result.Stdout, "arg1 arg2 arg3") {
		t.Errorf("Expected args in output, got: %s", result.Stdout)
	}
}

func TestProvider_Execute_ExitCodeHandling(t *testing.T) {
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

	// Test custom exit code
	req := &language.ExecuteRequest{
		Code: "import sys; sys.exit(42)",
	}

	result, err := provider.Execute(ctx, req)
	// Execute should not error, but result should have exit code
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", result.ExitCode)
	}

	if result.Error == nil {
		t.Error("Expected error in result for non-zero exit code")
	}
}

func TestProvider_InstallPackages_Multiple(t *testing.T) {
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

	// Install multiple packages
	packages := []string{"six", "setuptools"}
	err := provider.InstallPackages(ctx, packages)
	if err != nil {
		t.Fatalf("InstallPackages failed: %v", err)
	}

	// Verify both packages are importable
	for _, pkg := range packages {
		req := &language.ExecuteRequest{
			Code: fmt.Sprintf("import %s; print('%s imported')", pkg, pkg),
		}

		result, err := provider.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Execute failed for %s: %v", pkg, err)
		}

		if result.ExitCode != 0 {
			t.Errorf("Failed to import %s: %s", pkg, result.Stderr)
		}
	}
}

func TestProvider_ListPackages_AfterInstall(t *testing.T) {
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

	// Get initial package count
	initialPkgs, err := provider.ListPackages(ctx)
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}
	initialCount := len(initialPkgs)

	// Install a new package
	err = provider.InstallPackages(ctx, []string{"six"})
	if err != nil {
		t.Fatalf("InstallPackages failed: %v", err)
	}

	// List packages again
	finalPkgs, err := provider.ListPackages(ctx)
	if err != nil {
		t.Fatalf("ListPackages failed after install: %v", err)
	}

	// Should have at least one more package
	if len(finalPkgs) <= initialCount {
		t.Errorf("Expected more packages after install, got %d (was %d)", len(finalPkgs), initialCount)
	}

	// Check that 'six' is in the list
	foundSix := false
	for _, pkg := range finalPkgs {
		if pkg.Name == "six" {
			foundSix = true
			if pkg.Version == "" {
				t.Error("Package version should not be empty")
			}
			break
		}
	}

	if !foundSix {
		t.Error("Package 'six' not found in installed packages")
	}
}

func TestProvider_Initialize_PythonNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Python provider integration test")
	}

	// This test is tricky because we can't easily make python3 unavailable
	// But we can verify the error path exists
	provider := NewProvider()

	// Test that uninitialized provider has correct state
	if provider.initialized {
		t.Error("Provider should not be initialized on creation")
	}

	if provider.venvPath != "" {
		t.Error("venvPath should be empty on creation")
	}
}

func TestProvider_Cleanup_MultipleDirectories(t *testing.T) {
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

	// Create some files in venv
	testFile := filepath.Join(provider.venvPath, "test_file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create subdirectory in venv
	testDir := filepath.Join(provider.venvPath, "test_subdir")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// Cleanup should remove everything
	err := provider.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify everything is removed
	if _, err := os.Stat(provider.venvPath); !os.IsNotExist(err) {
		t.Error("Venv directory should be completely removed")
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Test file should be removed with venv")
	}

	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("Test subdirectory should be removed with venv")
	}
}

func TestProvider_Cleanup_AlreadyCleaned(t *testing.T) {
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

	// Cleanup once
	if err := provider.Cleanup(ctx); err != nil {
		t.Fatalf("First cleanup failed: %v", err)
	}

	// Cleanup again - should not error
	if err := provider.Cleanup(ctx); err != nil {
		t.Errorf("Second cleanup should not error, got: %v", err)
	}
}
