package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
)

func TestCacheIntegration(t *testing.T) {
	// Create a cache instance
	cacheInstance := cache.NewResultCache(5 * time.Minute)
	defer cacheInstance.Close()

	// Create a task result
	result := &TaskResult{
		Success:  true,
		Output:   "test output",
		Error:    "",
		ExitCode: 0,
		Duration: 1 * time.Second,
	}

	// Create adapter
	adapter := NewTaskResultAdapter(result)

	// Store the result
	ctx := context.Background()
	taskID := "test-task-id"
	err := cacheInstance.Store(ctx, taskID, adapter)
	if err != nil {
		t.Fatalf("Failed to store result: %v", err)
	}

	// Retrieve the result
	cached, err := cacheInstance.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("Failed to retrieve result: %v", err)
	}

	// Verify the result
	if cached.GetSuccess() != result.Success {
		t.Errorf("Expected success %t, got %t", result.Success, cached.GetSuccess())
	}
	if cached.GetOutput() != result.Output {
		t.Errorf("Expected output %s, got %s", result.Output, cached.GetOutput())
	}
	if cached.GetError() != result.Error {
		t.Errorf("Expected error %s, got %s", result.Error, cached.GetError())
	}
	if cached.GetExitCode() != result.ExitCode {
		t.Errorf("Expected exit code %d, got %d", result.ExitCode, cached.GetExitCode())
	}
	if cached.GetDuration() != result.Duration {
		t.Errorf("Expected duration %v, got %v", result.Duration, cached.GetDuration())
	}
}