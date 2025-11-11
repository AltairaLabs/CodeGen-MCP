package config

import "testing"

func TestMessageConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"ErrSessionError", ErrSessionError, "session error: %v"},
		{"ErrNoWorkersAvail", ErrNoWorkersAvail, "no workers available to handle request"},
		{"MsgTaskQueued", MsgTaskQueued, "Task %s queued for execution"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.constant != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, test.constant)
			}
		})
	}
}