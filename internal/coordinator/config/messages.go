package config

// Error messages used throughout the coordinator
const (
	// ErrSessionError is the format string for session errors
	ErrSessionError = "session error: %v"
	// ErrNoWorkersAvail indicates no workers are available to handle requests
	ErrNoWorkersAvail = "no workers available to handle request"
	// MsgTaskQueued is the format string for task queued messages
	MsgTaskQueued = "Task %s queued for execution"
)
