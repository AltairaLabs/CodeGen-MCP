package config

import "time"

// TaskQueueConfig holds configuration for TaskQueue operations
type TaskQueueConfig struct {
	// DispatchInterval is how often to check for ready tasks
	DispatchInterval time.Duration
	// MaxDispatchBatch is the maximum tasks to dispatch per cycle
	MaxDispatchBatch int
}

// DefaultTaskQueueConfig returns default configuration for TaskQueue
func DefaultTaskQueueConfig() TaskQueueConfig {
	return TaskQueueConfig{
		DispatchInterval: DefaultTaskQueueDispatchInterval,
		MaxDispatchBatch: DefaultMaxDispatchBatch,
	}
}

// CacheConfig holds configuration for result caching
type CacheConfig struct {
	// TTL is the time-to-live for cached results
	TTL time.Duration
}

// DefaultCacheConfig returns default configuration for caching
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL: DefaultCacheTTL,
	}
}

// RetryConfig holds configuration for retry scheduling
type RetryConfig struct {
	// CheckInterval is how often to check for retry-ready tasks
	CheckInterval time.Duration
	// BatchSize is the maximum tasks to requeue per check
	BatchSize int
}

// DefaultRetryConfig returns default configuration for retry operations
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		CheckInterval: DefaultRetryCheckInterval,
		BatchSize:     DefaultRetryBatchSize,
	}
}
