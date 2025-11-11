package python

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// InstallHandler handles pkg.install tool
type InstallHandler struct {
	sessionManager types.SessionManagerWithWorkers
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewInstallHandler creates a new pkg.install handler
func NewInstallHandler(sessionMgr types.SessionManagerWithWorkers, taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger, resultStreamer types.ResultStreamer) *InstallHandler {
	return &InstallHandler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the pkg.install tool
func (h *InstallHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !h.sessionManager.HasWorkersAvailable() {
		return mcp.NewToolResultError(config.ErrNoWorkersAvail), nil
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
	session, err := h.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	args := map[string]interface{}{"packages": packages}

	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolPkgInstall,
		Arguments:   args,
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	// Split packages string into array
	pkgList := strings.Split(packages, " ")
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_PkgInstall{
			PkgInstall: &protov1.PkgInstallRequest{
				Packages: pkgList,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolPkgInstall,
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

func (h *InstallHandler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
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
