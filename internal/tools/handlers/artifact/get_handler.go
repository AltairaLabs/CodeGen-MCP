// Package artifact provides artifact management tool handlers
package artifact

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

// GetHandler handles artifact.get tool
type GetHandler struct {
	sessionManager types.SessionManagerWithWorkers
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewGetHandler creates a new artifact.get handler
func NewGetHandler(
	sessionMgr types.SessionManagerWithWorkers,
	taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger,
	resultStreamer types.ResultStreamer,
) *GetHandler {
	return &GetHandler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the artifact.get tool
func (h *GetHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available
	if !h.sessionManager.HasWorkersAvailable() {
		return mcp.NewToolResultError(config.ErrNoWorkersAvail), nil
	}

	artifactID, err := request.RequireString("artifact_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get or create session
	session, err := h.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolArtifactGet,
		Arguments:   map[string]interface{}{"artifact_id": artifactID},
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_ArtifactGet{
			ArtifactGet: &protov1.ArtifactGetRequest{
				ArtifactId: artifactID,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolArtifactGet,
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

func (h *GetHandler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
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

func getSessionID(ctx context.Context) string {
	if sessionID := ctx.Value("session_id"); sessionID != nil {
		if sid, ok := sessionID.(string); ok {
			return sid
		}
	}
	return "default-session"
}
