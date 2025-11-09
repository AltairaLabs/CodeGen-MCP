package memory

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

func TestNewInMemoryTaskQueueStorage(t *testing.T) {
	tests := []struct {
		name               string
		maxQueueSize       int
		maxPerSession      int
		expectedQueueSize  int
		expectedPerSession int
	}{
		{
			name:               "default values",
			maxQueueSize:       0,
			maxPerSession:      0,
			expectedQueueSize:  100000,
			expectedPerSession: 1000,
		},
		{
			name:               "custom values",
			maxQueueSize:       5000,
			maxPerSession:      100,
			expectedQueueSize:  5000,
			expectedPerSession: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewInMemoryTaskQueueStorage(tt.maxQueueSize, tt.maxPerSession)
			if s == nil {
				t.Fatal("expected non-nil storage")
			}
			if s.maxQueueSize != tt.expectedQueueSize {
				t.Errorf("maxQueueSize = %d, want %d", s.maxQueueSize, tt.expectedQueueSize)
			}
			if s.maxPerSession != tt.expectedPerSession {
				t.Errorf("maxPerSession = %d, want %d", s.maxPerSession, tt.expectedPerSession)
			}
		})
	}
}

func TestEnqueue(t *testing.T) {
	tests := []struct {
		name      string
		task      *storage.QueuedTask
		sessionID string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "nil task",
			task:      nil,
			sessionID: "session1",
			wantErr:   true,
			errMsg:    "task cannot be nil",
		},
		{
			name: "empty task ID",
			task: &storage.QueuedTask{
				ID: "",
			},
			sessionID: "session1",
			wantErr:   true,
			errMsg:    "task ID cannot be empty",
		},
		{
			name: "empty session ID",
			task: &storage.QueuedTask{
				ID: "task1",
			},
			sessionID: "",
			wantErr:   true,
			errMsg:    "session ID cannot be empty",
		},
		{
			name: "valid task",
			task: &storage.QueuedTask{
				ID:        "task1",
				SessionID: "session1",
				ToolName:  "read_file",
				Args:      coordinator.TaskArgs{"path": "/test"},
				State:     storage.TaskStateQueued,
				CreatedAt: time.Now(),
			},
			sessionID: "session1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewInMemoryTaskQueueStorage(100, 10)
			err := s.Enqueue(context.Background(), tt.sessionID, tt.task)

			if (err != nil) != tt.wantErr {
				t.Errorf("Enqueue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("Enqueue() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestEnqueueDuplicate(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	task := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "read_file",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}

	// First enqueue should succeed
	err := s.Enqueue(context.Background(), "session1", task)
	if err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}

	// Second enqueue should fail
	err = s.Enqueue(context.Background(), "session1", task)
	if err == nil {
		t.Fatal("Expected error for duplicate task, got nil")
	}
	if err.Error() != "task with ID task1 already exists" {
		t.Errorf("Unexpected error message: %v", err.Error())
	}
}

func TestEnqueueQueueFull(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(2, 10) // Max 2 tasks total

	// Enqueue first task
	task1 := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "read_file",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err := s.Enqueue(context.Background(), "session1", task1)
	if err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}

	// Enqueue second task
	task2 := &storage.QueuedTask{
		ID:        "task2",
		SessionID: "session2",
		ToolName:  "write_file",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err = s.Enqueue(context.Background(), "session2", task2)
	if err != nil {
		t.Fatalf("Second enqueue failed: %v", err)
	}

	// Third task should fail
	task3 := &storage.QueuedTask{
		ID:        "task3",
		SessionID: "session3",
		ToolName:  "list_dir",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err = s.Enqueue(context.Background(), "session3", task3)
	if err == nil {
		t.Fatal("Expected error for queue full, got nil")
	}
}

func TestEnqueueSessionQueueFull(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 2) // Max 2 tasks per session

	// Enqueue first task
	task1 := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "read_file",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err := s.Enqueue(context.Background(), "session1", task1)
	if err != nil {
		t.Fatalf("First enqueue failed: %v", err)
	}

	// Enqueue second task
	task2 := &storage.QueuedTask{
		ID:        "task2",
		SessionID: "session1",
		ToolName:  "write_file",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err = s.Enqueue(context.Background(), "session1", task2)
	if err != nil {
		t.Fatalf("Second enqueue failed: %v", err)
	}

	// Third task for same session should fail
	task3 := &storage.QueuedTask{
		ID:        "task3",
		SessionID: "session1",
		ToolName:  "list_dir",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	err = s.Enqueue(context.Background(), "session1", task3)
	if err == nil {
		t.Fatal("Expected error for session queue full, got nil")
	}
}

func TestDequeue(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	// Enqueue some tasks
	for i := 1; i <= 3; i++ {
		task := &storage.QueuedTask{
			ID:        string(rune('0' + i)),
			SessionID: "session1",
			ToolName:  "test_tool",
			State:     storage.TaskStateQueued,
			CreatedAt: time.Now(),
		}
		if err := s.Enqueue(context.Background(), "session1", task); err != nil {
			t.Fatalf("Enqueue task%d failed: %v", i, err)
		}
	}

	// Dequeue with limit
	tasks, err := s.Dequeue(context.Background(), "session1", 2)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("Dequeue returned %d tasks, want 2", len(tasks))
	}

	// Dequeue remaining
	tasks, err = s.Dequeue(context.Background(), "session1", 0)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("Dequeue returned %d tasks, want 1", len(tasks))
	}

	// Dequeue from empty queue
	tasks, err = s.Dequeue(context.Background(), "session1", 0)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("Dequeue returned %d tasks, want 0", len(tasks))
	}
}

func TestDequeueEmptySessionID(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	_, err := s.Dequeue(context.Background(), "", 0)
	if err == nil {
		t.Fatal("Expected error for empty session ID, got nil")
	}
}

func TestDequeueNonExistentSession(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	tasks, err := s.Dequeue(context.Background(), "nonexistent", 0)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(tasks))
	}
}

func TestUpdateTaskState(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	task := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}

	if err := s.Enqueue(context.Background(), "session1", task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Update to dispatched
	err := s.UpdateTaskState(context.Background(), "task1", storage.TaskStateDispatched)
	if err != nil {
		t.Fatalf("UpdateTaskState failed: %v", err)
	}

	// Verify state changed
	retrieved, err := s.GetTask(context.Background(), "task1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if retrieved.State != storage.TaskStateDispatched {
		t.Errorf("State = %v, want %v", retrieved.State, storage.TaskStateDispatched)
	}
	if retrieved.DispatchedAt == nil {
		t.Error("DispatchedAt should be set")
	}
}

func TestUpdateTaskStateNonExistent(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	err := s.UpdateTaskState(context.Background(), "nonexistent", storage.TaskStateCompleted)
	if err == nil {
		t.Fatal("Expected error for nonexistent task, got nil")
	}
}

func TestGetTask(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	original := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}

	if err := s.Enqueue(context.Background(), "session1", original); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Get task
	retrieved, err := s.GetTask(context.Background(), "task1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected task, got nil")
	}
	if retrieved.ID != original.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, original.ID)
	}
}

func TestGetTaskNonExistent(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	task, err := s.GetTask(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if task != nil {
		t.Errorf("Expected nil task, got %v", task)
	}
}

func TestPurgeSessionQueue(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	// Enqueue tasks for session1
	for i := 1; i <= 3; i++ {
		task := &storage.QueuedTask{
			ID:        string(rune('0' + i)),
			SessionID: "session1",
			ToolName:  "test_tool",
			State:     storage.TaskStateQueued,
			CreatedAt: time.Now(),
		}
		if err := s.Enqueue(context.Background(), "session1", task); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	// Enqueue task for session2
	task := &storage.QueuedTask{
		ID:        "task4",
		SessionID: "session2",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	if err := s.Enqueue(context.Background(), "session2", task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Purge session1
	count, err := s.PurgeSessionQueue(context.Background(), "session1")
	if err != nil {
		t.Fatalf("PurgeSessionQueue failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Purged %d tasks, want 3", count)
	}

	// Verify session1 tasks are gone
	tasks, err := s.GetQueuedTasksForSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks for session1, got %d", len(tasks))
	}

	// Verify session2 task still exists
	tasks, err = s.GetQueuedTasksForSession(context.Background(), "session2")
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task for session2, got %d", len(tasks))
	}
}

func TestPurgeNonExistentSession(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)
	count, err := s.PurgeSessionQueue(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("PurgeSessionQueue failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Purged %d tasks, want 0", count)
	}
}

func TestGetQueueStats(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	// Enqueue tasks
	task1 := &storage.QueuedTask{
		ID:        "task1",
		SessionID: "session1",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now().Add(-5 * time.Second),
	}
	task2 := &storage.QueuedTask{
		ID:        "task2",
		SessionID: "session1",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	task3 := &storage.QueuedTask{
		ID:        "task3",
		SessionID: "session2",
		ToolName:  "test_tool",
		State:     storage.TaskStateDispatched,
		CreatedAt: time.Now(),
	}

	if err := s.Enqueue(context.Background(), "session1", task1); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if err := s.Enqueue(context.Background(), "session1", task2); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if err := s.Enqueue(context.Background(), "session2", task3); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Get stats
	stats, err := s.GetQueueStats(context.Background())
	if err != nil {
		t.Fatalf("GetQueueStats failed: %v", err)
	}

	if stats.TotalQueued != 2 {
		t.Errorf("TotalQueued = %d, want 2", stats.TotalQueued)
	}
	if stats.TotalDispatched != 1 {
		t.Errorf("TotalDispatched = %d, want 1", stats.TotalDispatched)
	}
	if stats.QueuedBySession["session1"] != 2 {
		t.Errorf("QueuedBySession[session1] = %d, want 2", stats.QueuedBySession["session1"])
	}
	if stats.OldestTaskAge < 4*time.Second {
		t.Errorf("OldestTaskAge = %v, want >= 4s", stats.OldestTaskAge)
	}
}

func TestGetQueuedTasksForSession(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(100, 10)

	// Enqueue tasks for multiple sessions
	for i := 1; i <= 2; i++ {
		task := &storage.QueuedTask{
			ID:        string(rune('0' + i)),
			SessionID: "session1",
			ToolName:  "test_tool",
			State:     storage.TaskStateQueued,
			CreatedAt: time.Now(),
		}
		if err := s.Enqueue(context.Background(), "session1", task); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	task3 := &storage.QueuedTask{
		ID:        "task3",
		SessionID: "session2",
		ToolName:  "test_tool",
		State:     storage.TaskStateQueued,
		CreatedAt: time.Now(),
	}
	if err := s.Enqueue(context.Background(), "session2", task3); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Get tasks for session1
	tasks, err := s.GetQueuedTasksForSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks for session1, got %d", len(tasks))
	}

	// Get tasks for session2
	tasks, err = s.GetQueuedTasksForSession(context.Background(), "session2")
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task for session2, got %d", len(tasks))
	}
}

func TestConcurrentEnqueue(t *testing.T) {
	s := NewInMemoryTaskQueueStorage(1000, 100)

	const numGoroutines = 10
	const tasksPerGoroutine = 10
	done := make(chan bool, numGoroutines)

	// Concurrently enqueue tasks
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < tasksPerGoroutine; j++ {
				task := &storage.QueuedTask{
					ID:        string(rune('a' + goroutineID*tasksPerGoroutine + j)),
					SessionID: "session1",
					ToolName:  "test_tool",
					State:     storage.TaskStateQueued,
					CreatedAt: time.Now(),
				}
				_ = s.Enqueue(context.Background(), "session1", task)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify tasks were enqueued
	tasks, err := s.GetQueuedTasksForSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetQueuedTasksForSession failed: %v", err)
	}
	if len(tasks) != numGoroutines*tasksPerGoroutine {
		t.Errorf("Expected %d tasks, got %d", numGoroutines*tasksPerGoroutine, len(tasks))
	}
}
