package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
)

const (
	errInvalidPath = "invalid path: %w"
)

// Typed tool handlers - these handle strongly-typed protobuf requests/responses

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

// handleEchoTyped handles the echo tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleEchoTyped(ctx context.Context, session *WorkerSession, req *protov1.EchoRequest) (*protov1.TaskResult, error) {
	// Create typed response
	response := &protov1.EchoResponse{
		Message:  req.Message,
		Metadata: req.Metadata, // Pass through metadata if provided
	}

	// Wrap in ToolResponse
	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_Echo{
			Echo: response,
		},
	}

	return &protov1.TaskResult{
		Status:        protov1.TaskResult_STATUS_SUCCESS,
		TypedResponse: toolResponse,
	}, nil
}

// handleFsReadTyped handles the fs.read tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsReadTyped(ctx context.Context, session *WorkerSession, req *protov1.FsReadRequest) (*protov1.TaskResult, error) {
	// Validate path is within workspace
	if err := validateWorkspacePath(req.Path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, req.Path)

	// Debug logging
	slog.Debug("fs.read operation",
		"session_id", session.SessionID,
		"workspace_path", session.WorkspacePath,
		"relative_path", req.Path,
		"full_path", fullPath)

	// Read file
	//nolint:gosec // G304: Path is validated and constrained to session workspace
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Create typed response
	response := &protov1.FsReadResponse{
		Contents:  string(data),
		SizeBytes: int64(len(data)),
		Encoding:  "utf-8",
		Metadata:  req.Metadata,
	}

	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_FsRead{
			FsRead: response,
		},
	}

	return &protov1.TaskResult{
		Status:        protov1.TaskResult_STATUS_SUCCESS,
		TypedResponse: toolResponse,
	}, nil
}

// handleFsWriteTyped handles the fs.write tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsWriteTyped(ctx context.Context, session *WorkerSession, req *protov1.FsWriteRequest) (*protov1.TaskResult, error) {
	// Validate path is within workspace
	if err := validateWorkspacePath(req.Path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, req.Path)

	// Debug logging
	slog.Debug("fs.write operation",
		"session_id", session.SessionID,
		"workspace_path", session.WorkspacePath,
		"relative_path", req.Path,
		"full_path", fullPath,
		"content_length", len(req.Contents))

	// Create parent directories
	//nolint:gosec // G301: Workspace directories need 0755 for proper file operations
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Write file
	//nolint:gosec // G306: Workspace files use 0644 for user read/write and group/other read
	if err := os.WriteFile(fullPath, []byte(req.Contents), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Create typed response
	response := &protov1.FsWriteResponse{
		Path:         req.Path,
		BytesWritten: int64(len(req.Contents)),
		Metadata:     req.Metadata,
	}

	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_FsWrite{
			FsWrite: response,
		},
	}

	return &protov1.TaskResult{
		Status:        protov1.TaskResult_STATUS_SUCCESS,
		TypedResponse: toolResponse,
	}, nil
}

// handleFsListTyped handles the fs.list tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleFsListTyped(ctx context.Context, session *WorkerSession, req *protov1.FsListRequest) (*protov1.TaskResult, error) {
	path := req.Path
	if path == "" {
		path = "."
	}

	// Validate path is within workspace
	if err := validateWorkspacePath(path); err != nil {
		return nil, fmt.Errorf(errInvalidPath, err)
	}

	// Create full path within session workspace
	fullPath := filepath.Join(session.WorkspacePath, path)

	// Debug logging
	slog.Debug("fs.list operation",
		"session_id", session.SessionID,
		"workspace_path", session.WorkspacePath,
		"relative_path", path,
		"full_path", fullPath)

	// List directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	// Build file list with FileInfo structs
	var files []*protov1.FileInfo
	for _, entry := range entries {
		info, _ := entry.Info()
		size := int64(0)
		modTime := int64(0)
		if info != nil {
			size = info.Size()
			modTime = info.ModTime().UnixMilli()
		}

		files = append(files, &protov1.FileInfo{
			Path:           filepath.Join(path, entry.Name()),
			Name:           entry.Name(),
			SizeBytes:      size,
			IsDirectory:    entry.IsDir(),
			ModifiedTimeMs: modTime,
		})
	}

	response := &protov1.FsListResponse{
		Files:      files,
		TotalCount: int32(len(files)),
		Metadata:   req.Metadata,
	}

	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_FsList{
			FsList: response,
		},
	}

	return &protov1.TaskResult{
		Status:        protov1.TaskResult_STATUS_SUCCESS,
		TypedResponse: toolResponse,
	}, nil
}

// handleRunPythonTyped handles the run.python tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handleRunPythonTyped(ctx context.Context, session *WorkerSession, req *protov1.RunPythonRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	var code string

	// Extract code based on source type
	switch src := req.Source.(type) {
	case *protov1.RunPythonRequest_Code:
		code = src.Code
	case *protov1.RunPythonRequest_File:
		// Read file from workspace
		filePath := filepath.Join(session.WorkspacePath, src.File)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return &protov1.TaskResult{
				Status: protov1.TaskResult_STATUS_FAILURE,
			}, fmt.Errorf("failed to read file %s: %w", src.File, err)
		}
		code = string(content)
	default:
		return nil, fmt.Errorf("missing required source: code or file")
	}

	// Check if provider supports Python execution
	if session.Provider == nil || !session.Provider.Supports("run.python") {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
		}, fmt.Errorf("python execution not supported by session provider")
	}

	slog.Debug("run.python operation",
		"session_id", session.SessionID,
		"workspace_path", session.WorkspacePath,
		"code_length", len(code))

	// Execute via provider
	result, err := session.Provider.Execute(ctx, &language.ExecuteRequest{
		Code:       code,
		WorkingDir: session.WorkspacePath,
		Env: map[string]string{
			"PYTHONPATH": session.WorkspacePath,
		},
	})

	if err != nil {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
		}, err
	}

	// Determine status based on exit code
	status := protov1.TaskResult_STATUS_SUCCESS
	if result.ExitCode != 0 {
		status = protov1.TaskResult_STATUS_FAILURE
	}

	// Create typed response
	response := &protov1.RunPythonResponse{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: int32(result.ExitCode),
		Metadata: req.Metadata,
	}

	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_RunPython{
			RunPython: response,
		},
	}

	return &protov1.TaskResult{
		Status:        status,
		TypedResponse: toolResponse,
	}, nil
}

// handlePkgInstallTyped handles the pkg.install tool with typed request/response
//
//nolint:lll,unparam // Protobuf types create long signatures; ctx required for interface consistency
func (te *TaskExecutor) handlePkgInstallTyped(ctx context.Context, session *WorkerSession, req *protov1.PkgInstallRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	if len(req.Packages) == 0 {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
		}, fmt.Errorf("no packages specified")
	}

	// Check if provider supports package installation
	if session.Provider == nil || !session.Provider.Supports("pip.install") {
		return &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
		}, fmt.Errorf("package installation not supported by session provider")
	}

	slog.Debug("pkg.install operation",
		"session_id", session.SessionID,
		"workspace_path", session.WorkspacePath,
		"packages", strings.Join(req.Packages, ", "))

	// Install via provider
	err := session.Provider.InstallPackages(ctx, req.Packages)

	status := protov1.TaskResult_STATUS_SUCCESS
	installed := make([]*protov1.PackageInfo, 0, len(req.Packages))
	failed := []string{}

	if err != nil {
		status = protov1.TaskResult_STATUS_FAILURE
		// All packages failed in this simple implementation
		failed = req.Packages
	} else {
		// All packages succeeded
		for _, pkg := range req.Packages {
			installed = append(installed, &protov1.PackageInfo{
				Name:    pkg,
				Version: "", // Provider doesn't return version info
			})
		}

		// Update session's installed packages list
		session.mu.Lock()
		session.InstalledPkgs = append(session.InstalledPkgs, req.Packages...)
		session.mu.Unlock()
	}

	// Create typed response
	response := &protov1.PkgInstallResponse{
		Installed: installed,
		Failed:    failed,
		ExitCode:  0,
		Metadata:  req.Metadata,
	}

	if err != nil {
		response.ExitCode = 1
	}

	toolResponse := &protov1.ToolResponse{
		Response: &protov1.ToolResponse_PkgInstall{
			PkgInstall: response,
		},
	}

	return &protov1.TaskResult{
		Status:        status,
		TypedResponse: toolResponse,
	}, nil
}
