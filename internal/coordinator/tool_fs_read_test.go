package coordinator

import (
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestFsReadParser_ParseResult_Success(t *testing.T) {
	parser := &FsReadParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsRead{
				FsRead: &protov1.FsReadResponse{
					Contents:  "Hello, World!\nThis is file content.",
					SizeBytes: 35,
					Encoding:  "utf-8",
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "Hello, World!\nThis is file content." {
		t.Errorf("Expected file contents, got '%s'", output)
	}
}

func TestFsReadParser_ParseResult_EmptyFile(t *testing.T) {
	parser := &FsReadParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsRead{
				FsRead: &protov1.FsReadResponse{
					Contents:  "",
					SizeBytes: 0,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output, got '%s'", output)
	}
}

func TestFsReadParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &FsReadParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
}

func TestFsReadParser_ParseResult_InvalidType(t *testing.T) {
	parser := &FsReadParser{}

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
