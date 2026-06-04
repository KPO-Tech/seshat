package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// paletteItem is a single entry in the commands palette.
type paletteItem struct {
	id       string
	name     string
	shortcut string
	desc     string
	// action receives a pointer to the Model so it can mutate state and return a Cmd.
	// The palette is already closed before action is called.
	action func(m *Model) tea.Cmd
}

// commandPalette is the ctrl+p overlay listing all actions and slash commands.
type commandPalette struct {
	items    []paletteItem
	filtered []paletteItem
	selected int
	filter   string
	styles   Styles
	width    int
	height   int
}

func newCommandPalette(styles Styles) *commandPalette {
	p := &commandPalette{styles: styles}
	p.items = defaultPaletteItems()
	p.filtered = make([]paletteItem, len(p.items))
	copy(p.filtered, p.items)
	return p
}

func defaultPaletteItems() []paletteItem {
	return []paletteItem{
		{
			id:       "new-session",
			name:     "New Session",
			shortcut: "ctrl+n",
			desc:     "Start a fresh conversation",
			action: func(m *Model) tea.Cmd {
				return m.createSession()
			},
		},
		{
			id:       "sessions",
			name:     "Sessions",
			shortcut: "ctrl+s",
			desc:     "Browse and resume past sessions",
			action: func(m *Model) tea.Cmd {
				m.state = stateSessions
				return m.loadSessions()
			},
		},
		{
			id:       "model",
			name:     "Switch Model",
			shortcut: "ctrl+m",
			desc:     "Change the active AI model",
			action: func(m *Model) tea.Cmd {
				m.returnState = stateCommands
				m.state = stateModelSelect
				m.modelSelect.ClearFilter()
				return m.listModels()
			},
		},
		{
			id:       "thinking",
			name:     "Toggle Thinking",
			shortcut: "ctrl+t",
			desc:     "Expand or collapse the thinking block",
			action: func(m *Model) tea.Cmd {
				m.chat.ToggleThinking()
				return nil
			},
		},
		{
			id:       "select",
			name:     "Select Mode",
			shortcut: "ctrl+e",
			desc:     "Enable native mouse text selection",
			action: func(m *Model) tea.Cmd {
				m.selectMode = !m.selectMode
				return nil
			},
		},
		{
			id:       "copy-msg",
			name:     "Copy Last Message",
			shortcut: "ctrl+u",
			desc:     "Copy your last message to clipboard",
			action: func(m *Model) tea.Cmd {
				text := m.chat.GetLastUserText()
				if text != "" {
					return m.copyToClipboard(text, "Message copied")
				}
				return nil
			},
		},
		{
			id:       "clear",
			name:     "/clear",
			shortcut: "",
			desc:     "Clear the chat display",
			action: func(m *Model) tea.Cmd {
				m.chat.Clear()
				if m.activeSession != "" {
					m.chat.AddSystem("Chat cleared")
				}
				return nil
			},
		},
		{
			id:       "quit",
			name:     "Quit",
			shortcut: "ctrl+c",
			desc:     "Exit Nexus",
			action: func(m *Model) tea.Cmd {
				m.cancel()
				return tea.Quit
			},
		},
	}
}

func (p *commandPalette) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Open resets and optionally pre-fills the filter.
func (p *commandPalette) Open(filter string) {
	p.filter = filter
	p.selected = 0
	p.applyFilter()
}

func (p *commandPalette) TypeFilter(ch string) {
	p.filter += ch
	p.selected = 0
	p.applyFilter()
}

func (p *commandPalette) DeleteFilter() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.selected = 0
		p.applyFilter()
	}
}

func (p *commandPalette) Up() {
	if p.selected > 0 {
		p.selected--
	}
}

func (p *commandPalette) Down() {
	if p.selected < len(p.filtered)-1 {
		p.selected++
	}
}

// Execute runs the selected item's action against m and returns the Cmd.
func (p *commandPalette) Execute(m *Model) tea.Cmd {
	if p.selected < 0 || p.selected >= len(p.filtered) {
		return nil
	}
	return p.filtered[p.selected].action(m)
}

func (p *commandPalette) applyFilter() {
	if p.filter == "" {
		p.filtered = make([]paletteItem, len(p.items))
		copy(p.filtered, p.items)
		return
	}
	needle := strings.ToLower(p.filter)
	p.filtered = p.filtered[:0]
	for _, item := range p.items {
		if strings.Contains(strings.ToLower(item.name), needle) ||
			strings.Contains(strings.ToLower(item.desc), needle) {
			p.filtered = append(p.filtered, item)
		}
	}
	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}
}

// View renders the palette box.
func (p *commandPalette) View() string {
	// Match model dialog width: 80% of terminal, capped at 90, minimum 54.
	w := clamp(p.width*4/5, 54, 90)
	// innerW is the usable content width within the border+padding.
	// BrowserBorder has PaddingLeft(1)+PaddingRight(1) included in Width(w),
	// so the content width passed to inner elements is w-2. We use w-4 to
	// leave a small margin and match the model dialog pattern.
	innerW := w - 4

	title := p.styles.BrowserTitle.Render("  Commands")

	// Filter line — same width calc as model dialog.
	filterContent := "  / " + p.filter + "█"
	filterLine := p.styles.BrowserFilter.Width(innerW).Render(filterContent)

	// Separator — use innerW (not innerW+2) so it never exceeds the actual
	// content area regardless of how lipgloss v2 interprets Width(w).
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	// Build item rows (one per item, blank line between items for readability).
	var rows []string
	for i, item := range p.filtered {
		row := p.renderItem(item, i == p.selected, innerW)
		rows = append(rows, row)
		// Blank spacer between items (not after the last one).
		if i < len(p.filtered)-1 {
			rows = append(rows, "")
		}
	}
	if len(rows) == 0 {
		rows = append(rows, p.styles.BrowserItem.Render("  no matches"))
	}

	hint := p.styles.Footer.Render("  ↑↓ navigate  enter confirm  esc close")

	parts := []string{title, filterLine, sep, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep, hint)

	content := strings.Join(parts, "\n")
	return p.styles.BrowserBorder.Width(w).Render(content)
}

// renderItem renders a single command item row.
func (p *commandPalette) renderItem(item paletteItem, selected bool, innerW int) string {
	// Shortcut column (right side) — render with key style.
	shortcutStr := ""
	shortcutW := 0
	if item.shortcut != "" {
		shortcutStr = p.styles.Key.Render(item.shortcut)
		shortcutW = lipgloss.Width(shortcutStr)
	}

	// Name column.
	nameW := lipgloss.Width(item.name)

	// Description fills the middle — compute available space.
	// Layout: "  ▶ " (4) + name + "  " (2) + desc + "  " (2) + shortcut
	leftPad := 4 // "  ▶ " or "    "
	descMax := innerW - leftPad - nameW - 4 - shortcutW
	if descMax < 0 {
		descMax = 0
	}

	desc := item.desc
	if len(desc) > descMax {
		if descMax > 1 {
			desc = desc[:descMax-1] + "…"
		} else {
			desc = ""
		}
	}

	if selected {
		indicator := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("▶ ")
		nameStr := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(item.name)
		descStr := p.styles.MsgTimestamp.Render(desc)

		left := "  " + indicator + nameStr
		if desc != "" {
			left += "  " + descStr
		}

		// Pad to push shortcut to the right.
		pad := innerW - lipgloss.Width(left) - shortcutW - 2
		if pad < 1 {
			pad = 1
		}
		line := left + strings.Repeat(" ", pad) + shortcutStr
		return p.styles.BrowserSelected.Width(innerW).Render(line)
	}

	nameStr := lipgloss.NewStyle().Foreground(colorText).Render(item.name)
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

// centred returns the palette positioned horizontally centred.
// Vertical centering is handled by overlayOn().
func (p *commandPalette) centred() string {
	box := p.View()
	lines := strings.Split(box, "\n")
	// Use the first line (top border) to measure the true rendered width.
	boxW := lipgloss.Width(lines[0])
	left := max(0, (p.width-boxW)/2)
	pad := strings.Repeat(" ", left)
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pad + l)
	}
	return sb.String()
}
