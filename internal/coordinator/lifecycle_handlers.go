package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// CoordinatorServer implements the gRPC services for the coordinator
type CoordinatorServer struct {
	protov1.UnimplementedWorkerLifecycleServer

	workerRegistry *WorkerRegistry
	sessionManager *SessionManager
	logger         *slog.Logger
}

// NewCoordinatorServer creates a new coordinator server
func NewCoordinatorServer(
	registry *WorkerRegistry,
	sessionMgr *SessionManager,
	logger *slog.Logger,
) *CoordinatorServer {
	if logger == nil {
		logger = slog.Default()
	}

	return &CoordinatorServer{
		workerRegistry: registry,
		sessionManager: sessionMgr,
		logger:         logger,
	}
}

// RegisterWorker handles worker registration requests
func (cs *CoordinatorServer) RegisterWorker(
	ctx context.Context,
	req *protov1.RegisterRequest,
) (*protov1.RegisterResponse, error) {
	cs.logger.Info("Worker registration request",
		"worker_id", req.WorkerId,
		"version", req.Version,
		"max_sessions", req.Capabilities.GetMaxSessions())

	// Validate request
	if req.WorkerId == "" {
		return &protov1.RegisterResponse{
			Accepted: false,
			Reason:   "worker_id is required",
		}, nil
	}

	// Check if worker already registered
	if existing := cs.workerRegistry.GetWorker(req.WorkerId); existing != nil {
		cs.logger.Warn("Worker already registered, updating registration",
			"worker_id", req.WorkerId)

		// Allow re-registration (worker might have restarted)
		if err := cs.workerRegistry.DeregisterWorker(req.WorkerId); err != nil {
			cs.logger.Warn("Failed to deregister existing worker",
				"worker_id", req.WorkerId,
				"error", err)
		}
	}

	// Register the worker - task stream will be established separately via TaskStream RPC
	sessionID := fmt.Sprintf("reg-%s-%d", req.WorkerId, time.Now().UnixNano())
	const defaultHeartbeatInterval = 30 * time.Second
	heartbeatInterval := defaultHeartbeatInterval

	cs.logger.Info("Registering worker without task stream (will be established via TaskStream RPC)",
		"worker_id", req.WorkerId)

	// Register the worker with initial status
	worker := &RegisteredWorker{
		WorkerID:              req.WorkerId,
		SessionID:             sessionID,
		TaskStream:            nil, // Will be set when worker calls TaskStream RPC
		PendingTasks:          make(map[string]*PendingTask),
		PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),
		Capabilities:          req.Capabilities,
		Limits:                req.Limits,
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     req.Capabilities.GetMaxSessions(),
			ActiveSessions:    0,
			AvailableSessions: req.Capabilities.GetMaxSessions(),
		},
		LastHeartbeat:     time.Now(),
		RegisteredAt:      time.Now(),
		HeartbeatInterval: heartbeatInterval,
	}

	if err := cs.workerRegistry.RegisterWorker(req.WorkerId, worker); err != nil {
		cs.logger.Error("Failed to register worker",
			"worker_id", req.WorkerId,
			"error", err)
		return &protov1.RegisterResponse{
			Accepted: false,
			Reason:   fmt.Sprintf("registration failed: %v", err),
		}, nil
	}

	cs.logger.Info("Worker registered successfully",
		"worker_id", req.WorkerId,
		"session_id", sessionID)

	// Check for existing sessions that were assigned to this worker
	// This handles worker reconnection scenarios
	existingSessions, err := cs.sessionManager.GetSessionsByWorkerID(req.WorkerId)
	if err != nil {
		cs.logger.Warn("Failed to query existing sessions for worker",
			"worker_id", req.WorkerId,
			"error", err)
	} else if len(existingSessions) > 0 {
		cs.logger.Info("Found existing sessions for reconnected worker",
			"worker_id", req.WorkerId,
			"session_count", len(existingSessions))

		// TODO: Implement session recovery protocol (Phase 5 Step 5.2)
		// For each existing session:
		// 1. Check if session is still valid (not failed/terminated)
		// 2. Retrieve session metadata to send to worker
		// 3. Send session recovery request to worker via task stream
		// 4. Worker reconciles: accepts recovery (restores state) or rejects (abandons)
		// 5. Update session state based on worker response
		//
		// For now, we log that sessions exist and worker should abandon them
		// Worker will handle cleanup when it realizes sessions aren't recovered
		for _, sess := range existingSessions {
			cs.logger.Debug("Existing session for worker",
				"session_id", sess.ID,
				"workspace_id", sess.WorkspaceID,
				"state", sess.State,
				"worker_id", req.WorkerId)
		}
	}

	return &protov1.RegisterResponse{
		SessionId:            sessionID,
		TaskEndpoint:         "", // Worker connects to us, not the other way
		HeartbeatIntervalSec: int32(heartbeatInterval.Seconds()),
		Accepted:             true,
		Reason:               "registration successful",
	}, nil
}

// Heartbeat handles periodic worker heartbeat requests
func (cs *CoordinatorServer) Heartbeat(
	ctx context.Context,
	req *protov1.HeartbeatRequest,
) (*protov1.HeartbeatResponse, error) {
	// Validate worker is registered
	worker := cs.workerRegistry.GetWorker(req.WorkerId)
	if worker == nil {
		cs.logger.Warn("Heartbeat from unregistered worker",
			"worker_id", req.WorkerId)
		return &protov1.HeartbeatResponse{
			ContinueServing: false,
			Commands:        []string{},
		}, fmt.Errorf("worker not registered: %s", req.WorkerId)
	}

	// Validate session ID matches
	worker.mu.RLock()
	registeredSessionID := worker.SessionID
	worker.mu.RUnlock()

	if req.SessionId != registeredSessionID {
		cs.logger.Warn("Heartbeat with mismatched session ID",
			"worker_id", req.WorkerId,
			"expected", registeredSessionID,
			"received", req.SessionId)
		return &protov1.HeartbeatResponse{
			ContinueServing: false,
			Commands:        []string{},
		}, fmt.Errorf("invalid session_id")
	}

	// Update worker state
	if err := cs.workerRegistry.UpdateHeartbeat(req.WorkerId, req.Status, req.Capacity); err != nil {
		cs.logger.Error("Failed to update heartbeat",
			"worker_id", req.WorkerId,
			"error", err)
		return &protov1.HeartbeatResponse{
			ContinueServing: true,
			Commands:        []string{},
		}, err
	}

	// Log capacity info periodically (debug)
	if req.Capacity != nil {
		cs.logger.Debug("Worker heartbeat",
			"worker_id", req.WorkerId,
			"state", req.Status.State.String(),
			"active_sessions", req.Capacity.ActiveSessions,
			"available_sessions", req.Capacity.AvailableSessions)
	}

	return &protov1.HeartbeatResponse{
		ContinueServing: true,
		Commands:        []string{}, // No commands for now
	}, nil
}

// DeregisterWorker handles worker deregistration requests
func (cs *CoordinatorServer) DeregisterWorker(
	ctx context.Context,
	req *protov1.DeregisterRequest,
) (*protov1.DeregisterResponse, error) {
	cs.logger.Info("Worker deregistration request",
		"worker_id", req.WorkerId,
		"reason", req.Reason)

	// Validate worker exists
	worker := cs.workerRegistry.GetWorker(req.WorkerId)
	if worker == nil {
		cs.logger.Warn("Deregistration request for unknown worker",
			"worker_id", req.WorkerId)
		return &protov1.DeregisterResponse{
			Acknowledged: false,
		}, fmt.Errorf("worker not registered: %s", req.WorkerId)
	}

	// Validate session ID
	worker.mu.RLock()
	registeredSessionID := worker.SessionID
	worker.mu.RUnlock()

	if req.SessionId != registeredSessionID {
		cs.logger.Warn("Deregistration with mismatched session ID",
			"worker_id", req.WorkerId,
			"expected", registeredSessionID,
			"received", req.SessionId)
		return &protov1.DeregisterResponse{
			Acknowledged: false,
		}, fmt.Errorf("invalid session_id")
	}

	// Remove worker from registry
	if err := cs.workerRegistry.DeregisterWorker(req.WorkerId); err != nil {
		cs.logger.Error("Failed to deregister worker",
			"worker_id", req.WorkerId,
			"error", err)
		return &protov1.DeregisterResponse{
			Acknowledged: false,
		}, err
	}

	cs.logger.Info("Worker deregistered successfully",
		"worker_id", req.WorkerId)

	return &protov1.DeregisterResponse{
		Acknowledged: true,
	}, nil
}

// StartCleanupLoop starts a background goroutine to clean up stale workers
func (cs *CoordinatorServer) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cs.logger.Info("Cleanup loop stopping")
			return
		case <-ticker.C:
			const workerStaleTimeout = 5 * time.Minute
			removed := cs.workerRegistry.CleanupStaleWorkers(workerStaleTimeout)
			if removed > 0 {
				cs.logger.Info("Cleaned up stale workers", "count", removed)
			}
		}
	}
}

// TaskStream handles the bidirectional stream for task assignment
// Worker opens this stream and coordinator sends tasks over it
func (cs *CoordinatorServer) identifyWorkerByTaskResponse(taskID string, stream protov1.WorkerLifecycle_TaskStreamServer) (string, *RegisteredWorker) {
	cs.workerRegistry.mu.RLock()
	defer cs.workerRegistry.mu.RUnlock()

	for _, w := range cs.workerRegistry.workers {
		w.mu.RLock()
		if _, ok := w.PendingTasks[taskID]; ok {
			workerID := w.WorkerID
			w.mu.RUnlock()

			w.mu.Lock()
			w.TaskStream = stream
			w.mu.Unlock()

			cs.logger.Info("Associated stream with worker via task response", "worker_id", workerID)
			return workerID, w
		}
		w.mu.RUnlock()
	}
	return "", nil
}

func (cs *CoordinatorServer) identifyWorkerByKeepalive(stream protov1.WorkerLifecycle_TaskStreamServer) (string, *RegisteredWorker) {
	cs.workerRegistry.mu.RLock()
	var candidates []*RegisteredWorker
	for _, w := range cs.workerRegistry.workers {
		w.mu.RLock()
		if w.TaskStream == nil {
			candidates = append(candidates, w)
		}
		w.mu.RUnlock()
	}
	cs.workerRegistry.mu.RUnlock()

	if len(candidates) == 1 {
		worker := candidates[0]
		worker.mu.Lock()
		worker.TaskStream = stream
		worker.mu.Unlock()
		cs.logger.Info("Associated stream with worker via keepalive", "worker_id", worker.WorkerID)
		return worker.WorkerID, worker
	}

	if len(candidates) > 1 {
		cs.logger.Warn("Multiple workers without streams, cannot identify worker from keepalive alone", "candidate_count", len(candidates))
	} else {
		cs.logger.Warn("Received keepalive but all workers already have streams")
	}
	return "", nil
}

func (cs *CoordinatorServer) handleStreamMessage(
	ctx context.Context,
	msg *protov1.TaskStreamMessage,
	workerID *string,
	worker **RegisteredWorker,
	stream protov1.WorkerLifecycle_TaskStreamServer,
) {
	switch payload := msg.Message.(type) {
	case *protov1.TaskStreamMessage_Response:
		if *workerID == "" {
			newID, newWorker := cs.identifyWorkerByTaskResponse(payload.Response.TaskId, stream)
			if newID == "" {
				cs.logger.Error("Could not identify worker for task response", "task_id", payload.Response.TaskId)
				return
			}
			*workerID = newID
			*worker = newWorker
		}
		cs.handleTaskStreamResponse(ctx, *workerID, payload.Response)

	case *protov1.TaskStreamMessage_Keepalive:
		if *workerID == "" {
			newID, newWorker := cs.identifyWorkerByKeepalive(stream)
			if newID != "" {
				*workerID = newID
				*worker = newWorker
			}
		} else {
			cs.logger.Debug("Received keepalive from worker", "worker_id", *workerID)
		}

	case *protov1.TaskStreamMessage_SessionCreated:
		cs.handleSessionCreated(ctx, payload.SessionCreated, workerID, worker, stream)

	default:
		cs.logger.Warn("Unknown task stream message type")
	}
}

func (cs *CoordinatorServer) TaskStream(stream protov1.WorkerLifecycle_TaskStreamServer) error {
	ctx := stream.Context()
	cs.logger.Info("Worker task stream connected, waiting for identification")

	var workerID string
	var worker *RegisteredWorker

	msgChan := make(chan *protov1.TaskStreamMessage, 10)
	errChan := make(chan error, 1)

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				errChan <- err
				return
			}
			msgChan <- msg
		}
	}()

	defer func() {
		if worker != nil {
			worker.mu.Lock()
			worker.TaskStream = nil
			worker.mu.Unlock()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			cs.logger.Info("Task stream context cancelled", "worker_id", workerID)
			return ctx.Err()

		case err := <-errChan:
			cs.logger.Error("Error receiving from task stream", "error", err, "worker_id", workerID)
			return err

		case msg := <-msgChan:
			cs.handleStreamMessage(ctx, msg, &workerID, &worker, stream)
		}
	}
}

func (cs *CoordinatorServer) handleSessionCreated(
	ctx context.Context,
	resp *protov1.SessionCreateResponse,
	workerID *string,
	worker **RegisteredWorker,
	stream protov1.WorkerLifecycle_TaskStreamServer,
) {
	// Identify worker if not already identified
	if *workerID == "" {
		cs.identifyWorkerBySession(resp.SessionId, workerID, worker, stream)
	}

	// Find coordinator session by worker session ID
	coordSessionID := cs.findCoordinatorSessionID(resp.SessionId)
	if coordSessionID == "" {
		return
	}

	// Update session state based on success/failure
	cs.updateSessionStateFromResponse(coordSessionID, resp, *workerID)

	// Route response to waiting channel
	cs.routeSessionCreateResponse(resp, *worker)
}

func (cs *CoordinatorServer) identifyWorkerBySession(
	sessionID string,
	workerID *string,
	worker **RegisteredWorker,
	stream protov1.WorkerLifecycle_TaskStreamServer,
) {
	cs.workerRegistry.mu.RLock()
	for _, w := range cs.workerRegistry.workers {
		w.mu.RLock()
		if _, ok := w.PendingSessionCreates[sessionID]; ok {
			*worker = w
			*workerID = w.WorkerID
			w.mu.RUnlock()
			break
		}
		w.mu.RUnlock()
	}
	cs.workerRegistry.mu.RUnlock()

	if *worker != nil {
		(*worker).mu.Lock()
		(*worker).TaskStream = stream
		(*worker).mu.Unlock()
		cs.logger.Info("Associated stream with worker via session create response", "worker_id", *workerID)
	}
}

func (cs *CoordinatorServer) findCoordinatorSessionID(workerSessionID string) string {
	allSessions := cs.sessionManager.GetAllSessions()
	for id, session := range allSessions {
		if session.WorkerSessionID == workerSessionID {
			return id
		}
	}
	return ""
}

func (cs *CoordinatorServer) updateSessionStateFromResponse(
	coordSessionID string,
	resp *protov1.SessionCreateResponse,
	workerID string,
) {
	if resp.Success {
		if err := cs.sessionManager.UpdateSessionState(coordSessionID, SessionStateReady, ""); err == nil {
			cs.logger.Info("Session ready on worker",
				"session_id", coordSessionID,
				"worker_session_id", resp.SessionId,
				"worker_id", workerID)
		}
	} else {
		if err := cs.sessionManager.UpdateSessionState(coordSessionID, SessionStateFailed, resp.Error); err == nil {
			cs.logger.Error("Session creation failed on worker",
				"session_id", coordSessionID,
				"worker_session_id", resp.SessionId,
				"worker_id", workerID,
				"error", resp.Error)
		}
	}
}

func (cs *CoordinatorServer) routeSessionCreateResponse(resp *protov1.SessionCreateResponse, worker *RegisteredWorker) {
	if worker == nil {
		return
	}

	worker.mu.RLock()
	ch, ok := worker.PendingSessionCreates[resp.SessionId]
	worker.mu.RUnlock()

	if ok {
		select {
		case ch <- resp:
		default:
		}
	}
}

// handleTaskStreamResponse processes task responses from workers and routes them to waiting channels
func (cs *CoordinatorServer) handleTaskStreamResponse(ctx context.Context, workerID string, response *protov1.TaskStreamResponse) {
	taskID := response.TaskId

	cs.logger.Debug("Received task response",
		"task_id", taskID,
		"worker_id", workerID,
		"has_result", response.GetResult() != nil,
		"has_error", response.GetError() != nil)

	// Find worker and route response to pending task channel
	worker := cs.workerRegistry.GetWorker(workerID)
	if worker == nil {
		cs.logger.Error("Received response from unknown worker", "worker_id", workerID)
		return
	}

	worker.mu.RLock()
	pendingTask, ok := worker.PendingTasks[taskID]
	worker.mu.RUnlock()

	if !ok {
		cs.logger.Warn("Received response for unknown task",
			"task_id", taskID,
			"worker_id", workerID)
		return
	}

	// Send response to waiting channel (non-blocking)
	select {
	case pendingTask.ResponseChan <- response:
		cs.logger.Debug("Routed task response to waiting client", "task_id", taskID)
	default:
		cs.logger.Warn("Response channel full or closed", "task_id", taskID)
	}
}

// RegisterWithServer registers the coordinator's gRPC services with a gRPC server
func (cs *CoordinatorServer) RegisterWithServer(server *grpc.Server) {
	protov1.RegisterWorkerLifecycleServer(server, cs)
}
