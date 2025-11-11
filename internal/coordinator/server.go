package coordinator

import (
	"context"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"github.com/AltairaLabs/codegen-mcp/internal/taskqueue"
	"github.com/AltairaLabs/codegen-mcp/internal/tools"
	"github.com/AltairaLabs/codegen-mcp/internal/tools/handlers/task"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer wraps the mcp-go server with our business logic and manages tool execution
type MCPServer struct {
	server         *server.MCPServer
	sessionManager *SessionManager
	workerClient   WorkerClient
	auditLogger    *AuditLogger
	taskQueue      taskqueue.TaskQueueInterface
	resultStreamer *ResultStreamer
	resultCache    cache.CacheInterface
	sseManager     *SSESessionManager
	toolRegistry   *tools.ToolHandlerRegistry
}

// Config holds configuration for the MCP server
type Config struct {
	Name    string
	Version string
}

// Server returns the underlying mcp-go server for serving
func (ms *MCPServer) Server() *server.MCPServer {
	return ms.server
}

// Task handler adapters
// These adapt the coordinator's internal interfaces to the task handler package interfaces

// resultCacheTaskAdapter adapts cache.CacheInterface to task.ResultCache
type resultCacheTaskAdapter struct {
	rc cache.CacheInterface
}

func (a *resultCacheTaskAdapter) Get(ctx context.Context, taskID string) (task.Result, error) {
	result, err := a.rc.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &taskResultAdapter{result: result}, nil
}

// taskResultAdapter adapts cache.TaskResultInterface to task.Result
type taskResultAdapter struct {
	result cache.TaskResultInterface
}

func (r *taskResultAdapter) GetOutput() string {
	return r.result.GetOutput()
}

// taskQueueTaskAdapter adapts taskqueue.TaskQueueInterface to task.TaskQueue
type taskQueueTaskAdapter struct {
	tq taskqueue.TaskQueueInterface
}

func (a *taskQueueTaskAdapter) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	return a.tq.GetTask(ctx, taskID)
}

// Note: NewMCPServer initialization logic has been moved to server_init.go
// Note: Tool registration logic has been moved to tool_registration.go
// Note: Serve methods have been moved to server_serve.go (untestable infrastructure code)
