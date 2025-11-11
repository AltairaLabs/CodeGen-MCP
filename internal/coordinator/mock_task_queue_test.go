package coordinator

import (
	"context"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"github.com/AltairaLabs/codegen-mcp/internal/taskqueue"
)

// MockTaskQueue is a mock implementation of TaskQueueInterface for tests
type MockTaskQueue struct {
	tasks        map[string]*storage.QueuedTask
	workerClient WorkerClient // Expected by existing tests
}

func NewMockTaskQueue() *MockTaskQueue {
	return &MockTaskQueue{
		tasks: make(map[string]*storage.QueuedTask),
	}
}

func (m *MockTaskQueue) EnqueueTask(ctx context.Context, sessionID, toolName string, args taskqueue.TaskArgs) (string, error) {
	return "mock-task-id", nil
}

func (m *MockTaskQueue) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	return "mock-task-id", nil
}

func (m *MockTaskQueue) GetTaskResult(ctx context.Context, taskID string) (*taskqueue.TaskResult, error) {
	return &taskqueue.TaskResult{Output: "mock result"}, nil
}

func (m *MockTaskQueue) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	if task, exists := m.tasks[taskID]; exists {
		return task, nil
	}
	return &storage.QueuedTask{
		ID:        taskID,
		SessionID: "mock-session",
		ToolName:  "mock-tool",
		Sequence:  1,
		State:     storage.TaskStateQueued,
		Args:      storage.TaskArgs{},
	}, nil
}

func (m *MockTaskQueue) SetResultStreamer(streamer taskqueue.ResultStreamer) {
	// No-op for mock
}

func (m *MockTaskQueue) Start() {
	// No-op for mock
}

func (m *MockTaskQueue) Stop() {
	// No-op for mock
}
