package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

type PaletteItemKind string

const (
	PaletteSectionKind PaletteItemKind = "section"
	PaletteActionKind  PaletteItemKind = "action"
	PaletteRouteKind   PaletteItemKind = "route"
	PaletteInfoKind    PaletteItemKind = "info"
)

// PaletteItem is one entry in the settings hub or one of its subsections.
type PaletteItem struct {
	Kind     PaletteItemKind
	ID       string
	Name     string
	Shortcut string
	Desc     string
}

type paletteView string

const (
	paletteViewRoot    paletteView = "root"
	paletteViewSection paletteView = "section"
)

// CommandPalette is the ctrl+p overlay. It now behaves as a settings hub with
// nested sections such as Commands, Providers, Models, Tools, MCP, and Skills.
type CommandPalette struct {
	rootItems     []PaletteItem
	sectionItems  map[string][]PaletteItem
	list          common.ListState[PaletteItem]
	styles        common.Styles
	width         int
	height        int
	view          paletteView
	activeSection string
}

func NewCommandPalette(styles common.Styles) *CommandPalette {
	p := &CommandPalette{
		styles: styles,
		list: common.NewListState(func(item PaletteItem, needle string) bool {
			return strings.Contains(strings.ToLower(item.Name), needle) ||
				strings.Contains(strings.ToLower(item.Desc), needle)
		}),
		sectionItems: defaultPaletteSections(),
	}
	p.rootItems = defaultPaletteRootItems()
	p.Open("")
	return p
}

func defaultPaletteRootItems() []PaletteItem {
	return []PaletteItem{
		{Kind: PaletteSectionKind, ID: "commands", Name: "Commands", Desc: "Shortcuts, sessions, copy actions, and app controls"},
		{Kind: PaletteRouteKind, ID: "providers", Name: "Providers", Shortcut: "ctrl+,", Desc: "Configure API keys and provider credentials"},
		{Kind: PaletteRouteKind, ID: "models", Name: "Models", Shortcut: "ctrl+m", Desc: "Switch the active AI model"},
		{Kind: PaletteSectionKind, ID: "tools", Name: "Tools", Desc: "Current tool UX and future browser entry point"},
		{Kind: PaletteSectionKind, ID: "mcp", Name: "MCP", Desc: "Server usage notes and future management surface"},
		{Kind: PaletteSectionKind, ID: "skills", Name: "Skills", Desc: "Slash-skill workflow and future skill discovery"},
	}
}

func defaultPaletteSections() map[string][]PaletteItem {
	return map[string][]PaletteItem{
		"commands": {
			{Kind: PaletteActionKind, ID: "new-session", Name: "New Session", Shortcut: "ctrl+n", Desc: "Start a fresh conversation"},
			{Kind: PaletteActionKind, ID: "sessions", Name: "Sessions", Shortcut: "ctrl+s", Desc: "Browse and resume past sessions"},
			{Kind: PaletteActionKind, ID: "copy-msg", Name: "Copy Last Message", Shortcut: "ctrl+u", Desc: "Copy your last message to clipboard"},
			{Kind: PaletteActionKind, ID: "quit", Name: "Quit", Shortcut: "ctrl+c", Desc: "Exit Nexus"},
		},
		"tools": {
			{Kind: PaletteInfoKind, ID: "tool-inline", Name: "Inline Tool Previews", Desc: "Expand tools in chat with space or a click on the expander"},
			{Kind: PaletteInfoKind, ID: "tool-details", Name: "Tool Details Pane", Desc: "Open the right-side tool details pane with o or the details hit target"},
			{Kind: PaletteInfoKind, ID: "tool-browser", Name: "Dedicated Tool Browser", Desc: "A richer tool browser can land here later without changing the root settings flow"},
		},
		"mcp": {
			{Kind: PaletteInfoKind, ID: "mcp-usage", Name: "MCP During Runs", Desc: "Configured MCP servers are available to the agent during execution"},
			{Kind: PaletteInfoKind, ID: "mcp-manage", Name: "MCP Management", Desc: "Dedicated MCP browsing and management will land here"},
		},
		"skills": {
			{Kind: PaletteInfoKind, ID: "skill-run", Name: "Run a Skill", Desc: "Type /skill_name directly in chat to invoke a skill"},
			{Kind: PaletteInfoKind, ID: "skill-discovery", Name: "Skill Discovery", Desc: "A dedicated TUI skill browser can land here once workspace-side discovery is wired"},
		},
	}
}

func (p *CommandPalette) SetSize(width, height int) { p.width = width; p.height = height }

func (p *CommandPalette) Open(filter string) {
	p.view = paletteViewRoot
	p.activeSection = ""
	p.list.ResetItems(p.rootItems, false)
	p.list.SetFilter(filter)
}

func (p *CommandPalette) OpenSection(sectionID string) bool {
	items, ok := p.sectionItems[sectionID]
	if !ok {
		return false
	}
	p.view = paletteViewSection
	p.activeSection = sectionID
	p.list.ResetItems(items, false)
	p.list.ClearFilter()
	return true
}

func (p *CommandPalette) Back() bool {
	if p.view == paletteViewSection {
		p.Open("")
		return true
	}
	return false
}

func (p *CommandPalette) TypeFilter(ch string) { p.list.TypeFilter(ch) }
func (p *CommandPalette) DeleteFilter()        { p.list.DeleteFilter() }
func (p *CommandPalette) Up()                  { p.list.Up() }
func (p *CommandPalette) Down()                { p.list.Down() }

func (p *CommandPalette) Selected() *PaletteItem {
	item, ok := p.list.Selected()
	if !ok {
		return nil
	}
	return &item
}

func (p *CommandPalette) View() string {
	w := common.Clamp(p.width*4/5, 54, 90)
	innerW := w - 4
	title := p.styles.BrowserTitle.Render("  " + p.title())
	filterContent := "  search " + p.list.Filter() + "█"
	filterLine := p.styles.BrowserFilter.Width(innerW).Render(filterContent)
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	rows := p.renderRows(innerW)
	if len(rows) == 0 {
		rows = append(rows, p.styles.BrowserItem.Render("  no matches"))
	}

	hint := p.styles.Footer.Render("  ↑↓ navigate  enter open  ← back  esc close")
	if p.view == paletteViewRoot {
		hint = p.styles.Footer.Render("  ↑↓ navigate  enter open  esc close  /skill in chat runs a skill")
	}
	parts := []string{title, filterLine, sep, "", p.subtitle(innerW), ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep, hint)
	content := strings.Join(parts, "\n")
	return p.styles.BrowserBorder.Width(w).Render(content)
}

func (p *CommandPalette) title() string {
	if p.view == paletteViewRoot {
		return "Settings"
	}
	return "Settings / " + sectionLabel(p.activeSection)
}

func (p *CommandPalette) subtitle(innerW int) string {
	var text string
	if p.view == paletteViewRoot {
		text = "choose a section"
	} else {
		switch p.activeSection {
		case "commands":
			text = "run commands and workspace actions"
		case "tools":
			text = "tool UX and browsing"
		case "mcp":
			text = "MCP usage and future management"
		case "skills":
			text = "slash skills and future discovery"
		default:
			text = "browse settings"
		}
	}
	return p.styles.MsgTimestamp.Width(innerW).Render("  " + text)
}

func (p *CommandPalette) renderRows(innerW int) []string {
	filtered := p.list.FilteredItems()
	cursor := p.list.Cursor()
	rows := make([]string, 0, len(filtered))
	for i, item := range filtered {
		rows = append(rows, p.renderItem(item, i == cursor, innerW))
		if i < len(filtered)-1 {
			rows = append(rows, "")
		}
	}
	return rows
}

func sectionLabel(sectionID string) string {
	switch sectionID {
	case "commands":
		return "Commands"
	case "tools":
		return "Tools"
	case "mcp":
		return "MCP"
	case "skills":
		return "Skills"
	default:
		return strings.Title(sectionID)
	}
}

func (p *CommandPalette) renderItem(item PaletteItem, selected bool, innerW int) string {
	shortcutStr := ""
	shortcutW := 0
	if item.Shortcut != "" {
		shortcutStr = p.styles.Key.Render(item.Shortcut)
		shortcutW = lipgloss.Width(shortcutStr)
	}

	nameW := lipgloss.Width(item.Name)
	leftPad := 4
	descMax := innerW - leftPad - nameW - 4 - shortcutW
	if descMax < 0 {
		descMax = 0
	}

	desc := item.Desc
	if len(desc) > descMax {
		if descMax > 1 {
			desc = desc[:descMax-1] + "…"
		} else {
			desc = ""
		}
	}

	indicatorSymbol := "▶ "
	if item.Kind == PaletteInfoKind {
		indicatorSymbol = "• "
	}

	if selected {
		indicator := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(indicatorSymbol)
		nameStr := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(item.Name)
		descStr := p.styles.MsgTimestamp.Render(desc)
		left := "  " + indicator + nameStr
		if desc != "" {
			left += "  " + descStr
		}
		pad := innerW - lipgloss.Width(left) - shortcutW - 2
		if pad < 1 {
			pad = 1
		}
		line := left + strings.Repeat(" ", pad) + shortcutStr
		return p.styles.BrowserSelected.Width(innerW).Render(line)
	}

	prefix := "    "
	if item.Kind == PaletteInfoKind {
		prefix = "  • "
	}
	nameStr := lipgloss.NewStyle().Foreground(common.ColorText).Render(item.Name)
	descStr := p.styles.MsgTimestamp.Render(desc)
	left := prefix + nameStr
	if desc != "" {
		left += "  " + descStr
	}
	pad := innerW - lipgloss.Width(left) - shortcutW - 2
	if pad < 1 {
		pad = 1
	}
	line := left + strings.Repeat(" ", pad) + shortcutStr
	return p.styles.BrowserItem.Width(innerW).Render(line)
}

func (p *CommandPalette) Centered() string {
	return common.CenterHorizontally(p.View(), p.width)
}
