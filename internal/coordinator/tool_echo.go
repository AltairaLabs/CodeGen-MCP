package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

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

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{
				Message: message,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolEcho,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Subscribe to result notifications
	ms.resultStreamer.Subscribe(taskID, session.ID)

	// Get task for sequence number
	task, err := ms.taskQueue.GetTask(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Return immediately with task ID and status
	response := TaskResponse{
		TaskID:    taskID,
		Status:    "queued",
		SessionID: session.ID,
		Sequence:  task.Sequence,
		Message:   fmt.Sprintf(msgTaskQueued, taskID),
		CreatedAt: time.Now(),
	}

	responseJSON, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(responseJSON)), nil
}

// EchoParser handles echo tool response parsing
type EchoParser struct{}

func (p *EchoParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("echo response missing typed response")
	}

	echoResp := result.Response.GetEcho()
	if echoResp == nil {
		return "", fmt.Errorf("echo response has invalid type")
	}

	return echoResp.Message, nil
}
