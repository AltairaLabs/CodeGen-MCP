package coordinator

import (
	"context"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// This file contains gRPC bidirectional streaming handlers that are untestable in unit tests
// as they require complex stream mocking. These should be tested via integration tests.

// TaskStream handles the bidirectional streaming RPC for task execution between coordinator and workers
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
			cs.logger.Info("Task stream context canceled", "worker_id", workerID)
			return ctx.Err()

		case err := <-errChan:
			cs.logger.Error("Error receiving from task stream", "error", err, "worker_id", workerID)
			return err

		case msg := <-msgChan:
			cs.handleStreamMessage(ctx, msg, &workerID, &worker, stream)
		}
	}
}

// handleStreamMessage processes incoming messages on the task stream
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

// identifyWorkerByTaskResponse attempts to identify a worker by matching a task ID
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

// identifyWorkerByKeepalive attempts to identify a worker via keepalive when there's only one candidate
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

// handleSessionCreated processes session creation responses from workers
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

// identifyWorkerBySession attempts to identify a worker by matching a session ID
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

// findCoordinatorSessionID finds the coordinator session ID for a given worker session ID
func (cs *CoordinatorServer) findCoordinatorSessionID(workerSessionID string) string {
	allSessions := cs.sessionManager.GetAllSessions()
	for id, session := range allSessions {
		if session.WorkerSessionID == workerSessionID {
			return id
		}
	}
	return ""
}

// updateSessionStateFromResponse updates the session state based on the worker's response
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

// routeSessionCreateResponse routes the session creation response to the waiting channel
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
