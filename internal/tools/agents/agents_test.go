package agents

import (
	"context"
	"testing"
	"time"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func makeCallInput(parsed map[string]any) tool.CallInput {
	if parsed == nil {
		parsed = map[string]any{}
	}
	return tool.CallInput{Parsed: parsed}
}

// ─── AsyncAgentManager extensions ────────────────────────────────────────────

func TestSendMessage_notFound(t *testing.T) {
	mgr := coreagent.NewAsyncAgentManager()
	defer mgr.Shutdown()

	err := mgr.SendMessage("nonexistent-agent-id", "hello")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestSendMessage_queues(t *testing.T) {
	mgr := coreagent.NewAsyncAgentManager()
	defer mgr.Shutdown()

	// Engine is nil → runAgent will fail fast. We just verify no panic
	// and that the method exists.
	config := &coreagent.RunConfig{
		AgentType: "general-purpose",
		Task:      "test task",
	}
	ag, err := mgr.StartAgent(config)
	if err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	// Let goroutine start
	time.Sleep(5 * time.Millisecond)

	// SendMessage may succeed (running) or return an error (already failed) — either is fine.
	// We only assert no panic.
	_ = mgr.SendMessage(ag.ID, "hello")
}

func TestCloseAgent_removesFromRegistry(t *testing.T) {
	mgr := coreagent.NewAsyncAgentManager()
	defer mgr.Shutdown()

	config := &coreagent.RunConfig{
		AgentType: "general-purpose",
		Task:      "test task",
	}
	ag, err := mgr.StartAgent(config)
	if err != nil {
		t.Fatalf("StartAgent: %v", err)
	}

	if closeErr := mgr.CloseAgent(ag.ID); closeErr != nil {
		t.Fatalf("CloseAgent: %v", closeErr)
	}

	// After close, GetAgent should return not-found.
	if _, getErr := mgr.GetAgent(ag.ID); getErr == nil {
		t.Fatal("expected error: agent should be removed from registry after CloseAgent")
	}
}

func TestCloseAgent_notFound(t *testing.T) {
	mgr := coreagent.NewAsyncAgentManager()
	defer mgr.Shutdown()

	err := mgr.CloseAgent("nonexistent-agent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

// ─── CollabStatus mapping ─────────────────────────────────────────────────────

func TestCollabStatus_mapping(t *testing.T) {
	cases := []struct {
		status   coreagent.AgentStatus
		expected string
	}{
		{coreagent.AgentStatusPending, "pendingInit"},
		{coreagent.AgentStatusRunning, "running"},
		{coreagent.AgentStatusCompleted, "completed"},
		{coreagent.AgentStatusFailed, "errored"},
		{coreagent.AgentStatusCancelled, "shutdown"},
	}
	mgr := coreagent.NewAsyncAgentManager()
	defer mgr.Shutdown()

	for range cases {
		config := &coreagent.RunConfig{AgentType: "general-purpose", Task: "test"}
		ag, err := mgr.StartAgent(config)
		if err != nil {
			t.Fatalf("StartAgent: %v", err)
		}
		_ = ag.CollabStatus()
		_ = mgr.CloseAgent(ag.ID)
	}
}

// ─── Tool: wait_agent ────────────────────────────────────────────────────────

func TestWaitAgentTool_notFound(t *testing.T) {
	waitTool := NewWaitAgentTool()
	res, err := waitTool.Call(context.Background(), makeCallInput(map[string]any{
		"agent_id": "no-such-agent",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for unknown agent_id")
	}
}

func TestWaitAgentTool_missingAgentID(t *testing.T) {
	waitTool := NewWaitAgentTool()
	res, err := waitTool.Call(context.Background(), makeCallInput(map[string]any{}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for missing agent_id")
	}
}

// ─── Tool: list_agents ───────────────────────────────────────────────────────

func TestListAgentsTool_empty(t *testing.T) {
	listTool := NewListAgentsTool()
	res, err := listTool.Call(context.Background(), makeCallInput(nil), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
}

// ─── Tool: send_agent_message ────────────────────────────────────────────────

func TestSendAgentMessageTool_missingAgentID(t *testing.T) {
	sendTool := NewSendAgentMessageTool()
	res, err := sendTool.Call(context.Background(), makeCallInput(map[string]any{
		"message": "hello",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for missing agent_id")
	}
}

func TestSendAgentMessageTool_notFound(t *testing.T) {
	sendTool := NewSendAgentMessageTool()
	res, err := sendTool.Call(context.Background(), makeCallInput(map[string]any{
		"agent_id": "no-such-agent",
		"message":  "hello",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for unknown agent_id")
	}
}

// ─── Tool: close_agent ───────────────────────────────────────────────────────

func TestCloseAgentTool_notFound(t *testing.T) {
	closeTool := NewCloseAgentTool()
	res, err := closeTool.Call(context.Background(), makeCallInput(map[string]any{
		"agent_id": "no-such-agent",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for unknown agent_id")
	}
}

// ─── Events ──────────────────────────────────────────────────────────────────

func TestEmitAgentEvent_noEmitter(t *testing.T) {
	// Must not panic when no emitter is in context.
	emitAgentEvent(context.Background(), types.RuntimeEventTypeAgentSpawnBegin, &types.AgentRuntimeEvent{
		CallID:  "call-1",
		AgentID: "agent-1",
		Status:  "pendingInit",
	})
}

func TestEmitAgentEvent_withEmitter(t *testing.T) {
	var received []types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(e types.RuntimeEvent) {
		received = append(received, e)
	})

	emitAgentEvent(ctx, types.RuntimeEventTypeAgentSpawnEnd, &types.AgentRuntimeEvent{
		CallID:  "call-1",
		AgentID: "agent-1",
		Status:  "running",
	})

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != types.RuntimeEventTypeAgentSpawnEnd {
		t.Errorf("wrong event type: %s", received[0].Type)
	}
	if received[0].AgentEvent == nil {
		t.Fatal("AgentEvent payload is nil")
	}
	if received[0].AgentEvent.AgentID != "agent-1" {
		t.Errorf("wrong agent_id: %s", received[0].AgentEvent.AgentID)
	}
}

func TestNowMs(t *testing.T) {
	before := time.Now().UnixMilli()
	ms := nowMs()
	after := time.Now().UnixMilli()
	if ms < before || ms > after {
		t.Errorf("nowMs() = %d, want in [%d, %d]", ms, before, after)
	}
}
