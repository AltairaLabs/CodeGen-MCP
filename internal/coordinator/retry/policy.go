package retry

import (
	"errors"
	"math"
	"time"
)

// Policy defines retry behavior for tasks
type Policy struct {
	MaxRetries        int           // Maximum number of retry attempts (0 = no retries)
	InitialDelay      time.Duration // Initial delay before first retry
	MaxDelay          time.Duration // Maximum delay between retries
	BackoffMultiplier float64       // Multiplier for exponential backoff (e.g., 2.0)
}

// DefaultPolicy returns the default retry policy for tasks
func DefaultPolicy() Policy {
	return Policy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// NetworkErrorPolicy returns a retry policy for network errors (more retries)
func NetworkErrorPolicy() Policy {
	return Policy{
		MaxRetries:        5,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// QuickRetryPolicy returns a policy for tasks that should retry quickly with minimal backoff
func QuickRetryPolicy() Policy {
	return Policy{
		MaxRetries:        2,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 1.5,
	}
}

// CalculateDelay calculates the next retry delay based on the current attempt number
// Uses exponential backoff with jitter
func (p *Policy) CalculateDelay(retryCount int) time.Duration {
	if retryCount <= 0 {
		return p.InitialDelay
	}

	// Calculate exponential backoff: initialDelay * (multiplier ^ retryCount)
	delay := float64(p.InitialDelay) * math.Pow(p.BackoffMultiplier, float64(retryCount))

	// Cap at maximum delay
	if time.Duration(delay) > p.MaxDelay {
		return p.MaxDelay
	}

	return time.Duration(delay)
}

// ShouldRetry determines if a task should be retried based on the attempt count
func (p *Policy) ShouldRetry(retryCount int) bool {
	return retryCount < p.MaxRetries
}

// IsRetriableError determines if an error should trigger a retry
func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network and timeout errors are retriable
	retriableErrors := []string{
		"connection refused",
		"timeout",
		"network is unreachable",
		"temporary failure",
		"service unavailable",
		"too many requests",
		"context deadline exceeded",
	}

	for _, retriable := range retriableErrors {
		if errStr != "" && retriable != "" && errStr == retriable {
			return true
		}
	}

	return false
}

// Validate checks if the retry policy configuration is valid
func (p *Policy) Validate() error {
	if p.MaxRetries < 0 {
		return errors.New("MaxRetries must be non-negative")
	}
	if p.InitialDelay <= 0 {
		return errors.New("InitialDelay must be positive")
	}
	if p.MaxDelay <= 0 {
		return errors.New("MaxDelay must be positive")
	}
	if p.BackoffMultiplier <= 0 {
		return errors.New("BackoffMultiplier must be positive")
	}
	if p.InitialDelay > p.MaxDelay {
		return errors.New("InitialDelay cannot be greater than MaxDelay")
	}
	return nil
}
