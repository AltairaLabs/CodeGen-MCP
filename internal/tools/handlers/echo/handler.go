// Package echo provides the echo tool handler implementation
package echo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Handler implements the echo tool handler
type Handler struct {
	sessionManager types.SessionManager
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewHandler creates a new echo handler
func NewHandler(
	sessionMgr types.SessionManager,
	taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger,
	resultStreamer types.ResultStreamer,
) *Handler {
	return &Handler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the echo tool
func (h *Handler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("PANIC in echo handler",
				"panic", r,
				"session_manager_nil", h.sessionManager == nil,
				"audit_logger_nil", h.auditLogger == nil)
			panic(r) // re-panic so mcp-go can handle it
		}
	}()

	message, err := request.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	slog.Info("echo handler: before getOrCreateSession")
	// Get or create session
	session, err := h.getOrCreateSession(ctx)
	slog.Info("echo handler: after getOrCreateSession", "error", err, "session_nil", session == nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	// Audit log
	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolEcho,
		Arguments:   map[string]interface{}{"message": message},
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{
				Message: message,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolEcho,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Subscribe to result notifications
	h.resultStreamer.Subscribe(taskID, session.ID)

	// Get task for sequence number
	task, err := h.taskQueue.GetTask(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Return immediately with task ID and status
	response := types.TaskResponse{
		TaskID:    taskID,
		Status:    "queued",
		SessionID: session.ID,
		Sequence:  task.Sequence,
		Message:   fmt.Sprintf(config.MsgTaskQueued, taskID),
		CreatedAt: time.Now(),
	}

	responseJSON, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(responseJSON)), nil
}

// getOrCreateSession retrieves an existing session or creates a new one
func (h *Handler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
	sessionID := h.getSessionID(ctx)

	// Check if session exists
	session, ok := h.sessionManager.GetSession(sessionID)
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

	session = h.sessionManager.CreateSession(ctx, sessionID, "default-user", "default-workspace")
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

// getSessionID extracts session ID from context
func (h *Handler) getSessionID(ctx context.Context) string {
	// Extract session ID from mcp-go's ClientSession in context
	// The SSE/HTTP transport automatically injects this
	clientSession := server.ClientSessionFromContext(ctx)
	if clientSession != nil {
		return clientSession.SessionID()
	}
	// Fallback for stdio transport or testing
	if sessionID, ok := ctx.Value("session_id").(string); ok {
		return sessionID
	}
	return "default-session"
}
