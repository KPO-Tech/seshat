package workspace

import (
	"testing"

	modeexec "github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
)

// Approving a plan is a decision about the plan, and it has to be recorded even
// when the workspace cannot also drive the mode change. The review dialog falls
// back to sending the bare word "Proceed" when ApprovePlan fails, which tells the
// model to call exit_plan_mode - and exit_plan_mode refuses while a submitted
// plan is unapproved. If the record were made after the session check, that
// fallback would hand the model an instruction it is not allowed to follow.
func TestApprovePlanRecordsTheDecisionEvenForAnInactiveSession(t *testing.T) {
	modeexec.ClearAllStates()

	w := NewSeshatWorkspace(nil, "", "")
	modeexec.MarkPlanSubmitted("other-session")

	// No session is active in this workspace, so the mode change cannot happen.
	if _, err := w.ApprovePlan("other-session"); err == nil {
		t.Fatal("expected ApprovePlan to fail without an active session")
	}

	if modeexec.PlanAwaitingApproval("other-session") {
		t.Fatal("expected the approval to be recorded despite the failed mode change")
	}
}
