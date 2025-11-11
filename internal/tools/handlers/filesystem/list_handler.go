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

// ListHandler handles fs.list tool
type ListHandler struct {
	sessionManager types.SessionManagerWithWorkers
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewListHandler creates a new fs.list handler
func NewListHandler(sessionMgr types.SessionManagerWithWorkers, taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger, resultStreamer types.ResultStreamer) *ListHandler {
	return &ListHandler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the fs.list tool
func (h *ListHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !h.sessionManager.HasWorkersAvailable() {
		return mcp.NewToolResultError(config.ErrNoWorkersAvail), nil
	}

	// Path is optional, defaults to root
	path := request.GetString("path", "")

	// Validate path if provided
	if path != "" {
		if vErr := validateWorkspacePath(path); vErr != nil {
			return mcp.NewToolResultError(vErr.Error()), nil
		}
	}

	// Get or create session
	session, err := h.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolFsList,
		Arguments:   map[string]interface{}{"path": path},
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_FsList{
			FsList: &protov1.FsListRequest{
				Path: path,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolFsList,
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

func (h *ListHandler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
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
