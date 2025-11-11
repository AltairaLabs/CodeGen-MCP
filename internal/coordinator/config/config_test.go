package config

import (
	"testing"
	"time"
)

func TestDefaultTaskQueueConfig(t *testing.T) {
	config := DefaultTaskQueueConfig()
	
	if config.DispatchInterval != DefaultTaskQueueDispatchInterval {
		t.Errorf("Expected DispatchInterval %v, got %v", DefaultTaskQueueDispatchInterval, config.DispatchInterval)
	}
	
	if config.MaxDispatchBatch != DefaultMaxDispatchBatch {
		t.Errorf("Expected MaxDispatchBatch %d, got %d", DefaultMaxDispatchBatch, config.MaxDispatchBatch)
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()
	
	if config.TTL != DefaultCacheTTL {
		t.Errorf("Expected TTL %v, got %v", DefaultCacheTTL, config.TTL)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()
	
	if config.CheckInterval != DefaultRetryCheckInterval {
		t.Errorf("Expected CheckInterval %v, got %v", DefaultRetryCheckInterval, config.CheckInterval)
	}
	
	if config.BatchSize != DefaultRetryBatchSize {
		t.Errorf("Expected BatchSize %d, got %d", DefaultRetryBatchSize, config.BatchSize)
	}
}

func TestTimingConstants(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"DefaultCacheTTL", DefaultCacheTTL, 5 * time.Minute},
		{"DefaultTaskQueueDispatchInterval", DefaultTaskQueueDispatchInterval, 100 * time.Millisecond},
		{"DefaultHeartbeatInterval", DefaultHeartbeatInterval, 30 * time.Second},
		{"DefaultWorkerStaleTimeout", DefaultWorkerStaleTimeout, 5 * time.Minute},
		{"DefaultRetryCheckInterval", DefaultRetryCheckInterval, 1 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.duration != test.expected {
				t.Errorf("Expected %v, got %v", test.expected, test.duration)
			}
		})
	}
}

func TestIntegerConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		expected int
	}{
		{"DefaultMaxDispatchBatch", DefaultMaxDispatchBatch, 10},
		{"DefaultRetryBatchSize", DefaultRetryBatchSize, 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.value != test.expected {
				t.Errorf("Expected %d, got %d", test.expected, test.value)
			}
		})
	}
}