package tools

import (
	"fmt"
	"strings"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// EchoParser handles echo tool response parsing
type EchoParser struct{}

func (p *EchoParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("echo response missing typed response")
	}

	echoResp := result.Response.GetEcho()
	if echoResp == nil {
		return "", fmt.Errorf("echo response has invalid type")
	}

	return echoResp.Message, nil
}

// FsReadParser handles fs_read tool response parsing
type FsReadParser struct{}

func (p *FsReadParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.read response missing typed response")
	}

	fsReadResp := result.Response.GetFsRead()
	if fsReadResp == nil {
		return "", fmt.Errorf("fs.read response has invalid type")
	}

	return fsReadResp.Contents, nil
}

// FsWriteParser handles fs_write tool response parsing
type FsWriteParser struct{}

func (p *FsWriteParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.write response missing typed response")
	}

	fsWriteResp := result.Response.GetFsWrite()
	if fsWriteResp == nil {
		return "", fmt.Errorf("fs.write response has invalid type")
	}

	return fmt.Sprintf("Wrote %d bytes to %s", fsWriteResp.BytesWritten, fsWriteResp.Path), nil
}

// FsListParser handles fs_list tool response parsing
type FsListParser struct{}

func (p *FsListParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("fs.list response missing typed response")
	}

	fsListResp := result.Response.GetFsList()
	if fsListResp == nil {
		return "", fmt.Errorf("fs.list response has invalid type")
	}

	// Format the file info array into a readable string
	var output string
	for _, fileInfo := range fsListResp.Files {
		fileType := "file"
		if fileInfo.IsDirectory {
			fileType = "dir"
		}
		output += fmt.Sprintf("%s (%s, %d bytes)\n", fileInfo.Name, fileType, fileInfo.SizeBytes)
	}
	return output, nil
}

// RunPythonParser handles run_python tool response parsing
type RunPythonParser struct{}

func (p *RunPythonParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("run_python response missing typed response")
	}

	runPythonResp := result.Response.GetRunPython()
	if runPythonResp == nil {
		return "", fmt.Errorf("run_python response has invalid type")
	}

	output := fmt.Sprintf("Exit Code: %d\n", runPythonResp.ExitCode)
	if runPythonResp.Stdout != "" {
		output += fmt.Sprintf("Stdout:\n%s\n", runPythonResp.Stdout)
	}
	if runPythonResp.Stderr != "" {
		output += fmt.Sprintf("Stderr:\n%s\n", runPythonResp.Stderr)
	}

	return output, nil
}

// PkgInstallParser handles pkg_install tool response parsing
type PkgInstallParser struct{}

func (p *PkgInstallParser) ParseResult(result *protov1.TaskStreamResult) (string, error) {
	if result.Response == nil {
		return "", fmt.Errorf("pkg.install response missing typed response")
	}

	pkgInstallResp := result.Response.GetPkgInstall()
	if pkgInstallResp == nil {
		return "", fmt.Errorf("pkg.install response has invalid type")
	}

	// Format the installed packages
	var output string
	for _, pkg := range pkgInstallResp.Installed {
		output += fmt.Sprintf("Installed: %s (version: %s)\n", pkg.Name, pkg.Version)
	}
	if len(pkgInstallResp.Failed) > 0 {
		output += fmt.Sprintf("Failed: %s\n", strings.Join(pkgInstallResp.Failed, ", "))
	}
	return output, nil
}
