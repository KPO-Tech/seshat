package requestpermissions

import (
	"context"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── input parsing ────────────────────────────────────────────────────────────

func TestParseInputFilesystemOnly(t *testing.T) {
	in, err := parseInput(map[string]any{
		"reason": "Need to read deployment config",
		"permissions": map[string]any{
			"filesystem": map[string]any{
				"paths":  []any{"/etc/deploy/config.yml"},
				"access": []any{"read"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if in.Reason != "Need to read deployment config" {
		t.Errorf("reason mismatch: %q", in.Reason)
	}
	if in.Permissions.Filesystem == nil {
		t.Fatal("expected filesystem permissions")
	}
	if len(in.Permissions.Filesystem.Paths) != 1 || in.Permissions.Filesystem.Paths[0] != "/etc/deploy/config.yml" {
		t.Errorf("paths mismatch: %v", in.Permissions.Filesystem.Paths)
	}
	if len(in.Permissions.Filesystem.Access) != 1 || in.Permissions.Filesystem.Access[0] != "read" {
		t.Errorf("access mismatch: %v", in.Permissions.Filesystem.Access)
	}
	if in.Scope != GrantScopeTurn {
		t.Errorf("expected default scope turn, got %q", in.Scope)
	}
}

func TestParseInputNetworkOnly(t *testing.T) {
	in, err := parseInput(map[string]any{
		"reason": "Need to fetch package metadata",
		"permissions": map[string]any{
			"network": map[string]any{
				"targets": []any{"registry.npmjs.org", "api.github.com"},
			},
		},
		"scope": "session",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if in.Permissions.Filesystem != nil {
		t.Error("expected no filesystem permissions")
	}
	if in.Permissions.Network == nil {
		t.Fatal("expected network permissions")
	}
	if len(in.Permissions.Network.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(in.Permissions.Network.Targets))
	}
	if in.Scope != GrantScopeSession {
		t.Errorf("expected session scope, got %q", in.Scope)
	}
}

func TestParseInputBothPermissions(t *testing.T) {
	in, err := parseInput(map[string]any{
		"reason": "Deploy script needs read config + remote registry",
		"permissions": map[string]any{
			"filesystem": map[string]any{
				"paths":  []any{"/home/ci/.ssh/id_rsa.pub"},
				"access": []any{"read"},
			},
			"network": map[string]any{
				"targets": []any{"deploy.internal"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if in.Permissions.Filesystem == nil || in.Permissions.Network == nil {
		t.Error("expected both permissions")
	}
}

func TestParseInputMissingPermissions(t *testing.T) {
	_, err := parseInput(map[string]any{
		"reason": "some reason",
	})
	if err == nil {
		t.Error("expected parse error for missing permissions")
	}
}

func TestParseInputNetworkEnabled(t *testing.T) {
	in, err := parseInput(map[string]any{
		"reason": "Need general internet access",
		"permissions": map[string]any{
			"network": map[string]any{
				"enabled": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if in.Permissions.Network == nil || !in.Permissions.Network.Enabled {
		t.Error("expected network.enabled=true")
	}
}

// ─── validation ───────────────────────────────────────────────────────────────

func TestValidateRejectsEmptyReason(t *testing.T) {
	in := &Input{
		Reason: "",
		Permissions: RequestedPermissions{
			Filesystem: &FilesystemPermissions{
				Paths:  []string{"/tmp"},
				Access: []string{"read"},
			},
		},
		Scope: GrantScopeTurn,
	}
	if err := in.validate(); err == nil {
		t.Error("expected validation error for empty reason")
	}
}

func TestValidateRejectsNoPermissions(t *testing.T) {
	in := &Input{
		Reason:      "some reason",
		Permissions: RequestedPermissions{},
		Scope:       GrantScopeTurn,
	}
	if err := in.validate(); err == nil {
		t.Error("expected validation error when no permissions specified")
	}
}

func TestValidateRejectsFilesystemWithNoPaths(t *testing.T) {
	in := &Input{
		Reason: "some reason",
		Permissions: RequestedPermissions{
			Filesystem: &FilesystemPermissions{
				Paths:  nil,
				Access: []string{"read"},
			},
		},
		Scope: GrantScopeTurn,
	}
	if err := in.validate(); err == nil {
		t.Error("expected validation error for empty filesystem paths")
	}
}

func TestValidateRejectsFilesystemWithNoAccess(t *testing.T) {
	in := &Input{
		Reason: "some reason",
		Permissions: RequestedPermissions{
			Filesystem: &FilesystemPermissions{
				Paths:  []string{"/tmp"},
				Access: nil,
			},
		},
		Scope: GrantScopeTurn,
	}
	if err := in.validate(); err == nil {
		t.Error("expected validation error for empty filesystem access list")
	}
}

func TestValidateRejectsNetworkWithNoTargetsAndNotEnabled(t *testing.T) {
	in := &Input{
		Reason: "some reason",
		Permissions: RequestedPermissions{
			Network: &NetworkPermissions{
				Targets: nil,
				Enabled: false,
			},
		},
		Scope: GrantScopeTurn,
	}
	if err := in.validate(); err == nil {
		t.Error("expected validation error for network with no targets and enabled=false")
	}
}

func TestValidateAcceptsNetworkEnabled(t *testing.T) {
	in := &Input{
		Reason: "some reason",
		Permissions: RequestedPermissions{
			Network: &NetworkPermissions{Enabled: true},
		},
		Scope: GrantScopeTurn,
	}
	if err := in.validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// ─── description builder ──────────────────────────────────────────────────────

func TestBuildDescriptionFilesystem(t *testing.T) {
	in := &Input{
		Reason: "deploy config read",
		Permissions: RequestedPermissions{
			Filesystem: &FilesystemPermissions{
				Paths:  []string{"/etc/app/config.yml"},
				Access: []string{"read"},
			},
		},
		Scope: GrantScopeTurn,
	}
	desc := buildDescription(in)
	if !containsStr(desc, "deploy config read") {
		t.Errorf("description should contain reason, got: %q", desc)
	}
	if !containsStr(desc, "/etc/app/config.yml") {
		t.Errorf("description should contain path, got: %q", desc)
	}
	if !containsStr(desc, "this turn") {
		t.Errorf("description should contain scope, got: %q", desc)
	}
}

func TestBuildDescriptionSessionScope(t *testing.T) {
	in := &Input{
		Reason: "persistent access",
		Permissions: RequestedPermissions{
			Network: &NetworkPermissions{Targets: []string{"api.example.com"}},
		},
		Scope: GrantScopeSession,
	}
	desc := buildDescription(in)
	if !containsStr(desc, "this session") {
		t.Errorf("description should contain session scope, got: %q", desc)
	}
}

// ─── CheckPermissions ─────────────────────────────────────────────────────────

func TestCheckPermissionsAlwaysPassthrough(t *testing.T) {
	t.Parallel()
	tl := NewTool()
	input := map[string]any{"reason": "test", "permissions": map[string]any{}}
	result := tl.CheckPermissions(context.Background(), input, tool.ToolUseContext{})
	if result.Behavior != types.PermissionBehaviorPassthrough {
		t.Errorf("expected passthrough, got %q", result.Behavior)
	}
}

// ─── Call — approval granted ──────────────────────────────────────────────────

func TestCallGranted(t *testing.T) {
	t.Parallel()
	tl := NewTool()

	// permissionCheck that always approves
	checker := types.CanUseToolFn(func(_ context.Context, _ types.ToolPermissionRequest) types.PermissionResult {
		return types.AllowWithDecisionReason("user approved", nil)
	})

	input := tool.CallInput{
		Parsed: map[string]any{
			"reason": "Need to read SSH key",
			"permissions": map[string]any{
				"filesystem": map[string]any{
					"paths":  []any{"/home/user/.ssh/id_rsa.pub"},
					"access": []any{"read"},
				},
			},
			"scope": "turn",
		},
	}

	result, err := tl.Call(context.Background(), input, checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected call error: %v", result.Error)
	}

	resp, ok := result.Data.(GrantedResponse)
	if !ok {
		t.Fatalf("expected GrantedResponse, got %T", result.Data)
	}
	if !resp.Granted {
		t.Error("expected granted=true")
	}
	if resp.Scope != GrantScopeTurn {
		t.Errorf("expected scope turn, got %q", resp.Scope)
	}
}

// ─── Call — approval denied ───────────────────────────────────────────────────

func TestCallDenied(t *testing.T) {
	t.Parallel()
	tl := NewTool()

	checker := types.CanUseToolFn(func(_ context.Context, _ types.ToolPermissionRequest) types.PermissionResult {
		return types.Deny("user declined access")
	})

	input := tool.CallInput{
		Parsed: map[string]any{
			"reason": "Need to write to /etc",
			"permissions": map[string]any{
				"filesystem": map[string]any{
					"paths":  []any{"/etc/hosts"},
					"access": []any{"write"},
				},
			},
		},
	}

	result, err := tl.Call(context.Background(), input, checker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// denial is a normal JSON response, not an error
	if result.Error != nil {
		t.Fatalf("expected denial to be a normal result, got error: %v", result.Error)
	}

	resp, ok := result.Data.(DeniedResponse)
	if !ok {
		t.Fatalf("expected DeniedResponse, got %T", result.Data)
	}
	if resp.Granted {
		t.Error("expected granted=false")
	}
	if !containsStr(resp.Reason, "user declined access") {
		t.Errorf("reason mismatch: %q", resp.Reason)
	}
}

// ─── RequiresUserInteraction ──────────────────────────────────────────────────

func TestRequiresUserInteraction(t *testing.T) {
	tl := NewTool()
	if !tl.RequiresUserInteraction() {
		t.Error("request_permissions must require user interaction")
	}
}

// ─── Definition ───────────────────────────────────────────────────────────────

func TestDefinition(t *testing.T) {
	tl := NewTool()
	def := tl.Definition()
	if def.Name != ToolName {
		t.Errorf("name mismatch: %q", def.Name)
	}
	if def.InputSchema.Properties == nil {
		t.Error("input schema properties is nil")
	}
	if !def.RequiresPermission {
		t.Error("expected RequiresPermission=true")
	}
}

// helper
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
