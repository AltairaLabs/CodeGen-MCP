package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

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

	// Create strongly-typed request
	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_FsList{
			FsList: &protov1.FsListRequest{
				Path: path,
			},
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolFsList,
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

// FsListParser handles fs.list tool response parsing
type FsListParser struct{}

func (p *FsListParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.list response missing typed response")
	}

	fsListResp := result.Response.GetFsList()
	if fsListResp == nil {
		return "", fmt.Errorf("fs.list response has invalid type")
	}

	// Format the file info array into a readable string
	var output string
	for _, fileInfo := range fsListResp.Files {
		fileType := "file"
		if fileInfo.IsDirectory {
			fileType = "dir"
		}
		output += fmt.Sprintf("%s (%s, %d bytes)\n", fileInfo.Name, fileType, fileInfo.SizeBytes)
	}
	return output, nil
}
