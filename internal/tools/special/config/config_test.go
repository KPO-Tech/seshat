package config

import (
	"context"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
)

func TestTool_Call_ReturnsEffectivePolicy(t *testing.T) {
	tl := NewTool("/some/working/dir")
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected result.Data to be a map[string]any, got %T", result.Data)
	}

	if data["working_directory"] != "/some/working/dir" {
		t.Errorf("expected working_directory to fall back to the tool's configured dir, got %v", data["working_directory"])
	}

	commands, ok := data["commands"].(map[string]any)
	if !ok {
		t.Fatalf("expected commands to be a map, got %T", data["commands"])
	}
	requiresApproval, ok := commands["requires_approval"].([]string)
	if !ok || len(requiresApproval) == 0 {
		t.Errorf("expected a non-empty requires_approval list, got %v", commands["requires_approval"])
	}
	found := false
	for _, c := range requiresApproval {
		if c == "rm" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'rm' to be in requires_approval, got %v", requiresApproval)
	}

	fs, ok := data["filesystem"].(map[string]any)
	if !ok {
		t.Fatalf("expected filesystem to be a map, got %T", data["filesystem"])
	}
	writeDenied, ok := fs["write_denied_path_prefixes"].([]string)
	if !ok || len(writeDenied) == 0 {
		t.Errorf("expected a non-empty write_denied_path_prefixes list, got %v", fs["write_denied_path_prefixes"])
	}

	limits, ok := data["file_read_limits"].(map[string]any)
	if !ok {
		t.Fatalf("expected file_read_limits to be a map, got %T", data["file_read_limits"])
	}
	if limits["max_lines"] == nil {
		t.Errorf("expected max_lines to be present in file_read_limits")
	}

	if _, ok := data["default_shell"].(string); !ok {
		t.Errorf("expected default_shell to be a string, got %v", data["default_shell"])
	}
}

func TestTool_Definition_IsReadOnlyNoPermissionRequired(t *testing.T) {
	tl := NewTool("/tmp")
	def := tl.Definition()
	if def.Name != ToolName {
		t.Errorf("expected name %q, got %q", ToolName, def.Name)
	}
	if !def.IsReadOnly {
		t.Error("expected get_config to be read-only")
	}
	if def.RequiresPermission {
		t.Error("expected get_config to not require permission")
	}
	if def.IsDestructive {
		t.Error("expected get_config to not be destructive")
	}
}

func TestTool_ValidateInput_AcceptsEmptyInput(t *testing.T) {
	tl := NewTool("/tmp")
	if _, err := tl.ValidateInput(context.Background(), map[string]any{}); err != nil {
		t.Errorf("expected empty input to be valid: %v", err)
	}
}
