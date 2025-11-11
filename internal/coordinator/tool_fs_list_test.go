package coordinator

import (
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestFsListParser_ParseResult_Success(t *testing.T) {
	parser := &FsListParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsList{
				FsList: &protov1.FsListResponse{
					Files: []*protov1.FileInfo{
						{
							Name:        "file1.txt",
							IsDirectory: false,
							SizeBytes:   1024,
						},
						{
							Name:        "dir1",
							IsDirectory: true,
							SizeBytes:   0,
						},
					},
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !strings.Contains(output, "file1.txt") {
		t.Error("Expected output to contain 'file1.txt'")
	}
	if !strings.Contains(output, "dir1") {
		t.Error("Expected output to contain 'dir1'")
	}
	if !strings.Contains(output, "(file, 1024 bytes)") {
		t.Error("Expected output to contain file size")
	}
	if !strings.Contains(output, "(dir, 0 bytes)") {
		t.Error("Expected output to contain directory marker")
	}
}

func TestFsListParser_ParseResult_EmptyList(t *testing.T) {
	parser := &FsListParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsList{
				FsList: &protov1.FsListResponse{
					Files: []*protov1.FileInfo{},
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output for empty file list, got '%s'", output)
	}
}

func TestFsListParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &FsListParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
}

func TestFsListParser_ParseResult_InvalidType(t *testing.T) {
	parser := &FsListParser{}

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
