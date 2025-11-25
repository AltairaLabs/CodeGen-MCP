package config

import "testing"

func TestAllTools(t *testing.T) {
	tools := AllTools()
	expectedCount := 9
	if len(tools) != expectedCount {
		t.Errorf("Expected %d tools, got %d", expectedCount, len(tools))
	}

	expectedTools := map[string]bool{
		ToolEcho:          true,
		ToolFsRead:        true,
		ToolFsWrite:       true,
		ToolFsList:        true,
		ToolRunPython:     true,
		ToolPkgInstall:    true,
		ToolGetTaskResult: true,
		ToolGetTaskStatus: true,
		ToolArtifactGet:   true,
	}

	for _, tool := range tools {
		if !expectedTools[tool] {
			t.Errorf("Unexpected tool: %s", tool)
		}
		delete(expectedTools, tool)
	}

	if len(expectedTools) > 0 {
		t.Errorf("Missing tools: %v", expectedTools)
	}
}

func TestToolConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Echo", ToolEcho, "echo"},
		{"FsRead", ToolFsRead, "fs.read"},
		{"FsWrite", ToolFsWrite, "fs.write"},
		{"FsList", ToolFsList, "fs.list"},
		{"RunPython", ToolRunPython, "run.python"},
		{"PkgInstall", ToolPkgInstall, "pkg.install"},
		{"GetTaskResult", ToolGetTaskResult, "task.get_result"},
		{"GetTaskStatus", ToolGetTaskStatus, "task.get_status"},
		{"ArtifactGet", ToolArtifactGet, "artifact.get"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.constant != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, test.constant)
			}
		})
	}
}
