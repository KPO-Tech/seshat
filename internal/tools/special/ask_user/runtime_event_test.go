package askuser

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// TestAskQuestionsEmitsPromptRequiredEvent guards H6 in TODO.md: prompt.request
// was defined and re-exported but nothing ever constructed it, so a host
// that only listens on the runtime-event channel (not PromptFn itself)
// never learned this tool was about to prompt the user.
func TestAskQuestionsEmitsPromptRequiredEvent(t *testing.T) {
	tool := NewTool(&Config{Timeout: DefaultConfig().Timeout})

	promptFn := func(ctx context.Context, req types.PromptRequest) (types.PromptResponse, error) {
		return types.PromptResponse{Value: "yes"}, nil
	}

	var events []types.RuntimeEvent
	ctx := types.WithPromptFn(context.Background(), promptFn)
	ctx = context.WithValue(ctx, types.RuntimeEventEmitterKey, func(e types.RuntimeEvent) {
		events = append(events, e)
	})

	input := &Input{Questions: []Question{{
		Question: "proceed?",
		Options:  []QuestionOption{{Label: "yes"}, {Label: "no"}},
	}}}

	if _, _, err := tool.askQuestions(ctx, input, "tool-use-1"); err != nil {
		t.Fatalf("askQuestions: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 runtime event, got %d: %#v", len(events), events)
	}
	event := events[0]
	if event.Type != types.RuntimeEventTypePromptRequired {
		t.Fatalf("expected event type %q, got %q", types.RuntimeEventTypePromptRequired, event.Type)
	}
	if event.PromptRequest == nil || event.PromptRequest.Message != "proceed?" {
		t.Fatalf("expected PromptRequest.Message=%q, got %#v", "proceed?", event.PromptRequest)
	}
}
