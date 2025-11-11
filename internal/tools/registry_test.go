package tools

import (
	"testing"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
)

func TestNewToolParserRegistry(t *testing.T) {
	registry := NewToolParserRegistry()

	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}

	if registry.parsers == nil {
		t.Fatal("Expected parsers map to be initialized")
	}

	// Verify all expected parsers are registered
	expectedTools := []string{
		config.ToolEcho,
		config.ToolFsRead,
		config.ToolFsWrite,
		config.ToolFsList,
		config.ToolRunPython,
		config.ToolPkgInstall,
	}

	for _, tool := range expectedTools {
		if _, ok := registry.parsers[tool]; !ok {
			t.Errorf("Expected parser for tool '%s' to be registered", tool)
		}
	}

	// Verify correct parser types
	if _, ok := registry.parsers[config.ToolEcho].(*EchoParser); !ok {
		t.Error("Expected EchoParser for config.ToolEcho")
	}
	if _, ok := registry.parsers[config.ToolFsRead].(*FsReadParser); !ok {
		t.Error("Expected FsReadParser for config.ToolFsRead")
	}
	if _, ok := registry.parsers[config.ToolFsWrite].(*FsWriteParser); !ok {
		t.Error("Expected FsWriteParser for config.ToolFsWrite")
	}
	if _, ok := registry.parsers[config.ToolFsList].(*FsListParser); !ok {
		t.Error("Expected FsListParser for config.ToolFsList")
	}
	if _, ok := registry.parsers[config.ToolRunPython].(*RunPythonParser); !ok {
		t.Error("Expected RunPythonParser for config.ToolRunPython")
	}
	if _, ok := registry.parsers[config.ToolPkgInstall].(*PkgInstallParser); !ok {
		t.Error("Expected PkgInstallParser for config.ToolPkgInstall")
	}
}

func TestGetParser_ValidTool(t *testing.T) {
	registry := NewToolParserRegistry()

	parser, err := registry.GetParser(config.ToolEcho)

	if err != nil {
		t.Errorf("Expected no error for valid tool, got %v", err)
	}
	if parser == nil {
		t.Fatal("Expected non-nil parser")
	}
	if _, ok := parser.(*EchoParser); !ok {
		t.Error("Expected EchoParser type")
	}
}

func TestGetParser_InvalidTool(t *testing.T) {
	registry := NewToolParserRegistry()

	parser, err := registry.GetParser("non_existent_tool")

	if err == nil {
		t.Fatal("Expected error for invalid tool, got nil")
	}
	if parser != nil {
		t.Error("Expected nil parser for invalid tool")
	}

	expectedError := "no parser registered for tool: non_existent_tool"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestGetParser_AllRegisteredTools(t *testing.T) {
	registry := NewToolParserRegistry()

	tools := []struct {
		name       string
		parserType interface{}
	}{
		{config.ToolEcho, &EchoParser{}},
		{config.ToolFsRead, &FsReadParser{}},
		{config.ToolFsWrite, &FsWriteParser{}},
		{config.ToolFsList, &FsListParser{}},
		{config.ToolRunPython, &RunPythonParser{}},
		{config.ToolPkgInstall, &PkgInstallParser{}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := registry.GetParser(tt.name)

			if err != nil {
				t.Errorf("Expected no error for tool '%s', got %v", tt.name, err)
			}
			if parser == nil {
				t.Errorf("Expected non-nil parser for tool '%s'", tt.name)
			}
		})
	}
}

func TestGetParser_EmptyString(t *testing.T) {
	registry := NewToolParserRegistry()

	parser, err := registry.GetParser("")

	if err == nil {
		t.Fatal("Expected error for empty tool name, got nil")
	}
	if parser != nil {
		t.Error("Expected nil parser for empty tool name")
	}
}

func TestGetParser_CaseSenitivity(t *testing.T) {
	registry := NewToolParserRegistry()

	// Tool names are case-sensitive
	parser, err := registry.GetParser("ECHO")

	if err == nil {
		t.Fatal("Expected error for wrong case, got nil")
	}
	if parser != nil {
		t.Error("Expected nil parser for wrong case")
	}
}

func TestToolParserRegistry_Immutability(t *testing.T) {
	registry1 := NewToolParserRegistry()
	registry2 := NewToolParserRegistry()

	// Each registry should have its own parsers map
	if &registry1.parsers == &registry2.parsers {
		t.Error("Expected different parser maps for different registries")
	}

	// But they should have the same keys
	if len(registry1.parsers) != len(registry2.parsers) {
		t.Errorf("Expected same number of parsers, got %d and %d",
			len(registry1.parsers), len(registry2.parsers))
	}
}
