package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	execution "github.com/EngineerProjects/nexus-engine/internal/modes/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

func TestSubmitPlanWritesSessionPlanFileAndEmitsContent(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	execution.ClearAllStates()
	execution.ClearAllPlanSlugs()
	t.Cleanup(func() {
		execution.ClearAllStates()
		execution.ClearAllPlanSlugs()
	})

	const sessionID = types.SessionID("session-1")
	const content = "# Plan\n\n- step 1"

	var emitted types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		emitted = ev
	})

	var persistCalled bool
	toolImpl := NewSubmitPlanTool(&SubmitPlanConfig{
		SessionID: sessionID,
		UserID:    "user-1",
		PersistFn: func(ctx context.Context, planID, gotSessionID, userID, slug, filename, gotContent string) (string, int, error) {
			persistCalled = true
			if gotSessionID != string(sessionID) {
				t.Fatalf("expected session %q, got %q", sessionID, gotSessionID)
			}
			if slug != "ship-submit-plan" {
				t.Fatalf("expected slug ship-submit-plan, got %q", slug)
			}
			if filename != "ship-submit-plan.md" {
				t.Fatalf("expected filename ship-submit-plan.md, got %q", filename)
			}
			if gotContent != content {
				t.Fatalf("unexpected content: %q", gotContent)
			}
			return "plan-1", 2, nil
		},
	})

	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: sessionID,
		Parsed: map[string]any{
			"slug":    "ship-submit-plan",
			"content": content,
		},
		ToolContext: &tool.ToolUseContext{
			SessionID:     sessionID,
			ExecutionMode: "plan",
		},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if !persistCalled {
		t.Fatal("expected plan store persist to be called")
	}

	planPath := filepath.Join(runtimepath.SessionPlansDir("", string(sessionID)), "ship-submit-plan.md")
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read mirrored plan file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected mirrored plan content %q, got %q", content, string(data))
	}
	if got := execution.GetPlanSlug(sessionID); got != "ship-submit-plan" {
		t.Fatalf("expected session plan slug ship-submit-plan, got %q", got)
	}
	if emitted.Type != types.RuntimeEventTypePlanSubmitted {
		t.Fatalf("expected runtime event %q, got %q", types.RuntimeEventTypePlanSubmitted, emitted.Type)
	}
	if emitted.PlanEvent == nil {
		t.Fatal("expected plan event payload")
	}
	if emitted.PlanEvent.Filename != "ship-submit-plan.md" {
		t.Fatalf("expected runtime filename ship-submit-plan.md, got %q", emitted.PlanEvent.Filename)
	}
	if emitted.PlanEvent.Content != content {
		t.Fatalf("expected runtime content %q, got %q", content, emitted.PlanEvent.Content)
	}
	if meta := result.Metadata.Additional["filename"]; meta != "ship-submit-plan.md" {
		t.Fatalf("expected result metadata filename ship-submit-plan.md, got %v", meta)
	}
	if !strings.Contains(result.GetContent(), "Filename: ship-submit-plan.md") {
		t.Fatalf("expected result content to mention stable filename, got %q", result.GetContent())
	}
}
