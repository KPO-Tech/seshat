package model

import "testing"

// TestTogglePlanIsBoundToShiftTab pins the user's own way into plan mode.
// shift+tab is deliberately the same key Claude Code, Cursor and Codex use for
// it: it is the binding a user already reaches for when they want to design a
// plan with the agent instead of letting it act, and Seshat had no equivalent
// (only --mode plan at start-up and the model's own enter_plan_mode, which is
// permission-free by design).
func TestTogglePlanIsBoundToShiftTab(t *testing.T) {
	km := DefaultKeyMap()

	if km.TogglePlan.Enabled() != true {
		t.Fatal("expected the plan-mode binding to be enabled")
	}
	keys := km.TogglePlan.Keys()
	if len(keys) != 1 || keys[0] != "shift+tab" {
		t.Fatalf("expected the plan-mode binding to be exactly shift+tab, got %v", keys)
	}
	if km.ToggleYolo.Keys()[0] != "ctrl+y" {
		t.Fatalf("expected yolo to keep ctrl+y, got %v", km.ToggleYolo.Keys())
	}
}
