package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// handlePkgInstall implements the pkg.install tool
func (ms *MCPServer) handlePkgInstall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !ms.hasWorkersAvailable() {
		return mcp.NewToolResultError(errNoWorkersAvail), nil
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
	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(errSessionError, err)), nil
	}

	ms.auditLogger.LogToolCall(ctx, &AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    toolPkgInstall,
		Arguments:   TaskArgs{"packages": packages},
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
	taskID, err := ms.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		ms.auditLogger.LogToolResult(ctx, &AuditEntry{
			SessionID: session.ID,
			ToolName:  toolPkgInstall,
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

// PkgInstallParser handles pkg.install tool response parsing
type PkgInstallParser struct{}

func (p *PkgInstallParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("pkg.install response missing typed response")
	}

	pkgInstallResp := result.Response.GetPkgInstall()
	if pkgInstallResp == nil {
		return "", fmt.Errorf("pkg.install response has invalid type")
	}

	// Format the installed packages
	var output string
	for _, pkg := range pkgInstallResp.Installed {
		output += fmt.Sprintf("Installed: %s (version: %s)\n", pkg.Name, pkg.Version)
	}
	if len(pkgInstallResp.Failed) > 0 {
		output += fmt.Sprintf("Failed: %s\n", strings.Join(pkgInstallResp.Failed, ", "))
	}
	return output, nil
}
