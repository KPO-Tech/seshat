package dialog

import (
	"context"
	"image"
	"testing"

	mcptools "github.com/KPO-Tech/seshat-runtime/internal/seshattui/agent/tools/mcp"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/config"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/csync"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/skills"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/common"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/styles"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/workspace"
	uv "github.com/charmbracelet/ultraviolet"
)

// panelStubWorkspace satisfies the parts of the workspace the settings dialogs
// touch while drawing. The embedded interface provides the rest; a method that
// is actually needed and not implemented here panics, which is what we want.
type panelStubWorkspace struct {
	workspace.Workspace
	cfg *config.Config
}

func (s panelStubWorkspace) Config() *config.Config { return s.cfg }

// ListTools, ListSkills and MCPGetStates are called when navigating to the
// tools, skills and MCP views; empty results are enough to lay them out.
func (s panelStubWorkspace) ListTools(context.Context) ([]workspace.ToolInfo, error) {
	return nil, nil
}

func (s panelStubWorkspace) ListSkills(context.Context) ([]skills.CatalogEntry, error) {
	return nil, nil
}

func (s panelStubWorkspace) MCPGetStates() map[string]mcptools.ClientInfo {
	return nil
}

func newSettingsForLayoutTest(t *testing.T) *Settings {
	t.Helper()
	st := styles.SeshatDark()
	cfg := &config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
	}
	com := &common.Common{Workspace: panelStubWorkspace{cfg: cfg}, Styles: &st}
	s, err := NewSettings(com)
	if err != nil {
		t.Fatalf("NewSettings: %v", err)
	}
	return s
}

func cellAt(scr uv.ScreenBuffer, x, y int) string {
	c := scr.CellAt(x, y)
	if c == nil {
		return " "
	}
	return c.Content
}

// A settings panel is exactly settingsCardMaxWidth columns wide and its
// horizontal separators reach the column just before the right border. A part
// even one column too wide pushes the border out and leaves the separators
// short of it, which reads as a broken rectangle.
func TestSettingsPanelKeepsCardWidth(t *testing.T) {
	const w, h = 100, 30

	views := []settingsView{
		settingsViewRoot,
		settingsViewProviders,
		settingsViewProvidersLLM,
		settingsViewImageGeneration,
		settingsViewTextToSpeech,
		settingsViewSpeechToText,
		settingsViewTheme,
		settingsViewWebSearch,
		settingsViewTools,
		settingsViewMCP,
		settingsViewSkills,
	}

	for _, view := range views {
		s := newSettingsForLayoutTest(t)
		s.gotoView(view)

		scr := uv.NewScreenBuffer(w, h)
		s.Draw(scr, uv.Rectangle{Max: image.Pt(w, h)})

		// Dialogs are centered vertically too, so find the top border row.
		left, right, top := -1, -1, -1
	scan:
		for y := range h {
			for x := range w {
				switch cellAt(scr, x, y) {
				case "╭":
					left, top = x, y
				case "╮":
					right = x
				}
			}
			if left >= 0 && right >= 0 {
				break scan
			}
		}
		if top < 0 {
			t.Fatalf("view %d: no panel frame drawn", view)
		}
		if got := right - left + 1; got != settingsCardMaxWidth {
			t.Errorf("view %d: panel is %d columns wide, want %d", view, got, settingsCardMaxWidth)
		}

		for y := range h {
			lastDash := -1
			for x := range w {
				if cellAt(scr, x, y) == "─" {
					lastDash = x
				}
			}
			if lastDash >= 0 && lastDash != right-1 {
				t.Errorf("view %d: row %d separator ends at column %d, right border at %d (gap of %d)",
					view, y, lastDash, right, right-1-lastDash)
			}
		}
	}
}
