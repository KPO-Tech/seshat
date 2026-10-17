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

// TestExitPlanModeAsksTheUserToApproveThePlan pins the plan-approval gate. Leaving plan mode is what grants
// write access, so a session whose plan the user has not approved (no submit_plan review) must put the plan
// in front of the user: the permission engine asks through the prompt function of the host, with the plan
// preview in the message. A host with no prompt function refuses, and one that answers yes lets the tool run.
func TestExitPlanModeAsksTheUserToApproveThePlan(t *testing.T) {
	for _, mode := range []types.PermissionMode{
		types.PermissionModeOnRequest,
		types.PermissionModeAuto,
	} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

			planInput := map[string]any{"plan": "# Plan\n\n1. Change the thing\n2. Verify it"}
			run := func(promptFn types.PromptFn) execution.ExecuteResult {
				exit := NewExitPlanModeTool(&ExitPlanModeConfig{SessionID: "s1"})
				integrator := permissions.NewIntegrator(permissions.NewEngine())
				if promptFn != nil {
					integrator.SetPromptFn(promptFn)
				}
				result, err := execution.NewOrchestrator().Execute(context.Background(), execution.ExecuteRequest{
					ToolUses:          []types.ToolUseContent{{ID: "t1", Name: ToolNameExitPlanMode, Input: planInput}},
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
				return result
			}

			// No prompt function: nobody can approve, so the exit is refused.
			if refused := run(nil); len(refused.Errors) != 1 {
				t.Fatalf("expected the exit to be refused without a prompt function, got %+v", refused.Errors)
			}

			// A prompt function that declines refuses it too, and it was shown the plan.
			var shown string
			declined := run(func(_ context.Context, request types.PromptRequest) (types.PromptResponse, error) {
				shown = request.Message
				return types.PromptResponse{Value: false}, nil
			})
			if len(declined.Errors) != 1 {
				t.Fatalf("expected the exit to be refused when the user declines, got %+v", declined.Errors)
			}
			if !strings.Contains(shown, "Approve this plan") || !strings.Contains(shown, "Change the thing") {
				t.Fatalf("expected the user to be shown the plan, got %q", shown)
			}

			// The user approves: the tool runs.
			approved := run(func(context.Context, types.PromptRequest) (types.PromptResponse, error) {
				return types.PromptResponse{Value: true}, nil
			})
			if len(approved.Errors) != 0 {
				t.Fatalf("expected the exit to run once the user approves, got %+v", approved.Errors)
			}
		})
	}
}
