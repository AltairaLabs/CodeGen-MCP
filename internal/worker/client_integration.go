// Integration-focused client methods that require gRPC connections, streams, and full coordinator setup.
// These functions are excluded from unit test coverage as they require integration test infrastructure.

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

// Start begins the registration and heartbeat process
func (rc *Client) Start(ctx context.Context) error {
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

// connectAndRegister establishes connection and registers with coordinator
func (rc *Client) connectAndRegister(ctx context.Context) error {
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
func (rc *Client) heartbeatLoop() {
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
func (rc *Client) sendHeartbeat(ctx context.Context) error {
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
func (rc *Client) openTaskStream(ctx context.Context) error {
	if rc.lifecycleClient == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	stream, err := rc.lifecycleClient.TaskStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open task stream: %w", err)
	}

	rc.taskStream = stream

	// Send initial keepalive to identify this worker to the coordinator
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
func (rc *Client) taskStreamLoop() {
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
			return
		}

		// Handle different message types
		switch payload := msg.Message.(type) {
		case *protov1.TaskStreamMessage_Assignment:
			go rc.handleTaskAssignment(payload.Assignment)

		case *protov1.TaskStreamMessage_SessionCreate:
			go rc.handleSessionCreate(payload.SessionCreate)

		case *protov1.TaskStreamMessage_Keepalive:
			rc.logger.Debug("Received keepalive from coordinator")

		default:
			rc.logger.Warn("Unknown task stream message type")
		}
	}
}

// handleTaskAssignment executes a task assignment and sends the response back
func (rc *Client) handleTaskAssignment(assignment *protov1.TaskAssignment) {
	taskID := assignment.TaskId
	sessionID := assignment.SessionId
	sequence := assignment.Sequence
	toolName := getToolNameFromRequest(assignment.Request)

	rc.logger.Info("Received task assignment",
		"task_id", taskID,
		"tool_name", toolName,
		"session_id", sessionID,
		"sequence", sequence)

	// Check sequence number for deduplication
	if sequence > 0 {
		lastSeq, err := rc.sessionPool.GetLastCompletedSequence(sessionID)
		if err != nil {
			rc.logger.Warn("Could not get last completed sequence",
				"session_id", sessionID,
				"error", err)
		} else if sequence <= lastSeq {
			rc.logger.Warn("Skipping duplicate task execution",
				"task_id", taskID,
				"sequence", sequence,
				"last_completed", lastSeq)

			successResp := &protov1.TaskStreamMessage{
				Message: &protov1.TaskStreamMessage_Response{
					Response: &protov1.TaskStreamResponse{
						TaskId: taskID,
						Payload: &protov1.TaskStreamResponse_Result{
							Result: &protov1.TaskStreamResult{
								Status:   protov1.TaskStreamResult_STATUS_SUCCESS,
								Response: nil,
								Sequence: sequence,
							},
						},
					},
				},
			}

			if err := rc.taskStream.Send(successResp); err != nil {
				rc.logger.Error("Failed to send duplicate task response", "error", err)
			}
			return
		}
	}

	// Execute task
	req := &protov1.TaskRequest{
		TaskId:       taskID,
		SessionId:    sessionID,
		TypedRequest: assignment.Request,
		Context:      assignment.Context,
		Constraints:  assignment.Constraints,
		Sequence:     sequence,
	}

	responseCollector := &taskResponseCollector{
		taskID:      taskID,
		sequence:    sequence,
		sessionID:   sessionID,
		responses:   make([]*protov1.TaskStreamResponse, 0),
		taskStream:  rc.taskStream,
		sessionPool: rc.sessionPool,
		logger:      rc.logger,
	}

	ctx := context.Background()
	err := rc.taskExecutor.Execute(ctx, req, responseCollector)
	if err != nil {
		rc.logger.Error("Task execution failed", "task_id", taskID, "error", err)

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
func (rc *Client) handleSessionCreate(req *protov1.SessionCreateRequest) {
	sessionID := req.SessionId
	rc.logger.Info("Received session creation request",
		"session_id", sessionID,
		"workspace_id", req.WorkspaceId,
		"user_id", req.UserId)

	ctx := context.Background()
	err := rc.sessionPool.CreateSessionWithID(ctx, sessionID, req.WorkspaceId, req.UserId, req.EnvVars, req.Metadata)

	sessionMetadata := make(map[string]string)
	if err == nil {
		if meta, metaErr := rc.sessionPool.GetSessionMetadata(sessionID); metaErr == nil {
			sessionMetadata = meta
		} else {
			rc.logger.Warn("Failed to retrieve session metadata after creation",
				"session_id", sessionID,
				"error", metaErr)
		}
	}

	response := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_SessionCreated{
			SessionCreated: &protov1.SessionCreateResponse{
				SessionId: sessionID,
				Success:   err == nil,
				Error:     "",
				Metadata:  sessionMetadata,
			},
		},
	}

	if err != nil {
		response.GetSessionCreated().Error = err.Error()
		rc.logger.Error("Failed to create session", "session_id", sessionID, "error", err)
	} else {
		rc.logger.Info("Session created successfully", "session_id", sessionID)
	}

	if sendErr := rc.taskStream.Send(response); sendErr != nil {
		rc.logger.Error("Failed to send session creation response",
			"session_id", sessionID,
			"error", sendErr)
	}
}

// deregister notifies the coordinator that this worker is shutting down
func (rc *Client) deregister(ctx context.Context) error {
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
func (rc *Client) reconnect(ctx context.Context) error {
	rc.logger.Info("Attempting to reconnect to coordinator")

	if rc.conn != nil {
		_ = rc.conn.Close()
		rc.conn = nil
		rc.lifecycleClient = nil
		rc.registrationID = ""
	}

	const exponentialBackoffFactor = 2
	delay := rc.baseReconnectDelay
	attempt := 0

	for {
		attempt++

		select {
		case <-rc.stopChan:
			return fmt.Errorf("reconnection canceled")
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rc.logger.Info("Reconnection attempt", "attempt", attempt, "delay", delay)

		if err := rc.connectAndRegister(ctx); err != nil {
			rc.logger.Warn("Reconnection attempt failed", "attempt", attempt, "error", err)

			select {
			case <-time.After(delay):
			case <-rc.stopChan:
				return fmt.Errorf("reconnection canceled")
			case <-ctx.Done():
				return ctx.Err()
			}

			delay = time.Duration(math.Min(
				float64(delay*exponentialBackoffFactor),
				float64(rc.maxReconnectDelay),
			))
			continue
		}

		rc.logger.Info("Reconnection successful", "attempts", attempt)

		if err := rc.openTaskStream(ctx); err != nil {
			rc.logger.Error("Failed to reopen task stream after reconnection", "error", err)
			delay = rc.baseReconnectDelay
			continue
		}

		go rc.taskStreamLoop()
		rc.logger.Info("Task stream restored after reconnection")
		return nil
	}
}

// taskResponseCollector adapts the task executor stream interface to task stream messages
type taskResponseCollector struct {
	taskID      string
	sequence    uint64
	sessionID   string
	responses   []*protov1.TaskStreamResponse
	taskStream  protov1.WorkerLifecycle_TaskStreamClient
	sessionPool *SessionPool
	logger      *slog.Logger
}

func (t *taskResponseCollector) Send(resp *protov1.TaskResponse) error {
	streamResp := &protov1.TaskStreamResponse{TaskId: t.taskID}

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
		sessionMetadata := make(map[string]string)
		if meta, metaErr := t.sessionPool.GetSessionMetadata(t.sessionID); metaErr == nil {
			sessionMetadata = meta
		} else {
			t.logger.Warn("Failed to retrieve session metadata for task result",
				"session_id", t.sessionID,
				"task_id", t.taskID,
				"error", metaErr)
		}

		streamResp.Payload = &protov1.TaskStreamResponse_Result{
			Result: &protov1.TaskStreamResult{
				Status:    protov1.TaskStreamResult_Status(payload.Result.Status),
				Response:  payload.Result.TypedResponse,
				Artifacts: payload.Result.Artifacts,
				Metadata: &protov1.TaskExecutionMetadata{
					StartTimeMs: payload.Result.Metadata.StartTimeMs,
					EndTimeMs:   payload.Result.Metadata.EndTimeMs,
					DurationMs:  payload.Result.Metadata.DurationMs,
					PeakUsage:   payload.Result.Metadata.PeakUsage,
					ExitCode:    payload.Result.Metadata.ExitCode,
				},
				Sequence:        t.sequence,
				SessionMetadata: sessionMetadata,
			},
		}

		if t.sequence > 0 && payload.Result.Status == protov1.TaskResult_STATUS_SUCCESS {
			if err := t.sessionPool.SetLastCompletedSequence(t.sessionID, t.sequence); err != nil {
				t.logger.Warn("Failed to update last completed sequence",
					"session_id", t.sessionID,
					"sequence", t.sequence,
					"error", err)
			} else {
				t.logger.Info("Updated last completed sequence",
					"session_id", t.sessionID,
					"sequence", t.sequence)
			}
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
	// No-op
}
