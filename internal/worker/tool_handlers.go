package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

const (
	errInvalidPath          = "invalid path: %w"
	progressPythonExecuting = 25
	progressPackageInstall  = 50
)

// handleEcho handles the echo tool (for testing)
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleEcho(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest) (*protov1.TaskResult, error) {
	message, ok := req.Arguments["message"]
	if !ok {
		message = "Hello from worker!"
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"message": message,
		},
	}, nil
}

// handleFsWrite handles the fs.write tool
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsWrite(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest) (*protov1.TaskResult, error) {
	path, ok := req.Arguments["path"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: path")
	}

	contents, ok := req.Arguments["contents"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: contents")
	}

	// Validate path is within workspace
	if err := validateWorkspacePath(path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, path)

	// Create parent directories
	//nolint:gosec // G301: Workspace directories need 0755 for proper file operations
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Write file
	//nolint:gosec // G306: Workspace files use 0644 for user read/write and group/other read
	if err := os.WriteFile(fullPath, []byte(contents), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"path":          path,
			"bytes_written": fmt.Sprintf("%d", len(contents)),
		},
	}, nil
}

// handleFsRead handles the fs.read tool
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsRead(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest) (*protov1.TaskResult, error) {
	path, ok := req.Arguments["path"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: path")
	}

	// Validate path is within workspace
	if err := validateWorkspacePath(path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, path)

	// Read file
	//nolint:gosec // G304: Path is validated and constrained to session workspace
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"path":     path,
			"contents": string(data),
			"size":     fmt.Sprintf("%d", len(data)),
		},
	}, nil
}

// handleFsList handles the fs.list tool
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsList(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest) (*protov1.TaskResult, error) {
	path, ok := req.Arguments["path"]
	if !ok {
		path = "."
	}

	// Validate path is within workspace
	if err := validateWorkspacePath(path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, path)

	// List directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	// Build file list
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"path":  path,
			"files": strings.Join(files, "\n"),
			"count": fmt.Sprintf("%d", len(files)),
		},
	}, nil
}

// handleRunPython handles the run.python tool
//
//nolint:lll // Protobuf types create inherently long function signatures
func (te *TaskExecutor) handleRunPython(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	code, ok := req.Arguments["code"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: code")
	}

	// Check if provider supports Python execution
	if session.Provider == nil || !session.Provider.Supports("run.python") {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": "Python execution not supported by session provider",
			},
		}, nil
	}

	// Send progress
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: progressPythonExecuting,
				Stage:           "executing_python",
				Message:         "Executing Python code...",
			},
		},
	})

	// Send execution log
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:   protov1.LogEntry_LEVEL_INFO,
				Message: "Executing Python script...",
				Source:  "python",
			},
		},
	})

	// Execute via provider
	result, err := session.Provider.Execute(ctx, &language.ExecuteRequest{
		Code:       code,
		WorkingDir: session.WorkspacePath,
		Env: map[string]string{
			"PYTHONPATH": session.WorkspacePath,
		},
	})

	if err != nil {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_ERROR,
					Message: fmt.Sprintf("Python error: %v", err),
					Source:  "python",
				},
			},
		})

		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error":     err.Error(),
				"stderr":    result.Stderr,
				"stdout":    result.Stdout,
				"exit_code": fmt.Sprintf("%d", result.ExitCode),
			},
		}, nil
	}

	// Send output logs
	if result.Stdout != "" {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_INFO,
					Message: fmt.Sprintf("Python stdout: %s", result.Stdout),
					Source:  "python",
				},
			},
		})
	}

	if result.ExitCode != 0 {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_ERROR,
					Message: fmt.Sprintf("Python stderr: %s", result.Stderr),
					Source:  "python",
				},
			},
		})

		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error":     "Python execution failed",
				"stderr":    result.Stderr,
				"stdout":    result.Stdout,
				"exit_code": fmt.Sprintf("%d", result.ExitCode),
			},
		}, nil
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"exit_code": fmt.Sprintf("%d", result.ExitCode),
		},
	}, nil
}

// handlePkgInstall handles the pkg.install tool
//
//nolint:lll // Protobuf types create inherently long function signatures
func (te *TaskExecutor) handlePkgInstall(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	requirements, ok := req.Arguments["requirements"]
	if !ok {
		return nil, fmt.Errorf("missing required argument: requirements")
	}

	// Check if provider supports package installation
	if session.Provider == nil || !session.Provider.Supports("pip.install") {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": "Package installation not supported by session provider",
			},
		}, nil
	}

	// Send progress
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: progressPackageInstall,
				Stage:           "installing_packages",
				Message:         "Installing Python packages...",
			},
		},
	})

	// Send log
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:   protov1.LogEntry_LEVEL_INFO,
				Message: fmt.Sprintf("Installing packages: %s", requirements),
				Source:  "pip",
			},
		},
	})

	// Split requirements by newline (requirements is already a string from map[string]string)
	var pkgList []string
	for _, line := range strings.Split(requirements, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			pkgList = append(pkgList, line)
		}
	}

	if len(pkgList) == 0 {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": "no packages specified",
			},
		}, nil
	}

	// Install via provider
	err := session.Provider.InstallPackages(ctx, pkgList)

	if err != nil {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_ERROR,
					Message: fmt.Sprintf("Package installation error: %v", err),
					Source:  "pip",
				},
			},
		})

		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": fmt.Sprintf("pip install failed: %v", err),
			},
		}, nil
	}

	// Send completion log
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:   protov1.LogEntry_LEVEL_INFO,
				Message: fmt.Sprintf("Successfully installed: %s", strings.Join(pkgList, ", ")),
				Source:  "pip",
			},
		},
	})

	// Update session's installed packages list
	session.mu.Lock()
	session.InstalledPkgs = append(session.InstalledPkgs, pkgList...)
	session.mu.Unlock()

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"packages_installed": strings.Join(pkgList, ", "),
		},
	}, nil
}

// validateWorkspacePath validates that a path is safe (no traversal attacks)
func validateWorkspacePath(path string) error {
	// Check for absolute paths
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}

	// Check for parent directory traversal
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "/..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	return nil
}
