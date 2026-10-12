package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// TestExitPlanModeApprovalDoesNotReachTheUser pins a KNOWN DEFECT (TODO.md,
// "the plan-approval gate does not reach the user"). It asserts the wrong
// behavior on purpose so the hole is executable rather than a comment, and so
// that landing the fix forces this test to be inverted.
//
// The tool builds the exact message the user is supposed to decide on,
// including an 800-character preview of the plan. resolveToolPermissions
// replaces it with an Allow, because a bare types.Ask carries no
// DecisionReason and only a Rule-sourced, safetyCheck-sourced or
// RequiresUserInteraction ask survives that branch; isAlwaysSafeTool then
// allows exit_plan_mode a second time.
//
// Leaving plan mode is what grants write access - write_file and edit_file deny
// outright while ExecutionMode is plan - so while this test passes, plan mode's
// read-only guarantee is advisory: the model can end the mode on its own, and
// the preview is shown to nobody.
//
// Making the ask survive is necessary but not sufficient. Routing it as a real
// Ask stops the tool in the permission stage, and the SDK's plan round-trip then
// never returns the session to execute even when its host prompt function
// answers "approved" (TestPersistedSessionsCarryPlanModeAcrossRestore,
// pkg/sdk/client_test.go), so the fix also needs the approval path to re-run
// the tool so its ContextModifier applies.
func TestExitPlanModeApprovalDoesNotReachTheUser(t *testing.T) {
	for _, mode := range []types.PermissionMode{
		types.PermissionModeOnRequest,
		types.PermissionModeAuto,
	} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

			exit := NewExitPlanModeTool(&ExitPlanModeConfig{SessionID: "s1"})
			planInput := map[string]any{"plan": "# Plan\n\n1. Change the thing\n2. Verify it"}

			// What the tool intends to ask.
			declared := exit.CheckPermissions(context.Background(), planInput, tool.ToolUseContext{
				SessionID:     "s1",
				ExecutionMode: string(modes.ExecutionModePlan),
			})
			if declared.Behavior != types.PermissionBehaviorAsk {
				t.Fatalf("expected the tool to declare an Ask, got %q", declared.Behavior)
			}
			if !strings.Contains(declared.Reason, "Approve this plan") || !strings.Contains(declared.Reason, "Change the thing") {
				t.Fatalf("expected the declared ask to carry the plan preview, got %q", declared.Reason)
			}

			// What the pipeline actually does with it.
			orch := execution.NewOrchestrator()
			integrator := permissions.NewIntegrator(permissions.NewEngine())
			result, err := orch.Execute(context.Background(), execution.ExecuteRequest{
				ToolUses: []types.ToolUseContent{{
					ID:    "t1",
					Name:  ToolNameExitPlanMode,
					Input: planInput,
				}},
				Tools:             map[string]tool.Tool{ToolNameExitPlanMode: exit},
				PermissionCheck:   integrator.CanUseTool("s1", "t1", mode),
				PermissionContext: &types.PermissionContext{ExecutionMode: string(modes.ExecutionModePlan)},
				SessionID:         types.SessionID("s1"),
				TurnID:            types.TurnID("t1"),
				PermissionMode:    mode,
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if len(result.Errors) != 0 {
				t.Fatalf("expected the ask to be discarded (the defect), got %+v", result.Errors)
			}
			if len(result.Traces) != 1 {
				t.Fatalf("expected 1 trace, got %d", len(result.Traces))
			}
			local := result.Traces[0].LocalPermission
			if local.Behavior != types.PermissionBehaviorAllow {
				t.Fatalf("expected the declared Ask to be replaced by an Allow (the defect), got %q", local.Behavior)
			}
			if local.Reason != "" {
				t.Fatalf("expected no reason to survive to the user, got %q", local.Reason)
			}
		})
	}
}
