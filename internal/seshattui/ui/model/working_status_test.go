package model

import "testing"

func TestWorkingStatusTextShowsThePhaseWithAMovingEllipsis(t *testing.T) {
	for elapsed, want := range map[int]string{
		0:   "running. (0s)",
		1:   "running.. (1s)",
		2:   "running... (2s)",
		65:  "running... (1m5s)",
		125: "running... (2m5s)",
	} {
		if got := workingStatusText("running", elapsed, ""); got != want {
			t.Errorf("elapsed %d: got %q, want %q", elapsed, got, want)
		}
	}
}

// A provider that refuses a request makes the runtime wait; without a notice the
// line reads like a model that is thinking and has stopped.
func TestWorkingStatusTextShowsWhatTheRuntimeWaitsFor(t *testing.T) {
	got := workingStatusText("running", 14, "rate limited (429), retrying in 12s (attempt 2/5)")
	want := "rate limited (429), retrying in 12s (attempt 2/5) (14s)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
