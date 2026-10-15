package execution

import (
	"context"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// What a tool starts (a sub-agent, a background task) records which session it
// belongs to, from the context of the call. Without it every sub-agent session
// looked like a conversation of its own.
func TestToolCallCarriesTheCallingSession(t *testing.T) {
	var seen types.SessionID
	stub := &stubTool{
		call: func(ctx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			seen = types.CallingSessionIDFromContext(ctx)
			return tool.NewTextResult("ok"), nil
		},
	}
	allow := func(ctx context.Context, request types.ToolPermissionRequest) types.PermissionResult {
		return types.AllowWithInput("allow", nil)
	}
	_, err := NewOrchestrator().Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: allow,
		SessionID:       types.SessionID("conversation-1"),
		TurnID:          types.TurnID("turn-1"),
		PermissionMode:  types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != "conversation-1" {
		t.Fatalf("the tool saw calling session %q, want conversation-1", seen)
	}
}

func TestCallingSessionContextHelpers(t *testing.T) {
	if got := types.CallingSessionIDFromContext(context.Background()); got != "" {
		t.Fatalf("empty context gave %q", got)
	}
	ctx := types.WithCallingSessionID(context.Background(), "s1")
	if types.WithCallingSessionID(ctx, "") != ctx {
		t.Fatal("an empty id must leave the context as it is")
	}
	if got := types.CallingSessionIDFromContext(types.WithCallingSessionID(ctx, "s2")); got != "s2" {
		t.Fatalf("the inner session must win (a sub-agent's own calls), got %q", got)
	}
}
