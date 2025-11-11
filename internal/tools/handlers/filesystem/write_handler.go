// Package filesystem provides filesystem tool handlers
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// WriteHandler handles fs.write tool
type WriteHandler struct {
	sessionManager types.SessionManagerWithWorkers
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewWriteHandler creates a new fs.write handler
func NewWriteHandler(sessionMgr types.SessionManagerWithWorkers, taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger, resultStreamer types.ResultStreamer) *WriteHandler {
	return &WriteHandler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the fs.write tool
func (h *WriteHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !h.sessionManager.HasWorkersAvailable() {
		return mcp.NewToolResultError(config.ErrNoWorkersAvail), nil
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
	if vErr := validateWorkspacePath(path); vErr != nil {
		return mcp.NewToolResultError(vErr.Error()), nil
	}

	// Get or create session
	session, err := h.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolFsWrite,
		Arguments:   map[string]interface{}{"path": path, "contents": contents},
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_FsWrite{
			FsWrite: &protov1.FsWriteRequest{
				Path:     path,
				Contents: contents,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolFsWrite,
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

func (h *WriteHandler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
	sessionID := getSessionID(ctx)

	// Check if session exists
	session, ok := h.sessionManager.GetSession(sessionID)
	if ok {
		return session, nil
	}

	// Create new session with worker assignment
	session = h.sessionManager.CreateSession(ctx, sessionID, "default-user", "default-workspace")
	if session == nil {
		return nil, fmt.Errorf("failed to create session")
	}

	// Verify worker was assigned
	if session.WorkerID == "" {
		return nil, fmt.Errorf("no workers available")
	}

	return session, nil
}
