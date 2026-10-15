package execution

import (
	"context"
	"fmt"
	"sync"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// Every tool of a concurrent batch used to receive the same Metadata map, and each
// one wrote transcript_messages into it: "fatal error: concurrent map iteration and
// map write", which kills the process and cannot be recovered.
func TestToolRuntimeStateDoesNotShareMetadataWithTheBatch(t *testing.T) {
	o := &Orchestrator{}
	base := map[string]any{"session": "s1"}
	batchCtx := tool.ToolUseContext{Metadata: base}
	req := ExecuteRequest{Transcript: []types.Message{{}}}

	state := o.newRuntimeStateFromPrepared(preparedToolUse{toolUse: types.ToolUseContent{ID: "t1"}}, req, batchCtx)

	if _, leaked := base["transcript_messages"]; leaked {
		t.Fatal("the tool wrote into the metadata map it shares with the rest of the batch")
	}
	if _, ok := state.toolCtx.Metadata["transcript_messages"]; !ok {
		t.Fatal("the tool must still receive the transcript in its own metadata")
	}
	if state.toolCtx.Metadata["session"] != "s1" {
		t.Fatal("the batch metadata must be copied into the tool's own map")
	}
	state.toolCtx.Metadata["x"] = 1
	if _, leaked := base["x"]; leaked {
		t.Fatal("a later write by the tool reached the shared map")
	}
}

func TestConcurrentToolsPreparedFromOneBatchContext(t *testing.T) {
	o := &Orchestrator{}
	batchCtx := tool.ToolUseContext{Metadata: map[string]any{"a": 1, "b": 2, "c": 3}}
	req := ExecuteRequest{Transcript: []types.Message{{}}}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				state := o.newRuntimeStateFromPrepared(
					preparedToolUse{toolUse: types.ToolUseContent{ID: fmt.Sprintf("t%d", i)}}, req, batchCtx)
				// What the permission probe does with the tool's metadata.
				_ = cloneMetadata(state.toolCtx.Metadata)
			}
		}(i)
	}
	wg.Wait()
}

// The crash seen in a long session, end to end: several concurrency-safe tools in
// one turn, a transcript, and a permission check that reads the tool metadata.
// Run with -race: without the per-tool copy of the metadata it reports the race
// (and, without -race, the runtime may stop the process with a fatal error).
func TestConcurrentBatchWithTranscriptAndPermissionCheck(t *testing.T) {
	const tools = 8
	allow := func(ctx context.Context, request types.ToolPermissionRequest) types.PermissionResult {
		// What the real permission engine does with the request metadata.
		for range request.Metadata {
		}
		return types.AllowWithInput("allow", nil)
	}
	stub := &stubTool{
		isConcurrencySafe: func(map[string]any) bool { return true },
		isReadOnly:        func(map[string]any) bool { return true },
		call: func(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			// A tool may read what the engine put in its context.
			_ = input.ToolContext.Metadata["transcript_messages"]
			return tool.NewTextResult("ok"), nil
		},
	}

	for round := 0; round < 30; round++ {
		uses := make([]types.ToolUseContent, tools)
		for i := range uses {
			uses[i] = types.ToolUseContent{ID: fmt.Sprintf("t%d-%d", round, i), Name: "stub", Input: map[string]any{"i": i}}
		}
		result, err := NewOrchestrator().Execute(context.Background(), ExecuteRequest{
			ToolUses:        uses,
			Tools:           map[string]tool.Tool{"stub": stub},
			PermissionCheck: allow,
			SessionID:       types.SessionID("s"),
			TurnID:          types.TurnID("t"),
			PermissionMode:  types.PermissionModeOnRequest,
			Transcript:      []types.Message{{}, {}},
		})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(result.Results) != tools {
			t.Fatalf("round %d: %d results, want %d", round, len(result.Results), tools)
		}
	}
}
