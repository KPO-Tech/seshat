package goal

import (
	"context"
	"strings"
	"testing"

	coregoal "github.com/EngineerProjects/nexus-engine/internal/agent/goal"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTools() (*CreateGoalTool, *GetGoalTool, *UpdateGoalTool) {
	store := coregoal.NewStore()
	return &CreateGoalTool{store: store},
		&GetGoalTool{store: store},
		&UpdateGoalTool{store: store}
}

func callInput(sessionID string, parsed map[string]any) tool.CallInput {
	return tool.CallInput{
		Parsed:    parsed,
		SessionID: types.SessionID(sessionID),
	}
}

func ctxWithEmitter(captured *[]types.RuntimeEvent) context.Context {
	emitter := func(e types.RuntimeEvent) { *captured = append(*captured, e) }
	return context.WithValue(context.Background(), types.RuntimeEventEmitterKey, emitter)
}

// ─── create_goal ──────────────────────────────────────────────────────────────

func TestCreateGoal_HappyPath(t *testing.T) {
	ct, _, _ := newTools()
	res, err := ct.Call(context.Background(), callInput("sess1", map[string]any{
		"objective": "Implement the payment flow",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success, got error: %v", res.Data)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res.Data)
	}
	if m["objective"] != "Implement the payment flow" {
		t.Errorf("objective mismatch: %v", m["objective"])
	}
	if m["status"] != "active" {
		t.Errorf("expected active status, got %v", m["status"])
	}
	if _, ok := m["message"]; !ok {
		t.Error("result should contain a message field")
	}
}

func TestCreateGoal_WithTokenBudget(t *testing.T) {
	ct, _, _ := newTools()
	res, err := ct.Call(context.Background(), callInput("sess2", map[string]any{
		"objective":    "Write tests for module X",
		"token_budget": float64(5000),
	}), nil)
	if err != nil || res.IsError() {
		t.Fatalf("unexpected error: %v / %v", err, res.Data)
	}
	m := res.Data.(map[string]any)
	if m["token_budget"] != int64(5000) {
		t.Errorf("token_budget mismatch: %v", m["token_budget"])
	}
	remaining, ok := m["remaining_tokens"]
	if !ok {
		t.Fatal("expected remaining_tokens field when budget is set")
	}
	if remaining != int64(5000) {
		t.Errorf("remaining_tokens should equal budget at creation, got %v", remaining)
	}
}

func TestCreateGoal_EmptyObjectiveErrors(t *testing.T) {
	ct, _, _ := newTools()
	res, err := ct.Call(context.Background(), callInput("sess3", map[string]any{
		"objective": "   ",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result for empty objective")
	}
}

func TestCreateGoal_ReplacesExistingGoal(t *testing.T) {
	ct, gt, _ := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess4", map[string]any{"objective": "First goal"}), nil)
	_, _ = ct.Call(context.Background(), callInput("sess4", map[string]any{"objective": "Second goal"}), nil)

	res, _ := gt.Call(context.Background(), callInput("sess4", nil), nil)
	m := res.Data.(map[string]any)["goal"].(map[string]any)
	if m["objective"] != "Second goal" {
		t.Errorf("expected second goal to replace first, got %v", m["objective"])
	}
}

func TestCreateGoal_EmitsEvent(t *testing.T) {
	ct, _, _ := newTools()
	var events []types.RuntimeEvent
	ctx := ctxWithEmitter(&events)

	_, _ = ct.Call(ctx, callInput("sess5", map[string]any{"objective": "Track event emission"}), nil)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.RuntimeEventTypeGoalUpdated {
		t.Errorf("expected goal.updated event, got %q", events[0].Type)
	}
	if events[0].GoalEvent == nil {
		t.Fatal("expected GoalEvent payload")
	}
	if events[0].GoalEvent.Objective != "Track event emission" {
		t.Errorf("event objective mismatch: %q", events[0].GoalEvent.Objective)
	}
}

func TestCreateGoal_DefaultSessionIDWhenEmpty(t *testing.T) {
	ct, gt, _ := newTools()
	// No SessionID → falls back to "default"
	_, _ = ct.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"objective": "default session goal"},
	}, nil)
	res, _ := gt.Call(context.Background(), tool.CallInput{Parsed: nil}, nil)
	m := res.Data.(map[string]any)["goal"].(map[string]any)
	if m["session_id"] != "default" {
		t.Errorf("expected session_id 'default', got %v", m["session_id"])
	}
}

// ─── get_goal ─────────────────────────────────────────────────────────────────

func TestGetGoal_NoGoalReturnsNull(t *testing.T) {
	_, gt, _ := newTools()
	res, err := gt.Call(context.Background(), callInput("empty-sess", nil), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success, got error: %v", res.Data)
	}
	m := res.Data.(map[string]any)
	if m["goal"] != nil {
		t.Errorf("expected goal=null, got %v", m["goal"])
	}
	if !strings.Contains(res.Content, "No active goal") {
		t.Errorf("content should mention no active goal, got: %q", res.Content)
	}
}

func TestGetGoal_ExistingGoalReturnsMap(t *testing.T) {
	ct, gt, _ := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess6", map[string]any{"objective": "Build CI pipeline"}), nil)

	res, err := gt.Call(context.Background(), callInput("sess6", nil), nil)
	if err != nil || res.IsError() {
		t.Fatalf("unexpected error: %v / %v", err, res.Data)
	}
	outer := res.Data.(map[string]any)
	goalMap, ok := outer["goal"].(map[string]any)
	if !ok || goalMap == nil {
		t.Fatal("expected goal map in response")
	}
	if goalMap["objective"] != "Build CI pipeline" {
		t.Errorf("objective mismatch: %v", goalMap["objective"])
	}
	if goalMap["status"] != "active" {
		t.Errorf("expected active status, got %v", goalMap["status"])
	}
}

func TestGetGoal_ContentSummary(t *testing.T) {
	ct, gt, _ := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess7", map[string]any{"objective": "Refactor auth module"}), nil)
	res, _ := gt.Call(context.Background(), callInput("sess7", nil), nil)
	if !strings.Contains(res.Content, "Refactor auth module") {
		t.Errorf("content should contain objective, got: %q", res.Content)
	}
}

// ─── update_goal ──────────────────────────────────────────────────────────────

func TestUpdateGoal_NoGoalErrors(t *testing.T) {
	_, _, ut := newTools()
	res, err := ut.Call(context.Background(), callInput("no-goal-sess", map[string]any{
		"status": "complete",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError() {
		t.Fatal("expected error result when no goal exists")
	}
}

func TestUpdateGoal_StatusOnly(t *testing.T) {
	ct, gt, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess8", map[string]any{"objective": "Deploy backend"}), nil)

	res, err := ut.Call(context.Background(), callInput("sess8", map[string]any{"status": "paused"}), nil)
	if err != nil || res.IsError() {
		t.Fatalf("unexpected error: %v / %v", err, res.Data)
	}
	getRes, _ := gt.Call(context.Background(), callInput("sess8", nil), nil)
	m := getRes.Data.(map[string]any)["goal"].(map[string]any)
	if m["status"] != "paused" {
		t.Errorf("expected paused, got %v", m["status"])
	}
	if m["objective"] != "Deploy backend" {
		t.Errorf("objective should be unchanged, got %v", m["objective"])
	}
}

func TestUpdateGoal_ObjectiveOnly(t *testing.T) {
	ct, gt, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess9", map[string]any{"objective": "Old objective"}), nil)

	_, err := ut.Call(context.Background(), callInput("sess9", map[string]any{"objective": "New refined objective"}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	getRes, _ := gt.Call(context.Background(), callInput("sess9", nil), nil)
	m := getRes.Data.(map[string]any)["goal"].(map[string]any)
	if m["objective"] != "New refined objective" {
		t.Errorf("objective not updated: %v", m["objective"])
	}
	if m["status"] != "active" {
		t.Errorf("status should be unchanged: %v", m["status"])
	}
}

func TestUpdateGoal_BothFields(t *testing.T) {
	ct, gt, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess10", map[string]any{"objective": "Draft plan"}), nil)

	_, _ = ut.Call(context.Background(), callInput("sess10", map[string]any{
		"status":    "blocked",
		"objective": "Draft plan — blocked on missing API spec",
	}), nil)
	getRes, _ := gt.Call(context.Background(), callInput("sess10", nil), nil)
	m := getRes.Data.(map[string]any)["goal"].(map[string]any)
	if m["status"] != "blocked" {
		t.Errorf("expected blocked, got %v", m["status"])
	}
	if m["objective"] != "Draft plan — blocked on missing API spec" {
		t.Errorf("objective not updated: %v", m["objective"])
	}
}

func TestUpdateGoal_EmitsEvent(t *testing.T) {
	ct, _, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess11", map[string]any{"objective": "Track updates"}), nil)

	var events []types.RuntimeEvent
	ctx := ctxWithEmitter(&events)
	_, _ = ut.Call(ctx, callInput("sess11", map[string]any{"status": "complete"}), nil)

	if len(events) != 1 {
		t.Fatalf("expected 1 event from update, got %d", len(events))
	}
	if events[0].GoalEvent.Status != "complete" {
		t.Errorf("event status mismatch: %q", events[0].GoalEvent.Status)
	}
}

func TestUpdateGoal_SummaryPrefixComplete(t *testing.T) {
	ct, _, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess12", map[string]any{"objective": "Finish migration"}), nil)
	res, _ := ut.Call(context.Background(), callInput("sess12", map[string]any{"status": "complete"}), nil)
	if !strings.Contains(res.Content, "Goal marked complete") {
		t.Errorf("expected 'Goal marked complete' prefix, got: %q", res.Content)
	}
}

func TestUpdateGoal_SummaryPrefixObjectiveOnly(t *testing.T) {
	ct, _, ut := newTools()
	_, _ = ct.Call(context.Background(), callInput("sess13", map[string]any{"objective": "Old"}), nil)
	res, _ := ut.Call(context.Background(), callInput("sess13", map[string]any{"objective": "New scope"}), nil)
	if !strings.Contains(res.Content, "Goal objective updated") {
		t.Errorf("expected 'Goal objective updated' prefix, got: %q", res.Content)
	}
}

// ─── ValidateInput ────────────────────────────────────────────────────────────

func TestUpdateGoal_ValidateInput_NoFields(t *testing.T) {
	_, _, ut := newTools()
	_, err := ut.ValidateInput(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when neither status nor objective provided")
	}
}

func TestUpdateGoal_ValidateInput_InvalidStatus(t *testing.T) {
	_, _, ut := newTools()
	_, err := ut.ValidateInput(context.Background(), map[string]any{"status": "budgetLimited"})
	if err == nil {
		t.Fatal("expected error for invalid status 'budgetLimited'")
	}
}

func TestUpdateGoal_ValidateInput_ValidStatuses(t *testing.T) {
	_, _, ut := newTools()
	for _, s := range []string{"active", "paused", "blocked", "complete"} {
		_, err := ut.ValidateInput(context.Background(), map[string]any{"status": s})
		if err != nil {
			t.Errorf("expected status %q to be valid, got error: %v", s, err)
		}
	}
}

// ─── shared helpers ───────────────────────────────────────────────────────────

func TestGoalToMap_WithBudget(t *testing.T) {
	store := coregoal.NewStore()
	budget := int64(1000)
	g := store.Set("sess", "objective", &budget)
	m := goalToMap(g)
	if _, ok := m["token_budget"]; !ok {
		t.Error("expected token_budget key")
	}
	if _, ok := m["remaining_tokens"]; !ok {
		t.Error("expected remaining_tokens key")
	}
}

func TestGoalToMap_NoBudget(t *testing.T) {
	store := coregoal.NewStore()
	g := store.Set("sess", "objective", nil)
	m := goalToMap(g)
	if _, ok := m["token_budget"]; ok {
		t.Error("token_budget should be absent when no budget")
	}
	if _, ok := m["remaining_tokens"]; ok {
		t.Error("remaining_tokens should be absent when no budget")
	}
}

func TestFormatGoalSummary_WithBudget(t *testing.T) {
	store := coregoal.NewStore()
	budget := int64(500)
	g := store.Set("sess", "Deploy to prod", &budget)
	s := formatGoalSummary("Test", g)
	if !strings.Contains(s, "Budget:") {
		t.Errorf("expected Budget: in summary, got: %q", s)
	}
}

func TestFormatGoalSummary_NoBudget(t *testing.T) {
	store := coregoal.NewStore()
	g := store.Set("sess", "Deploy to prod", nil)
	s := formatGoalSummary("Test", g)
	if !strings.Contains(s, "no budget") {
		t.Errorf("expected 'no budget' in summary, got: %q", s)
	}
}

func TestEmitGoalEvent_NoEmitter(t *testing.T) {
	store := coregoal.NewStore()
	g := store.Set("sess", "obj", nil)
	// Must not panic.
	emitGoalEvent(context.Background(), g)
}

func TestEmitGoalEvent_WithEmitter(t *testing.T) {
	store := coregoal.NewStore()
	g := store.Set("sess", "Captured objective", nil)
	var events []types.RuntimeEvent
	ctx := ctxWithEmitter(&events)
	emitGoalEvent(ctx, g)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].GoalEvent.Objective != "Captured objective" {
		t.Errorf("objective mismatch: %q", events[0].GoalEvent.Objective)
	}
	if events[0].GoalEvent.Status != "active" {
		t.Errorf("status mismatch: %q", events[0].GoalEvent.Status)
	}
}
