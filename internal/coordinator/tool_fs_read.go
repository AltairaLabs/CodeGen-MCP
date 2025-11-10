package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

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

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_FsRead{
			FsRead: &protov1.FsReadRequest{
				Path: path,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsRead,
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

	// Return task information immediately
	// Client will receive result via SSE notification when ready
	return mcp.NewToolResultText(string(responseJSON)), nil
}

// FsReadParser handles fs.read tool response parsing
type FsReadParser struct{}

func (p *FsReadParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.read response missing typed response")
	}

	fsReadResp := result.Response.GetFsRead()
	if fsReadResp == nil {
		return "", fmt.Errorf("fs.read response has invalid type")
	}

	return fsReadResp.Contents, nil
}
