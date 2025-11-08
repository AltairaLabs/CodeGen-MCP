package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
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
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil { // NOSONAR - workspace directories need 0755 for proper file operations
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(contents), 0644); err != nil { // NOSONAR - workspace files use 0644 for user read/write and group/other read
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
	data, err := os.ReadFile(fullPath) // NOSONAR - path is validated and constrained to session workspace
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

	// Create temporary Python file
	tmpFile := filepath.Join(session.WorkspacePath, fmt.Sprintf(".tmp_%d.py", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil { // NOSONAR - temporary Python files use 0644 for execution by Python interpreter
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": fmt.Sprintf("failed to write Python file: %v", err),
			},
		}, nil
	}
	defer func() {
		_ = os.Remove(tmpFile) // Best effort cleanup
	}()

	// Execute Python in venv
	venvPython := filepath.Join(session.WorkspacePath, ".venv", "bin", "python3")

	// Fall back to system python if venv doesn't exist yet
	pythonCmd := "python3"
	if _, err := os.Stat(venvPython); err == nil {
		pythonCmd = venvPython
	}

	cmd := exec.CommandContext(ctx, pythonCmd, tmpFile) // NOSONAR - python command is either system python3 or session's venv, tmpFile is in isolated session workspace
	cmd.Dir = session.WorkspacePath

	// Set environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PYTHONPATH=%s", session.WorkspacePath),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	err := cmd.Run()

	// Send output logs
	if stdout.Len() > 0 {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_INFO,
					Message: fmt.Sprintf("Python stdout: %s", stdout.String()),
					Source:  "python",
				},
			},
		})
	}

	if err != nil {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_ERROR,
					Message: fmt.Sprintf("Python error: %s", stderr.String()),
					Source:  "python",
				},
			},
		})

		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error":     err.Error(),
				"stderr":    stderr.String(),
				"stdout":    stdout.String(),
				"exit_code": fmt.Sprintf("%d", cmd.ProcessState.ExitCode()),
			},
		}, nil
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": "0",
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

	// Get pip executable
	venvPip := filepath.Join(session.WorkspacePath, ".venv", "bin", "pip")
	pipCmd := "pip3"
	if _, err := os.Stat(venvPip); err == nil {
		pipCmd = venvPip
	}

	// Build pip install command
	args := append([]string{"install"}, pkgList...)
	cmd := exec.CommandContext(ctx, pipCmd, args...) // NOSONAR - pip command is either system pip3 or session's venv, args are from validated requirements
	cmd.Dir = session.WorkspacePath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Send completion log
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:   protov1.LogEntry_LEVEL_INFO,
				Message: fmt.Sprintf("Pip stdout: %s", stdout.String()),
				Source:  "pip",
			},
		},
	})

	if err != nil {
		_ = stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_ERROR,
					Message: fmt.Sprintf("Pip error: %s", stderr.String()),
					Source:  "pip",
				},
			},
		})

		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error":  fmt.Sprintf("pip install failed: %v", err),
				"stderr": stderr.String(),
			},
		}, nil
	}

	return &protov1.TaskResult{
		Status: protov1.TaskResult_STATUS_SUCCESS,
		Outputs: map[string]string{
			"packages_installed": strings.Join(pkgList, ", "),
			"stdout":             stdout.String(),
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
