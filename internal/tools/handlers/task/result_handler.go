package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ResultHandler handles get.task.result requests
type ResultHandler struct {
	resultCache ResultCache
	taskQueue   TaskQueue
}

// NewResultHandler creates a new result handler
func NewResultHandler(resultCache ResultCache, taskQueue TaskQueue) *ResultHandler {
	return &ResultHandler{
		resultCache: resultCache,
		taskQueue:   taskQueue,
	}
}

// Handle processes get.task.result requests
func (h *ResultHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Try to get result from cache
	cachedResult, err := h.resultCache.Get(ctx, taskID)
	if err != nil {
		// Result not ready or not found
		// Check task status in storage
		task, taskErr := h.taskQueue.GetTask(ctx, taskID)
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
