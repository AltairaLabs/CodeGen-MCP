package python

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

// Provider implements language.Provider for Python environments.
type Provider struct {
	venvPath    string
	workspace   string
	initialized bool
}

var errNotInitialized = fmt.Errorf("provider not initialized")

// NewProvider creates a new Python language provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Name returns the language identifier.
func (p *Provider) Name() string {
	return "python"
}

// Initialize sets up a Python virtual environment for the session.
func (p *Provider) Initialize(ctx context.Context, config language.InitConfig) error {
	p.workspace = config.WorkspacePath
	p.venvPath = filepath.Join(config.WorkspacePath, ".venv")

	// Check if python3 is available
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("python3 not found in PATH: %w", err)
	}

	// Create venv
	//nolint:gosec // G204: python3 is system binary, venvPath is in isolated session workspace
	cmd := exec.CommandContext(ctx, "python3", "-m", "venv", p.venvPath)
	cmd.Dir = config.WorkspacePath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create venv: %w (output: %s)", err, string(output))
	}

	// Upgrade pip in the venv
	pipPath := filepath.Join(p.venvPath, "bin", "pip")
	//nolint:gosec // G204: pip binary is in session's isolated venv directory
	cmd = exec.CommandContext(ctx, pipPath, "install", "--upgrade", "pip")
	cmd.Dir = config.WorkspacePath

	if output, err := cmd.CombinedOutput(); err != nil {
		// Non-fatal - log but don't fail
		_ = fmt.Errorf("failed to upgrade pip: %w (output: %s)", err, string(output))
	}

	p.initialized = true
	return nil
}

// Execute runs Python code in the virtual environment.
func (p *Provider) Execute(ctx context.Context, req *language.ExecuteRequest) (*language.ExecuteResult, error) {
	if !p.initialized {
		return nil, errNotInitialized
	}

	pythonPath := filepath.Join(p.venvPath, "bin", "python")

	// Build command
	args := []string{"-c", req.Code}
	args = append(args, req.Args...)

	//nolint:gosec // G204: pythonPath is in session's isolated venv directory
	cmd := exec.CommandContext(ctx, pythonPath, args...)

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	} else {
		cmd.Dir = p.workspace
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range req.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Set stdin if provided
	if req.Stdin != nil {
		cmd.Stdin = req.Stdin
	}

	// Execute and capture output
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &language.ExecuteResult{
		Duration: duration,
	}

	// Split output into stdout/stderr (combined in this case)
	outputStr := string(output)
	if err != nil {
		// If there's an error, treat all output as stderr
		result.Stderr = outputStr
		result.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Error = err
	} else {
		result.Stdout = outputStr
		result.ExitCode = 0
	}

	return result, nil
}

// InstallPackages installs Python packages using pip.
func (p *Provider) InstallPackages(ctx context.Context, packages []string) error {
	if !p.initialized {
		return errNotInitialized
	}

	if len(packages) == 0 {
		return nil
	}

	pipPath := filepath.Join(p.venvPath, "bin", "pip")
	args := append([]string{"install"}, packages...)

	//nolint:gosec // G204: pip binary is in session's isolated venv directory
	cmd := exec.CommandContext(ctx, pipPath, args...)
	cmd.Dir = p.workspace

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install packages: %w (output: %s)", err, string(output))
	}

	return nil
}

// ListPackages returns installed Python packages.
func (p *Provider) ListPackages(ctx context.Context) ([]language.PackageInfo, error) {
	if !p.initialized {
		return nil, errNotInitialized
	}

	pipPath := filepath.Join(p.venvPath, "bin", "pip")

	//nolint:gosec // G204: pip binary is in session's isolated venv directory
	cmd := exec.CommandContext(ctx, pipPath, "list", "--format=json")
	cmd.Dir = p.workspace

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list packages: %w (output: %s)", err, string(output))
	}

	// Parse JSON output
	var pipPackages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if err := json.Unmarshal(output, &pipPackages); err != nil {
		return nil, fmt.Errorf("failed to parse pip list output: %w", err)
	}

	// Convert to PackageInfo
	result := make([]language.PackageInfo, len(pipPackages))
	for i, pkg := range pipPackages {
		result[i] = language.PackageInfo{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}

	return result, nil
}

// Cleanup removes the Python virtual environment.
func (p *Provider) Cleanup(ctx context.Context) error {
	if p.venvPath == "" {
		return nil
	}

	// Safety check: only remove if it's a .venv directory
	if !strings.HasSuffix(p.venvPath, ".venv") {
		return fmt.Errorf("refusing to remove non-venv path: %s", p.venvPath)
	}

	return os.RemoveAll(p.venvPath)
}

// Supports returns true for Python-related operations.
func (p *Provider) Supports(operation string) bool {
	supportedOps := map[string]bool{
		"run.python":  true,
		"pip.install": true,
		"pip.list":    true,
	}
	return supportedOps[operation]
}
