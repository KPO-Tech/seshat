package execution

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// newEnterPlanModeStub reproduces what EnterPlanModeTool.CheckPermissions
// returns - a decision the pipeline moves straight to Allow - under the real
// tool name, so the permission pipeline treats it exactly as it treats the real
// tool.
//
// The real type cannot be imported from here: internal/tools/system/plan
// imports this package (exitPlanMode persists the plan through
// execution.SetPlan), so importing it back would be a cycle.
func newEnterPlanModeStub(ran *bool) *stubTool {
	return &stubTool{
		definition: tool.Definition{Name: "enter_plan_mode", Description: "enter plan mode"},
		checkPermissions: func(context.Context, map[string]any, tool.ToolUseContext) types.PermissionResult {
			return types.Passthrough(map[string]any{})
		},
		call: func(context.Context, tool.CallInput, types.CanUseToolFn) (tool.CallResult, error) {
			*ran = true
			return tool.NewTextResult("plan mode enabled"), nil
		},
	}
}

// TestEnterPlanModeIsPermissionFreeByDesign pins the intended asymmetry of plan
// mode, which plan mode's read-only guarantee depends on:
//
//   - Entering is permission-free. It is read-only, reversible with one
//     keystroke, and it *reduces* what the agent can do, so a prompt there would
//     guard the safe direction at the moment the agent knows least about the
//     task. Claude Code draws the line in the same place: its EnterPlanMode tool
//     is documented as not requiring permission.
//   - Leaving is where the human gate belongs, because leaving is what grants
//     write access. That gate lives on exit_plan_mode and submit_plan; see
//     internal/tools/system/plan/exitPlanModeApproval_test.go.
//
// enter_plan_mode used to declare types.Ask here instead, which read as a human
// gate that never existed; two independent reasons made it unreachable, and the
// never-mode case below shows the second one (engine.isAlwaysSafeTool lists
// enter_plan_mode/exit_plan_mode and is skipped only in never mode).
func TestEnterPlanModeIsPermissionFreeByDesign(t *testing.T) {
	for _, mode := range []types.PermissionMode{
		types.PermissionModeOnRequest,
		types.PermissionModeAuto,
	} {
		t.Run(string(mode), func(t *testing.T) {
			ran := false
			orch := NewOrchestrator()
			integrator := permissions.NewIntegrator(permissions.NewEngine())

			result, err := orch.Execute(context.Background(), ExecuteRequest{
				ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "enter_plan_mode", Input: map[string]any{}}},
				Tools:           map[string]tool.Tool{"enter_plan_mode": newEnterPlanModeStub(&ran)},
				PermissionCheck: integrator.CanUseTool("s1", "t1", mode),
				SessionID:       types.SessionID("s1"),
				TurnID:          types.TurnID("t1"),
				PermissionMode:  mode,
			})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if len(result.Errors) != 0 {
				t.Fatalf("expected no execution error, got %+v", result.Errors)
			}
			if !ran {
				t.Fatal("expected entering plan mode to proceed without asking")
			}
			if len(result.Traces) != 1 {
				t.Fatalf("expected 1 trace, got %d", len(result.Traces))
			}
			trace := result.Traces[0]
			if trace.GlobalPermission.Behavior != types.PermissionBehaviorAllow {
				t.Fatalf("expected the global stage to allow, got %q", trace.GlobalPermission.Behavior)
			}
			if trace.LocalPermission.Behavior == types.PermissionBehaviorAsk {
				t.Fatal("a local Ask here would be an approval point that nothing honors")
			}
		})
	}

	t.Run(string(types.PermissionModeNever), func(t *testing.T) {
		ran := false
		orch := NewOrchestrator()
		integrator := permissions.NewIntegrator(permissions.NewEngine())

		result, err := orch.Execute(context.Background(), ExecuteRequest{
			ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "enter_plan_mode", Input: map[string]any{}}},
			Tools:           map[string]tool.Tool{"enter_plan_mode": newEnterPlanModeStub(&ran)},
			PermissionCheck: integrator.CanUseTool("s1", "t1", types.PermissionModeNever),
			SessionID:       types.SessionID("s1"),
			TurnID:          types.TurnID("t1"),
			PermissionMode:  types.PermissionModeNever,
		})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if ran {
			t.Fatal("expected never mode to refuse the switch")
		}
		if len(result.Errors) != 1 || result.Errors[0].Stage != ErrorStagePermission {
			t.Fatalf("expected a permission stage error in never mode, got %+v", result.Errors)
		}
	})
}
