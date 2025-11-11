package tools

import (
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestEchoParser_ParseResult(t *testing.T) {
	parser := &EchoParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		want    string
		wantErr bool
	}{
		{
			name: "valid echo response",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{
							Message: "Hello, World!",
						},
					},
				},
			},
			want:    "Hello, World!",
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_FsRead{
						FsRead: &protov1.FsReadResponse{Contents: "test"},
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFsReadParser_ParseResult(t *testing.T) {
	parser := &FsReadParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		want    string
		wantErr bool
	}{
		{
			name: "valid fs.read response",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_FsRead{
						FsRead: &protov1.FsReadResponse{
							Contents: "file contents here",
						},
					},
				},
			},
			want:    "file contents here",
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{Message: "test"},
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFsWriteParser_ParseResult(t *testing.T) {
	parser := &FsWriteParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		want    string
		wantErr bool
	}{
		{
			name: "valid fs.write response",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_FsWrite{
						FsWrite: &protov1.FsWriteResponse{
							Path:         "/test/file.txt",
							BytesWritten: 123,
						},
					},
				},
			},
			want:    "Wrote 123 bytes to /test/file.txt",
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{Message: "test"},
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFsListParser_ParseResult(t *testing.T) {
	parser := &FsListParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		wantErr bool
	}{
		{
			name: "valid fs.list response with files and dirs",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_FsList{
						FsList: &protov1.FsListResponse{
							Files: []*protov1.FileInfo{
								{
									Name:        "file1.txt",
									IsDirectory: false,
									SizeBytes:   100,
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
			},
			wantErr: false,
		},
		{
			name: "empty file list",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_FsList{
						FsList: &protov1.FsListResponse{
							Files: []*protov1.FileInfo{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{Message: "test"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Verify output contains expected file info
				if tt.result.Response != nil && tt.result.Response.GetFsList() != nil {
					for _, file := range tt.result.Response.GetFsList().Files {
						if !strings.Contains(got, file.Name) {
							t.Errorf("ParseResult() output doesn't contain file name %s", file.Name)
						}
					}
				}
			}
		})
	}
}

func TestRunPythonParser_ParseResult(t *testing.T) {
	parser := &RunPythonParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		wantErr bool
	}{
		{
			name: "valid run_python response with stdout and stderr",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_RunPython{
						RunPython: &protov1.RunPythonResponse{
							ExitCode: 0,
							Stdout:   "Success output",
							Stderr:   "Warning message",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error exit code",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_RunPython{
						RunPython: &protov1.RunPythonResponse{
							ExitCode: 1,
							Stdout:   "",
							Stderr:   "Error occurred",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{Message: "test"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Verify output contains exit code
				if !strings.Contains(got, "Exit Code:") {
					t.Errorf("ParseResult() output doesn't contain exit code")
				}
				// Check if stdout/stderr present in response are in output
				resp := tt.result.Response.GetRunPython()
				if resp.Stdout != "" && !strings.Contains(got, resp.Stdout) {
					t.Errorf("ParseResult() output doesn't contain stdout")
				}
				if resp.Stderr != "" && !strings.Contains(got, resp.Stderr) {
					t.Errorf("ParseResult() output doesn't contain stderr")
				}
			}
		})
	}
}

func TestPkgInstallParser_ParseResult(t *testing.T) {
	parser := &PkgInstallParser{}

	tests := []struct {
		name    string
		result  *protov1.TaskStreamResult
		wantErr bool
	}{
		{
			name: "valid pkg.install response with success",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_PkgInstall{
						PkgInstall: &protov1.PkgInstallResponse{
							Installed: []*protov1.PackageInfo{
								{Name: "requests", Version: "2.28.0"},
								{Name: "numpy", Version: "1.23.0"},
							},
							Failed: []string{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "with failed packages",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_PkgInstall{
						PkgInstall: &protov1.PkgInstallResponse{
							Installed: []*protov1.PackageInfo{
								{Name: "requests", Version: "2.28.0"},
							},
							Failed: []string{"invalid-package", "another-bad-one"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "nil response",
			result: &protov1.TaskStreamResult{
				Response: nil,
			},
			wantErr: true,
		},
		{
			name: "wrong response type",
			result: &protov1.TaskStreamResult{
				Response: &protov1.ToolResponse{
					Response: &protov1.ToolResponse_Echo{
						Echo: &protov1.EchoResponse{Message: "test"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				resp := tt.result.Response.GetPkgInstall()
				// Verify installed packages are in output
				for _, pkg := range resp.Installed {
					if !strings.Contains(got, pkg.Name) {
						t.Errorf("ParseResult() output doesn't contain package %s", pkg.Name)
					}
				}
				// Verify failed packages are in output if present
				if len(resp.Failed) > 0 && !strings.Contains(got, "Failed:") {
					t.Errorf("ParseResult() output doesn't contain failed packages")
				}
			}
		})
	}
}
