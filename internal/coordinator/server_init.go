package coordinator

import (
	"log/slog"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/taskqueue"
	"github.com/AltairaLabs/codegen-mcp/internal/tools"
	"github.com/AltairaLabs/codegen-mcp/internal/tools/handlers/echo"
	"github.com/AltairaLabs/codegen-mcp/internal/tools/handlers/filesystem"
	"github.com/AltairaLabs/codegen-mcp/internal/tools/handlers/python"
	"github.com/AltairaLabs/codegen-mcp/internal/tools/handlers/task"
	"github.com/mark3labs/mcp-go/server"
)

// NewMCPServer creates and configures a new MCP server with all dependencies initialized
func NewMCPServer(cfg Config, sessionMgr *SessionManager, worker WorkerClient, audit *AuditLogger, taskQueue taskqueue.TaskQueueInterface) *MCPServer {
	// Create the mcp-go server
	mcpServer := server.NewMCPServer(
		cfg.Name,
		cfg.Version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Initialize async result streaming components
	resultCache := cache.NewResultCache(config.DefaultCacheTTL) // Default cache TTL from config
	sseManager := NewSSESessionManager()
	logger := slog.Default()
	resultStreamer := NewResultStreamer(sseManager, resultCache, logger)

	// Wire up result streamer to task queue
	// TODO: Create adapter for coordinator ResultStreamer to taskqueue.ResultStreamer interface
	// taskQueue.SetResultStreamer(resultStreamer)

	ms := &MCPServer{
		server:         mcpServer,
		sessionManager: sessionMgr,
		workerClient:   worker,
		auditLogger:    audit,
		taskQueue:      taskQueue,
		resultStreamer: resultStreamer,
		resultCache:    resultCache,
		sseManager:     sseManager,
	}

	// Create shared adapters for all tool handlers
	sessionMgrAdapter := newSessionManagerAdapter(ms.sessionManager, ms.sessionManager.workerRegistry)
	taskQueueAdapterInst := newTaskQueueAdapter(ms.taskQueue)
	auditLoggerAdapterInst := newAuditLoggerAdapter(ms.auditLogger)
	resultStreamerAdapterInst := newResultStreamerAdapter(ms.resultStreamer)

	// Create tool handlers using shared adapters
	echoHandler := echo.NewHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	fsReadHandler := filesystem.NewReadHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	fsWriteHandler := filesystem.NewWriteHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	fsListHandler := filesystem.NewListHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	runPythonHandler := python.NewRunHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	pkgInstallHandler := python.NewInstallHandler(
		sessionMgrAdapter,
		taskQueueAdapterInst,
		auditLoggerAdapterInst,
		resultStreamerAdapterInst,
	)

	// Task handlers for get.task.result and get.task.status
	taskResultHandler := task.NewResultHandler(
		&resultCacheTaskAdapter{ms.resultCache},
		&taskQueueTaskAdapter{ms.taskQueue},
	)

	taskStatusHandler := task.NewStatusHandler(
		&taskQueueTaskAdapter{ms.taskQueue},
	)

	// Create tool handler registry
	ms.toolRegistry = tools.NewToolHandlerRegistry(map[string]tools.ToolHandlerFunc{
		config.ToolEcho:          echoHandler.Handle,
		config.ToolFsRead:        fsReadHandler.Handle,
		config.ToolFsWrite:       fsWriteHandler.Handle,
		config.ToolFsList:        fsListHandler.Handle,
		config.ToolRunPython:     runPythonHandler.Handle,
		config.ToolPkgInstall:    pkgInstallHandler.Handle,
		config.ToolGetTaskResult: taskResultHandler.Handle,
		config.ToolGetTaskStatus: taskStatusHandler.Handle,
	})

	// Register tools with MCP server
	ms.registerTools()

	return ms
}
