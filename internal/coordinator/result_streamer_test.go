package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
)

const (
	testTaskID1   = "task-1"
	testSessionID = "session-1"
)

func TestResultStreamerBasic(t *testing.T) {
	sseManager := NewSSESessionManager()
	cacheInstance := cache.NewResultCache(5 * time.Minute)
	defer cacheInstance.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cacheInstance, logger)

	// Test basic subscribe functionality
	streamer.Subscribe(testTaskID1, testSessionID)

	// Verify subscription exists
	count := streamer.GetSubscriberCount(testTaskID1)
	if count != 1 {
		t.Errorf("Expected 1 subscriber, got %d", count)
	}

	// Test unsubscribe
	streamer.Unsubscribe(testTaskID1, testSessionID)
	count = streamer.GetSubscriberCount(testTaskID1)
	if count != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", count)
	}
}

func TestResultStreamerPublishResult(t *testing.T) {
	sseManager := NewSSESessionManager()
	cacheInstance := cache.NewResultCache(5 * time.Minute)
	defer cacheInstance.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cacheInstance, logger)

	// Publish result with no subscribers (should not error)
	result := &TaskResultNotification{
		TaskID: testTaskID1,
		Status: "completed",
		Result: &TaskResult{
			Success: true,
			Output:  "test output",
		},
	}

	err := streamer.PublishResult(context.Background(), testTaskID1, result)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify result was cached
	cachedResult, err := cacheInstance.Get(context.Background(), testTaskID1)
	if err != nil {
		t.Errorf("Failed to get cached result: %v", err)
	}
	if cachedResult.GetOutput() != "test output" {
		t.Errorf("Expected cached output 'test output', got %s", cachedResult.GetOutput())
	}
}
