package python

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// Common interfaces and types
type SessionManager interface {
	GetSession(sessionID string) (*Session, bool)
	CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *Session
	HasWorkersAvailable() bool
}

type TaskQueueInterface interface {
	EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
}

type AuditLogger interface {
	LogToolCall(ctx context.Context, entry *AuditEntry)
	LogToolResult(ctx context.Context, entry *AuditEntry)
}

type ResultStreamer interface {
	Subscribe(taskID, sessionID string)
}

// Data structures
type Session struct {
	ID          string
	UserID      string
	WorkspaceID string
	WorkerID    string
}

type Task struct {
	ID       string
	Sequence int
}

type AuditEntry struct {
	SessionID   string
	UserID      string
	ToolName    string
	Arguments   map[string]interface{}
	WorkspaceID string
	ErrorMsg    string
}

type TaskResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	SessionID string    `json:"session_id"`
	Sequence  int       `json:"sequence"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// RunHandler handles run.python tool
type RunHandler struct {
	sessionManager types.SessionManagerWithWorkers
	taskQueue      types.TaskQueueInterface
	auditLogger    types.AuditLogger
	resultStreamer types.ResultStreamer
}

// NewRunHandler creates a new run.python handler
func NewRunHandler(sessionMgr types.SessionManagerWithWorkers, taskQueue types.TaskQueueInterface,
	auditLogger types.AuditLogger, resultStreamer types.ResultStreamer) *RunHandler {
	return &RunHandler{
		sessionManager: sessionMgr,
		taskQueue:      taskQueue,
		auditLogger:    auditLogger,
		resultStreamer: resultStreamer,
	}
}

// Handle implements the run.python tool
func (h *RunHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if workers are available before attempting to create session
	if !h.sessionManager.HasWorkersAvailable() {
		return mcp.NewToolResultError(config.ErrNoWorkersAvail), nil
	}

	// Get optional code and file parameters
	code := request.GetString("code", "")
	file := request.GetString("file", "")

	// Must have either code or file parameter
	if code == "" && file == "" {
		return mcp.NewToolResultError("must provide either 'code' or 'file' parameter"), nil
	}

	// Validate file path if provided
	if file != "" {
		if vErr := validateWorkspacePath(file); vErr != nil {
			return mcp.NewToolResultError(vErr.Error()), nil
		}
	}

	// Get or create session
	session, err := h.getOrCreateSession(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(config.ErrSessionError, err)), nil
	}

	args := map[string]interface{}{}
	if code != "" {
		args["code"] = code
	}
	if file != "" {
		args["file"] = file
	}

	h.auditLogger.LogToolCall(ctx, &types.AuditEntry{
		SessionID:   session.ID,
		UserID:      session.UserID,
		ToolName:    config.ToolRunPython,
		Arguments:   args,
		WorkspaceID: session.WorkspaceID,
	})

	// Create strongly-typed request
	runPythonReq := &protov1.RunPythonRequest{}
	if code != "" {
		runPythonReq.Source = &protov1.RunPythonRequest_Code{Code: code}
	} else if file != "" {
		runPythonReq.Source = &protov1.RunPythonRequest_File{File: file}
	}

	toolRequest := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_RunPython{
			RunPython: runPythonReq,
		},
	}

	// Enqueue task asynchronously with typed request
	taskID, err := h.taskQueue.EnqueueTypedTask(ctx, session.ID, toolRequest)
	if err != nil {
		h.auditLogger.LogToolResult(ctx, &types.AuditEntry{
			SessionID: session.ID,
			ToolName:  config.ToolRunPython,
			ErrorMsg:  err.Error(),
		})
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Subscribe to result notifications
	h.resultStreamer.Subscribe(taskID, session.ID)

	// Get task for sequence number
	task, err := h.taskQueue.GetTask(ctx, taskID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Return immediately with task ID and status
	response := types.TaskResponse{
		TaskID:    taskID,
		Status:    "queued",
		SessionID: session.ID,
		Sequence:  task.Sequence,
		Message:   fmt.Sprintf(config.MsgTaskQueued, taskID),
		CreatedAt: time.Now(),
	}

	responseJSON, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(responseJSON)), nil
}

// Helper functions
func validateWorkspacePath(path string) error {
	// Prevent absolute paths
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative to workspace root, got absolute path: %s", path)
	}

	// Prevent path traversal
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "/../") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	return nil
}

func (h *RunHandler) getOrCreateSession(ctx context.Context) (*types.Session, error) {
	sessionID := getSessionID(ctx)

	// Check if session exists
	session, ok := h.sessionManager.GetSession(sessionID)
	if ok {
		return session, nil
	}

	// Create new session with worker assignment
	session = h.sessionManager.CreateSession(ctx, sessionID, "default-user", "default-workspace")
	if session == nil {
		return nil, fmt.Errorf("failed to create session")
	}

	// Verify worker was assigned
	if session.WorkerID == "" {
		return nil, fmt.Errorf("no workers available")
	}

	return session, nil
}

// Common helper to extract session ID from context
func getSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value("session_id").(string); ok {
		return sessionID
	}
	return "default-session"
}
