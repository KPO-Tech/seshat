package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	"github.com/KPO-Tech/seshat-runtime/internal/prompt"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

func assistantWithCalls(ids ...string) types.Message {
	message := types.Message{ID: "assistant", Role: types.RoleAssistant}
	for _, id := range ids {
		message.Content = append(message.Content, types.ToolUseContent{ID: id, Name: "stub", Input: map[string]any{"value": "x"}})
	}
	return message
}

func resultFor(id string) types.Message {
	message := types.UserMessage("result-"+id, "")
	message.Content = []types.ContentBlock{types.ToolResultContent{ToolUseID: id, Content: "ok"}}
	return message
}

// answeredIDs lists, in order, the tool results of a transcript.
func answeredIDs(messages []types.Message) []string {
	var ids []string
	for _, message := range messages {
		for _, block := range message.Content {
			if result, ok := block.(types.ToolResultContent); ok {
				ids = append(ids, result.ToolUseID)
			}
		}
	}
	return ids
}

func TestRepairLeavesAValidTranscriptAlone(t *testing.T) {
	in := []types.Message{
		types.UserMessage("u", "hi"),
		assistantWithCalls("a", "b"),
		resultFor("a"),
		resultFor("b"),
		types.UserMessage("u2", "next"),
	}
	if out := repairUnansweredToolCalls(in); len(out) != len(in) {
		t.Fatalf("a complete transcript must not change: %d messages became %d", len(in), len(out))
	}
}

func TestRepairAnswersACallCutBeforeAnyResult(t *testing.T) {
	out := repairUnansweredToolCalls([]types.Message{types.UserMessage("u", "hi"), assistantWithCalls("a")})
	if got := answeredIDs(out); len(got) != 1 || got[0] != "a" {
		t.Fatalf("the call must get a result, got %v", got)
	}
	result := out[len(out)-1].Content[0].(types.ToolResultContent)
	if !result.IsError || !strings.Contains(result.Content, "may or may not have run") {
		t.Fatalf("the result must be an error that says the call may have run, got %+v", result)
	}
}

func TestRepairAnswersOnlyTheCallsThatHaveNoResult(t *testing.T) {
	out := repairUnansweredToolCalls([]types.Message{
		types.UserMessage("u", "hi"),
		assistantWithCalls("a", "b", "c"),
		resultFor("a"),
	})
	got := answeredIDs(out)
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("want results a,b,c in order, got %v", got)
	}
	// The result of "a" is the real one, not a replacement.
	first := out[2].Content[0].(types.ToolResultContent)
	if first.IsError || first.Content != "ok" {
		t.Fatalf("an existing result must be kept as is, got %+v", first)
	}
}

func TestRepairDoesNotTouchAnEarlierCompleteExchange(t *testing.T) {
	out := repairUnansweredToolCalls([]types.Message{
		types.UserMessage("u", "hi"),
		assistantWithCalls("a"),
		resultFor("a"),
		assistantWithCalls("b"),
	})
	if got := strings.Join(answeredIDs(out), ","); got != "a,b" {
		t.Fatalf("want a,b, got %s", got)
	}
}

func TestInterruptedTurnNoteSaysWhy(t *testing.T) {
	turn := types.TurnID("t1")
	if text := interruptedTurnNote(context.Canceled, turn).Content[0].(types.TextContent).Text; !strings.Contains(text, "interrupted by the user") || !strings.Contains(text, "do not start over") {
		t.Errorf("cancel note: %q", text)
	}
	text := interruptedTurnNote(errors.New("provider returned 529\nsecond line"), turn).Content[0].(types.TextContent).Text
	if !strings.Contains(text, "because of an error: provider returned 529") || strings.Contains(text, "second line") {
		t.Errorf("error note must carry the first line only: %q", text)
	}
}

// failingTurnEngine returns an engine whose first model call asks for a tool and
// whose second model call fails, so the turn ends in an error after real work.
func failingTurnEngine(t *testing.T, afterWork func(call int, state *MutableState, req RunRequest) (*types.APIResponse, error)) (*Engine, *int) {
	t.Helper()
	reg := registry.NewRegistry()
	if err := reg.Register(loopTestTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	config := DefaultConfig()
	config.PermissionMode = types.PermissionModeBypass
	engine := NewEngine(nil, execution.NewOrchestrator(), nil, prompt.NewAssembler(),
		permissions.NewIntegrator(permissions.NewEngine()), reg, nil, config, nil, nil)
	engine.loop.config.AutoCompact = false
	calls := 0
	engine.loop.callModelFn = func(ctx context.Context, state *MutableState, req RunRequest) (*types.APIResponse, error) {
		calls++
		if calls == 1 {
			return &types.APIResponse{
				Role:       types.RoleAssistant,
				Content:    []types.ContentBlock{types.ToolUseContent{ID: "call-1", Name: "stub", Input: map[string]any{"value": "x"}}},
				StopReason: types.StopReasonToolUse,
				Model:      req.Model,
				ID:         "resp-1",
			}, nil
		}
		return afterWork(calls, state, req)
	}
	return engine, &calls
}

func TestAFailedTurnKeepsItsWorkForTheNextMessage(t *testing.T) {
	// The loop retries a failed model call, so the provider keeps failing for
	// the whole first turn and recovers for the second.
	providerDown := true
	engine, _ := failingTurnEngine(t, func(call int, state *MutableState, req RunRequest) (*types.APIResponse, error) {
		if providerDown {
			return nil, errors.New("provider stream failed")
		}
		// The next user message must see the work of the failed turn.
		if got := strings.Join(answeredIDs(state.Messages), ","); got != "call-1" {
			t.Errorf("the failed turn's tool result must still be in the conversation, got %q", got)
		}
		last := state.Messages[len(state.Messages)-1]
		if last.Role != types.RoleUser {
			t.Errorf("the request must end with the user's message, got role %s", last.Role)
		}
		return &types.APIResponse{Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextContent{Text: "resumed"}}, StopReason: types.StopReasonEndTurn, Model: req.Model, ID: "resp-3"}, nil
	})
	session, err := engine.NewSession(context.Background())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if _, err := session.SubmitMessage(context.Background(), "do the long task"); err == nil {
		t.Fatal("the first turn must fail")
	}
	if session.state.TurnNumber != 0 {
		t.Fatalf("a failed turn must not count as a completed turn, turn number is %d", session.state.TurnNumber)
	}
	if got := strings.Join(answeredIDs(session.state.Messages), ","); got != "call-1" {
		t.Fatalf("the work of the failed turn must be kept, tool results: %q", got)
	}
	noteText := ""
	for _, block := range session.state.Messages[len(session.state.Messages)-1].Content {
		if text, ok := block.(types.TextContent); ok {
			noteText = text.Text
		}
	}
	if !strings.Contains(noteText, "provider stream failed") {
		t.Fatalf("the transcript must end with a note that names the error, got %q", noteText)
	}

	providerDown = false
	response, err := session.SubmitMessage(context.Background(), "continue")
	if err != nil {
		t.Fatalf("continue must work after a failed turn: %v", err)
	}
	if response.StopReason != types.StopReasonEndTurn {
		t.Fatalf("unexpected stop reason %q", response.StopReason)
	}
}

func TestAFailedTurnThatDidNothingLeavesTheTranscriptAlone(t *testing.T) {
	reg := registry.NewRegistry()
	config := DefaultConfig()
	config.PermissionMode = types.PermissionModeBypass
	engine := NewEngine(nil, execution.NewOrchestrator(), nil, prompt.NewAssembler(),
		permissions.NewIntegrator(permissions.NewEngine()), reg, nil, config, nil, nil)
	engine.loop.config.AutoCompact = false
	engine.loop.callModelFn = func(ctx context.Context, state *MutableState, req RunRequest) (*types.APIResponse, error) {
		return nil, errors.New("provider down")
	}
	session, err := engine.NewSession(context.Background())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if _, err := session.SubmitMessage(context.Background(), "hello"); err == nil {
		t.Fatal("the turn must fail")
	}
	if n := len(session.state.Messages); n != 1 {
		t.Fatalf("only the user's message must remain, got %d messages", n)
	}
}

func TestACancelledTurnKeepsItsWorkAndSaysTheUserInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine, _ := failingTurnEngine(t, func(call int, state *MutableState, req RunRequest) (*types.APIResponse, error) {
		cancel() // the user presses Esc while the model is answering
		return nil, ctx.Err()
	})
	session, err := engine.NewSession(context.Background())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if _, err := session.SubmitMessage(ctx, "do the long task"); err == nil {
		t.Fatal("a cancelled turn must return an error")
	}
	if got := strings.Join(answeredIDs(session.state.Messages), ","); got != "call-1" {
		t.Fatalf("the work done before the cancel must be kept, tool results: %q", got)
	}
	last := session.state.Messages[len(session.state.Messages)-1]
	text, _ := last.Content[0].(types.TextContent)
	if !strings.Contains(text.Text, "interrupted by the user") {
		t.Fatalf("the transcript must end with the interruption note, got %q", text.Text)
	}
	if session.state.TurnNumber != 0 {
		t.Fatalf("a cancelled turn must not count as completed, turn number is %d", session.state.TurnNumber)
	}
}
