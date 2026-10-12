package plan_test

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	modeexec "github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/workspace"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	plan "github.com/KPO-Tech/seshat-runtime/internal/tools/system/plan"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// A plan travels through several pieces that each hold one end of the same
// promise: submit_plan records that a plan is with the user, exit_plan_mode
// refuses until that plan is approved, and approving in the review dialog is what
// lifts the refusal. Each piece is unit-tested next to itself, and this walks the
// whole sequence in one place, because they are only correct together: a change
// to any step that leaves the others behind breaks this test rather than quietly
// producing a session that cannot leave plan mode, or one that leaves it before
// anyone looked.
func TestPlanLifecycleFromSubmissionToApprovedExit(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	modeexec.ClearAllStates()
	modeexec.ClearAllPlanSlugs()

	sessionID := types.SessionID("lifecycle")
	tools := lifecycleTools(t, sessionID)

	const (
		firstPlan  = "# Plan\n\n1. Change the thing\n2. Verify it"
		secondPlan = "# Plan v2\n\n1. Change the thing carefully\n2. Verify it"
		smuggled   = "# Plan\n\n1. Skip the review"
	)

	// 1. The agent submits a plan. It reaches the user, and it is now on record
	//    as awaiting their answer.
	submitted := runTool(t, sessionID, tools, plan.ToolNameSubmitPlan, map[string]any{
		"slug":    "lifecycle-plan",
		"content": firstPlan,
	})
	if len(submitted.Errors) != 0 {
		t.Fatalf("submit_plan failed: %+v", submitted.Errors)
	}
	assertPlanOnDisk(t, sessionID, firstPlan)
	if !modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the submitted plan to be awaiting the user's review")
	}

	// 2. The user asks for changes instead of approving. Nothing has been
	//    accepted, so the exit is refused - and refused means the tool did not
	//    run: the plan it carried must not reach disk, and the mode must not move.
	refused := runTool(t, sessionID, tools, plan.ToolNameExitPlanMode, map[string]any{"plan": smuggled})
	if len(refused.Errors) == 0 {
		t.Fatal("expected exit_plan_mode to be refused while the plan awaits review")
	}
	if len(refused.Traces) != 1 || refused.Traces[0].LocalPermission.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("expected a deny on the permission stage, got %+v", refused.Traces)
	}
	if !strings.Contains(refused.Traces[0].LocalPermission.Reason, "waiting for the user's review") {
		t.Fatalf("expected the refusal to name the pending review, got %q", refused.Traces[0].LocalPermission.Reason)
	}
	assertPlanOnDisk(t, sessionID, firstPlan)
	if modeexec.ExitedPlanMode(sessionID) {
		t.Fatal("the refused exit left plan mode")
	}

	// 3. The agent revises. The revised plan is a different plan: it goes back to
	//    awaiting review rather than inheriting anything.
	revised := runTool(t, sessionID, tools, plan.ToolNameSubmitPlan, map[string]any{
		"slug":    "lifecycle-plan",
		"content": secondPlan,
	})
	if len(revised.Errors) != 0 {
		t.Fatalf("submit_plan (revised) failed: %+v", revised.Errors)
	}
	assertPlanOnDisk(t, sessionID, secondPlan)
	if !modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the revised plan to await review again")
	}

	// 4. The user approves, through the entry point the review dialog uses. This
	//    workspace has no active session, so the mode change it would also make
	//    cannot happen ("session not active") - the decision itself still has to
	//    be on record, which is what the dialog's fallback path relies on.
	if _, err := workspace.NewSeshatWorkspace(nil, "", "").ApprovePlan(string(sessionID)); err == nil {
		t.Fatal("expected ApprovePlan to report that it could not change an inactive session's mode")
	}
	if modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the approval to be recorded even though the mode change failed")
	}

	// 5. The exit now runs, for real: the plan reaches disk and the session
	//    leaves plan mode.
	approved := runTool(t, sessionID, tools, plan.ToolNameExitPlanMode, map[string]any{"plan": secondPlan})
	if len(approved.Errors) != 0 {
		t.Fatalf("expected the approved exit to run, got %+v", approved.Errors)
	}
	if len(approved.Traces) != 1 || approved.Traces[0].LocalPermission.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("expected the approved exit to be allowed, got %+v", approved.Traces)
	}
	assertPlanOnDisk(t, sessionID, secondPlan)
	if !modeexec.ExitedPlanMode(sessionID) {
		t.Fatal("expected the approved exit to leave plan mode")
	}

	// 6. One cycle's approval does not carry into the next: entering plan mode
	//    again and submitting a new plan is refused all over again until that
	//    plan is answered.
	modeexec.MarkPlanApproved(sessionID)
	modeexec.EnterPlanMode(sessionID, nil)
	next := runTool(t, sessionID, tools, plan.ToolNameSubmitPlan, map[string]any{
		"slug":    "lifecycle-plan-2",
		"content": "# Plan 2\n\n1. Something else",
	})
	if len(next.Errors) != 0 {
		t.Fatalf("submit_plan (second cycle) failed: %+v", next.Errors)
	}
	if !modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the second cycle's plan to await review on its own")
	}
	secondCycle := runTool(t, sessionID, tools, plan.ToolNameExitPlanMode, map[string]any{"plan": smuggled})
	if len(secondCycle.Errors) == 0 {
		t.Fatal("expected the previous cycle's approval not to authorize this plan")
	}
}

// lifecycleTools builds the two real plan tools, with submit_plan persisting to a
// version counter the way a host's plan store does.
func lifecycleTools(t *testing.T, sessionID types.SessionID) map[string]tool.Tool {
	t.Helper()
	submit := plan.NewSubmitPlanTool(&plan.SubmitPlanConfig{
		SessionID: sessionID,
		UserID:    "user-1",
		PersistFn: func(_ context.Context, _, _, _, _, _, _ string) (string, int, error) {
			return "plan-1", 1, nil
		},
	})
	if submit == nil {
		t.Fatal("expected submit_plan to be built with a persist function")
	}
	return map[string]tool.Tool{
		plan.ToolNameSubmitPlan:   submit,
		plan.ToolNameExitPlanMode: plan.NewExitPlanModeTool(&plan.ExitPlanModeConfig{SessionID: sessionID}),
	}
}

// runTool drives one tool call through the real orchestrator in plan mode, which
// is where a tool's declared permission result is actually resolved.
func runTool(t *testing.T, sessionID types.SessionID, tools map[string]tool.Tool, name string, input map[string]any) execution.ExecuteResult {
	t.Helper()
	result, err := execution.NewOrchestrator().Execute(context.Background(), execution.ExecuteRequest{
		ToolUses:          []types.ToolUseContent{{ID: "t1", Name: name, Input: input}},
		Tools:             tools,
		PermissionCheck:   permissions.NewIntegrator(permissions.NewEngine()).CanUseTool(sessionID, "t1", types.PermissionModeOnRequest),
		PermissionContext: &types.PermissionContext{ExecutionMode: string(modes.ExecutionModePlan)},
		SessionID:         sessionID,
		TurnID:            types.TurnID("t1"),
		PermissionMode:    types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute(%s) failed: %v", name, err)
	}
	return result
}

func assertPlanOnDisk(t *testing.T, sessionID types.SessionID, want string) {
	t.Helper()
	if !modeexec.PlanExists(sessionID, nil) {
		t.Fatal("expected a plan file on disk")
	}
	got, err := modeexec.GetPlan(sessionID, nil)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected plan on disk:\nwant %q\ngot  %q", want, got)
	}
}
