package workspace

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/pubsub"
	"github.com/KPO-Tech/seshat/pkg/sdk"
)

func TestOnChunkStartsNewAssistantMessageAfterMessageStop(t *testing.T) {
	w := NewSeshatWorkspace(nil, "", "")
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
	w := NewSeshatWorkspace(nil, "", "")
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

func TestNewUserMessagePreservesAttachmentPayloads(t *testing.T) {
	now := time.Now().UnixMilli()
	msg := newUserMessage("session-1", "look at this", []message.Attachment{{
		FilePath: "paste_1.txt",
		FileName: "paste_1.txt",
		MimeType: "text/plain",
		Content:  []byte("hello world"),
	}}, now)

	if got := msg.Content().Text; got != "look at this" {
		t.Fatalf("expected prompt text to stay compact in transcript, got %q", got)
	}
	binary := msg.BinaryContent()
	if len(binary) != 1 {
		t.Fatalf("expected 1 binary attachment, got %d", len(binary))
	}
	if got := string(binary[0].Data); got != "hello world" {
		t.Fatalf("expected attachment payload to be preserved, got %q", got)
	}
	if got := binary[0].Path; got != "paste_1.txt" {
		t.Fatalf("expected attachment path paste_1.txt, got %q", got)
	}
}

func TestImageContentsFromAttachmentsFiltersAndEncodesImages(t *testing.T) {
	attachments := []message.Attachment{
		{FileName: "note.txt", MimeType: "text/plain", Content: []byte("skip")},
		{FileName: "shot.png", MimeType: "image/png", Content: []byte("png-bytes")},
	}

	images := imageContentsFromAttachments(attachments)
	if len(images) != 1 {
		t.Fatalf("expected 1 image content block, got %d", len(images))
	}
	if got := images[0].Source.Type; got != "base64" {
		t.Fatalf("expected source type base64, got %q", got)
	}
	if got := images[0].Source.MediaType; got != "image/png" {
		t.Fatalf("expected media type image/png, got %q", got)
	}
	if got := images[0].Source.Data; got != base64.StdEncoding.EncodeToString([]byte("png-bytes")) {
		t.Fatalf("unexpected encoded payload %q", got)
	}
}
