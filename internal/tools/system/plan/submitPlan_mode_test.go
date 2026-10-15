package plan

import (
	"context"
	"strings"
	"testing"

	execution "github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

// submitIn runs submit_plan in the given execution mode and returns the result and
// the runtime events it emitted.
func submitIn(t *testing.T, mode string) (tool.CallResult, []types.RuntimeEvent) {
	t.Helper()
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	execution.ClearAllStates()
	execution.ClearAllPlanSlugs()
	t.Cleanup(func() {
		execution.ClearAllStates()
		execution.ClearAllPlanSlugs()
	})

	const sessionID = types.SessionID("session-mode")
	var events []types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		events = append(events, ev)
	})
	impl := NewSubmitPlanTool(&SubmitPlanConfig{
		SessionID: sessionID,
		UserID:    "u",
		PersistFn: func(context.Context, string, string, string, string, string, string) (string, int, error) {
			return "plan-1", 1, nil
		},
	})
	result, err := impl.Call(ctx, tool.CallInput{
		SessionID:   sessionID,
		Parsed:      map[string]any{"slug": "research-plan", "content": "# Plan\n\n1. read\n2. write"},
		ToolContext: &tool.ToolUseContext{SessionID: sessionID, ExecutionMode: mode},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	return result, events
}

// submit_plan used to answer "can only be called in plan mode", so an agent in
// execute mode could only write the plan as text and ask "do you approve?", with no
// review screen to answer on.
func TestSubmitPlanWorksOutsidePlanMode(t *testing.T) {
	for _, mode := range []string{"", "execute", "default"} {
		result, events := submitIn(t, mode)
		if result.IsError() {
			t.Fatalf("mode %q: submit_plan refused: %s", mode, result.GetContent())
		}
		if len(events) != 1 || events[0].Type != types.RuntimeEventTypePlanSubmitted || events[0].PlanEvent == nil {
			t.Fatalf("mode %q: the plan must reach the review screen, events = %+v", mode, events)
		}
		text := result.GetContent()
		if !strings.Contains(text, "wait") {
			t.Fatalf("mode %q: the agent must be told to wait for the answer: %q", mode, text)
		}
		if strings.Contains(text, "Plan mode is on") {
			t.Fatalf("mode %q: plan mode instructions given outside plan mode: %q", mode, text)
		}
	}
}

func TestSubmitPlanInPlanModeKeepsTheExitWarning(t *testing.T) {
	result, events := submitIn(t, "plan")
	if result.IsError() || len(events) != 1 {
		t.Fatalf("plan mode: error = %v events = %d", result.IsError(), len(events))
	}
	if !strings.Contains(result.GetContent(), "do NOT call exit_plan_mode before the approval") {
		t.Fatalf("plan mode must still warn against leaving it early: %q", result.GetContent())
	}
}
