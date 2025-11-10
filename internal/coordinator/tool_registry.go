package coordinator

import (
	"fmt"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// ToolResultParser knows how to parse worker responses for a specific tool
type ToolResultParser interface {
	// ParseResult extracts the output string from tool-specific response format
	ParseResult(result *protov1.TaskStreamResult) (output string, err error)
}

// ToolParserRegistry maps tool names to their result parsers
type ToolParserRegistry struct {
	parsers map[string]ToolResultParser
}

// NewToolParserRegistry creates a registry with all tool parsers
func NewToolParserRegistry() *ToolParserRegistry {
	return &ToolParserRegistry{
		parsers: map[string]ToolResultParser{
			toolEcho:       &EchoParser{},
			toolFsRead:     &FsReadParser{},
			toolFsWrite:    &FsWriteParser{},
			toolFsList:     &FsListParser{},
			toolRunPython:  &RunPythonParser{},
			toolPkgInstall: &PkgInstallParser{},
		},
	}
}

// GetParser returns the parser for a given tool name
func (r *ToolParserRegistry) GetParser(toolName string) (ToolResultParser, error) {
	parser, ok := r.parsers[toolName]
	if !ok {
		return nil, fmt.Errorf("no parser registered for tool: %s", toolName)
	}
	return parser, nil
}
