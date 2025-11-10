package coordinator

import (
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestEchoParser_ParseResult_Success(t *testing.T) {
	parser := &EchoParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_Echo{
				Echo: &protov1.EchoResponse{
					Message: "Hello, World!",
				},
			},
		},
	}

	message, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if message != "Hello, World!" {
		t.Errorf("Expected message 'Hello, World!', got '%s'", message)
	}
}

func TestEchoParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &EchoParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
	expectedError := "echo response missing typed response"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestEchoParser_ParseResult_InvalidType(t *testing.T) {
	parser := &EchoParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_FsList{
				FsList: &protov1.FsListResponse{
					Files: []*protov1.FileInfo{},
				},
			},
		},
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for invalid type, got nil")
	}
	expectedError := "echo response has invalid type"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestEchoParser_ParseResult_EmptyMessage(t *testing.T) {
	parser := &EchoParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_Echo{
				Echo: &protov1.EchoResponse{
					Message: "",
				},
			},
		},
	}

	message, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if message != "" {
		t.Errorf("Expected empty message, got '%s'", message)
	}
}

func TestEchoParser_ParseResult_LongMessage(t *testing.T) {
	parser := &EchoParser{}

	longMessage := "This is a very long message that contains multiple sentences and special characters like !@#$%^&*() and unicode 世界 🌍"

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_Echo{
				Echo: &protov1.EchoResponse{
					Message: longMessage,
				},
			},
		},
	}

	message, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if message != longMessage {
		t.Errorf("Expected message to match, got different value")
	}
}
