package components

import (
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"strings"
	"testing"
)

func TestChatAddToolProgressSealsAssistantAndCreatesContinuation(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)

	c.AddUserMessage("user prompt")
	c.StartAssistantMessage()
	c.AppendChunk("first answer", false)
	c.AddToolProgress("tool-1", "bash", "running", "running")
	c.AppendChunk("second answer", false)
	c.FinishAssistantMessage()

	if got := len(c.messages); got != 4 {
		t.Fatalf("expected 4 chat items, got %d", got)
	}

	firstAssistant, ok := c.messages[1].(*assistantItem)
	if !ok {
		t.Fatalf("expected first assistant item at index 1, got %T", c.messages[1])
	}
	if !firstAssistant.showLabel {
		t.Fatalf("expected first assistant item to keep the label")
	}

	tool, ok := c.messages[2].(*toolItem)
	if !ok {
		t.Fatalf("expected tool item at index 2, got %T", c.messages[2])
	}
	if tool.id != "tool-1" {
		t.Fatalf("expected tool id tool-1, got %q", tool.id)
	}

	continuation, ok := c.messages[3].(*assistantItem)
	if !ok {
		t.Fatalf("expected continuation assistant item at index 3, got %T", c.messages[3])
	}
	if continuation.showLabel {
		t.Fatalf("expected continuation assistant item to omit the label")
	}

	if got := c.GetLastAssistantText(); got != "first answer\n\nsecond answer" {
		t.Fatalf("unexpected assistant text: %q", got)
	}
}

func TestChatAddToolProgressDropsEmptyAssistantPlaceholder(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)

	c.StartAssistantMessage()
	c.AddToolProgress("tool-1", "bash", "running", "running")

	if got := len(c.messages); got != 1 {
		t.Fatalf("expected 1 chat item after sealing empty assistant, got %d", got)
	}
	if _, ok := c.messages[0].(*toolItem); !ok {
		t.Fatalf("expected only tool item to remain, got %T", c.messages[0])
	}
}

func TestThinkingBlockToggleChangesCollapsedState(t *testing.T) {
	tb := newThinkingBlock()
	for i := 0; i < 12; i++ {
		tb.append("line\n")
	}
	tb.finish()

	if !tb.collapsed {
		t.Fatalf("expected thinking block to start collapsed")
	}

	collapsed := tb.render(common.DefaultStyles(), 50)
	if want := "2 lines hidden"; !strings.Contains(collapsed, want) {
		t.Fatalf("expected collapsed render to mention %q, got %q", want, collapsed)
	}

	tb.toggle()
	if tb.collapsed {
		t.Fatalf("expected toggle to expand thinking block")
	}

	expanded := tb.render(common.DefaultStyles(), 50)
	if strings.Contains(expanded, "lines hidden") {
		t.Fatalf("expected expanded render to show all lines, got %q", expanded)
	}
}
