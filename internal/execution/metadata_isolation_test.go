package execution

import (
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
