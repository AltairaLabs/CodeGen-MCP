package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// StatusHandler handles get.task.status requests
type StatusHandler struct {
	taskQueue TaskQueue
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(taskQueue TaskQueue) *StatusHandler {
	return &StatusHandler{
		taskQueue: taskQueue,
	}
}

// Handle processes get.task.status requests
func (h *StatusHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get task from storage
	task, err := h.taskQueue.GetTask(ctx, taskID)
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
