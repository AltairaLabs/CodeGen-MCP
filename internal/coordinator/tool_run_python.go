package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleRunPython implements the run.python tool
func (ms *MCPServer) handleRunPython(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
	}

	// Get optional code and file parameters
	code := request.GetString("code", "")
	file := request.GetString("file", "")

	// Must have either code or file parameter
	if code == "" && file == "" {
		return mcp.NewToolResultError("must provide either 'code' or 'file' parameter"), nil
	}

	// Validate file path if provided
	if file != "" {
		if vErr := ms.validateWorkspacePath(file); vErr != nil {
			return mcp.NewToolResultError(vErr.Error()), nil
		}
	}

	// Get or create session
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	args := TaskArgs{}
	if code != "" {
		args["code"] = code
	}
	if file != "" {
		args["file"] = file
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolRunPython,
		Arguments:   args,
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	runPythonReq := &protov1.RunPythonRequest{}
	if code != "" {
		runPythonReq.Source = &protov1.RunPythonRequest_Code{Code: code}
	} else if file != "" {
		runPythonReq.Source = &protov1.RunPythonRequest_File{File: file}
	}

	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_RunPython{
			RunPython: runPythonReq,
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolRunPython,
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

// RunPythonParser handles run.python tool response parsing
type RunPythonParser struct{}

func (p *RunPythonParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("run.python response missing typed response")
	}

	runPythonResp := result.Response.GetRunPython()
	if runPythonResp == nil {
		return "", fmt.Errorf("run.python response has invalid type")
	}

	// Combine stdout and stderr
	output := runPythonResp.Stdout
	if runPythonResp.Stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += runPythonResp.Stderr
	}
	// Include exit code if non-zero
	if runPythonResp.ExitCode != 0 {
		output += fmt.Sprintf("\nExit code: %d", runPythonResp.ExitCode)
	}
	return output, nil
}
