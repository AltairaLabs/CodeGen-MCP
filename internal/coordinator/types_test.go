package coordinator

import (
	"testing"
)

// Test utility functions

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"exact match", "timeout", "timeout", true},
		{"substring in middle", "connection timeout error", "timeout", true},
		{"substring at start", "timeout occurred", "timeout", true},
		{"substring at end", "connection timeout", "timeout", true},
		{"not found", "hello world", "timeout", false},
		{"empty substring", "hello", "", true},
		{"empty string", "", "timeout", false},
		{"case sensitive", "Timeout", "timeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestFindInString(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"found in middle", "hello world", "wor", true},
		{"found at start", "hello world", "hel", true},
		{"found at end", "hello world", "rld", true},
		{"not found", "hello world", "xyz", false},
		{"empty substring", "hello", "", true},
		{"substring longer than string", "hi", "hello", false},
		{"exact match", "test", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findInString(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("findInString(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"connection refused", "connection refused by server", true},
		{"connection reset", "connection reset by peer", true},
		{"timeout", "operation timeout", true},
		{"EOF", "unexpected EOF", true},
		{"broken pipe", "write: broken pipe", true},
		{"network unreachable", "network is unreachable", true},
		{"not network error", "invalid argument", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isNetworkError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestIsWorkerUnavailableError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"worker not found", "worker not found: worker-123", true},
		{"worker unavailable", "worker unavailable due to maintenance", true},
		{"no workers available", "no workers available to handle request", true},
		{"not worker error", "connection timeout", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkerUnavailableError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isWorkerUnavailableError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"validation failed", "validation failed: missing field", true},
		{"invalid argument", "invalid argument provided", true},
		{"missing required field", "missing required field: name", true},
		{"metadata validation", "metadata validation failed", true},
		{"permission denied", "permission denied", true},
		{"not found", "resource not found", true},
		{"unauthorized", "unauthorized access", true},
		{"forbidden", "forbidden action", true},
		{"network error", "connection timeout", false},
		{"transient error", "temporary failure", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPermanentError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isPermanentError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}
