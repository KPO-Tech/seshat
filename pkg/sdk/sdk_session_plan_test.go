package sdk

import (
	"context"
	"testing"

	modeexec "github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
)

// A host using the SDK has no plan review surface of its own. Approving the plan
// with ApprovePlan is what lifts the exit_plan_mode gate, and ForcePlanMode starts
// a clean cycle so a review left pending from a previous one cannot block the
// next. Both are asserted here because they are the SDK's end of the same
// promise the runtime's plan lifecycle test walks.
// A host that persists sessions has no live *Session between turns, so the
// approval it records when the user accepts a plan arrives by session id alone.
// The package-level ApprovePlan is that path: it must settle the review state
// without a session object.
func TestPackageApprovePlanRecordsWithoutALiveSession(t *testing.T) {
	modeexec.ClearAllStates()

	id := SessionID("approve-by-id")
	modeexec.MarkPlanSubmitted(id)
	if !modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected the submitted plan to await review")
	}

	ApprovePlan(id)
	if modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected package-level ApprovePlan to settle the review")
	}
}

func TestSessionApprovePlanAndForcePlanModeResetTheReview(t *testing.T) {
	modeexec.ClearAllStates()

	client, err := NewClient(&ClientConfig{PersistSessions: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	session, err := client.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer session.Close()

	id := session.GetID()

	// ForcePlanMode starts a fresh cycle: a stale pending review is cleared.
	modeexec.MarkPlanSubmitted(id)
	if !modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected the submitted plan to await review")
	}
	session.ForcePlanMode()
	if modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected ForcePlanMode to clear a review left pending from a previous cycle")
	}

	// ApprovePlan records the user's decision, so exit_plan_mode is no longer
	// refused for a submitted plan.
	modeexec.MarkPlanSubmitted(id)
	if !modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected the resubmitted plan to await review")
	}
	session.ApprovePlan()
	if modeexec.PlanAwaitingApproval(id) {
		t.Fatal("expected ApprovePlan to record the user's decision")
	}
}
