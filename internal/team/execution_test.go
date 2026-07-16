package team_test

import (
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/mailbox"
	"github.com/KPO-Tech/seshat/internal/team"
)

func TestFormatIncoming_WithBody(t *testing.T) {
	msg := mailbox.Message{
		Kind:    mailbox.KindTask,
		Subject: "Research Go generics",
		Body:    "Find three real-world examples.",
	}
	got := team.FormatIncoming(msg)
	if !strings.HasPrefix(got, "[task] Research Go generics") {
		t.Fatalf("unexpected prefix: %q", got)
	}
	if !strings.Contains(got, "Find three real-world examples.") {
		t.Fatalf("body missing from formatted message: %q", got)
	}
}

func TestFormatIncoming_EmptyBody(t *testing.T) {
	msg := mailbox.Message{
		Kind:    mailbox.KindBroadcast,
		Subject: "Stand-up",
		Body:    "   ",
	}
	got := team.FormatIncoming(msg)
	if got != "[broadcast] Stand-up" {
		t.Fatalf("expected %q, got %q", "[broadcast] Stand-up", got)
	}
}
