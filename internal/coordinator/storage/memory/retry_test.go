package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

func TestGetTasksReadyForRetry(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	past := now.Add(-1 * time.Minute)
	future := now.Add(1 * time.Minute)

	// Create tasks with different states and retry times
	tasks := []*storage.QueuedTask{
		{
			ID:          "task1",
			SessionID:   "session1",
			State:       storage.TaskStateRetrying,
			NextRetryAt: &past, // Ready for retry (past time)
			RetryCount:  1,
			MaxRetries:  3,
			CreatedAt:   now,
		},
		{
			ID:          "task2",
			SessionID:   "session1",
			State:       storage.TaskStateRetrying,
			NextRetryAt: &future, // Not ready yet (future time)
			RetryCount:  1,
			MaxRetries:  3,
			CreatedAt:   now,
		},
		{
			ID:          "task3",
			SessionID:   "session2",
			State:       storage.TaskStateQueued, // Not in retrying state
			NextRetryAt: &past,
			RetryCount:  0,
			MaxRetries:  3,
			CreatedAt:   now,
		},
		{
			ID:          "task4",
			SessionID:   "session2",
			State:       storage.TaskStateRetrying,
			NextRetryAt: &past, // Ready for retry
			RetryCount:  2,
			MaxRetries:  3,
			CreatedAt:   now,
		},
	}

	// Enqueue all tasks
	for _, task := range tasks {
		if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
			t.Fatalf("Failed to enqueue task %s: %v", task.ID, err)
		}
	}

	// Get tasks ready for retry (no limit)
	readyTasks, err := s.GetTasksReadyForRetry(ctx, 0)
	if err != nil {
		t.Fatalf("GetTasksReadyForRetry failed: %v", err)
	}

	// Should return task1 and task4 (both retrying and past NextRetryAt)
	if len(readyTasks) != 2 {
		t.Errorf("Expected 2 ready tasks, got %d", len(readyTasks))
	}

	// Verify correct tasks returned
	taskIDs := make(map[string]bool)
	for _, task := range readyTasks {
		taskIDs[task.ID] = true
	}

	if !taskIDs["task1"] {
		t.Error("Expected task1 to be ready for retry")
	}
	if !taskIDs["task4"] {
		t.Error("Expected task4 to be ready for retry")
	}
	if taskIDs["task2"] {
		t.Error("task2 should not be ready (future retry time)")
	}
	if taskIDs["task3"] {
		t.Error("task3 should not be ready (not in retrying state)")
	}
}

func TestGetTasksReadyForRetryWithLimit(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	past := now.Add(-1 * time.Minute)

	// Create 5 tasks ready for retry
	for i := 0; i < 5; i++ {
		task := &storage.QueuedTask{
			ID:          fmt.Sprintf("task%d", i),
			SessionID:   "session1",
			State:       storage.TaskStateRetrying,
			NextRetryAt: &past,
			RetryCount:  1,
			MaxRetries:  3,
			CreatedAt:   now,
		}
		if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
			t.Fatalf("Failed to enqueue task: %v", err)
		}
	}

	// Get with limit of 3
	readyTasks, err := s.GetTasksReadyForRetry(ctx, 3)
	if err != nil {
		t.Fatalf("GetTasksReadyForRetry failed: %v", err)
	}

	if len(readyTasks) != 3 {
		t.Errorf("Expected 3 ready tasks (limit), got %d", len(readyTasks))
	}
}

func TestUpdateTaskForRetry(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	task := &storage.QueuedTask{
		ID:         "task1",
		SessionID:  "session1",
		State:      storage.TaskStateDispatched,
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  now,
	}

	if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	// Update task for retry
	nextRetryAt := now.Add(5 * time.Second)
	errorMsg := "Task failed: connection refused"

	err := s.UpdateTaskForRetry(ctx, task.ID, nextRetryAt, errorMsg)
	if err != nil {
		t.Fatalf("UpdateTaskForRetry failed: %v", err)
	}

	// Verify task was updated
	updatedTask, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if updatedTask.State != storage.TaskStateRetrying {
		t.Errorf("Expected state TaskStateRetrying, got %s", updatedTask.State)
	}

	if updatedTask.RetryCount != 1 {
		t.Errorf("Expected RetryCount=1, got %d", updatedTask.RetryCount)
	}

	if updatedTask.NextRetryAt == nil {
		t.Error("NextRetryAt should not be nil")
	} else if !updatedTask.NextRetryAt.Equal(nextRetryAt) {
		t.Errorf("Expected NextRetryAt=%v, got %v", nextRetryAt, updatedTask.NextRetryAt)
	}

	if updatedTask.Error != errorMsg {
		t.Errorf("Expected Error=%q, got %q", errorMsg, updatedTask.Error)
	}
}

func TestUpdateTaskForRetryMaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	task := &storage.QueuedTask{
		ID:         "task1",
		SessionID:  "session1",
		State:      storage.TaskStateDispatched,
		RetryCount: 3, // Already at max
		MaxRetries: 3,
		CreatedAt:  now,
	}

	if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	// Try to update task for retry (should fail)
	nextRetryAt := now.Add(5 * time.Second)
	err := s.UpdateTaskForRetry(ctx, task.ID, nextRetryAt, "error")

	if err == nil {
		t.Error("Expected error when max retries exceeded, got nil")
	}
}

func TestUpdateTaskForRetryNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	nextRetryAt := time.Now().Add(5 * time.Second)
	err := s.UpdateTaskForRetry(ctx, "nonexistent", nextRetryAt, "error")

	if err == nil {
		t.Error("Expected error for nonexistent task, got nil")
	}
}

func TestRequeueTaskForRetry(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	nextRetryAt := now.Add(5 * time.Second)
	task := &storage.QueuedTask{
		ID:          "task1",
		SessionID:   "session1",
		State:       storage.TaskStateRetrying,
		RetryCount:  1,
		MaxRetries:  3,
		NextRetryAt: &nextRetryAt,
		CreatedAt:   now,
	}

	if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	// Requeue task for retry
	err := s.RequeueTaskForRetry(ctx, task.ID)
	if err != nil {
		t.Fatalf("RequeueTaskForRetry failed: %v", err)
	}

	// Verify task was requeued
	requeuedTask, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if requeuedTask.State != storage.TaskStateQueued {
		t.Errorf("Expected state TaskStateQueued, got %s", requeuedTask.State)
	}

	if requeuedTask.NextRetryAt != nil {
		t.Error("NextRetryAt should be nil after requeue")
	}

	// Verify task is back in session queue
	queuedTasks, err := s.GetQueuedTasksForSession(ctx, task.SessionID)
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}

	found := false
	for _, qt := range queuedTasks {
		if qt.ID == task.ID {
			found = true
			break
		}
	}

	if !found {
		t.Error("Task should be in session queue after requeue")
	}
}

func TestRequeueTaskForRetryWrongState(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	now := time.Now()
	task := &storage.QueuedTask{
		ID:         "task1",
		SessionID:  "session1",
		State:      storage.TaskStateQueued, // Not in retrying state
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  now,
	}

	if err := s.Enqueue(ctx, task.SessionID, task); err != nil {
		t.Fatalf("Failed to enqueue task: %v", err)
	}

	// Try to requeue task (should fail - not in retrying state)
	err := s.RequeueTaskForRetry(ctx, task.ID)

	if err == nil {
		t.Error("Expected error for task not in retrying state, got nil")
	}
}

func TestRequeueTaskForRetryNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryTaskQueueStorage(100, 10)

	err := s.RequeueTaskForRetry(ctx, "nonexistent")

	if err == nil {
		t.Error("Expected error for nonexistent task, got nil")
	}
}
