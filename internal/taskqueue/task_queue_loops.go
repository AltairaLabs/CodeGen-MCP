package taskqueue

import (
	"time"
)

// This file contains background goroutine loop wrappers that are untestable in unit tests
// as they run indefinitely. The logic inside the loops (dispatchReadyTasks, cleanupExpiredResults)
// is testable and should be tested separately.

// dispatchLoop runs in a goroutine to periodically dispatch ready tasks
func (tq *TaskQueue) dispatchLoop() {
	defer tq.wg.Done()

	ticker := time.NewTicker(tq.dispatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tq.dispatchReadyTasks()
		case <-tq.ctx.Done():
			return
		}
	}
}

// resultTimeoutLoop monitors for tasks that have been waiting too long
func (tq *TaskQueue) resultTimeoutLoop() {
	defer tq.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tq.cleanupExpiredResults()
		case <-tq.ctx.Done():
			return
		}
	}
}
