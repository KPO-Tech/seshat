package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// PaletteItem is one entry in the command and settings palette.
type PaletteItem struct {
	Section  string
	ID       string
	Name     string
	Shortcut string
	Desc     string
}

// CommandPalette is the ctrl+p overlay listing actions and settings.
type CommandPalette struct {
	items  []PaletteItem
	list   common.ListState[PaletteItem]
	styles common.Styles
	width  int
	height int
}

func NewCommandPalette(styles common.Styles) *CommandPalette {
	p := &CommandPalette{
		styles: styles,
		list: common.NewListState(func(item PaletteItem, needle string) bool {
			return strings.Contains(strings.ToLower(item.Name), needle) ||
				strings.Contains(strings.ToLower(item.Desc), needle) ||
				strings.Contains(strings.ToLower(item.Section), needle)
		}),
	}
	p.items = defaultPaletteItems()
	p.list.SetItems(p.items)
	return p
}

func defaultPaletteItems() []PaletteItem {
	return []PaletteItem{
		{Section: "Sessions", ID: "new-session", Name: "New Session", Shortcut: "ctrl+n", Desc: "Start a fresh conversation"},
		{Section: "Sessions", ID: "sessions", Name: "Sessions", Shortcut: "ctrl+s", Desc: "Browse and resume past sessions"},
		{Section: "Workspace", ID: "copy-msg", Name: "Copy Last Message", Shortcut: "ctrl+u", Desc: "Copy your last message to clipboard"},
		{Section: "Workspace", ID: "clear", Name: "Clear Chat", Shortcut: "", Desc: "Clear the current chat display"},
		{Section: "Settings", ID: "model", Name: "Switch Model", Shortcut: "ctrl+m", Desc: "Change the active AI model"},
		{Section: "Settings", ID: "provider-config", Name: "Provider Config", Shortcut: "ctrl+,", Desc: "Configure API keys and providers"},
		{Section: "App", ID: "quit", Name: "Quit", Shortcut: "ctrl+c", Desc: "Exit Nexus"},
	}
}

func (p *CommandPalette) SetSize(width, height int) { p.width = width; p.height = height }
func (p *CommandPalette) Open(filter string)        { p.list.SetFilter(filter) }
func (p *CommandPalette) TypeFilter(ch string)      { p.list.TypeFilter(ch) }
func (p *CommandPalette) DeleteFilter()             { p.list.DeleteFilter() }
func (p *CommandPalette) Up()                       { p.list.Up() }
func (p *CommandPalette) Down()                     { p.list.Down() }

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
	title := p.styles.BrowserTitle.Render("  Commands & Settings")
	filterContent := "  search " + p.list.Filter() + "█"
	filterLine := p.styles.BrowserFilter.Width(innerW).Render(filterContent)
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	filtered := p.list.FilteredItems()
	cursor := p.list.Cursor()
	sectionOrder := []string{"Sessions", "Workspace", "Settings", "App"}
	grouped := make(map[string][]paletteViewItem)
	for i, item := range filtered {
		grouped[item.Section] = append(grouped[item.Section], paletteViewItem{item: item, selected: i == cursor})
	}

	var rows []string
	for _, section := range sectionOrder {
		items := grouped[section]
		if len(items) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, p.renderSection(section, innerW))
		for _, entry := range items {
			rows = append(rows, p.renderItem(entry.item, entry.selected, innerW))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, p.styles.BrowserItem.Render("  no matches"))
	}

	hint := p.styles.Footer.Render("  ↑↓ navigate  enter confirm  esc close  /skill in chat runs a skill")
	parts := []string{title, filterLine, sep, "", p.styles.MsgTimestamp.Render("  slash commands are reserved for skills"), ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep, hint)
	content := strings.Join(parts, "\n")
	return p.styles.BrowserBorder.Width(w).Render(content)
}

type paletteViewItem struct {
	item     PaletteItem
	selected bool
}

func (p *CommandPalette) renderSection(section string, innerW int) string {
	return p.styles.MsgTimestamp.Width(innerW).Render("  " + strings.ToUpper(section))
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

	if selected {
		indicator := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render("▶ ")
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

	nameStr := lipgloss.NewStyle().Foreground(common.ColorText).Render(item.Name)
	descStr := p.styles.MsgTimestamp.Render(desc)
	left := "    " + nameStr
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
