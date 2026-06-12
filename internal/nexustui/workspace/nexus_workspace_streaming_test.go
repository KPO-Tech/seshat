package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func TestOnChunkStartsNewAssistantMessageAfterMessageStop(t *testing.T) {
	w := NewNexusWorkspace(nil, "", "")
	sessionID := "session-1"
	initial := w.newStreamingAssistantMessage(sessionID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := w.msgBroker.Subscribe(ctx)

	w.streamMu.Lock()
	w.streamMsg = &initial
	w.streamSess = sessionID
	w.streamResponseDone = true
	w.streamMu.Unlock()

	w.OnChunk(sdk.ResponseChunk{
		Type:      sdk.ResponseChunkTypeContentBlockDelta,
		DeltaType: "text_delta",
		Delta:     "done",
	})

	msgs := w.msgStore[sessionID]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 assistant messages, got %d", len(msgs))
	}
	if got := msgs[0].Content().Text; got != "" {
		t.Fatalf("expected first assistant message to remain empty, got %q", got)
	}
	if got := msgs[1].Content().Text; got != "done" {
		t.Fatalf("expected second assistant message text %q, got %q", "done", got)
	}
	if msgs[0].ID == msgs[1].ID {
		t.Fatal("expected a new assistant message ID after message_stop")
	}

	select {
	case ev := <-events:
		if ev.Type != pubsub.CreatedEvent {
			t.Fatalf("expected created event, got %q", ev.Type)
		}
		if ev.Payload.ID != msgs[1].ID {
			t.Fatalf("expected created event for %q, got %q", msgs[1].ID, ev.Payload.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for created event")
	}
}

func TestOnChunkDoesNotSplitWhileSameResponseIsStillStreaming(t *testing.T) {
	w := NewNexusWorkspace(nil, "", "")
	sessionID := "session-1"
	initial := w.newStreamingAssistantMessage(sessionID)

	w.streamMu.Lock()
	w.streamMsg = &initial
	w.streamSess = sessionID
	w.streamResponseDone = false
	w.streamMu.Unlock()

	cur := w.streamMsg
	cur.AddToolCall(message.ToolCall{ID: "tool-1", Name: "stub"})
	cur.AddToolResult(message.ToolResult{ToolCallID: "tool-1", Name: "stub", Content: "ok"})
	w.updateMsg(sessionID, *cur)

	w.OnChunk(sdk.ResponseChunk{
		Type:      sdk.ResponseChunkTypeContentBlockDelta,
		DeltaType: "text_delta",
		Delta:     "same response",
	})

	msgs := w.msgStore[sessionID]
	if len(msgs) != 1 {
		t.Fatalf("expected 1 assistant message, got %d", len(msgs))
	}
	if got := msgs[0].Content().Text; got != "same response" {
		t.Fatalf("expected text to stay on the current assistant message, got %q", got)
	}
}
