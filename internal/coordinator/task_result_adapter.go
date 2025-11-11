package coordinator

import (
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
)

// TaskResultAdapter adapts TaskResult to implement cache.TaskResultInterface
// This allows TaskResult to work with the cache package without circular dependencies
type TaskResultAdapter struct {
	*TaskResult
}

// NewTaskResultAdapter creates a new adapter for TaskResult
func NewTaskResultAdapter(tr *TaskResult) cache.TaskResultInterface {
	return &TaskResultAdapter{TaskResult: tr}
}

// GetSuccess implements cache.TaskResultInterface
func (tra *TaskResultAdapter) GetSuccess() bool {
	return tra.Success
}

// GetOutput implements cache.TaskResultInterface
func (tra *TaskResultAdapter) GetOutput() string {
	return tra.Output
}

// GetError implements cache.TaskResultInterface
func (tra *TaskResultAdapter) GetError() string {
	return tra.Error
}

// GetExitCode implements cache.TaskResultInterface
func (tra *TaskResultAdapter) GetExitCode() int {
	return tra.ExitCode
}

// GetDuration implements cache.TaskResultInterface
func (tra *TaskResultAdapter) GetDuration() time.Duration {
	return tra.Duration
}
