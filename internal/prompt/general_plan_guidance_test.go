package prompt

import (
	"strings"
	"testing"
)

// A session waited for six background agents with "Start-Sleep 120" in a shell,
// because task_get could only poll. The prompt points to the wait that exists now.
func TestGeneralPromptWaitsForAgentsWithTaskGet(t *testing.T) {
	for _, want := range []string{"`task_get` and `wait_seconds`", "Never wait with `sleep`"} {
		if !strings.Contains(generalAgentText, want) {
			t.Errorf("the general prompt lost %q", want)
		}
	}
}

// The agent wrote its plans as text ("do you approve?") because the prompt sent it
// to plan mode, which it never entered, and submit_plan refused outside that mode.
func TestGeneralPromptSendsPlansToSubmitPlan(t *testing.T) {
	text := generalAgentText
	for _, want := range []string{
		"`submit_plan`, in any mode",
		"Never write a plan as plain text",
		"send it with `submit_plan` when the user asked to validate it",
		"You do not need it to propose a plan",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the general prompt lost %q", want)
		}
	}
	if strings.Contains(text, "| `enter_plan_mode` |") {
		t.Error("the situation table must not send a plan proposal to enter_plan_mode")
	}
}
