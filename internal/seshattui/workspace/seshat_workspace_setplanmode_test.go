package workspace

import "testing"

// TestSetPlanModeWithoutSessionReportsAnError covers the one branch of the
// user's plan-mode switch that can be reached without a live SDK session: the
// caller gets an error instead of a panic or a silent no-op, so the UI can
// report why shift+tab did nothing before a session exists.
//
// The mode flip itself is delegated to the SDK's ForcePlanMode/ClearPlanMode,
// which engine-level tests already cover
// (TestForcePlanMode_ThenClearPlanMode_RestoresPriorMode,
// internal/engine/session_config_test.go); this only pins the guard in front of
// it.
func TestSetPlanModeWithoutSessionReportsAnError(t *testing.T) {
	w := NewSeshatWorkspace(nil, "", "")

	if err := w.SetPlanMode(true); err == nil {
		t.Fatal("expected an error when entering plan mode without a session")
	}
	if err := w.SetPlanMode(false); err == nil {
		t.Fatal("expected an error when leaving plan mode without a session")
	}
}
