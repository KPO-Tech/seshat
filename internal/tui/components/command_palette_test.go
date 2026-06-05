package components

import (
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

func TestCommandPaletteViewShowsSettingsRootSections(t *testing.T) {
	p := NewCommandPalette(common.DefaultStyles())
	p.SetSize(100, 30)
	view := p.View()
	for _, want := range []string{"Settings", "Commands", "Providers", "Models", "Tools", "MCP", "Skills", "choose a section"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected settings root to contain %q, got %q", want, view)
		}
	}
	if strings.Contains(view, "Clear Chat") {
		t.Fatalf("expected root settings view not to show nested command entries, got %q", view)
	}
}

func TestCommandPaletteOpenCommandsSection(t *testing.T) {
	p := NewCommandPalette(common.DefaultStyles())
	p.SetSize(100, 30)
	if !p.OpenSection("commands") {
		t.Fatalf("expected commands section to open")
	}
	view := p.View()
	for _, want := range []string{"Settings / Commands", "run commands and workspace actions", "New Session", "Quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected commands section to contain %q, got %q", want, view)
		}
	}
}

func TestCommandPaletteBackReturnsToRoot(t *testing.T) {
	p := NewCommandPalette(common.DefaultStyles())
	if !p.OpenSection("skills") {
		t.Fatalf("expected skills section to open")
	}
	if !p.Back() {
		t.Fatalf("expected back to return to root settings view")
	}
	view := p.View()
	if !strings.Contains(view, "choose a section") {
		t.Fatalf("expected root view after back, got %q", view)
	}
}

func TestCommandPaletteSetSectionItemsReplacesLiveSection(t *testing.T) {
	p := NewCommandPalette(common.DefaultStyles())
	p.SetSectionItems("tools", []PaletteItem{{
		Kind: PaletteInfoKind,
		ID:   "tool-bash",
		Name: "bash",
		Desc: "system · Run shell commands",
	}})
	if !p.OpenSection("tools") {
		t.Fatalf("expected tools section to open")
	}
	view := p.View()
	if !strings.Contains(view, "bash") || !strings.Contains(view, "Run shell commands") {
		t.Fatalf("expected live tools section to contain replacement item, got %q", view)
	}
}
