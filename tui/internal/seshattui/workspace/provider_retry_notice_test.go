package workspace

import (
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/pubsub"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/session"
)

func TestProviderRetryNoticeWording(t *testing.T) {
	got := providerRetryNotice(&sdk.ProviderRetryRuntimeEvent{Attempt: 2, MaxAttempts: 5, WaitMs: 12000, Reason: "rate limited (429)"})
	want := "rate limited (429), retrying in 12s (attempt 2/5)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatNoticeWait(t *testing.T) {
	for in, want := range map[time.Duration]string{
		800 * time.Millisecond:  "800ms",
		1200 * time.Millisecond: "1s",
		12 * time.Second:        "12s",
		90 * time.Second:        "1m30s",
	} {
		if got := formatNoticeWait(in); got != want {
			t.Errorf("formatNoticeWait(%v) = %q, want %q", in, got, want)
		}
	}
}

func newNoticeWorkspace(t *testing.T, id string) (*SeshatWorkspace, <-chan pubsub.Event[session.Session]) {
	t.Helper()
	w := &SeshatWorkspace{
		sessStore:  map[string]session.Session{id: {ID: id}},
		sessBroker: pubsub.NewBroker[session.Session](),
	}
	ctx := t.Context()
	return w, w.sessBroker.Subscribe(ctx)
}

func TestARetryEventSetsTheNoticeAndTheNextChunkClearsIt(t *testing.T) {
	w, events := newNoticeWorkspace(t, "s1")

	w.OnRuntimeEvent(sdk.RuntimeEvent{
		Type:               sdk.RuntimeEventTypeProviderRetry,
		SessionID:          "s1",
		ProviderRetryEvent: &sdk.ProviderRetryRuntimeEvent{Attempt: 2, MaxAttempts: 5, WaitMs: 3000, Reason: "server error (503)"},
	})
	if got := w.sessStore["s1"].Notice; got != "server error (503), retrying in 3s (attempt 2/5)" {
		t.Fatalf("the notice must be set, got %q", got)
	}
	select {
	case ev := <-events:
		if ev.Payload.Notice == "" {
			t.Error("the UI must be told about the notice")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the session update must be published")
	}

	w.OnRuntimeEvent(sdk.RuntimeEvent{Type: sdk.RuntimeEventTypeResponseChunk, SessionID: "s1"})
	if got := w.sessStore["s1"].Notice; got != "" {
		t.Errorf("a response chunk means the turn moves again, the notice must clear, got %q", got)
	}
}

func TestChunksWithNoNoticeDoNotPublishAnything(t *testing.T) {
	w, events := newNoticeWorkspace(t, "s2")
	for i := 0; i < 50; i++ {
		w.OnRuntimeEvent(sdk.RuntimeEvent{Type: sdk.RuntimeEventTypeResponseChunk, SessionID: "s2"})
	}
	select {
	case <-events:
		t.Error("response chunks arrive by the hundred: with no notice to clear they must publish nothing")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAnEventOfAnotherSessionDoesNotTouchTheNotice(t *testing.T) {
	w, _ := newNoticeWorkspace(t, "s3")
	w.OnRuntimeEvent(sdk.RuntimeEvent{
		Type:               sdk.RuntimeEventTypeProviderRetry,
		SessionID:          "other",
		ProviderRetryEvent: &sdk.ProviderRetryRuntimeEvent{Attempt: 2, MaxAttempts: 5, WaitMs: 1000, Reason: "rate limited (429)"},
	})
	if got := w.sessStore["s3"].Notice; got != "" {
		t.Errorf("a notice for another session must not leak, got %q", got)
	}
}
