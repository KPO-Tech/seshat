package engine

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	compact "github.com/KPO-Tech/seshat-runtime/internal/runtime/memory"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

func runWithUsageEvents(t *testing.T, usage types.TokenUsage) []*types.ContextUsageRuntimeEvent {
	t.Helper()
	loop := NewLoop(nil, nil, compact.NewEngine(nil, nil), nil, permissions.NewIntegrator(permissions.NewEngine()), nil,
		&LoopConfig{MaxIterations: 1, AutoCompact: true, EnableStreaming: false}, nil)
	loop.callModelFn = func(ctx context.Context, state *MutableState, req RunRequest) (*types.APIResponse, error) {
		return &types.APIResponse{
			Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextContent{Text: "done"}},
			StopReason: types.StopReasonEndTurn, Model: req.Model, ID: "r1", Usage: usage,
		}, nil
	}
	var events []*types.ContextUsageRuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		if ev.Type == types.RuntimeEventTypeContextUsage {
			events = append(events, ev.ContextUsageEvent)
		}
	})
	result := loop.Run(ctx, RunRequest{
		Messages:  []types.Message{types.UserMessage("m1", "hi")},
		SessionID: types.SessionID("s1"),
		TurnID:    types.TurnID("t1"),
		Model:     types.ModelIdentifier{Provider: types.APIProviderAnthropic, Model: "claude-3-5-sonnet-20241022"},
	})
	if result.Error != nil {
		t.Fatalf("Run failed: %v", result.Error)
	}
	return events
}

func TestContextUsageIsEmittedBeforeAndAfterTheModelCall(t *testing.T) {
	events := runWithUsageEvents(t, types.TokenUsage{InputTokens: 1_000, OutputTokens: 234, CacheReadInputTokens: 66})
	if len(events) != 2 {
		t.Fatalf("expected an estimate before the call and the reported figure after it, got %d events", len(events))
	}
	before, after := events[0], events[1]
	if before.Source != "estimate" || before.UsedTokens <= 0 {
		t.Errorf("before the call: %+v, want a positive estimate", before)
	}
	if after.Source != "reported" || after.UsedTokens != 1_300 {
		t.Errorf("after the call: %+v, want the reported 1,000 + 66 cached + 234 output = 1,300", after)
	}
	if after.WindowTokens <= 0 || after.AutoCompactAtTokens <= 0 || after.AutoCompactAtTokens >= after.BlockingLimitTokens {
		t.Errorf("thresholds out of order: %+v", after)
	}
}

func TestContextUsageFallsBackToTheEstimateWhenTheProviderReportsNothing(t *testing.T) {
	events := runWithUsageEvents(t, types.TokenUsage{})
	if len(events) != 2 || events[1].Source != "estimate" || events[1].UsedTokens <= 0 {
		t.Errorf("without usage the figure stays an estimate, got %+v", events)
	}
}

func TestContextUsageStaysSilentWithoutAnEmitter(t *testing.T) {
	loop := NewLoop(nil, nil, compact.NewEngine(nil, nil), nil, permissions.NewIntegrator(permissions.NewEngine()), nil,
		&LoopConfig{MaxIterations: 1, AutoCompact: true}, nil)
	// No emitter in the context: must not panic.
	loop.emitContextUsage(context.Background(), &MutableState{}, RunRequest{})
}
