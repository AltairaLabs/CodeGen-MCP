package taskqueue

import (
	"context"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// MockWorkerClient simulates worker task execution for testing
type MockWorkerClient struct{}

// NewMockWorkerClient creates a mock worker client
func NewMockWorkerClient() *MockWorkerClient {
	return &MockWorkerClient{}
}

// ExecuteTask simulates task execution (POC implementation)
const mockDuration = 10 * time.Millisecond

func (m *MockWorkerClient) ExecuteTask(
	ctx context.Context,
	workspaceID string,
	toolName string,
	args TaskArgs,
) (*TaskResult, error) {
	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate some work with context awareness
	select {
	case <-time.After(mockDuration):
		// Normal execution
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	switch toolName {
	case "echo":
		message, ok := args["message"].(string)
		if !ok {
			return nil, fmt.Errorf("message argument must be a string")
		}
		return &TaskResult{
			Success:  true,
			Output:   message,
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case "fs.read":
		path, ok := args["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path argument must be a string")
		}
		// Mock file content
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Content of %s in workspace %s", path, workspaceID),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case "fs.write":
		path, ok := args["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path argument must be a string")
		}
		contents, ok := args["contents"].(string)
		if !ok {
			return nil, fmt.Errorf("contents argument must be a string")
		}
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Wrote %d bytes to %s", len(contents), path),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ExecuteTypedTask simulates typed task execution
func (m *MockWorkerClient) ExecuteTypedTask(
	ctx context.Context,
	workspaceID string,
	request *protov1.ToolRequest,
) (*TaskResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate some work
	select {
	case <-time.After(mockDuration):
		// Normal execution
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Handle each tool type
	switch req := request.Request.(type) {
	case *protov1.ToolRequest_Echo:
		return &TaskResult{
			Success:  true,
			Output:   req.Echo.Message,
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case *protov1.ToolRequest_FsRead:
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Content of %s in workspace %s", req.FsRead.Path, workspaceID),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case *protov1.ToolRequest_FsWrite:
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Wrote %d bytes to %s", len(req.FsWrite.Contents), req.FsWrite.Path),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool type: %T", request.Request)
	}
}

// MockSessionStorage simulates session storage for testing
type MockSessionStorage struct {
	sessions map[string]*Session
	nextSeq  map[string]uint64
}

func newTestSessionStorage() *MockSessionStorage {
	return &MockSessionStorage{
		sessions: make(map[string]*Session),
		nextSeq:  make(map[string]uint64),
	}
}

func (s *MockSessionStorage) GetSession(sessionID string) (*Session, error) {
	if session, exists := s.sessions[sessionID]; exists {
		return session, nil
	}
	return nil, fmt.Errorf("session not found")
}

// MockWorkerRegistry simulates worker registry for testing
type MockWorkerRegistry struct{}

func NewWorkerRegistry() *MockWorkerRegistry {
	return &MockWorkerRegistry{}
}
