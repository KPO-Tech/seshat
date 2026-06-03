package write

import (
	"context"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// defaultToolCtx returns a minimal ToolUseContext for tests that don't need
// a real workspace (absolute paths bypass working-dir resolution).
func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

// checkPermissions is a convenience wrapper.
func checkPermissions(t *testing.T, filePath, content string) types.PermissionResult {
	t.Helper()
	w := NewWriteTool("/tmp")
	return w.CheckPermissions(context.Background(), map[string]any{
		"file_path": filePath,
		"content":   content,
	}, defaultToolCtx())
}

// ---------------------------------------------------------------------------
// Device paths
// ---------------------------------------------------------------------------

func TestWriteTool_CheckPermissions_DevicePaths(t *testing.T) {
	devices := []string{
		"/dev/zero", "/dev/random", "/dev/urandom",
		"/dev/null", "/dev/stdin", "/dev/stdout",
	}
	for _, p := range devices {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "data")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for device path %q, got %v", p, got.Behavior)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Sensitive files
// ---------------------------------------------------------------------------

func TestWriteTool_CheckPermissions_EnvFileBlocked(t *testing.T) {
	cases := []string{
		"/project/.env",
		"/project/.env.local",
		"/project/.env.production",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "SECRET=value")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for .env path %q, got %v", p, got.Behavior)
			}
		})
	}
}

func TestWriteTool_CheckPermissions_SSHKeyBlocked(t *testing.T) {
	cases := []string{
		"/home/user/.ssh/id_rsa",
		"/root/.ssh/authorized_keys",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "ssh-rsa AAAA...")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for SSH key path %q, got %v", p, got.Behavior)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Normal paths are not denied at permission-check time
// (they require a read-first guard, but that fires in Call, not here)
// ---------------------------------------------------------------------------

func TestWriteTool_CheckPermissions_NormalPathPassthrough(t *testing.T) {
	cases := []string{
		"/project/main.go",
		"/project/README.md",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "content")
			if got.Behavior == types.PermissionBehaviorDeny {
				t.Errorf("expected non-Deny for safe path %q, got Deny: %s", p, got.Reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateInput schema checks
// ---------------------------------------------------------------------------

func TestWriteTool_ValidateInput(t *testing.T) {
	w := NewWriteTool("/tmp")
	ctx := context.Background()

	// Missing file_path
	if _, err := w.ValidateInput(ctx, map[string]any{"content": "x"}); err == nil {
		t.Error("expected error for missing file_path")
	}

	// Empty file_path
	if _, err := w.ValidateInput(ctx, map[string]any{"file_path": "  ", "content": "x"}); err == nil {
		t.Error("expected error for blank file_path")
	}

	// Missing content
	if _, err := w.ValidateInput(ctx, map[string]any{"file_path": "a.txt"}); err == nil {
		t.Error("expected error for missing content")
	}

	// Valid input passes
	if _, err := w.ValidateInput(ctx, map[string]any{"file_path": "a.txt", "content": "hello"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
}
