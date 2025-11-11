package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

var (
	errTaskIDEmpty    = errors.New("task ID cannot be empty")
	errSessionIDEmpty = errors.New("session ID cannot be empty")
)

// InMemoryTaskQueueStorage implements TaskQueueStorage interface using in-memory maps
type InMemoryTaskQueueStorage struct {
	mu            sync.RWMutex
	tasks         map[string]*storage.QueuedTask // taskID -> task
	sessionQueues map[string][]string            // sessionID -> []taskID (FIFO order)
	maxQueueSize  int                            // Maximum total tasks across all sessions
	maxPerSession int                            // Maximum tasks per session
}

// NewInMemoryTaskQueueStorage creates a new in-memory task queue storage
func NewInMemoryTaskQueueStorage(maxQueueSize, maxPerSession int) *InMemoryTaskQueueStorage {
	if maxQueueSize <= 0 {
		maxQueueSize = 100000 // Default: 100k total tasks
	}
	if maxPerSession <= 0 {
		maxPerSession = 1000 // Default: 1k tasks per session
	}

	return &InMemoryTaskQueueStorage{
		tasks:         make(map[string]*storage.QueuedTask),
		sessionQueues: make(map[string][]string),
		maxQueueSize:  maxQueueSize,
		maxPerSession: maxPerSession,
	}
}

// Enqueue adds a task to the queue for a session
func (s *InMemoryTaskQueueStorage) Enqueue(ctx context.Context, sessionID string, task *storage.QueuedTask) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if task.ID == "" {
		return errTaskIDEmpty
	}
	if sessionID == "" {
		return errSessionIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if task already exists
	if _, exists := s.tasks[task.ID]; exists {
		return fmt.Errorf("task with ID %s already exists", task.ID)
	}

	// Check total queue size limit
	if len(s.tasks) >= s.maxQueueSize {
		return fmt.Errorf("queue full: %d tasks (max %d)", len(s.tasks), s.maxQueueSize)
	}

	// Check per-session queue size limit
	sessionQueue := s.sessionQueues[sessionID]
	if len(sessionQueue) >= s.maxPerSession {
		return fmt.Errorf("session queue full: %d tasks for session %s (max %d)",
			len(sessionQueue), sessionID, s.maxPerSession)
	}

	// Add task to storage
	s.tasks[task.ID] = task

	// Add task ID to session queue (FIFO order)
	s.sessionQueues[sessionID] = append(s.sessionQueues[sessionID], task.ID)

	return nil
}

// Dequeue returns up to 'limit' ready tasks from a session's queue
// (0 = all ready tasks)
func (s *InMemoryTaskQueueStorage) Dequeue(
	ctx context.Context,
	sessionID string,
	limit int,
) ([]*storage.QueuedTask, error) {
	if sessionID == "" {
		return nil, errSessionIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionQueue, exists := s.sessionQueues[sessionID]
	if !exists || len(sessionQueue) == 0 {
		return []*storage.QueuedTask{}, nil
	}

	// Determine how many tasks to return
	count := s.determineDequeueCount(sessionQueue, limit)

	// Collect ready tasks
	result, tasksToRemove := s.collectReadyTasks(sessionQueue, count)

	// Update session queue
	if len(tasksToRemove) > 0 {
		s.removeTasksFromQueue(sessionID, sessionQueue, tasksToRemove)
	}

	return result, nil
}

func (s *InMemoryTaskQueueStorage) determineDequeueCount(sessionQueue []string, limit int) int {
	count := len(sessionQueue)
	if limit > 0 && limit < count {
		count = limit
	}
	return count
}

func (s *InMemoryTaskQueueStorage) collectReadyTasks(
	sessionQueue []string,
	count int,
) (result []*storage.QueuedTask, tasksToRemove []string) {
	result = make([]*storage.QueuedTask, 0, count)
	tasksToRemove = make([]string, 0, count)

	for i := 0; i < count; i++ {
		taskID := sessionQueue[i]
		task, exists := s.tasks[taskID]
		if !exists {
			continue
		}

		if task.State == storage.TaskStateQueued {
			result = append(result, task)
			tasksToRemove = append(tasksToRemove, taskID)
		}
	}

	return result, tasksToRemove
}

func (s *InMemoryTaskQueueStorage) removeTasksFromQueue(
	sessionID string,
	sessionQueue, tasksToRemove []string,
) {
	removeSet := make(map[string]bool, len(tasksToRemove))
	for _, id := range tasksToRemove {
		removeSet[id] = true
	}

	newQueue := make([]string, 0, len(sessionQueue)-len(tasksToRemove))
	for _, id := range sessionQueue {
		if !removeSet[id] {
			newQueue = append(newQueue, id)
		}
	}
	s.sessionQueues[sessionID] = newQueue
}

// UpdateTaskState updates the state of a task
func (s *InMemoryTaskQueueStorage) UpdateTaskState(ctx context.Context, taskID string, state storage.TaskState) error {
	if taskID == "" {
		return errTaskIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.State = state

	// Update timestamps based on state
	now := time.Now()
	switch state {
	case storage.TaskStateDispatched:
		if task.DispatchedAt == nil {
			task.DispatchedAt = &now
		}
	case storage.TaskStateCompleted, storage.TaskStateFailed, storage.TaskStateTimeout:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
	case storage.TaskStateQueued, storage.TaskStateRetrying:
		// No timestamp updates for these states
	}

	return nil
}

// GetTask retrieves a specific task by ID
func (s *InMemoryTaskQueueStorage) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	if taskID == "" {
		return nil, errTaskIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, nil // Not found, not an error
	}

	// Return a copy to avoid external mutations
	taskCopy := *task
	return &taskCopy, nil
}

// PurgeSessionQueue removes all tasks for a session
func (s *InMemoryTaskQueueStorage) PurgeSessionQueue(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, errSessionIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionQueue, exists := s.sessionQueues[sessionID]
	if !exists {
		return 0, nil // No tasks to purge
	}

	// Remove all tasks for this session
	count := 0
	for _, taskID := range sessionQueue {
		if _, exists := s.tasks[taskID]; exists {
			delete(s.tasks, taskID)
			count++
		}
	}

	// Clear the session queue
	delete(s.sessionQueues, sessionID)

	return count, nil
}

// GetQueueStats returns statistics about queued tasks
func (s *InMemoryTaskQueueStorage) GetQueueStats(ctx context.Context) (*storage.QueueStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &storage.QueueStats{
		QueuedBySession: make(map[string]int),
	}

	var oldestTime *time.Time

	// Count tasks by state and session
	for _, task := range s.tasks {
		switch task.State {
		case storage.TaskStateQueued:
			stats.TotalQueued++
			stats.QueuedBySession[task.SessionID]++

			// Track oldest task
			if oldestTime == nil || task.CreatedAt.Before(*oldestTime) {
				oldestTime = &task.CreatedAt
			}
		case storage.TaskStateDispatched:
			stats.TotalDispatched++
		case storage.TaskStateCompleted, storage.TaskStateFailed,
			storage.TaskStateTimeout, storage.TaskStateRetrying:
			// These states are not counted in stats
		}
	}

	// Calculate oldest task age
	if oldestTime != nil {
		stats.OldestTaskAge = time.Since(*oldestTime)
	}

	return stats, nil
}

// GetQueuedTasksForSession retrieves all queued tasks for a session
func (s *InMemoryTaskQueueStorage) GetQueuedTasksForSession(
	ctx context.Context,
	sessionID string,
) ([]*storage.QueuedTask, error) {
	if sessionID == "" {
		return nil, errSessionIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionQueue, exists := s.sessionQueues[sessionID]
	if !exists {
		return []*storage.QueuedTask{}, nil
	}

	// Collect all tasks for this session
	result := make([]*storage.QueuedTask, 0, len(sessionQueue))
	for _, taskID := range sessionQueue {
		task, exists := s.tasks[taskID]
		if exists {
			// Return a copy to avoid external mutations
			taskCopy := *task
			result = append(result, &taskCopy)
		}
	}

	return result, nil
}

// GetTasksReadyForRetry retrieves tasks that are ready to be retried
func (s *InMemoryTaskQueueStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*storage.QueuedTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]*storage.QueuedTask, 0)

	// Scan all tasks for retry-ready tasks
	for _, task := range s.tasks {
		if task.State == storage.TaskStateRetrying && task.NextRetryAt != nil && !task.NextRetryAt.After(now) {
			// Return a copy to avoid external mutations
			taskCopy := *task
			result = append(result, &taskCopy)

			// Check limit
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// UpdateTaskForRetry updates a task to prepare it for retry
func (s *InMemoryTaskQueueStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	if taskID == "" {
		return errTaskIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Check if max retries exceeded
	if task.RetryCount >= task.MaxRetries {
		return fmt.Errorf("max retries (%d) exceeded for task %s", task.MaxRetries, taskID)
	}

	// Update retry fields
	task.RetryCount++
	task.NextRetryAt = &nextRetryAt
	task.State = storage.TaskStateRetrying
	task.Error = errorMsg

	return nil
}

// RequeueTaskForRetry moves a task from retrying state back to queued
func (s *InMemoryTaskQueueStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errTaskIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.State != storage.TaskStateRetrying {
		return fmt.Errorf("task %s is not in retrying state (current: %s)", taskID, task.State)
	}

	// Move back to queued state
	task.State = storage.TaskStateQueued
	task.NextRetryAt = nil

	// Add task back to session queue if not already present
	sessionQueue := s.sessionQueues[task.SessionID]
	taskExists := false
	for _, id := range sessionQueue {
		if id == taskID {
			taskExists = true
			break
		}
	}

	if !taskExists {
		s.sessionQueues[task.SessionID] = append(s.sessionQueues[task.SessionID], taskID)
	}

	return nil
}
