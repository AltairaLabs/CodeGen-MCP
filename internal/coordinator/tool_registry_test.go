package coordinator

import (
	"testing"
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
		toolEcho,
		toolFsRead,
		toolFsWrite,
		toolFsList,
		toolRunPython,
		toolPkgInstall,
	}

	for _, tool := range expectedTools {
		if _, ok := registry.parsers[tool]; !ok {
			t.Errorf("Expected parser for tool '%s' to be registered", tool)
		}
	}

	// Verify correct parser types
	if _, ok := registry.parsers[toolEcho].(*EchoParser); !ok {
		t.Error("Expected EchoParser for toolEcho")
	}
	if _, ok := registry.parsers[toolFsRead].(*FsReadParser); !ok {
		t.Error("Expected FsReadParser for toolFsRead")
	}
	if _, ok := registry.parsers[toolFsWrite].(*FsWriteParser); !ok {
		t.Error("Expected FsWriteParser for toolFsWrite")
	}
	if _, ok := registry.parsers[toolFsList].(*FsListParser); !ok {
		t.Error("Expected FsListParser for toolFsList")
	}
	if _, ok := registry.parsers[toolRunPython].(*RunPythonParser); !ok {
		t.Error("Expected RunPythonParser for toolRunPython")
	}
	if _, ok := registry.parsers[toolPkgInstall].(*PkgInstallParser); !ok {
		t.Error("Expected PkgInstallParser for toolPkgInstall")
	}
}

func TestGetParser_ValidTool(t *testing.T) {
	registry := NewToolParserRegistry()

	parser, err := registry.GetParser(toolEcho)

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
		{toolEcho, &EchoParser{}},
		{toolFsRead, &FsReadParser{}},
		{toolFsWrite, &FsWriteParser{}},
		{toolFsList, &FsListParser{}},
		{toolRunPython, &RunPythonParser{}},
		{toolPkgInstall, &PkgInstallParser{}},
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
