package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// RegistrationClient handles worker registration and lifecycle with the coordinator
type RegistrationClient struct {
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

// RegistrationConfig holds configuration for the registration client
type RegistrationConfig struct {
	WorkerID        string
	GRPCAddress     string // Worker's gRPC address (e.g., "localhost:50051")
	CoordinatorAddr string
	Version         string
	SessionPool     *SessionPool
	TaskExecutor    *TaskExecutor
	Logger          *slog.Logger
}

// NewRegistrationClient creates a new registration client
func NewRegistrationClient(cfg *RegistrationConfig) *RegistrationClient {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	const (
		defaultMaxReconnectDelay  = 5 * time.Minute
		defaultBaseReconnectDelay = 1 * time.Second
	)

	return &RegistrationClient{
		workerID:           cfg.WorkerID,
		grpcAddress:        cfg.GRPCAddress,
		coordinatorAddr:    cfg.CoordinatorAddr,
		version:            cfg.Version,
		sessionPool:        cfg.SessionPool,
		taskExecutor:       cfg.TaskExecutor,
		logger:             cfg.Logger,
		stopChan:           make(chan struct{}),
		doneChan:           make(chan struct{}),
		maxReconnectDelay:  defaultMaxReconnectDelay,
		baseReconnectDelay: defaultBaseReconnectDelay,
	}
}

// Start begins the registration and heartbeat process
func (rc *RegistrationClient) Start(ctx context.Context) error {
	rc.logger.Info("Starting registration client",
		"worker_id", rc.workerID,
		"coordinator", rc.coordinatorAddr)

	// Initial registration with retry
	if err := rc.connectAndRegister(ctx); err != nil {
		return fmt.Errorf("initial registration failed: %w", err)
	}

	// Open task stream for receiving task assignments
	if err := rc.openTaskStream(ctx); err != nil {
		return fmt.Errorf("failed to open task stream: %w", err)
	}

	// Start heartbeat loop in background
	go rc.heartbeatLoop()

	// Start task stream listener in background
	go rc.taskStreamLoop()

	rc.logger.Info("Registration client started successfully",
		"registration_id", rc.registrationID,
		"heartbeat_interval", rc.heartbeatInterval)

	return nil
}

// Stop gracefully stops the registration client and deregisters
func (rc *RegistrationClient) Stop(ctx context.Context) error {
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
} // connectAndRegister establishes connection and registers with coordinator
func (rc *RegistrationClient) connectAndRegister(ctx context.Context) error {
	// Establish gRPC connection
	conn, err := grpc.NewClient(
		rc.coordinatorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create gRPC client: %w", err)
	}

	rc.conn = conn
	rc.lifecycleClient = protov1.NewWorkerLifecycleClient(conn)

	// Get current capacity
	capacity := rc.sessionPool.GetCapacity()

	// Resource limit constants
	const (
		bytesInGB               = 1024 * 1024 * 1024
		defaultMaxCPUMillicores = 4000
		tasksPerSession         = 2
		memoryInGB              = 8
		diskInGB                = 100
		memoryPerSessionGB      = 2
		diskPerSessionGB        = 10
	)

	// Build registration request
	req := &protov1.RegisterRequest{
		WorkerId:  rc.workerID,
		AuthToken: "", // Authentication token support to be added in future
		Capabilities: &protov1.WorkerCapabilities{
			SupportedTools: []string{
				"echo",
				"fs.write",
				"fs.read",
				"fs.list",
				"run.python",
				"pkg.install",
			},
			Languages:   []string{"python"},
			MaxSessions: capacity.TotalSessions,
			Metadata:    map[string]string{},
		},
		Limits: &protov1.ResourceLimits{
			MaxMemoryBytes:      memoryInGB * bytesInGB,
			MaxCpuMillicores:    defaultMaxCPUMillicores,
			MaxDiskBytes:        diskInGB * bytesInGB,
			MaxConcurrentTasks:  capacity.TotalSessions * tasksPerSession,
			MaxMemoryPerSession: memoryPerSessionGB * bytesInGB,
			MaxDiskPerSession:   diskPerSessionGB * bytesInGB,
		},
		Version:     rc.version,
		GrpcAddress: rc.grpcAddress,
	}

	// Register with coordinator
	resp, err := rc.lifecycleClient.RegisterWorker(ctx, req)
	if err != nil {
		return fmt.Errorf("registration RPC failed: %w", err)
	}

	if !resp.Accepted {
		return fmt.Errorf("registration rejected: %s", resp.Reason)
	}

	// Store registration info
	rc.registrationID = resp.SessionId
	rc.heartbeatInterval = time.Duration(resp.HeartbeatIntervalSec) * time.Second

	rc.logger.Info("Successfully registered with coordinator",
		"registration_id", rc.registrationID,
		"heartbeat_interval", rc.heartbeatInterval)

	return nil
}

// heartbeatLoop sends periodic heartbeats to the coordinator
func (rc *RegistrationClient) heartbeatLoop() {
	defer close(rc.doneChan)

	ticker := time.NewTicker(rc.heartbeatInterval)
	defer ticker.Stop()

	consecutiveFailures := 0
	const (
		heartbeatTimeout         = 10 * time.Second
		maxConsecutiveFailures   = 3
		exponentialBackoffFactor = 2
	)

	for {
		select {
		case <-rc.stopChan:
			rc.logger.Info("Heartbeat loop stopping")
			return

		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), heartbeatTimeout)

			if err := rc.sendHeartbeat(ctx); err != nil {
				consecutiveFailures++
				rc.logger.Warn("Heartbeat failed",
					"error", err,
					"consecutive_failures", consecutiveFailures)

				// Attempt reconnection after multiple failures
				if consecutiveFailures >= maxConsecutiveFailures {
					rc.logger.Warn("Multiple heartbeat failures, attempting reconnection")
					if err := rc.reconnect(ctx); err != nil {
						rc.logger.Error("Reconnection failed", "error", err)
					} else {
						consecutiveFailures = 0
					}
				}
			} else {
				consecutiveFailures = 0
			}

			cancel()
		}
	}
}

// sendHeartbeat sends a single heartbeat to the coordinator
func (rc *RegistrationClient) sendHeartbeat(ctx context.Context) error {
	if rc.lifecycleClient == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	capacity := rc.sessionPool.GetCapacity()
	status := rc.getWorkerStatus(capacity)

	req := &protov1.HeartbeatRequest{
		WorkerId:  rc.workerID,
		SessionId: rc.registrationID,
		Status:    status,
		Capacity:  capacity,
	}

	resp, err := rc.lifecycleClient.Heartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("heartbeat RPC failed: %w", err)
	}

	// Handle any commands from coordinator
	if !resp.ContinueServing {
		rc.logger.Warn("Coordinator requested worker to stop serving")
	}

	for _, cmd := range resp.Commands {
		rc.logger.Info("Received command from coordinator", "command", cmd)
		// Command handling (drain, reload_config, etc.) will be implemented in future
	}

	return nil
}

// openTaskStream opens the bidirectional task stream with the coordinator
func (rc *RegistrationClient) openTaskStream(ctx context.Context) error {
	if rc.lifecycleClient == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	stream, err := rc.lifecycleClient.TaskStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open task stream: %w", err)
	}

	rc.taskStream = stream

	// Send initial keepalive to identify this worker to the coordinator
	// The coordinator will match this stream to the worker by checking active workers
	identifyMsg := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_Keepalive{
			Keepalive: &protov1.StreamKeepAlive{
				TimestampMs: time.Now().UnixMilli(),
			},
		},
	}

	if err := rc.taskStream.Send(identifyMsg); err != nil {
		return fmt.Errorf("failed to send initial identification: %w", err)
	}

	rc.logger.Info("Opened task stream with coordinator and sent identification", "worker_id", rc.workerID)
	return nil
}

// taskStreamLoop listens for task assignments from coordinator and executes them
func (rc *RegistrationClient) taskStreamLoop() {
	rc.logger.Info("Starting task stream listener")

	for {
		select {
		case <-rc.stopChan:
			rc.logger.Info("Task stream loop stopping")
			return
		default:
		}

		// Receive message from coordinator
		msg, err := rc.taskStream.Recv()
		if err != nil {
			rc.logger.Error("Error receiving from task stream", "error", err)
			// Stream broken, will be recreated on reconnect
			return
		}

		// Handle different message types
		switch payload := msg.Message.(type) {
		case *protov1.TaskStreamMessage_Assignment:
			// Coordinator sending task assignment
			go rc.handleTaskAssignment(payload.Assignment)

		case *protov1.TaskStreamMessage_SessionCreate:
			// Coordinator requesting session creation
			go rc.handleSessionCreate(payload.SessionCreate)

		case *protov1.TaskStreamMessage_Keepalive:
			// Keepalive from coordinator
			rc.logger.Debug("Received keepalive from coordinator")

		default:
			rc.logger.Warn("Unknown task stream message type")
		}
	}
}

// handleTaskAssignment executes a task assignment and sends the response back
func (rc *RegistrationClient) handleTaskAssignment(assignment *protov1.TaskAssignment) {
	taskID := assignment.TaskId
	rc.logger.Info("Received task assignment",
		"task_id", taskID,
		"tool_name", assignment.ToolName,
		"session_id", assignment.SessionId)

	// Convert to TaskRequest format for task executor
	req := &protov1.TaskRequest{
		TaskId:      taskID,
		SessionId:   assignment.SessionId,
		ToolName:    assignment.ToolName,
		Arguments:   assignment.Arguments,
		Context:     assignment.Context,
		Constraints: assignment.Constraints,
	}

	// Create a mock stream for collecting responses
	responseCollector := &taskResponseCollector{
		taskID:     taskID,
		responses:  make([]*protov1.TaskStreamResponse, 0),
		taskStream: rc.taskStream,
		logger:     rc.logger,
	}

	// Execute the task
	ctx := context.Background()
	err := rc.taskExecutor.Execute(ctx, req, responseCollector)
	if err != nil {
		rc.logger.Error("Task execution failed",
			"task_id", taskID,
			"error", err)

		// Send error response
		errorResp := &protov1.TaskStreamMessage{
			Message: &protov1.TaskStreamMessage_Response{
				Response: &protov1.TaskStreamResponse{
					TaskId: taskID,
					Payload: &protov1.TaskStreamResponse_Error{
						Error: &protov1.TaskStreamError{
							Code:      "EXECUTION_ERROR",
							Message:   err.Error(),
							Details:   "",
							Retriable: false,
						},
					},
				},
			},
		}

		if err := rc.taskStream.Send(errorResp); err != nil {
			rc.logger.Error("Failed to send error response", "error", err)
		}
	}

	rc.logger.Info("Task execution completed", "task_id", taskID)
}

// handleSessionCreate handles a session creation request from the coordinator
func (rc *RegistrationClient) handleSessionCreate(req *protov1.SessionCreateRequest) {
	sessionID := req.SessionId
	rc.logger.Info("Received session creation request",
		"session_id", sessionID,
		"workspace_id", req.WorkspaceId,
		"user_id", req.UserId)

	// Create session in the session pool with coordinator-provided ID
	ctx := context.Background()
	err := rc.sessionPool.CreateSessionWithID(ctx, sessionID, req.WorkspaceId, req.UserId, req.EnvVars)

	// Send response back to coordinator
	response := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_SessionCreated{
			SessionCreated: &protov1.SessionCreateResponse{
				SessionId: sessionID,
				Success:   err == nil,
				Error:     "",
			},
		},
	}

	if err != nil {
		response.GetSessionCreated().Error = err.Error()
		rc.logger.Error("Failed to create session",
			"session_id", sessionID,
			"error", err)
	} else {
		rc.logger.Info("Session created successfully",
			"session_id", sessionID)
	}

	// Send response over task stream
	if sendErr := rc.taskStream.Send(response); sendErr != nil {
		rc.logger.Error("Failed to send session creation response",
			"session_id", sessionID,
			"error", sendErr)
	}
}

// taskResponseCollector adapts the task executor stream interface to task stream messages
type taskResponseCollector struct {
	taskID     string
	responses  []*protov1.TaskStreamResponse
	taskStream protov1.WorkerLifecycle_TaskStreamClient
	logger     *slog.Logger
}

func (t *taskResponseCollector) Send(resp *protov1.TaskResponse) error {
	// Convert TaskResponse to TaskStreamResponse
	streamResp := &protov1.TaskStreamResponse{
		TaskId: t.taskID,
	}

	switch payload := resp.Payload.(type) {
	case *protov1.TaskResponse_Progress:
		streamResp.Payload = &protov1.TaskStreamResponse_Progress{
			Progress: &protov1.TaskProgressUpdate{
				PercentComplete: payload.Progress.PercentComplete,
				Stage:           payload.Progress.Stage,
				Message:         payload.Progress.Message,
			},
		}
	case *protov1.TaskResponse_Log:
		streamResp.Payload = &protov1.TaskStreamResponse_Log{
			Log: &protov1.TaskLogEntry{
				Level:       protov1.TaskLogEntry_Level(payload.Log.Level),
				Message:     payload.Log.Message,
				TimestampMs: payload.Log.TimestampMs,
				Source:      payload.Log.Source,
			},
		}
	case *protov1.TaskResponse_Result:
		streamResp.Payload = &protov1.TaskStreamResponse_Result{
			Result: &protov1.TaskStreamResult{
				Status:    protov1.TaskStreamResult_Status(payload.Result.Status),
				Outputs:   payload.Result.Outputs,
				Artifacts: payload.Result.Artifacts,
				Metadata: &protov1.TaskExecutionMetadata{
					StartTimeMs: payload.Result.Metadata.StartTimeMs,
					EndTimeMs:   payload.Result.Metadata.EndTimeMs,
					DurationMs:  payload.Result.Metadata.DurationMs,
					PeakUsage:   payload.Result.Metadata.PeakUsage,
					ExitCode:    payload.Result.Metadata.ExitCode,
				},
			},
		}
	case *protov1.TaskResponse_Error:
		streamResp.Payload = &protov1.TaskStreamResponse_Error{
			Error: &protov1.TaskStreamError{
				Code:      payload.Error.Code,
				Message:   payload.Error.Message,
				Details:   payload.Error.Details,
				Retriable: payload.Error.Retriable,
			},
		}
	}

	// Send over task stream
	msg := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_Response{
			Response: streamResp,
		},
	}

	return t.taskStream.Send(msg)
}

func (t *taskResponseCollector) Context() context.Context {
	return context.Background()
}

func (t *taskResponseCollector) SendMsg(m interface{}) error {
	return nil
}

func (t *taskResponseCollector) RecvMsg(m interface{}) error {
	return nil
}

func (t *taskResponseCollector) SetHeader(metadata.MD) error {
	return nil
}

func (t *taskResponseCollector) SendHeader(metadata.MD) error {
	return nil
}

func (t *taskResponseCollector) SetTrailer(metadata.MD) {
	// No-op: metadata not used in task stream
}

// getWorkerStatus builds the current worker status
func (rc *RegistrationClient) getWorkerStatus(capacity *protov1.SessionCapacity) *protov1.WorkerStatus {
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

// deregister notifies the coordinator that this worker is shutting down
func (rc *RegistrationClient) deregister(ctx context.Context) error {
	if rc.lifecycleClient == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	req := &protov1.DeregisterRequest{
		WorkerId:  rc.workerID,
		SessionId: rc.registrationID,
		Reason:    "graceful shutdown",
	}

	resp, err := rc.lifecycleClient.DeregisterWorker(ctx, req)
	if err != nil {
		return fmt.Errorf("deregister RPC failed: %w", err)
	}

	if !resp.Acknowledged {
		return fmt.Errorf("deregistration not acknowledged")
	}

	rc.logger.Info("Successfully deregistered from coordinator")
	return nil
}

// reconnect attempts to reconnect to the coordinator with exponential backoff
func (rc *RegistrationClient) reconnect(ctx context.Context) error {
	rc.logger.Info("Attempting to reconnect to coordinator")

	// Close existing connection
	if rc.conn != nil {
		_ = rc.conn.Close()
		rc.conn = nil
		rc.lifecycleClient = nil
		rc.registrationID = ""
	}

	// Retry with exponential backoff
	const (
		exponentialBackoffFactor = 2
		maxAttempts              = 10
	)
	delay := rc.baseReconnectDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-rc.stopChan:
			return fmt.Errorf("reconnection canceled")
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rc.logger.Info("Reconnection attempt",
			"attempt", attempt+1,
			"max_attempts", maxAttempts,
			"delay", delay)

		// Try to connect and register
		if err := rc.connectAndRegister(ctx); err != nil {
			rc.logger.Warn("Reconnection attempt failed",
				"attempt", attempt+1,
				"error", err)

			// Wait before next attempt
			select {
			case <-time.After(delay):
			case <-rc.stopChan:
				return fmt.Errorf("reconnection canceled")
			case <-ctx.Done():
				return ctx.Err()
			}

			// Exponential backoff with max delay
			delay = time.Duration(math.Min(
				float64(delay*exponentialBackoffFactor),
				float64(rc.maxReconnectDelay),
			))
			continue
		}

		rc.logger.Info("Reconnection successful")
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}
