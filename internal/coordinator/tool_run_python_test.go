package coordinator

import (
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestRunPythonParser_ParseResult_Success(t *testing.T) {
	parser := &RunPythonParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_RunPython{
				RunPython: &protov1.RunPythonResponse{
					Stdout:     "Hello from Python!",
					Stderr:     "",
					ExitCode:   0,
					DurationMs: 150,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "Hello from Python!" {
		t.Errorf("Expected stdout output, got '%s'", output)
	}
}

func TestRunPythonParser_ParseResult_WithStderr(t *testing.T) {
	parser := &RunPythonParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_RunPython{
				RunPython: &protov1.RunPythonResponse{
					Stdout:   "Normal output",
					Stderr:   "Warning message",
					ExitCode: 0,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "Normal output") {
		t.Error("Expected output to contain stdout")
	}
	if !strings.Contains(output, "Warning message") {
		t.Error("Expected output to contain stderr")
	}
}

func TestRunPythonParser_ParseResult_ErrorOnly(t *testing.T) {
	parser := &RunPythonParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_RunPython{
				RunPython: &protov1.RunPythonResponse{
					Stdout:   "",
					Stderr:   "Error: module not found",
					ExitCode: 1,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "Error: module not found") {
		t.Errorf("Expected error message in output, got '%s'", output)
	}
}

func TestRunPythonParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &RunPythonParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
}

func TestRunPythonParser_ParseResult_InvalidType(t *testing.T) {
	parser := &RunPythonParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_Echo{
				Echo: &protov1.EchoResponse{
					Message: "wrong type",
				},
			},
		},
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for invalid type, got nil")
	}
}
