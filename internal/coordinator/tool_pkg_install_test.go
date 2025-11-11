package coordinator

import (
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestPkgInstallParser_ParseResult_Success(t *testing.T) {
	parser := &PkgInstallParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_PkgInstall{
				PkgInstall: &protov1.PkgInstallResponse{
					Installed: []*protov1.PackageInfo{
						{
							Name:     "requests",
							Version:  "2.28.0",
							Location: "/usr/local/lib/python3.9/site-packages",
						},
						{
							Name:     "numpy",
							Version:  "1.24.0",
							Location: "/usr/local/lib/python3.9/site-packages",
						},
					},
					Failed:   []string{},
					Output:   "Successfully installed requests-2.28.0 numpy-1.24.0",
					ExitCode: 0,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "requests") {
		t.Error("Expected output to contain 'requests'")
	}
	if !strings.Contains(output, "2.28.0") {
		t.Error("Expected output to contain version")
	}
	if !strings.Contains(output, "numpy") {
		t.Error("Expected output to contain 'numpy'")
	}
}

func TestPkgInstallParser_ParseResult_WithFailures(t *testing.T) {
	parser := &PkgInstallParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_PkgInstall{
				PkgInstall: &protov1.PkgInstallResponse{
					Installed: []*protov1.PackageInfo{
						{
							Name:    "requests",
							Version: "2.28.0",
						},
					},
					Failed:   []string{"invalid-package", "another-bad-pkg"},
					Output:   "Partially successful",
					ExitCode: 1,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "requests") {
		t.Error("Expected output to contain successful package")
	}
	if !strings.Contains(output, "invalid-package") {
		t.Error("Expected output to contain failed package")
	}
}

func TestPkgInstallParser_ParseResult_AllFailed(t *testing.T) {
	parser := &PkgInstallParser{}

	result := &protov1.TaskStreamResult{
		Response: &protov1.ToolResponse{
			Response: &protov1.ToolResponse_PkgInstall{
				PkgInstall: &protov1.PkgInstallResponse{
					Installed: []*protov1.PackageInfo{},
					Failed:    []string{"bad-package"},
					Output:    "ERROR: Could not find a version",
					ExitCode:  1,
				},
			},
		},
	}

	output, err := parser.ParseResult(result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !strings.Contains(output, "bad-package") {
		t.Error("Expected output to contain failed package")
	}
}

func TestPkgInstallParser_ParseResult_MissingResponse(t *testing.T) {
	parser := &PkgInstallParser{}

	result := &protov1.TaskStreamResult{
		Response: nil,
	}

	_, err := parser.ParseResult(result)

	if err == nil {
		t.Fatal("Expected error for missing response, got nil")
	}
}

func TestPkgInstallParser_ParseResult_InvalidType(t *testing.T) {
	parser := &PkgInstallParser{}

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
