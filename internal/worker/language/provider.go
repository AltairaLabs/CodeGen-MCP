package language

import (
	"context"
	"io"
	"time"
)

// Provider defines the interface for language-specific operations.
// Implementations handle environment setup, code execution, and package management
// for specific programming languages (Python, Node.js, etc.).
type Provider interface {
	// Name returns the language identifier (e.g., "python", "node", "go")
	Name() string

	// Initialize sets up the language environment for a session.
	// This might create virtual environments, install runtimes, configure paths, etc.
	Initialize(ctx context.Context, config InitConfig) error

	// Execute runs code in the language environment.
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error)

	// InstallPackages installs language-specific packages/dependencies.
	InstallPackages(ctx context.Context, packages []string) error

	// ListPackages returns currently installed packages with version information.
	ListPackages(ctx context.Context) ([]PackageInfo, error)

	// Cleanup tears down the language environment and releases resources.
	Cleanup(ctx context.Context) error

	// Supports returns true if this provider supports the given operation.
	// Used for capability checking (e.g., "run.python", "pip.install").
	Supports(operation string) bool
}

// InitConfig provides configuration for initializing a language environment.
type InitConfig struct {
	// WorkspacePath is the absolute path to the session workspace
	WorkspacePath string

	// SessionID is the unique identifier for this session
	SessionID string

	// EnvVars contains environment variables to set in the language environment
	EnvVars map[string]string

	// Config contains language-specific configuration options
	Config map[string]interface{}
}

// ExecuteRequest defines parameters for code execution.
type ExecuteRequest struct {
	// Code is the source code to execute
	Code string

	// Args are command-line arguments to pass to the execution
	Args []string

	// Stdin provides input to the executing code
	Stdin io.Reader

	// WorkingDir is the directory to run the code in (defaults to workspace)
	WorkingDir string

	// Timeout specifies maximum execution time
	Timeout time.Duration

	// Env contains additional environment variables for this execution
	Env map[string]string
}

// ExecuteResult contains the results of code execution.
type ExecuteResult struct {
	// Stdout contains standard output from the execution
	Stdout string

	// Stderr contains standard error output from the execution
	Stderr string

	// ExitCode is the process exit code (0 for success)
	ExitCode int

	// Duration is how long the execution took
	Duration time.Duration

	// Error contains any execution error (distinct from non-zero exit codes)
	Error error
}

// PackageInfo describes an installed package.
type PackageInfo struct {
	// Name is the package identifier
	Name string

	// Version is the installed version
	Version string

	// Location is the installation path (optional)
	Location string
}
