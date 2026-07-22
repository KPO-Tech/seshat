package engine

import (
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

func newBareSession() *Session {
	return &Session{state: &SessionState{}, config: &Config{}}
}

func TestForcePlanMode_EntersPlanModeAndCapturesPrePlanMode(t *testing.T) {
	s := newBareSession()
	s.SetPermissionMode(types.PermissionModeAuto)

	s.ForcePlanMode()

	if got := s.GetExecutionMode(); got != "plan" {
		t.Fatalf("expected execution mode 'plan', got %q", got)
	}
	ctx := s.GetPermissionContext()
	if ctx == nil {
		t.Fatal("expected non-nil permission context")
	}
	if ctx.PrePlanMode != types.PermissionModeAuto {
		t.Fatalf("expected PrePlanMode to capture prior mode 'auto', got %q", ctx.PrePlanMode)
	}
	// Mode itself must stay untouched while entering plan mode — only
	// PrePlanMode/ExecutionMode change (mirrors enter_plan_mode.Call's
	// ContextModifier, which re-assigns PermissionMode to itself).
	if s.GetPermissionMode() != types.PermissionModeAuto {
		t.Fatalf("expected permission mode to remain 'auto', got %q", s.GetPermissionMode())
	}
}

func TestForcePlanMode_SetsBypassAvailableWhenCurrentlyBypass(t *testing.T) {
	s := newBareSession()
	s.SetPermissionMode(types.PermissionModeBypass)

	s.ForcePlanMode()

	ctx := s.GetPermissionContext()
	if !ctx.IsBypassPermissionsModeAvailable {
		t.Fatal("expected IsBypassPermissionsModeAvailable to be true when entering plan mode from bypass")
	}
}

func TestForcePlanMode_DefaultsToOnRequestOnFreshSession(t *testing.T) {
	s := newBareSession()

	s.ForcePlanMode()

	if got := s.GetExecutionMode(); got != "plan" {
		t.Fatalf("expected execution mode 'plan', got %q", got)
	}
	if got := s.GetPermissionMode(); got != types.PermissionModeOnRequest {
		t.Fatalf("expected default permission mode 'onRequest' on a fresh session, got %q", got)
	}
}

func TestForcePlanMode_IsIdempotentWhenAlreadyInPlanMode(t *testing.T) {
	s := newBareSession()
	s.SetPermissionMode(types.PermissionModeAuto)
	s.ForcePlanMode()
	s.ForcePlanMode() // calling again must not panic or corrupt state

	if got := s.GetExecutionMode(); got != "plan" {
		t.Fatalf("expected execution mode to remain 'plan', got %q", got)
	}
	if got := s.GetPermissionContext().PrePlanMode; got != types.PermissionModeAuto {
		t.Fatalf("expected PrePlanMode to remain 'auto', got %q", got)
	}
}

func TestForcePlanMode_ThenClearPlanMode_RestoresPriorMode(t *testing.T) {
	s := newBareSession()
	s.SetPermissionMode(types.PermissionModeAuto)

	s.ForcePlanMode()
	s.ClearPlanMode()

	// ClearPlanMode sets ExecutionMode to "" which CurrentExecutionMode
	// reports back as the default "execute".
	if got := s.GetExecutionMode(); got != "execute" {
		t.Fatalf("expected execution mode 'execute' after clearing plan mode, got %q", got)
	}
	if got := s.GetPermissionMode(); got != types.PermissionModeAuto {
		t.Fatalf("expected permission mode restored to 'auto', got %q", got)
	}
}
