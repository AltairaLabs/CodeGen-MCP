package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

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
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsWrite,
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

// FsWriteParser handles fs.write tool response parsing
type FsWriteParser struct{}

func (p *FsWriteParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.write response missing typed response")
	}

	fsWriteResp := result.Response.GetFsWrite()
	if fsWriteResp == nil {
		return "", fmt.Errorf("fs.write response has invalid type")
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", fsWriteResp.BytesWritten, fsWriteResp.Path), nil
}
