package nexus

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestQueryResponseRuntimeFieldsRoundTrip(t *testing.T) {
	original := &QueryResponse{
		Content:        "delta",
		Model:          "anthropic:claude-test",
		ConversationId: "sess-1",
		Stopped:        false,
		ItemType:       "runtime_event",
		Chunk: &ChunkDelta{
			Type:      "content_block_delta",
			DeltaType: "text_delta",
			Delta:     "delta",
		},
		RuntimeEvent: &RuntimeEvent{
			Type:                "tool.progress",
			SessionId:           "sess-1",
			TurnId:              "turn-1",
			TurnNumber:          2,
			Timestamp:           "2026-05-09T10:11:12Z",
			StopReason:          "tool_use",
			TokenUsage:          &TokenUsage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
			Chunk:               &ChunkDelta{Type: "content_block_delta", DeltaType: "text_delta", Delta: "delta"},
			ToolName:            "bash",
			ToolStage:           "running",
			ToolMessage:         "calling tool",
			ToolPercentComplete: 66,
			Error:               "",
		},
	}

	wire, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded QueryResponse
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ItemType != "runtime_event" {
		t.Fatalf("expected item type to round-trip, got %q", decoded.ItemType)
	}
	if decoded.Chunk == nil || decoded.Chunk.Delta != "delta" {
		t.Fatalf("expected chunk payload to round-trip, got %#v", decoded.Chunk)
	}
	if decoded.RuntimeEvent == nil {
		t.Fatal("expected runtime event payload to round-trip")
	}
	if decoded.RuntimeEvent.ToolName != "bash" || decoded.RuntimeEvent.ToolStage != "running" {
		t.Fatalf("expected tool progress fields to round-trip, got %#v", decoded.RuntimeEvent)
	}
	if decoded.RuntimeEvent.TokenUsage == nil || decoded.RuntimeEvent.TokenUsage.TotalTokens != 14 {
		t.Fatalf("expected token usage to round-trip, got %#v", decoded.RuntimeEvent.TokenUsage)
	}
}
