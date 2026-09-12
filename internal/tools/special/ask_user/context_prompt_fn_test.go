package askuser

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

// TestConcurrentTurnsUseTheirOwnContextPromptFn is the regression test for
// the exact bug this file's production code was changed to prevent: a host
// that reuses one *Tool instance across concurrent turns (e.g. a per-user/
// provider sdk.Client cache in seshat-backend) must never have one turn's
// question routed to another turn's prompt handler. Before the fix, the only
// routing mechanism was t.promptFn, a single field mutated via SetPromptFn -
// two concurrent turns sharing one *Tool would race on it. Now each turn
// carries its own promptFn via types.WithPromptFn(ctx, fn), resolved fresh
// per call via effectivePromptFn - this test runs many such "turns"
// concurrently on ONE shared *Tool (with no construction-time default) and
// verifies every one gets exactly its own answer back, never another's.
// Run with -race to also catch any literal data race, not just a logical mixup.
func TestConcurrentTurnsUseTheirOwnContextPromptFn(t *testing.T) {
	tool := NewTool(&Config{Timeout: DefaultConfig().Timeout}) // no PromptFn - forces context resolution

	const turns = 50
	var wg sync.WaitGroup
	errs := make(chan error, turns)

	for i := 0; i < turns; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			wantAnswer := fmt.Sprintf("answer-%d", i)
			promptFn := func(ctx context.Context, req types.PromptRequest) (types.PromptResponse, error) {
				// Confirms this call was routed to the goroutine that
				// injected it - not a different concurrently-running turn.
				gotToolUseID, _ := req.Metadata["tool_use_id"].(string)
				wantToolUseID := fmt.Sprintf("tool-use-%d", i)
				if gotToolUseID != wantToolUseID {
					return types.PromptResponse{}, fmt.Errorf("turn %d: got tool_use_id %q, want %q (cross-talk between concurrent turns)", i, gotToolUseID, wantToolUseID)
				}
				return types.PromptResponse{Value: wantAnswer}, nil
			}

			ctx := types.WithPromptFn(context.Background(), promptFn)
			input := &Input{Questions: []Question{{
				Question: fmt.Sprintf("question-%d?", i),
				Options:  []QuestionOption{{Label: "yes"}, {Label: "no"}},
			}}}

			answers, _, err := tool.askQuestions(ctx, input, fmt.Sprintf("tool-use-%d", i))
			if err != nil {
				errs <- fmt.Errorf("turn %d: %w", i, err)
				return
			}
			got := answers[input.Questions[0].Question]
			if got != wantAnswer {
				errs <- fmt.Errorf("turn %d: got answer %q, want %q (cross-talk between concurrent turns)", i, got, wantAnswer)
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
