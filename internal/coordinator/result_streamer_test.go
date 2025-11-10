package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestResultStreamer_Subscribe(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe a session to a task
	streamer.Subscribe("task-1", "session-1")

	// Verify subscription exists
	streamer.subMu.RLock()
	sessions := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if len(sessions) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(sessions))
	}
	if sessions[0] != "session-1" {
		t.Errorf("Expected session-1, got %s", sessions[0])
	}
}

func TestResultStreamer_SubscribeMultipleSessions(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe multiple sessions to same task
	streamer.Subscribe("task-1", "session-1")
	streamer.Subscribe("task-1", "session-2")
	streamer.Subscribe("task-1", "session-3")

	// Verify all subscriptions exist
	streamer.subMu.RLock()
	sessions := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if len(sessions) != 3 {
		t.Errorf("Expected 3 subscribers, got %d", len(sessions))
	}
}

func TestResultStreamer_Unsubscribe(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe and then unsubscribe
	streamer.Subscribe("task-1", "session-1")
	streamer.Subscribe("task-1", "session-2")

	streamer.Unsubscribe("task-1", "session-1")

	// Verify only session-2 remains
	streamer.subMu.RLock()
	sessions := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if len(sessions) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(sessions))
	}
	if sessions[0] != "session-2" {
		t.Errorf("Expected session-2, got %s", sessions[0])
	}
}

func TestResultStreamer_UnsubscribeLastSession(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe and unsubscribe last session
	streamer.Subscribe("task-1", "session-1")
	streamer.Unsubscribe("task-1", "session-1")

	// Verify task is cleaned up
	streamer.subMu.RLock()
	_, exists := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if exists {
		t.Error("Expected task subscription to be cleaned up")
	}
}

func TestResultStreamer_PublishResult_NoSubscribers(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Publish result with no subscribers
	result := &TaskResultNotification{
		TaskID: "task-1",
		Status: "completed",
		Result: &TaskResult{
			Success: true,
			Output:  "test output",
		},
	}

	err := streamer.PublishResult(context.Background(), "task-1", result)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestResultStreamer_PublishResult_WithCache(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Publish result (it should be cached)
	taskID := "task-1"
	result := &TaskResultNotification{
		TaskID: taskID,
		Status: "completed",
		Result: &TaskResult{
			Success: true,
			Output:  "test output",
		},
	}

	err := streamer.PublishResult(context.Background(), taskID, result)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify result was cached
	cachedResult, err := cache.Get(context.Background(), taskID)
	if err != nil {
		t.Errorf("Failed to get cached result: %v", err)
	}
	if cachedResult.Output != "test output" {
		t.Errorf("Expected cached output 'test output', got %s", cachedResult.Output)
	}
}

func TestResultStreamer_PublishProgress_NoSubscribers(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Publish progress with no subscribers
	progress := &TaskProgress{
		Stage:      "processing",
		Percentage: 50,
		Message:    "halfway done",
	}

	err := streamer.PublishProgress(context.Background(), "task-1", progress)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestResultStreamer_GetSubscriberCount(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Initially no subscribers
	count := streamer.GetSubscriberCount("task-1")
	if count != 0 {
		t.Errorf("Expected 0 subscribers, got %d", count)
	}

	// Add some subscribers
	streamer.Subscribe("task-1", "session-1")
	streamer.Subscribe("task-1", "session-2")

	count = streamer.GetSubscriberCount("task-1")
	if count != 2 {
		t.Errorf("Expected 2 subscribers, got %d", count)
	}
}

func TestResultStreamer_ClearSubscriptions(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Add multiple subscriptions for different tasks
	streamer.Subscribe("task-1", "session-1")
	streamer.Subscribe("task-1", "session-2")
	streamer.Subscribe("task-2", "session-1")

	// Clear all subscriptions
	streamer.ClearSubscriptions()

	// Verify all cleared
	streamer.subMu.RLock()
	count := len(streamer.subscriptions)
	streamer.subMu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 subscriptions after clear, got %d", count)
	}
}

func TestResultStreamer_SendNotification_SessionNotFound(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Try to send notification to non-existent session
	notification := &TaskResultNotification{
		TaskID: "task-1",
		Status: "completed",
	}

	err := streamer.sendNotification("nonexistent-session", notification)
	if err == nil {
		t.Error("Expected error when sending to non-existent session")
	}
}

func TestResultStreamer_PublishResult_CompletedCleansup(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe a session
	streamer.Subscribe("task-1", "session-1")

	// Publish completed result (should cleanup subscriptions)
	result := &TaskResultNotification{
		TaskID: "task-1",
		Status: "completed",
		Result: &TaskResult{
			Success: true,
		},
	}

	_ = streamer.PublishResult(context.Background(), "task-1", result)

	// Verify subscriptions were cleaned up
	streamer.subMu.RLock()
	_, exists := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if exists {
		t.Error("Expected subscriptions to be cleaned up after completed status")
	}
}

func TestResultStreamer_PublishResult_FailedCleansup(t *testing.T) {
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Subscribe a session
	streamer.Subscribe("task-1", "session-1")

	// Publish failed result (should cleanup subscriptions)
	result := &TaskResultNotification{
		TaskID: "task-1",
		Status: "failed",
		Error:  "test error",
	}

	_ = streamer.PublishResult(context.Background(), "task-1", result)

	// Verify subscriptions were cleaned up
	streamer.subMu.RLock()
	_, exists := streamer.subscriptions["task-1"]
	streamer.subMu.RUnlock()

	if exists {
		t.Error("Expected subscriptions to be cleaned up after failed status")
	}
}
