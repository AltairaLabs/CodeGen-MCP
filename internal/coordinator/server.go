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
	toolEcho       = "echo"
	toolFsRead     = "fs.read"
	toolFsWrite    = "fs.write"
	toolFsList     = "fs.list"
	toolRunPython  = "run.python"
	toolPkgInstall = "pkg.install"

	// Error messages
	errSessionError   = "session error: %v"
	errNoWorkersAvail = "no workers available to handle request"
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
}

// Config holds configuration for the MCP server
type Config struct {
	Name    string
	Version string
}

// NewMCPServer creates and configures a new MCP server
func NewMCPServer(cfg Config, sessionMgr *SessionManager, worker WorkerClient, audit *AuditLogger) *MCPServer {
	// Create the mcp-go server
	mcpServer := server.NewMCPServer(
		cfg.Name,
		cfg.Version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	ms := &MCPServer{
		server:         mcpServer,
		sessionManager: sessionMgr,
		workerClient:   worker,
		auditLogger:    audit,
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
}

// handleEcho implements the echo tool
func (ms *MCPServer) handleEcho(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("PANIC in handleEcho",
				"panic", r,
				"session_manager_nil", ms.sessionManager == nil,
				"worker_client_nil", ms.workerClient == nil,
				"audit_logger_nil", ms.auditLogger == nil,
				"session_manager_registry_nil", ms.sessionManager != nil && ms.sessionManager.workerRegistry == nil,
			)
			panic(r) // re-panic so mcp-go can handle it
		}
	}()

	message, err := request.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	slog.Info("handleEcho: before getOrCreateSession")
	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	slog.Info("handleEcho: after getOrCreateSession", "error", err, "session_nil", session == nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	// Audit log
	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolEcho,
		Arguments:   TaskArgs{"message": message},
		WorkspaceID: session.WorkspaceID,
	})

	// Execute via worker (add session ID to context)
	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolEcho, TaskArgs{
		"message": message,
	})

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolEcho,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolEcho,
		Result:    result,
	})

	return mcp.NewToolResultText(result.Output), nil
}

// handleFsRead implements the fs.read tool
func (ms *MCPServer) handleFsRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate path is safe
	if vErr := ms.validateWorkspacePath(path); vErr != nil {
		return mcp.NewToolResultError(vErr.Error()), nil
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolFsRead,
		Arguments:   TaskArgs{"path": path},
		WorkspaceID: session.WorkspaceID,
	})

	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolFsRead, TaskArgs{"path": path})

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsRead,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolFsRead,
		Result:    result,
	})

	return mcp.NewToolResultText(result.Output), nil
}

// handleFsWrite implements the fs.write tool
func (ms *MCPServer) handleFsWrite(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	contents, err := request.RequireString("contents")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate path is safe
	if vErr := ms.validateWorkspacePath(path); vErr != nil {
		return mcp.NewToolResultError(vErr.Error()), nil
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolFsWrite,
		Arguments:   TaskArgs{"path": path, "contents": contents},
		WorkspaceID: session.WorkspaceID,
	})

	// Add session ID to context for worker execution
	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolFsWrite, TaskArgs{
		"path":     path,
		"contents": contents,
	})

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsWrite,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolFsWrite,
		Result:    result,
	})

	return mcp.NewToolResultText(result.Output), nil
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

// Serve starts the MCP server with stdio transport
func (ms *MCPServer) Serve() error {
	return server.ServeStdio(ms.server)
}

// ServeWithLogger starts the MCP server with stdio transport and custom logger
func (ms *MCPServer) ServeWithLogger(logger *slog.Logger) error {
	logger.Info("Starting MCP server with stdio transport")
	return ms.Serve()
}

// ServeHTTP starts the MCP server with HTTP/SSE transport on the specified address
func (ms *MCPServer) ServeHTTP(addr string) error {
	sseServer := server.NewSSEServer(ms.server,
		server.WithBaseURL("http://"+addr),
		server.WithStaticBasePath("/mcp"),
	)
	return sseServer.Start(addr)
}

// ServeHTTPWithLogger starts the MCP server with HTTP/SSE transport and custom logger
func (ms *MCPServer) ServeHTTPWithLogger(addr string, logger *slog.Logger) error {
	logger.Info("Starting MCP server with HTTP/SSE transport", "address", addr, "base_path", "/mcp")
	return ms.ServeHTTP(addr)
}

// handleFsList implements the fs.list tool
func (ms *MCPServer) handleFsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	// Path is optional, defaults to root
	path := request.GetString("path", "")

	// Validate path if provided
	if path != "" {
		if vErr := ms.validateWorkspacePath(path); vErr != nil {
			return mcp.NewToolResultError(vErr.Error()), nil
		}
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolFsList,
		Arguments:   TaskArgs{"path": path},
		WorkspaceID: session.WorkspaceID,
	})

	args := TaskArgs{"path": path}
	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolFsList, args)

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsList,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolFsList,
		Result:    result,
	})

	return mcp.NewToolResultText(result.Output), nil
}

// handleRunPython implements the run.python tool
func (ms *MCPServer) handleRunPython(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	// Get optional code and file parameters
	code := request.GetString("code", "")
	file := request.GetString("file", "")

	// Must have either code or file parameter
	if code == "" && file == "" {
		return mcp.NewToolResultError("must provide either 'code' or 'file' parameter"), nil
	}

	// Validate file path if provided
	if file != "" {
		if vErr := ms.validateWorkspacePath(file); vErr != nil {
			return mcp.NewToolResultError(vErr.Error()), nil
		}
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	args := TaskArgs{}
	if code != "" {
		args["code"] = code
	}
	if file != "" {
		args["file"] = file
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolRunPython,
		Arguments:   args,
		WorkspaceID: session.WorkspaceID,
	})

	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolRunPython, args)

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolRunPython,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolRunPython,
		Result:    result,
	})

	// Include both stdout and stderr in result
	output := result.Output
	if result.Error != "" {
		output = fmt.Sprintf("stdout:\n%s\n\nstderr:\n%s", result.Output, result.Error)
	}

	return mcp.NewToolResultText(output), nil
}

// handlePkgInstall implements the pkg.install tool
func (ms *MCPServer) handlePkgInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	packages, err := request.RequireString("packages")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate packages string is not empty
	packages = strings.TrimSpace(packages)
	if packages == "" {
		return mcp.NewToolResultError("packages parameter cannot be empty"), nil
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolPkgInstall,
		Arguments:   TaskArgs{"packages": packages},
		WorkspaceID: session.WorkspaceID,
	})

	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := ms.workerClient.ExecuteTask(ctxWithSession, session.WorkspaceID, toolPkgInstall, TaskArgs{
		"packages": packages,
	})

	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolPkgInstall,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	ms.auditLogger.LogToolResult(ctx, &AuditEntry{
		SessionID: session.ID,
		ToolName:  toolPkgInstall,
		Result:    result,
	})

	return mcp.NewToolResultText(result.Output), nil
}
