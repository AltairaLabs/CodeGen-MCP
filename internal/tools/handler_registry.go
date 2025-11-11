package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolHandlerFunc is a function that handles a tool call
type ToolHandlerFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolHandlerRegistry maps tool names to handler functions
type ToolHandlerRegistry struct {
	handlers map[string]ToolHandlerFunc
}

// NewToolHandlerRegistry creates an empty registry
func NewToolHandlerRegistry(initial map[string]ToolHandlerFunc) *ToolHandlerRegistry {
	r := &ToolHandlerRegistry{
		handlers: make(map[string]ToolHandlerFunc),
	}
	for k, v := range initial {
		r.handlers[k] = v
	}
	return r
}

// Register adds or replaces a handler for a tool name
func (r *ToolHandlerRegistry) Register(toolName string, handler ToolHandlerFunc) {
	r.handlers[toolName] = handler
}

// GetHandler returns the handler function for a given tool name
func (r *ToolHandlerRegistry) GetHandler(toolName string) (ToolHandlerFunc, error) {
	h, ok := r.handlers[toolName]
	if !ok {
		return nil, fmt.Errorf("no handler registered for tool: %s", toolName)
	}
	return h, nil
}

// GetAllHandlers returns a shallow copy slice of handlers
func (r *ToolHandlerRegistry) GetAllHandlers() map[string]ToolHandlerFunc {
	out := make(map[string]ToolHandlerFunc, len(r.handlers))
	for k, v := range r.handlers {
		out[k] = v
	}
	return out
}
