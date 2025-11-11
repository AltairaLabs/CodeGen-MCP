package worker

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// Client is the main worker implementation that handles coordinator communication,
// session management, and task execution.
type Client struct {
	workerID        string
	grpcAddress     string // Worker's own gRPC address (for coordinator to connect back)
	coordinatorAddr string
	version         string
	sessionPool     *SessionPool
	taskExecutor    *TaskExecutor
	logger          *slog.Logger

	// gRPC connection and client
	conn            *grpc.ClientConn
	lifecycleClient protov1.WorkerLifecycleClient

	// Registration state
	registrationID    string
	heartbeatInterval time.Duration

	// Task stream
	taskStream protov1.WorkerLifecycle_TaskStreamClient

	// Control channels
	stopChan chan struct{}
	doneChan chan struct{}

	// Reconnection settings
	maxReconnectDelay  time.Duration
	baseReconnectDelay time.Duration
}

// Config holds configuration for the worker client
type Config struct {
	WorkerID        string
	CoordinatorAddr string
	Version         string
	MaxSessions     int32
	BaseWorkspace   string
	Logger          *slog.Logger
}

// NewClient creates a new worker client with all components
func NewClient(cfg *Config) *Client {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Create worker components
	sessionPool := NewSessionPool(cfg.WorkerID, cfg.MaxSessions, cfg.BaseWorkspace)
	taskExecutor := NewTaskExecutor(sessionPool)

	const (
		defaultMaxReconnectDelay  = 5 * time.Minute
		defaultBaseReconnectDelay = 1 * time.Second
	)

	return &Client{
		workerID:           cfg.WorkerID,
		grpcAddress:        "", // Worker doesn't listen, all communication via task stream
		coordinatorAddr:    cfg.CoordinatorAddr,
		version:            cfg.Version,
		sessionPool:        sessionPool,
		taskExecutor:       taskExecutor,
		logger:             cfg.Logger,
		stopChan:           make(chan struct{}),
		doneChan:           make(chan struct{}),
		maxReconnectDelay:  defaultMaxReconnectDelay,
		baseReconnectDelay: defaultBaseReconnectDelay,
	}
}

// Stop gracefully stops the registration client and deregisters
func (rc *Client) Stop(ctx context.Context) error {
	rc.logger.Info("Stopping registration client", "worker_id", rc.workerID)

	// Check if we were ever started
	select {
	case <-rc.stopChan:
		// Already stopped
		return nil
	default:
		// Signal stop
		close(rc.stopChan)
	}

	// Deregister if we have a connection
	if rc.conn != nil && rc.registrationID != "" {
		if err := rc.deregister(ctx); err != nil {
			rc.logger.Warn("Failed to deregister cleanly", "error", err)
		}
	}

	// Close connection
	if rc.conn != nil {
		if err := rc.conn.Close(); err != nil {
			rc.logger.Warn("Failed to close gRPC connection", "error", err)
		}
	}

	// Wait for heartbeat loop to finish (only if it was started)
	if rc.registrationID != "" {
		<-rc.doneChan
	}

	rc.logger.Info("Registration client stopped")
	return nil
}

// getWorkerStatus builds the current worker status
func (rc *Client) getWorkerStatus(capacity *protov1.SessionCapacity) *protov1.WorkerStatus {
	// Determine state based on capacity
	state := protov1.WorkerStatus_STATE_IDLE
	if capacity.ActiveSessions > 0 {
		state = protov1.WorkerStatus_STATE_BUSY
	}

	// Count total active tasks across all sessions
	activeTasks := int32(0)
	for _, session := range capacity.Sessions {
		activeTasks += session.ActiveTasks
	}

	return &protov1.WorkerStatus{
		State:       state,
		ActiveTasks: activeTasks,
		CurrentUsage: &protov1.ResourceUsage{
			MemoryBytes:   0, // Actual resource tracking will be implemented later
			CpuMillicores: 0,
			DiskBytes:     0,
		},
		Errors: []string{}, // Error tracking will be implemented later
	}
}

// getToolNameFromRequest extracts the tool name from a ToolRequest
func getToolNameFromRequest(request *protov1.ToolRequest) string {
	if request == nil {
		return "unknown"
	}

	switch request.Request.(type) {
	case *protov1.ToolRequest_Echo:
		return "echo"
	case *protov1.ToolRequest_FsRead:
		return "fs.read"
	case *protov1.ToolRequest_FsWrite:
		return "fs.write"
	case *protov1.ToolRequest_FsList:
		return "fs.list"
	case *protov1.ToolRequest_RunPython:
		return "run.python"
	case *protov1.ToolRequest_PkgInstall:
		return "pkg.install"
	default:
		return "unknown"
	}
}
