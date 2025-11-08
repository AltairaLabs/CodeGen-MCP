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
	toolEcho    = "echo"
	toolFsRead  = "fs.read"
	toolFsWrite = "fs.write"
)

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
}

// handleEcho implements the echo tool
func (ms *MCPServer) handleEcho(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message, err := request.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get session from context
	sessionID := ms.getSessionID(ctx)
	session, ok := ms.sessionManager.GetSession(sessionID)
	if !ok {
		// Create default session if none exists
		session = ms.sessionManager.CreateSession(ctx, sessionID, "default", "default")
	}

	// Audit log
	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolEcho,
		Arguments:   TaskArgs{"message": message},
		WorkspaceID: session.WorkspaceID,
	})

	// Execute via worker
	result, err := ms.workerClient.ExecuteTask(ctx, session.WorkspaceID, toolEcho, TaskArgs{
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
	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate path is safe
	if vErr := ms.validateWorkspacePath(path); vErr != nil {
		return mcp.NewToolResultError(vErr.Error()), nil
	}

	sessionID := ms.getSessionID(ctx)
	session, ok := ms.sessionManager.GetSession(sessionID)
	if !ok {
		session = ms.sessionManager.CreateSession(ctx, sessionID, "default", "default")
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolFsRead,
		Arguments:   TaskArgs{"path": path},
		WorkspaceID: session.WorkspaceID,
	})

	result, err := ms.workerClient.ExecuteTask(ctx, session.WorkspaceID, toolFsRead, TaskArgs{"path": path})

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

	sessionID := ms.getSessionID(ctx)
	session, ok := ms.sessionManager.GetSession(sessionID)
	if !ok {
		session = ms.sessionManager.CreateSession(ctx, sessionID, "default", "default")
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolFsWrite,
		Arguments:   TaskArgs{"path": path, "contents": contents},
		WorkspaceID: session.WorkspaceID,
	})

	result, err := ms.workerClient.ExecuteTask(ctx, session.WorkspaceID, toolFsWrite, TaskArgs{
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

// getSessionID extracts session ID from context (placeholder implementation)
func (ms *MCPServer) getSessionID(ctx context.Context) string {
	// In a real implementation, this would extract from context
	// For now, return a default session ID
	sessionID, ok := ctx.Value("session_id").(string)
	if !ok {
		return "default-session"
	}
	return sessionID
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
