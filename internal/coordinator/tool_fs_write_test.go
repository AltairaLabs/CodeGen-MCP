package coordinator

import (
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestFsWriteParser_ParseResult_Success(t *testing.T) {
	parser := &FsWriteParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsWrite{
				FsWrite: &protov1.FsWriteResponse{
					Path:         "test.txt",
					BytesWritten: 42,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "42 bytes") {
		t.Errorf("Expected output to contain '42 bytes', got '%s'", output)
	}
	if !strings.Contains(output, "test.txt") {
		t.Errorf("Expected output to contain 'test.txt', got '%s'", output)
	}
}

func TestFsWriteParser_ParseResult_ZeroBytes(t *testing.T) {
	parser := &FsWriteParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsWrite{
				FsWrite: &protov1.FsWriteResponse{
					Path:         "empty.txt",
					BytesWritten: 0,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "0 bytes") {
		t.Errorf("Expected output to contain '0 bytes', got '%s'", output)
	}
}

func TestFsWriteParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &FsWriteParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
}

func TestFsWriteParser_ParseResult_InvalidType(t *testing.T) {
	parser := &FsWriteParser{}

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
