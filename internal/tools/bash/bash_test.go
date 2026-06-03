//go:build linux

package bash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"golang.org/x/sys/unix"
)

func TestLandlockAccessMasksFollowABI(t *testing.T) {
	_, rwV1 := landlockAccess(1)
	if rwV1&unix.LANDLOCK_ACCESS_FS_REFER != 0 {
		t.Fatal("ABI v1 must not include REFER")
	}
	if rwV1&unix.LANDLOCK_ACCESS_FS_TRUNCATE != 0 {
		t.Fatal("ABI v1 must not include TRUNCATE")
	}
	if rwV1&unix.LANDLOCK_ACCESS_FS_IOCTL_DEV != 0 {
		t.Fatal("ABI v1 must not include IOCTL_DEV")
	}

	_, rwV5 := landlockAccess(5)
	for name, access := range map[string]uint64{
		"REFER":    unix.LANDLOCK_ACCESS_FS_REFER,
		"TRUNCATE": unix.LANDLOCK_ACCESS_FS_TRUNCATE,
		"IOCTL":    unix.LANDLOCK_ACCESS_FS_IOCTL_DEV,
	} {
		if rwV5&access == 0 {
			t.Fatalf("ABI v5 must include %s", name)
		}
	}
}

func TestCommandWithLandlockFallsBackWithoutWorkspace(t *testing.T) {
	path, args, env, enabled := commandWithLandlock("/bin/sh", []string{"-c", "true"}, "")
	if enabled {
		t.Fatal("landlock should not be enabled without a workspace")
	}
	if path != "/bin/sh" || len(args) != 2 || len(env) != 0 {
		t.Fatalf("unexpected fallback command: path=%q args=%v env=%v", path, args, env)
	}
}

func TestCommandWithLandlockUsesCurrentExecutableWhenAvailable(t *testing.T) {
	if !landlockAvailable() {
		t.Skip("Landlock is not available on this host")
	}

	root := t.TempDir()
	path, args, env, enabled := commandWithLandlock("/bin/sh", []string{"-c", "true"}, root)
	if !enabled {
		t.Fatal("expected landlock helper to be enabled")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if path != exe {
		t.Fatalf("expected helper path %q, got %q", exe, path)
	}
	if len(args) < 3 || args[0] != landlockHelperArg || args[1] != "/bin/sh" {
		t.Fatalf("unexpected helper args: %v", args)
	}
	wantEnv := "NEXUS_LANDLOCK_WORKSPACE=" + filepath.Clean(root)
	if len(env) != 1 || env[0] != wantEnv {
		t.Fatalf("unexpected helper env: %v", env)
	}
}

func TestValidateWorkspaceAllowsRelativePaths(t *testing.T) {
	validator := NewSecurityValidator()
	if got := validator.ValidateWorkspace("mkdir -p src && touch src/file.txt", t.TempDir()); got != nil {
		t.Fatalf("expected command to be allowed, got %v", got)
	}
}

func TestValidateWorkspaceRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	validator := NewSecurityValidator()
	if got := validator.ValidateWorkspace("cat /etc/passwd", t.TempDir()); got == nil {
		t.Fatal("expected absolute path outside workspace to be rejected")
	}
}

func TestValidateWorkspaceAllowsAbsolutePathInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "file.txt")
	validator := NewSecurityValidator()
	if got := validator.ValidateWorkspace("cat "+inside, root); got != nil {
		t.Fatalf("expected workspace path to be allowed, got %v", got)
	}
}

func TestValidateWorkspaceRejectsHomeExpansion(t *testing.T) {
	validator := NewSecurityValidator()
	if got := validator.ValidateWorkspace("rm -rf ~", t.TempDir()); got == nil {
		t.Fatal("expected home expansion to be rejected")
	}
}

// ─── apply_patch interception ─────────────────────────────────────────────────

func TestIsApplyPatchInvocation_DirectCall(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"apply_patch '*** Begin Patch\n*** End Patch'", true},
		{"applypatch '*** Begin Patch\n*** End Patch'", true},
		{"/usr/local/bin/apply_patch patch.txt", true},
		{"cd /tmp && apply_patch some_patch", true},
		{"ls -la", false},
		{"git status", false},
		{"grep apply_patch file.txt", false}, // grep mentions it but doesn't run it
		{"echo apply_patch", false},          // echo mentions it but doesn't run it
		{"bash -c 'git log'", false},
	}
	for _, tc := range cases {
		got := isApplyPatchInvocation(tc.cmd)
		if got != tc.want {
			t.Errorf("isApplyPatchInvocation(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestCheckPermissions_RejectsApplyPatchViaShell(t *testing.T) {
	tl := NewTool(DefaultToolConfig())
	result := tl.CheckPermissions(
		t.Context(),
		map[string]any{"command": "apply_patch '*** Begin Patch\n*** End Patch'"},
		tool.ToolUseContext{},
	)
	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny, got %q", result.Behavior)
	}
	if !strings.Contains(result.Reason, "apply_patch tool") {
		t.Errorf("reason should mention apply_patch tool, got: %q", result.Reason)
	}
}
