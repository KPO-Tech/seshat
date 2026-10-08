package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/permissions"
	"github.com/KPO-Tech/seshat/internal/types"
)

func TestManagedInstructionsTextCarriesTheRulesAndWhereTheyComeFrom(t *testing.T) {
	if got := managedInstructionsText(nil); got != "" {
		t.Fatalf("no policy must add nothing, got %q", got)
	}
	if got := managedInstructionsText(&ManagedPolicy{Instructions: "   \n "}); got != "" {
		t.Fatalf("blank instructions must add nothing, got %q", got)
	}
	got := managedInstructionsText(&ManagedPolicy{Instructions: "  Never send customer data outside.  "})
	if !strings.Contains(got, "Never send customer data outside.") || !strings.Contains(got, "administrator") || !strings.Contains(got, "precedence") {
		t.Fatalf("the text must say who set the rules and that they win, got %q", got)
	}
	if strings.HasSuffix(got, " ") || strings.Contains(got, "  Never") {
		t.Fatalf("the instructions must be trimmed, got %q", got)
	}
}

func TestManagedForbiddenToolsAreTrimmedAndUnique(t *testing.T) {
	got := managedForbiddenTools(&ManagedPolicy{ForbiddenTools: []string{" bash ", "", "write_file", "bash", "   "}})
	if len(got) != 2 || got[0] != "bash" || got[1] != "write_file" {
		t.Fatalf("expected [bash write_file], got %v", got)
	}
	if managedForbiddenTools(nil) != nil {
		t.Fatal("no policy must forbid nothing")
	}
}

func checkTool(t *testing.T, engine *permissions.Engine, mode types.PermissionMode, tool string) types.PermissionBehavior {
	t.Helper()
	result, err := engine.CheckPermission(context.Background(), &permissions.PermissionContext{
		Mode: mode, ToolName: tool, ToolInput: map[string]any{},
	})
	if err != nil {
		t.Fatalf("check %s: %v", tool, err)
	}
	return result.Behavior
}

func TestForbiddenToolsAreDeniedInEveryModeIncludingBypass(t *testing.T) {
	engine, err := newPermissionEngine(&ClientConfig{ManagedPolicy: &ManagedPolicy{ForbiddenTools: []string{"bash", "mcp__crm__delete_contact"}}})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	for _, mode := range []types.PermissionMode{types.PermissionModeOnRequest, types.PermissionModeAuto, types.PermissionModeAcceptEdits, types.PermissionModeBypass, types.PermissionModeNever} {
		for _, tool := range []string{"bash", "mcp__crm__delete_contact"} {
			if got := checkTool(t, engine, mode, tool); got != types.PermissionBehaviorDeny {
				t.Fatalf("%s in mode %s must be denied, got %s", tool, mode, got)
			}
		}
	}
	// A tool that is not forbidden is untouched: in bypass mode it is allowed.
	if got := checkTool(t, engine, types.PermissionModeBypass, "read_file"); got != types.PermissionBehaviorAllow {
		t.Fatalf("a tool that is not forbidden must stay allowed in bypass mode, got %s", got)
	}
}

func TestNoManagedPolicyChangesNothing(t *testing.T) {
	engine, err := newPermissionEngine(&ClientConfig{ManagedPolicy: &ManagedPolicy{Instructions: "only text"}})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	if rules := managedDenyRules(nil); rules != nil {
		t.Fatalf("no policy must produce no rule, got %v", rules)
	}
	if got := checkTool(t, engine, types.PermissionModeBypass, "bash"); got != types.PermissionBehaviorAllow {
		t.Fatalf("without forbidden tools bash must stay allowed in bypass mode, got %s", got)
	}
}

func TestForbiddenBuiltinToolsAreHiddenFromTheModel(t *testing.T) {
	reg, err := initBuiltinRegistry(&ClientConfig{ManagedPolicy: &ManagedPolicy{ForbiddenTools: []string{"bash"}}}, nil, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if _, found := reg.Get("bash"); found {
		t.Fatal("a forbidden tool must be removed from the registry")
	}
	if _, found := reg.Get("read_file"); !found {
		t.Fatal("a tool that is not forbidden must stay registered")
	}
}
