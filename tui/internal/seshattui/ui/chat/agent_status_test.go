package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/ui/styles"
)

func TestFormatElapsed(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{12 * time.Second, "12s"},
		{125 * time.Second, "2m05s"},
		{12*time.Minute + 48*time.Second, "12m48s"},
		{63 * time.Minute, "1h03m"},
	} {
		if got := formatElapsed(tc.in); got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSubAgentStatusLineShowsNameElapsedAndToolCount(t *testing.T) {
	sty := styles.SeshatDark()
	line := subAgentStatusLine(&sty, "Explore Agent", time.Now().Add(-125*time.Second), 53)
	for _, want := range []string{"Explore Agent working", "2m05s", "53 tools"} {
		if !strings.Contains(line, want) {
			t.Errorf("status line %q lacks %q", line, want)
		}
	}
	if got := subAgentStatusLine(&sty, "Agent", time.Now(), 1); !strings.Contains(got, "1 tool") || strings.Contains(got, "1 tools") {
		t.Errorf("singular tool count wrong: %q", got)
	}
	if got := subAgentStatusLine(&sty, "Agent", time.Now(), 0); strings.Contains(got, "tool") {
		t.Errorf("no tool count expected before the first tool: %q", got)
	}
	if got := subAgentStatusLine(&sty, "Agent", time.Time{}, 3); got != "" {
		t.Errorf("an unset start time must fall back to the default spinner, got %q", got)
	}
}
