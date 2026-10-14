package bash

import (
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// Found by an audit: the monitor tool allowed every command in its own check,
// without the checks the bash tool applies to the same command.
func TestMonitorRefusesWhatBashRefuses(t *testing.T) {
	dir := t.TempDir()
	monitor := NewMonitorTool(dir)
	bash := NewTool(&ToolConfig{WorkingDirectory: dir})

	for _, command := range []string{
		"git push --force origin main",
		"git reset --hard HEAD~5",
		"git -c core.sshCommand='touch /tmp/pwn' fetch",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda",
	} {
		input := map[string]any{"command": command}
		if got := bash.CheckPermissions(t.Context(), input, tool.ToolUseContext{}); got.Behavior != types.PermissionBehaviorDeny {
			t.Fatalf("premise: bash must refuse %q, got %s", command, got.Behavior)
		}
		if got := monitor.CheckPermissions(t.Context(), input, tool.ToolUseContext{}); got.Behavior != types.PermissionBehaviorDeny {
			t.Errorf("monitor must refuse what bash refuses: %q, got %s", command, got.Behavior)
		}
	}
}

func TestMonitorStillAcceptsAnOrdinaryCommand(t *testing.T) {
	monitor := NewMonitorTool(t.TempDir())
	got := monitor.CheckPermissions(t.Context(), map[string]any{"command": "tail -f app.log"}, tool.ToolUseContext{})
	if got.Behavior == types.PermissionBehaviorDeny {
		t.Errorf("an ordinary command must not be refused by the hard checks, got %s (%s)", got.Behavior, got.Reason)
	}
}

func TestMonitorCallRefusesAHardDeniedCommandEvenWithoutAPermissionCheck(t *testing.T) {
	monitor := NewMonitorTool(t.TempDir())
	result, err := monitor.Call(t.Context(), tool.CallInput{Parsed: map[string]any{"command": "git push --force origin main"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError() {
		t.Error("a direct call must not start a command the bash tool refuses")
	}
}
