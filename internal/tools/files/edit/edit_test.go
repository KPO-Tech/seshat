package edit

import (
	"context"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

func checkPermissions(t *testing.T, filePath, oldStr, newStr string) types.PermissionResult {
	t.Helper()
	e := NewEditTool("/tmp")
	return e.CheckPermissions(context.Background(), map[string]any{
		"file_path":  filePath,
		"old_string": oldStr,
		"new_string": newStr,
	}, defaultToolCtx())
}

// ---------------------------------------------------------------------------
// Device paths
// ---------------------------------------------------------------------------

func TestEditTool_CheckPermissions_DevicePaths(t *testing.T) {
	devices := []string{
		"/dev/zero", "/dev/random", "/dev/urandom",
		"/dev/null", "/dev/stdin", "/dev/stdout",
	}
	for _, p := range devices {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "old", "new")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for device path %q, got %v", p, got.Behavior)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Path traversal
// ---------------------------------------------------------------------------

func TestEditTool_CheckPermissions_TraversalBlocked(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"../../root/.ssh/id_rsa",
		"foo/../../../etc/hosts",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "old", "new")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for traversal path %q, got %v", p, got.Behavior)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UNC paths — edit's CheckPermissions calls ValidateUNCPathSecurity on the
// resolved absolute path, so only the // form is caught on Linux (the \\
// form is treated as a relative path by filepath.IsAbs on non-Windows).
// ---------------------------------------------------------------------------

func TestEditTool_CheckPermissions_UNCBlocked(t *testing.T) {
	got := checkPermissions(t, "//server/share/file.txt", "old", "new")
	if got.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny for UNC // path, got %v", got.Behavior)
	}
}

// ---------------------------------------------------------------------------
// Sensitive files
// ---------------------------------------------------------------------------

func TestEditTool_CheckPermissions_EnvFileBlocked(t *testing.T) {
	cases := []string{
		"/project/.env",
		"/project/.env.local",
		"/project/.env.production",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "OLD_SECRET=x", "NEW_SECRET=y")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for .env path %q, got %v", p, got.Behavior)
			}
		})
	}
}

func TestEditTool_CheckPermissions_SSHKeyBlocked(t *testing.T) {
	cases := []string{
		"/home/user/.ssh/id_rsa",
		"/root/.ssh/authorized_keys",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "old key", "new key")
			if got.Behavior != types.PermissionBehaviorDeny {
				t.Errorf("expected Deny for SSH key path %q, got %v", p, got.Behavior)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Normal paths pass CheckPermissions
// ---------------------------------------------------------------------------

func TestEditTool_CheckPermissions_NormalPathPassthrough(t *testing.T) {
	cases := []string{
		"/project/main.go",
		"/project/README.md",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			got := checkPermissions(t, p, "foo", "bar")
			if got.Behavior == types.PermissionBehaviorDeny {
				t.Errorf("expected non-Deny for safe path %q, got Deny: %s", p, got.Reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateInput schema checks
// ---------------------------------------------------------------------------

func TestEditTool_ValidateInput(t *testing.T) {
	e := NewEditTool("/tmp")
	ctx := context.Background()

	// Missing file_path
	if _, err := e.ValidateInput(ctx, map[string]any{"old_string": "a", "new_string": "b"}); err == nil {
		t.Error("expected error for missing file_path")
	}

	// Missing old_string
	if _, err := e.ValidateInput(ctx, map[string]any{"file_path": "f.go", "new_string": "b"}); err == nil {
		t.Error("expected error for missing old_string")
	}

	// Missing new_string
	if _, err := e.ValidateInput(ctx, map[string]any{"file_path": "f.go", "old_string": "a"}); err == nil {
		t.Error("expected error for missing new_string")
	}

	// Identical old/new
	if _, err := e.ValidateInput(ctx, map[string]any{"file_path": "f.go", "old_string": "same", "new_string": "same"}); err == nil {
		t.Error("expected error for identical old_string and new_string")
	}

	// Valid input passes
	if _, err := e.ValidateInput(ctx, map[string]any{"file_path": "f.go", "old_string": "old", "new_string": "new"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
}
