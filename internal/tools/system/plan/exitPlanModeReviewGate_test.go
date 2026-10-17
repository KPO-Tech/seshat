package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	modeexec "github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// The plan-approval condition: exit_plan_mode may only take a session out of
// plan mode once the user has approved the plan they were shown. Leaving plan
// mode is what grants write access, so a submitted plan that is still under
// review must not be implementable.
//
// These run through the real orchestrator with the real tool, because the
// enforcing decision is not the tool's own return value - it is what
// resolveToolPermissions does with it. A Deny survives that path; the Ask in the
// same function does not (see exitPlanModeApproval_test.go).

func reviewGateRequest(t *testing.T, sessionID types.SessionID, mode types.PermissionMode) (execution.ExecuteRequest, *ExitPlanModeTool, map[string]any) {
	t.Helper()

	exit := NewExitPlanModeTool(&ExitPlanModeConfig{SessionID: sessionID})
	planInput := map[string]any{"plan": "# Plan\n\n1. Change the thing\n2. Verify it"}

	req := execution.ExecuteRequest{
		ToolUses: []types.ToolUseContent{{
			ID:    "t1",
			Name:  ToolNameExitPlanMode,
			Input: planInput,
		}},
		Tools:             map[string]tool.Tool{ToolNameExitPlanMode: exit},
		PermissionCheck:   permissions.NewIntegrator(permissions.NewEngine()).CanUseTool(sessionID, "t1", mode),
		PermissionContext: &types.PermissionContext{ExecutionMode: string(modes.ExecutionModePlan)},
		SessionID:         sessionID,
		TurnID:            types.TurnID("t1"),
		PermissionMode:    mode,
	}
	return req, exit, planInput
}

func TestExitPlanModeDeniedWhilePlanAwaitsReview(t *testing.T) {
	for _, mode := range []types.PermissionMode{
		types.PermissionModeOnRequest,
		types.PermissionModeAuto,
	} {
		t.Run(string(mode), func(t *testing.T) {
			t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
			modeexec.ClearAllStates()

			sessionID := types.SessionID("review-pending")
			req, _, _ := reviewGateRequest(t, sessionID, mode)

			// The user has a plan in front of them and has not answered yet.
			modeexec.MarkPlanSubmitted(sessionID)

			result, err := execution.NewOrchestrator().Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if len(result.Errors) == 0 {
				t.Fatal("expected the tool to be refused while the plan awaits review, but it ran")
			}
			if len(result.Traces) != 1 {
				t.Fatalf("expected 1 trace, got %d", len(result.Traces))
			}
			if got := result.Traces[0].LocalPermission.Behavior; got != types.PermissionBehaviorDeny {
				t.Fatalf("expected a deny on the permission stage, got %q", got)
			}
			if !strings.Contains(result.Traces[0].LocalPermission.Reason, "waiting for the user's review") {
				t.Fatalf("expected the refusal to name the pending review, got %q", result.Traces[0].LocalPermission.Reason)
			}

			// The tool must not have run: no plan written, mode still plan.
			if modeexec.PlanExists(sessionID, nil) {
				t.Fatal("the refused tool wrote the plan file")
			}
			if modeexec.ExitedPlanMode(sessionID) {
				t.Fatal("the refused tool left plan mode")
			}

			// Still pending: the gate does not consume the user's pending review.
			if !modeexec.PlanAwaitingApproval(sessionID) {
				t.Fatal("expected the plan to still be awaiting review")
			}
		})
	}
}

func TestExitPlanModeAllowedAfterTheUserApproves(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	modeexec.ClearAllStates()

	sessionID := types.SessionID("review-approved")
	req, _, planInput := reviewGateRequest(t, sessionID, types.PermissionModeOnRequest)

	modeexec.MarkPlanSubmitted(sessionID)
	modeexec.MarkPlanApproved(sessionID)

	result, err := execution.NewOrchestrator().Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected the approved plan to be implementable, got %+v", result.Errors)
	}
	if len(result.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(result.Traces))
	}
	if got := result.Traces[0].LocalPermission.Behavior; got != types.PermissionBehaviorAllow {
		t.Fatalf("expected the approved exit to be allowed, got %q", got)
	}

	// The tool ran for real: the plan reached disk and the mode was left.
	if !modeexec.PlanExists(sessionID, nil) {
		t.Fatal("expected the allowed tool to write the plan file")
	}
	stored, err := modeexec.GetPlan(sessionID, nil)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if stored != planInput["plan"] {
		t.Fatalf("expected the plan to be persisted, got %q", stored)
	}
	if !modeexec.ExitedPlanMode(sessionID) {
		t.Fatal("expected the allowed tool to leave plan mode")
	}
	if modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the settled review to stop blocking")
	}
}

// Revising after feedback must not inherit the earlier answer: the plan under
// review now is a different one, so it has to be approved on its own.
func TestResubmittingAPlanClearsThePreviousApproval(t *testing.T) {
	modeexec.ClearAllStates()
	sessionID := types.SessionID("review-revised")

	modeexec.MarkPlanSubmitted(sessionID)
	modeexec.MarkPlanApproved(sessionID)
	if modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected an approved plan not to be awaiting review")
	}

	modeexec.MarkPlanSubmitted(sessionID)
	if !modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected the revised plan to await review again")
	}
}

// A session that never submitted a plan has nothing under review, so the tool itself does not refuse (plan
// mode keeps its exit); the permission engine asks the user to approve the plan instead. Without a prompt
// function that ask is refused, with one that approves the exit runs.
func TestExitPlanModeIsNotGatedWithoutASubmission(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	modeexec.ClearAllStates()

	sessionID := types.SessionID("review-never-submitted")
	if modeexec.PlanAwaitingApproval(sessionID) {
		t.Fatal("expected a session with no submission not to await review")
	}

	req, _, _ := reviewGateRequest(t, sessionID, types.PermissionModeOnRequest)
	integrator := permissions.NewIntegrator(permissions.NewEngine())
	integrator.SetPromptFn(func(context.Context, types.PromptRequest) (types.PromptResponse, error) {
		return types.PromptResponse{Value: true}, nil
	})
	req.PermissionCheck = integrator.CanUseTool(sessionID, "t1", types.PermissionModeOnRequest)
	result, err := execution.NewOrchestrator().Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected the exit to be permitted once the user approves, got %+v", result.Errors)
	}
}

// A plan the user approved in the review dialog is not asked about a second time.
func TestExitPlanModeDoesNotAskAgainAfterTheReviewApproved(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	modeexec.ClearAllStates()

	sessionID := types.SessionID("review-approved-no-reask")
	modeexec.MarkPlanSubmitted(sessionID)
	modeexec.MarkPlanApproved(sessionID)

	req, _, _ := reviewGateRequest(t, sessionID, types.PermissionModeOnRequest)
	asked := false
	integrator := permissions.NewIntegrator(permissions.NewEngine())
	integrator.SetPromptFn(func(context.Context, types.PromptRequest) (types.PromptResponse, error) {
		asked = true
		return types.PromptResponse{Value: false}, nil
	})
	req.PermissionCheck = integrator.CanUseTool(sessionID, "t1", types.PermissionModeOnRequest)
	result, err := execution.NewOrchestrator().Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if asked || len(result.Errors) != 0 {
		t.Fatalf("expected no second question after the review approved (asked=%v, errors=%+v)", asked, result.Errors)
	}
}
