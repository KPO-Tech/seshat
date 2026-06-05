package components

import (
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

func TestCommandPaletteViewShowsSettingsSections(t *testing.T) {
	p := NewCommandPalette(common.DefaultStyles())
	p.SetSize(100, 30)
	view := p.View()
	for _, want := range []string{"Commands & Settings", "SESSIONS", "WORKSPACE", "SETTINGS", "slash commands are reserved for skills", "Clear Chat"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected palette view to contain %q, got %q", want, view)
		}
	}
	if strings.Contains(view, "/clear") {
		t.Fatalf("expected palette view to stop advertising generic slash commands, got %q", view)
	}
	if strings.Contains(view, "Toggle Thinking") {
		t.Fatalf("expected palette view to avoid old thinking shortcut item, got %q", view)
	}
}
