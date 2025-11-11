package task

import (
	"context"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// ResultCache defines the interface for task result caching
type ResultCache interface {
	Get(ctx context.Context, taskID string) (Result, error)
}

// TaskQueue defines the interface for task queue operations
type TaskQueue interface {
	GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error)
}

// Result represents a task result from cache
type Result interface {
	GetOutput() string
}

// TaskResponse represents the response format for task queries
type TaskResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	SessionID string    `json:"session_id"`
	Sequence  uint64    `json:"sequence,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
