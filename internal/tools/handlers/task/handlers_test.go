package task

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// Mock implementations for testing
type mockResultCache struct {
	results map[string]*mockResult
}

type mockResult struct {
	output string
}

func (r *mockResult) GetOutput() string {
	return r.output
}

func (c *mockResultCache) Get(ctx context.Context, taskID string) (Result, error) {
	if result, exists := c.results[taskID]; exists {
		return result, nil
	}
	return nil, fmt.Errorf("result not found")
}

type mockTaskQueue struct {
	tasks map[string]*storage.QueuedTask
}

func (q *mockTaskQueue) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	if task, exists := q.tasks[taskID]; exists {
		return task, nil
	}
	return nil, fmt.Errorf("task not found")
}

func TestResultHandlerWithCachedResult(t *testing.T) {
	cache := &mockResultCache{
		results: map[string]*mockResult{
			"task-123": {output: "cached output"},
		},
	}
	queue := &mockTaskQueue{tasks: map[string]*storage.QueuedTask{}}

	handler := NewResultHandler(cache, queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get.task.result",
			Arguments: map[string]interface{}{
				"task_id": "task-123",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if text.Text != "cached output" {
		t.Errorf("Expected 'cached output', got %s", text.Text)
	}
}

func TestResultHandlerWithoutCachedResult(t *testing.T) {
	cache := &mockResultCache{results: map[string]*mockResult{}}

	createdAt := time.Now()
	queue := &mockTaskQueue{
		tasks: map[string]*storage.QueuedTask{
			"task-456": {
				ID:        "task-456",
				SessionID: "session-1",
				State:     storage.TaskStateQueued,
				CreatedAt: createdAt,
			},
		},
	}

	handler := NewResultHandler(cache, queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get.task.result",
			Arguments: map[string]interface{}{
				"task_id": "task-456",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	// Should return task status as JSON
	var status TaskResponse
	err = json.Unmarshal([]byte(text.Text), &status)
	if err != nil {
		t.Fatalf("Failed to unmarshal status: %v", err)
	}

	if status.TaskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got %s", status.TaskID)
	}
	if status.Status != string(storage.TaskStateQueued) {
		t.Errorf("Expected status '%s', got %s", storage.TaskStateQueued, status.Status)
	}
	if status.SessionID != "session-1" {
		t.Errorf("Expected session ID 'session-1', got %s", status.SessionID)
	}
}

func TestResultHandlerTaskNotFound(t *testing.T) {
	cache := &mockResultCache{results: map[string]*mockResult{}}
	queue := &mockTaskQueue{tasks: map[string]*storage.QueuedTask{}}

	handler := NewResultHandler(cache, queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get.task.result",
			Arguments: map[string]interface{}{
				"task_id": "nonexistent",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result for nonexistent task")
	}
}

func TestStatusHandler(t *testing.T) {
	createdAt := time.Now()
	queue := &mockTaskQueue{
		tasks: map[string]*storage.QueuedTask{
			"task-789": {
				ID:        "task-789",
				SessionID: "session-2",
				State:     storage.TaskStateCompleted,
				Sequence:  42,
				CreatedAt: createdAt,
			},
		},
	}

	handler := NewStatusHandler(queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get.task.status",
			Arguments: map[string]interface{}{
				"task_id": "task-789",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	// Should return task status as JSON
	var status TaskResponse
	err = json.Unmarshal([]byte(text.Text), &status)
	if err != nil {
		t.Fatalf("Failed to unmarshal status: %v", err)
	}

	if status.TaskID != "task-789" {
		t.Errorf("Expected task ID 'task-789', got %s", status.TaskID)
	}
	if status.Status != string(storage.TaskStateCompleted) {
		t.Errorf("Expected status '%s', got %s", storage.TaskStateCompleted, status.Status)
	}
	if status.SessionID != "session-2" {
		t.Errorf("Expected session ID 'session-2', got %s", status.SessionID)
	}
	if status.Sequence != 42 {
		t.Errorf("Expected sequence 42, got %d", status.Sequence)
	}
}

func TestStatusHandlerTaskNotFound(t *testing.T) {
	queue := &mockTaskQueue{tasks: map[string]*storage.QueuedTask{}}

	handler := NewStatusHandler(queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get.task.status",
			Arguments: map[string]interface{}{
				"task_id": "nonexistent",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result for nonexistent task")
	}
}

func TestStatusHandlerMissingTaskID(t *testing.T) {
	queue := &mockTaskQueue{tasks: map[string]*storage.QueuedTask{}}
	handler := NewStatusHandler(queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get.task.status",
			Arguments: map[string]interface{}{
				// No task_id provided
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result for missing task_id")
	}
}

func TestResultHandlerMissingTaskID(t *testing.T) {
	cache := &mockResultCache{results: map[string]*mockResult{}}
	queue := &mockTaskQueue{tasks: map[string]*storage.QueuedTask{}}
	handler := NewResultHandler(cache, queue)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get.task.result",
			Arguments: map[string]interface{}{
				// No task_id provided
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result for missing task_id")
	}
}
