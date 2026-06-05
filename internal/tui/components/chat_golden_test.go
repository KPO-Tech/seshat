package components

import (
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func TestChatGoldenTurnWithTool(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 60, 40)
	fixed := time.Date(2026, 6, 4, 14, 5, 0, 0, time.UTC)

	c.messages = []msgItem{
		&userItem{content: "Run the tool", timestamp: fixed},
		&assistantItem{content: "I will inspect the workspace.", showLabel: true},
		&toolItem{id: "tool-1", name: "bash", status: "completed", label: "ls -la", metadata: map[string]any{"tool_input": map[string]any{"command": "ls -la"}}, startedAt: fixed, finishedAt: fixed.Add(500 * time.Millisecond)},
		&assistantItem{content: "The workspace contains 3 files.", showLabel: false},
	}
	c.refresh()

	assertGolden(t, "testdata/chat/turn_with_tool.golden", normalizeChatView(c.View()))
}

func TestChatGoldenCollapsedThinking(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 60, 40)
	fixed := time.Date(2026, 6, 4, 14, 5, 0, 0, time.UTC)
	thinking := &thinkingBlock{
		content: strings.Join([]string{
			"line 1", "line 2", "line 3", "line 4", "line 5", "line 6",
			"line 7", "line 8", "line 9", "line 10", "line 11", "line 12",
		}, "\n"),
		streaming:  false,
		startedAt:  fixed,
		finishedAt: fixed.Add(1500 * time.Millisecond),
		collapsed:  true,
	}
	c.messages = []msgItem{
		&assistantItem{thinking: thinking, content: "Final answer.", showLabel: true},
	}
	c.refresh()

	assertGolden(t, "testdata/chat/collapsed_thinking.golden", normalizeChatView(c.View()))
}

func assertGolden(t *testing.T, rel string, got string) {
	t.Helper()
	path := filepath.Join(rel)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", rel, err)
	}
	want := strings.TrimSpace(string(wantBytes))
	if got != want {
		t.Fatalf("golden mismatch for %s\n\n--- got ---\n%s\n\n--- want ---\n%s", rel, got, want)
	}
}

func normalizeChatView(s string) string {
	s = ansiPattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}
