package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// Tool names
	toolEcho          = "echo"
	toolFsRead        = "fs.read"
	toolFsWrite       = "fs.write"
	toolFsList        = "fs.list"
	toolRunPython     = "run.python"
	toolPkgInstall    = "pkg.install"
	toolGetTaskResult = "task.get_result"
	toolGetTaskStatus = "task.get_status"

	// Error messages
	errSessionError   = "session error: %v"
	errNoWorkersAvail = "no workers available to handle request"

	// Task status messages
	msgTaskQueued = "Task %s queued for execution"
)

// sessionIDKey is the context key for session ID
type sessionIDKey struct{}

// contextWithSessionID adds the session ID to the context
func contextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

// MCPServer wraps the mcp-go server with our business logic
type MCPServer struct {
	server         *server.MCPServer
	sessionManager *SessionManager
	workerClient   WorkerClient
	auditLogger    *AuditLogger
	taskQueue      TaskQueueInterface
	resultStreamer *ResultStreamer
	resultCache    *ResultCache
	sseManager     *SSESessionManager
}

// Config holds configuration for the MCP server
type Config struct {
	Name    string
	Version string
}

// NewMCPServer creates and configures a new MCP server
func NewMCPServer(cfg Config, sessionMgr *SessionManager, worker WorkerClient, audit *AuditLogger, taskQueue TaskQueueInterface) *MCPServer {
	// Create the mcp-go server
	mcpServer := server.NewMCPServer(
		cfg.Name,
		cfg.Version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Initialize async result streaming components
	resultCache := NewResultCache(5 * 60 * 1000000000) // 5 minutes TTL
	sseManager := NewSSESessionManager()
	logger := slog.Default()
	resultStreamer := NewResultStreamer(sseManager, resultCache, logger)

	// Wire up result streamer to task queue
	taskQueue.SetResultStreamer(resultStreamer)

	ms := &MCPServer{
		server:         mcpServer,
		sessionManager: sessionMgr,
		workerClient:   worker,
		auditLogger:    audit,
		taskQueue:      taskQueue,
		resultStreamer: resultStreamer,
		resultCache:    resultCache,
		sseManager:     sseManager,
	}

	// Register tools
	ms.registerTools()

	return ms
}

// registerTools registers all MCP tools with handlers
func (ms *MCPServer) registerTools() {
	// Echo tool - simple test tool
	echoTool := mcp.NewTool(toolEcho,
		mcp.WithDescription("Echo a message back (test tool)"),
		mcp.WithString("message",
			mcp.Required(),
			mcp.Description("Message to echo"),
		),
	)
	ms.server.AddTool(echoTool, ms.handleEcho)

	// fs.read tool - read file from workspace
	fsReadTool := mcp.NewTool(toolFsRead,
		mcp.WithDescription("Read a file from the workspace"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File path relative to workspace root"),
		),
	)
	ms.server.AddTool(fsReadTool, ms.handleFsRead)

	// fs.write tool - write file to workspace
	fsWriteTool := mcp.NewTool(toolFsWrite,
		mcp.WithDescription("Write a file to the workspace"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File path relative to workspace root"),
		),
		mcp.WithString("contents",
			mcp.Required(),
			mcp.Description("File contents to write"),
		),
	)
	ms.server.AddTool(fsWriteTool, ms.handleFsWrite)

	// fs.list tool - list directory contents
	fsListTool := mcp.NewTool(toolFsList,
		mcp.WithDescription("List directory contents in the workspace"),
		mcp.WithString("path",
			mcp.Description("Directory path relative to workspace root (default: root)"),
		),
	)
	ms.server.AddTool(fsListTool, ms.handleFsList)

	// run.python tool - execute Python code
	runPythonTool := mcp.NewTool(toolRunPython,
		mcp.WithDescription("Execute Python code in the session's isolated virtual environment"),
		mcp.WithString("code",
			mcp.Description("Python code to execute"),
		),
		mcp.WithString("file",
			mcp.Description("Python file path to execute (alternative to code)"),
		),
	)
	ms.server.AddTool(runPythonTool, ms.handleRunPython)

	// pkg.install tool - install Python packages
	pkgInstallTool := mcp.NewTool(toolPkgInstall,
		mcp.WithDescription("Install Python packages in the session's virtual environment"),
		mcp.WithString("packages",
			mcp.Required(),
			mcp.Description("Space-separated package names (e.g., 'requests flask numpy')"),
		),
	)
	ms.server.AddTool(pkgInstallTool, ms.handlePkgInstall)

	// task.get_result tool - retrieve completed task result from cache
	getTaskResultTool := mcp.NewTool(toolGetTaskResult,
		mcp.WithDescription("Retrieve the result of a completed task from cache"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("The task ID returned when the task was queued"),
		),
	)
	ms.server.AddTool(getTaskResultTool, ms.handleGetTaskResult)

	// task.get_status tool - check task status
	getTaskStatusTool := mcp.NewTool(toolGetTaskStatus,
		mcp.WithDescription("Check the status of a queued or running task"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("The task ID to check status for"),
		),
	)
	ms.server.AddTool(getTaskStatusTool, ms.handleGetTaskStatus)
}

// validateWorkspacePath ensures the path is safe and relative to workspace
func (ms *MCPServer) validateWorkspacePath(path string) error {
	// Prevent absolute paths
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative to workspace root, got absolute path: %s", path)
	}

	// Prevent path traversal
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "/../") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	return nil
}

// getSessionID extracts session ID from context
func (ms *MCPServer) getSessionID(ctx context.Context) string {
	// Extract session ID from mcp-go's ClientSession in context
	// The SSE/HTTP transport automatically injects this
	clientSession := server.ClientSessionFromContext(ctx)
	if clientSession != nil {
		return clientSession.SessionID()
	}
	// Fallback for stdio transport or testing
	sessionID, ok := ctx.Value("session_id").(string)
	if !ok {
		return "default-session"
	}
	return sessionID
}

// hasWorkersAvailable checks if any workers are available to handle requests
func (ms *MCPServer) hasWorkersAvailable() bool {
	if ms.sessionManager == nil || ms.sessionManager.workerRegistry == nil {
		return false
	}
	_, _, available := ms.sessionManager.workerRegistry.GetTotalCapacity()
	return available > 0
}

// getOrCreateSession retrieves an existing session or creates a new one with worker assignment
func (ms *MCPServer) getOrCreateSession(ctx context.Context) (*Session, error) {
	sessionID := ms.getSessionID(ctx)

	// Check if session exists
	session, ok := ms.sessionManager.GetSession(sessionID)
	if ok {
		slog.Debug("Using existing session",
			"session_id", sessionID,
			"worker_id", session.WorkerID,
			"workspace_id", session.WorkspaceID)
		return session, nil
	}

	// Create new session with worker assignment
	slog.Info("Creating new session",
		"session_id", sessionID,
		"user_id", "default-user",
		"workspace_id", "default-workspace")

	session = ms.sessionManager.CreateSession(ctx, sessionID, "default-user", "default-workspace")
	if session == nil {
		slog.Error("Failed to create session", "session_id", sessionID)
		return nil, fmt.Errorf("failed to create session")
	}

	// Verify worker was assigned
	if session.WorkerID == "" {
		slog.Error("No workers available for session", "session_id", sessionID)
		return nil, fmt.Errorf("no workers available")
	}

	slog.Info("Session created successfully",
		"session_id", sessionID,
		"worker_id", session.WorkerID,
		"workspace_id", session.WorkspaceID)

	return session, nil
}

// Server returns the underlying mcp-go server for serving
func (ms *MCPServer) Server() *server.MCPServer {
	return ms.server
}

// Note: Serve methods have been moved to server_serve.go (untestable infrastructure code)
