package attachments

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat/internal/seshattui/message"
)

func TestHandleClickRemovesAttachmentAtDeleteChip(t *testing.T) {
	renderer := NewRenderer(
		lipgloss.NewStyle().Padding(0, 1).MarginRight(1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
	)
	m := New(renderer, Keymap{})
	_ = m.Update(message.Attachment{FileName: "paste_1.txt", MimeType: "text/plain"})
	_ = m.Update(message.Attachment{FileName: "paste_2.txt", MimeType: "text/plain"})

	_ = m.Render(120)
	if len(renderer.deleteHits) != 2 {
		t.Fatalf("expected 2 delete hit zones, got %d", len(renderer.deleteHits))
	}

	hit := renderer.deleteHits[0]
	if !m.HandleClick(hit.startX, 0, 120) {
		t.Fatal("expected delete click to be handled")
	}
	if got := len(m.List()); got != 1 {
		t.Fatalf("expected 1 attachment after delete, got %d", got)
	}
	if got := m.List()[0].FileName; got != "paste_2.txt" {
		t.Fatalf("expected remaining attachment paste_2.txt, got %q", got)
	}
}

func TestTranscriptRenderDoesNotShowDeleteAffordance(t *testing.T) {
	renderer := NewRenderer(
		lipgloss.NewStyle().Padding(0, 1).MarginRight(1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
		lipgloss.NewStyle().Padding(0, 1),
	)
	attachment := []message.Attachment{{FileName: "paste_1.txt", MimeType: "text/plain"}}

	composer := renderer.RenderComposer(attachment, false, 120)
	transcript := renderer.Render(attachment, false, 120)

	if !strings.Contains(composer, "×") {
		t.Fatal("expected composer render to include delete affordance")
	}
	if strings.Contains(transcript, "×") {
		t.Fatal("expected transcript render to omit delete affordance")
	}
}
