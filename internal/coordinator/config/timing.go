package config

import "time"

// Default timing configurations used throughout the coordinator
const (
	// DefaultCacheTTL is the default time-to-live for cached results
	DefaultCacheTTL = 5 * time.Minute

	// DefaultTaskQueueDispatchInterval is how often to check for ready tasks
	DefaultTaskQueueDispatchInterval = 100 * time.Millisecond

	// DefaultMaxDispatchBatch is the maximum tasks to dispatch per cycle
	DefaultMaxDispatchBatch = 10

	// DefaultHeartbeatInterval is the default worker heartbeat interval
	DefaultHeartbeatInterval = 30 * time.Second

	// DefaultWorkerStaleTimeout is how long before a worker is considered stale
	DefaultWorkerStaleTimeout = 5 * time.Minute

	// DefaultRetryCheckInterval is how often to check for retry-ready tasks
	DefaultRetryCheckInterval = 1 * time.Second

	// DefaultRetryBatchSize is the maximum tasks to requeue per check
	DefaultRetryBatchSize = 100
)
