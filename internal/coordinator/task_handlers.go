package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetTaskResult retrieves a completed task result from cache
func (ms *MCPServer) handleGetTaskResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Try to get result from cache
	cachedResult, err := ms.resultCache.Get(ctx, taskID)
	if err != nil {
		// Result not ready or not found
		// Check task status in storage
		task, taskErr := ms.taskQueue.GetTask(ctx, taskID)
		if taskErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Task not found: %s", taskID)), nil
		}

		// Return current task status
		status := TaskResponse{
			TaskID:    task.ID,
			Status:    string(task.State),
			SessionID: task.SessionID,
			Message:   fmt.Sprintf("Task is %s", task.State),
			CreatedAt: task.CreatedAt,
		}

		statusJSON, _ := json.Marshal(status)
		return mcp.NewToolResultText(string(statusJSON)), nil
	}

	// Result is ready, return it
	return mcp.NewToolResultText(cachedResult.GetOutput()), nil
}

// handleGetTaskStatus checks the current status of a task
func (ms *MCPServer) handleGetTaskStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get task from storage
	task, err := ms.taskQueue.GetTask(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Task not found: %s", taskID)), nil
	}

	// Build status response
	status := TaskResponse{
		TaskID:    task.ID,
		Status:    string(task.State),
		SessionID: task.SessionID,
		Sequence:  task.Sequence,
		CreatedAt: task.CreatedAt,
	}

	// Add completion time if available
	if task.CompletedAt != nil {
		status.Message = fmt.Sprintf("Task %s at %s", task.State, task.CompletedAt.Format(time.RFC3339))
	} else if task.DispatchedAt != nil {
		status.Message = fmt.Sprintf("Task %s, dispatched at %s", task.State, task.DispatchedAt.Format(time.RFC3339))
	} else {
		status.Message = fmt.Sprintf("Task %s, waiting in queue", task.State)
	}

	statusJSON, _ := json.Marshal(status)
	return mcp.NewToolResultText(string(statusJSON)), nil
}
