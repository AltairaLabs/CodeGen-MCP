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

	// Create gRPC clients for this worker (connection will be established later)
	// For now, we'll create placeholder clients - actual connection happens when needed
	sessionID := fmt.Sprintf("reg-%s-%d", req.WorkerId, time.Now().UnixNano())
	const defaultHeartbeatInterval = 30 * time.Second
	heartbeatInterval := defaultHeartbeatInterval

	// Register the worker
	worker := &RegisteredWorker{
		WorkerID:          req.WorkerId,
		SessionID:         sessionID,
		Capabilities:      req.Capabilities,
		Limits:            req.Limits,
		Status:            nil, // Will be updated on first heartbeat
		Capacity:          nil, // Will be updated on first heartbeat
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

// RegisterWithServer registers the coordinator's gRPC services with a gRPC server
func (cs *CoordinatorServer) RegisterWithServer(server *grpc.Server) {
	protov1.RegisterWorkerLifecycleServer(server, cs)
}
