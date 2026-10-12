package workflow

import (
	"context"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
)

func TestToolDraftsValidWorkflow(t *testing.T) {
	t.Parallel()

	result, err := NewTool().Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"name":   "deep-research",
			"format": "yaml",
			"nodes": []any{
				map[string]any{"id": "collect", "prompt": "Collect facts."},
				map[string]any{"id": "verify", "kind": "verifier", "prompt": "Check facts.", "needs": []any{"collect"}},
				map[string]any{"id": "final", "prompt": "Write final answer.", "needs": []any{"verify"}},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("call workflow_draft: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected tool error: %v", result.Error)
	}
	if !strings.Contains(result.Content, "Workflow draft is valid") {
		t.Fatalf("missing validation summary:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "name: deep-research") {
		t.Fatalf("missing rendered workflow:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "kind: verifier") {
		t.Fatalf("missing node kind:\n%s", result.Content)
	}
}

func TestToolReportsInvalidWorkflow(t *testing.T) {
	t.Parallel()

	result, err := NewTool().Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"name": "broken",
			"nodes": []any{
				map[string]any{"id": "final", "prompt": "Write final answer.", "needs": []any{"missing"}},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("call workflow_draft: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected tool error")
	}
	if !strings.Contains(result.Content, "unknown node") {
		t.Fatalf("missing diagnostic:\n%s", result.Content)
	}
}
