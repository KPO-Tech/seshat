package plan

import (
	"context"
	"testing"

	execution "github.com/EngineerProjects/nexus-engine/internal/modes/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

func TestEnterPlanModeEmitsExecutionModeChangedEvent(t *testing.T) {
	execution.ClearAllStates()
	t.Cleanup(execution.ClearAllStates)

	const sessionID = types.SessionID("session-1")
	var emitted types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		emitted = ev
	})

	toolImpl := NewEnterPlanModeTool(&EnterPlanModeConfig{SessionID: sessionID})
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: sessionID,
		ToolContext: &tool.ToolUseContext{
			SessionID:      sessionID,
			PermissionMode: types.PermissionModeOnRequest,
		},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if emitted.Type != types.RuntimeEventTypeExecutionModeChanged {
		t.Fatalf("expected runtime event %q, got %q", types.RuntimeEventTypeExecutionModeChanged, emitted.Type)
	}
	if emitted.SessionID != sessionID {
		t.Fatalf("expected session %q, got %q", sessionID, emitted.SessionID)
	}
	if emitted.ExecutionMode != "plan" {
		t.Fatalf("expected execution mode plan, got %q", emitted.ExecutionMode)
	}
}

func TestExitPlanModeEmitsExecutionModeChangedEvent(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	execution.ClearAllStates()
	execution.ClearAllPlanSlugs()
	t.Cleanup(func() {
		execution.ClearAllStates()
		execution.ClearAllPlanSlugs()
	})

	const sessionID = types.SessionID("session-2")
	var emitted types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		emitted = ev
	})

	toolImpl := NewExitPlanModeTool(&ExitPlanModeConfig{SessionID: sessionID})
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: sessionID,
		Parsed: map[string]any{
			"plan": `# Plan

- step 1`,
		},
		ToolContext: &tool.ToolUseContext{
			SessionID:      sessionID,
			ExecutionMode:  "plan",
			PrePlanMode:    types.PermissionModeOnRequest,
			PermissionMode: types.PermissionModeOnRequest,
		},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if emitted.Type != types.RuntimeEventTypeExecutionModeChanged {
		t.Fatalf("expected runtime event %q, got %q", types.RuntimeEventTypeExecutionModeChanged, emitted.Type)
	}
	if emitted.SessionID != sessionID {
		t.Fatalf("expected session %q, got %q", sessionID, emitted.SessionID)
	}
	if emitted.ExecutionMode != "execute" {
		t.Fatalf("expected execution mode execute, got %q", emitted.ExecutionMode)
	}
}
