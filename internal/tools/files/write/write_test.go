package write

import (
	"context"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
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

// ---------------------------------------------------------------------------
// Plan mode is read-only: writes are denied, not auto-allowed (TODO.md M5e -
// plan mode used to fall into the same AcceptEdits-style auto-allow as
// PermissionModeAcceptEdits, so a plan-mode session asked FEWER questions
// about writes than a normal session would, contradicting the mode's own
// "tools are described but not executed" doc comment and the instructions
// enter_plan_mode gives the model).
// ---------------------------------------------------------------------------

func TestWriteTool_CheckPermissions_PlanMode_DeniesWrite(t *testing.T) {
	w := NewWriteTool("/project")
	ctx := tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
	ctx.ExecutionMode = "plan"

	got := w.CheckPermissions(context.Background(), map[string]any{
		"file_path": "/project/main.go",
		"content":   "package main",
	}, ctx)

	if got.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("expected Deny in plan mode, got %v (reason: %s)", got.Behavior, got.Reason)
	}
}

func TestWriteTool_CheckPermissions_PlanMode_DeniesEvenInsideWorkingDir(t *testing.T) {
	// Before the fix, a path inside the working directory was exactly the
	// case that got auto-allowed (checkAcceptEditsPermissions's happy path).
	w := NewWriteTool("/project")
	ctx := tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
	ctx.ExecutionMode = "plan"

	got := w.CheckPermissions(context.Background(), map[string]any{
		"file_path": "/project/notes.txt",
		"content":   "harmless notes",
	}, ctx)

	if got.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("expected Deny in plan mode even for a safe in-working-dir path, got %v", got.Behavior)
	}
}

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
