package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

var searchModes = []struct {
	ID    string
	Label string
	Desc  string
}{
	{"auto", "Auto", "Try configured providers in priority order"},
	{"tavily", "Tavily", "AI-optimised search (requires API key)"},
	{"exa", "Exa", "Neural search engine (requires API key)"},
	{"jina", "Jina AI", "Reader-based web retrieval (requires API key)"},
	{"langsearch", "LangSearch", "Free AI-optimised search (requires API key)"},
	{"searxng", "SearXNG", "Self-hosted meta-search (no key needed)"},
	{"ddg", "DuckDuckGo", "Privacy-friendly fallback (no key needed)"},
}

// SearchPanel is the web-search configuration overlay.
// It has three modes:
//   - list mode: all providers + active mode row
//   - key-edit mode: typing an API key for one provider
//   - mode-select mode: choosing the active provider mode
type SearchPanel struct {
	styles common.Styles
	config tui.SearchConfig

	cursor int // 0 = mode row, 1..N = providers

	// key-edit mode
	editingKey bool
	editEntry  tui.SearchKeyStatus
	draft      string
	showSecret bool

	// mode-select mode
	editingMode bool
	modeCursor  int

	statusMsg     string
	width, height int
}

func NewSearchPanel(styles common.Styles) *SearchPanel {
	return &SearchPanel{styles: styles}
}

func (p *SearchPanel) SetSize(w, h int) { p.width = w; p.height = h }

func (p *SearchPanel) SetConfig(cfg tui.SearchConfig) {
	p.config = cfg
	mode := cfg.Mode
	if mode == "" {
		mode = "auto"
	}
	for i, m := range searchModes {
		if m.ID == mode {
			p.modeCursor = i
			break
		}
	}
}

func (p *SearchPanel) Up() {
	if p.editingKey {
		return
	}
	if p.editingMode {
		if p.modeCursor > 0 {
			p.modeCursor--
		}
		return
	}
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *SearchPanel) Down() {
	if p.editingKey {
		return
	}
	if p.editingMode {
		if p.modeCursor < len(searchModes)-1 {
			p.modeCursor++
		}
		return
	}
	if p.cursor < len(p.config.Providers) {
		p.cursor++
	}
}

// EnterList opens the editor for the currently selected entry.
// Returns (openedMode, openedKey, saveKey) — the model acts on these signals.
func (p *SearchPanel) EnterList() (openedMode, openedKey bool) {
	if p.cursor == 0 {
		// Mode row — open mode selector
		p.editingMode = true
		p.statusMsg = ""
		return true, false
	}
	idx := p.cursor - 1
	if idx < 0 || idx >= len(p.config.Providers) {
		return false, false
	}
	prov := p.config.Providers[idx]
	if !prov.NeedsKey {
		// Truly no configuration needed (e.g. DuckDuckGo).
		return false, false
	}
	p.editEntry = prov
	p.draft = ""
	// URL fields are not secrets — reveal by default.
	p.showSecret = prov.FieldLabel == ""
	p.statusMsg = ""
	p.editingKey = true
	return false, true
}

// ConfirmMode saves the selected mode and exits mode-select.
// Returns the chosen mode ID so the model can persist it.
func (p *SearchPanel) ConfirmMode() string {
	if p.modeCursor < len(searchModes) {
		chosen := searchModes[p.modeCursor].ID
		p.config.Mode = chosen
		p.editingMode = false
		p.statusMsg = ""
		return chosen
	}
	p.editingMode = false
	return ""
}

// ExitKeyEdit closes key-edit mode without saving.
func (p *SearchPanel) ExitKeyEdit() {
	p.editingKey = false
	p.statusMsg = ""
}

// ExitModeEdit closes mode-select mode without saving.
func (p *SearchPanel) ExitModeEdit() {
	p.editingMode = false
	p.statusMsg = ""
}

// TypeChar appends a character in key-edit mode.
func (p *SearchPanel) TypeChar(ch string) {
	if !p.editingKey {
		return
	}
	p.draft += ch
	p.statusMsg = ""
}

// TypeString appends an arbitrary string in key-edit mode (used for clipboard paste).
func (p *SearchPanel) TypeString(s string) {
	if !p.editingKey {
		return
	}
	p.draft += s
	p.statusMsg = ""
}

// DeleteChar removes the last character in key-edit mode.
func (p *SearchPanel) DeleteChar() {
	if !p.editingKey || len(p.draft) == 0 {
		return
	}
	p.draft = p.draft[:len(p.draft)-1]
	p.statusMsg = ""
}

func (p *SearchPanel) ToggleReveal() { p.showSecret = !p.showSecret }

// CurrentDraft returns the current key draft and the DB key to store it under.
func (p *SearchPanel) CurrentDraft() (draft, dbKey string) {
	return p.draft, p.editEntry.DBKey
}

func (p *SearchPanel) IsEditingKey() bool  { return p.editingKey }
func (p *SearchPanel) IsEditingMode() bool { return p.editingMode }

// SetKeySaved marks the provider as configured and clears the editor.
func (p *SearchPanel) SetKeySaved() {
	for i, pv := range p.config.Providers {
		if pv.ID == p.editEntry.ID {
			p.config.Providers[i].IsSet = true
			break
		}
	}
	p.editEntry.IsSet = true
	p.draft = ""
	p.editingKey = false
	p.statusMsg = "✓ Saved"
}

func (p *SearchPanel) SetModeSaved()       { p.statusMsg = "✓ Mode saved" }
func (p *SearchPanel) SetError(msg string) { p.statusMsg = "✗ " + msg }
func (p *SearchPanel) ClearStatus()        { p.statusMsg = "" }

// ─── View ─────────────────────────────────────────────────────────────────────

func (p *SearchPanel) View() string {
	w := common.Clamp(p.width*4/5, 56, 90)
	innerW := w - 4

	switch {
	case p.editingKey:
		return p.viewKeyEdit(w, innerW)
	case p.editingMode:
		return p.viewModeEdit(w, innerW)
	default:
		return p.viewList(w, innerW)
	}
}

func (p *SearchPanel) viewList(w, innerW int) string {
	title := p.styles.BrowserTitle.Render("  Web Search Providers")
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	var rows []string

	// ── Mode row (cursor = 0) ──────────────────────────────────────────────
	activeMode := p.config.Mode
	if activeMode == "" {
		activeMode = "auto"
	}
	modeStatus := p.styles.MsgTimestamp.Render("mode: " + activeMode)
	modeLabel := "Search Mode"

	if p.cursor == 0 {
		ind := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render("▶ ")
		name := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(modeLabel)
		left := "  " + ind + name
		pad := max(1, innerW-lipgloss.Width(left)-lipgloss.Width(modeStatus)-2)
		rows = append(rows, p.styles.BrowserSelected.Width(innerW).Render(
			left+strings.Repeat(" ", pad)+modeStatus,
		))
	} else {
		left := "    " + lipgloss.NewStyle().Foreground(common.ColorText).Render(modeLabel)
		pad := max(1, innerW-lipgloss.Width(left)-lipgloss.Width(modeStatus)-2)
		rows = append(rows, p.styles.BrowserItem.Width(innerW).Render(
			left+strings.Repeat(" ", pad)+modeStatus,
		))
	}

	// ── Provider rows ──────────────────────────────────────────────────────
	for i, pv := range p.config.Providers {
		cur := i + 1
		statusStr := p.statusTag(pv)

		var row string
		if p.cursor == cur {
			ind := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render("▶ ")
			name := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(pv.DisplayName)
			left := "  " + ind + name
			if pv.Description != "" {
				maxD := innerW - lipgloss.Width(left) - lipgloss.Width(statusStr) - 10
				d := pv.Description
				if len(d) > maxD && maxD > 4 {
					d = d[:maxD-1] + "…"
				}
				left += "  " + p.styles.MsgTimestamp.Render(d)
			}
			pad := max(1, innerW-lipgloss.Width(left)-lipgloss.Width(statusStr)-2)
			row = p.styles.BrowserSelected.Width(innerW).Render(
				left + strings.Repeat(" ", pad) + statusStr,
			)
		} else {
			left := "    " + lipgloss.NewStyle().Foreground(common.ColorText).Render(pv.DisplayName)
			if pv.Description != "" {
				maxD := innerW - lipgloss.Width(left) - lipgloss.Width(statusStr) - 10
				d := pv.Description
				if len(d) > maxD && maxD > 4 {
					d = d[:maxD-1] + "…"
				}
				left += "  " + p.styles.MsgTimestamp.Render(d)
			}
			pad := max(1, innerW-lipgloss.Width(left)-lipgloss.Width(statusStr)-2)
			row = p.styles.BrowserItem.Width(innerW).Render(
				left + strings.Repeat(" ", pad) + statusStr,
			)
		}
		rows = append(rows, row)
	}

	hint := p.styles.Footer.Render("  enter: configure  ↑↓ navigate  esc: close")
	parts := []string{title, sep, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep)
	if line := p.statusLine(); line != "" {
		parts = append(parts, line)
	}
	parts = append(parts, hint)
	return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
}

func (p *SearchPanel) viewKeyEdit(w, innerW int) string {
	title := p.styles.BrowserTitle.Render(
		"  " + lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(p.editEntry.DisplayName),
	)
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	fieldLabel := p.editEntry.FieldLabel
	if fieldLabel == "" {
		fieldLabel = "API Key"
	}
	isURL := p.editEntry.FieldLabel != "" // URL fields have an explicit label
	labelLine := "  " + lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(fieldLabel)
	if p.editEntry.EnvVar != "" {
		labelLine += "  " + p.styles.MsgTimestamp.Render("("+p.editEntry.EnvVar+")")
	}
	if p.editEntry.IsSet && p.draft == "" {
		labelLine += "  " + lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓ set")
	}

	display := p.draft
	if !isURL && !p.showSecret && display != "" {
		// Mask secrets (API keys), but not URLs.
		display = strings.Repeat("•", len(display))
	}
	if display == "" && p.editEntry.IsSet {
		display = p.styles.MsgTimestamp.Render("(keep existing — type to replace)")
	}
	cur := lipgloss.NewStyle().Foreground(common.ColorPrimary).Render("█")

	var valLine string
	if isURL {
		valLine = "  ▶ " + lipgloss.NewStyle().Foreground(common.ColorText).Render(display) + cur
	} else {
		revealHint := "ctrl+r: reveal"
		if p.showSecret {
			revealHint = "ctrl+r: hide"
		}
		valLine = "  ▶ " + lipgloss.NewStyle().Foreground(common.ColorText).Render(display) + cur +
			"  " + p.styles.MsgTimestamp.Render(revealHint)
	}

	hintText := "  enter: save  ctrl+v: paste  ← back  esc: close"
	if !isURL {
		hintText = "  enter: save  ctrl+v: paste  ctrl+r: reveal  ← back  esc: close"
	}
	hint := p.styles.Footer.Render(hintText)
	parts := []string{title, sep, "", labelLine, valLine, "", sep}
	if line := p.statusLine(); line != "" {
		parts = append(parts, line)
	}
	parts = append(parts, hint)
	return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
}

func (p *SearchPanel) viewModeEdit(w, innerW int) string {
	title := p.styles.BrowserTitle.Render("  Select Search Mode")
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	var rows []string
	for i, m := range searchModes {
		selected := i == p.modeCursor
		isActive := m.ID == p.config.Mode || (p.config.Mode == "" && m.ID == "auto")

		var row string
		if selected {
			ind := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render("▶ ")
			name := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(m.Label)
			left := "  " + ind + name
			if m.Desc != "" {
				left += "  " + p.styles.MsgTimestamp.Render(m.Desc)
			}
			suffix := ""
			if isActive {
				suffix = "  " + lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓")
			}
			row = p.styles.BrowserSelected.Width(innerW).Render(left + suffix)
		} else {
			left := "    " + lipgloss.NewStyle().Foreground(common.ColorText).Render(m.Label)
			if m.Desc != "" {
				maxD := innerW - lipgloss.Width(left) - 12
				d := m.Desc
				if len(d) > maxD && maxD > 4 {
					d = d[:maxD-1] + "…"
				}
				left += "  " + p.styles.MsgTimestamp.Render(d)
			}
			suffix := ""
			if isActive {
				suffix = "  " + lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓")
			}
			row = p.styles.BrowserItem.Width(innerW).Render(left + suffix)
		}
		rows = append(rows, row)
	}

	hint := p.styles.Footer.Render("  enter: select  ↑↓ navigate  ← back  esc: close")
	parts := []string{title, sep, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep, hint)
	return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
}

func (p *SearchPanel) statusTag(pv tui.SearchKeyStatus) string {
	if !pv.NeedsKey {
		return p.styles.MsgTimestamp.Render("─ no config needed")
	}
	if pv.IsSet {
		return lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓ configured")
	}
	return lipgloss.NewStyle().Foreground(common.ColorRed).Render("✗ not configured")
}

func (p *SearchPanel) statusLine() string {
	if p.statusMsg == "" {
		return ""
	}
	var st lipgloss.Style
	if strings.HasPrefix(p.statusMsg, "✓") {
		st = lipgloss.NewStyle().Foreground(common.ColorGreen).Bold(true)
	} else {
		st = lipgloss.NewStyle().Foreground(common.ColorRed).Bold(true)
	}
	return "  " + st.Render(p.statusMsg)
}

func (p *SearchPanel) Centered() string {
	return common.CenterHorizontally(p.View(), p.width)
}
