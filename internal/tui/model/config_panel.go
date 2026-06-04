package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
)

// configPanel is the provider configuration overlay (ctrl+, or via commands palette).
// It has two modes:
//   - list mode: shows all providers with their API key status
//   - edit mode: shows fields for a selected provider with text inputs
type configPanel struct {
	styles Styles

	// ── list mode ──────────────────────────────────────────────────────────
	providers []tui.ProviderStatus
	filtered  []tui.ProviderStatus
	cursor    int
	filter    string

	// ── edit mode ──────────────────────────────────────────────────────────
	editing      bool
	editProvider tui.ProviderStatus
	fieldCursor  int
	inputs       []cfgFieldInput // one per field in editProvider
	showSecret   bool            // reveal API key value

	statusMsg string // "✓ Saved" / error

	width, height int
}

type cfgFieldInput struct {
	field tui.ProviderFieldStatus
	draft string // what the user is currently typing
}

func newConfigPanel(styles Styles) *configPanel {
	return &configPanel{styles: styles}
}

func (p *configPanel) SetSize(w, h int) { p.width = w; p.height = h }

// SetProviders refreshes the provider list.
func (p *configPanel) SetProviders(providers []tui.ProviderStatus) {
	p.providers = providers
	p.cursor = 0
	p.applyFilter()
}

func (p *configPanel) applyFilter() {
	if p.filter == "" {
		p.filtered = make([]tui.ProviderStatus, len(p.providers))
		copy(p.filtered, p.providers)
		return
	}
	needle := strings.ToLower(p.filter)
	p.filtered = p.filtered[:0]
	for _, pv := range p.providers {
		if strings.Contains(strings.ToLower(pv.DisplayName), needle) ||
			strings.Contains(strings.ToLower(pv.Description), needle) {
			p.filtered = append(p.filtered, pv)
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

func (p *configPanel) TypeFilter(ch string) { p.filter += ch; p.applyFilter() }
func (p *configPanel) DeleteFilter() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.applyFilter()
	}
}

func (p *configPanel) Up() {
	if p.editing {
		if p.fieldCursor > 0 {
			p.fieldCursor--
		}
	} else {
		if p.cursor > 0 {
			p.cursor--
		}
	}
}

func (p *configPanel) Down() {
	if p.editing {
		if p.fieldCursor < len(p.inputs)-1 {
			p.fieldCursor++
		}
	} else {
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
	}
}

// EnterEdit switches to edit mode for the currently selected provider.
func (p *configPanel) EnterEdit() {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return
	}
	p.editProvider = p.filtered[p.cursor]
	p.inputs = make([]cfgFieldInput, len(p.editProvider.Fields))
	for i, f := range p.editProvider.Fields {
		p.inputs[i] = cfgFieldInput{field: f, draft: ""}
	}
	p.fieldCursor = 0
	p.showSecret = false
	p.statusMsg = ""
	p.editing = true
}

// ExitEdit returns to list mode.
func (p *configPanel) ExitEdit() {
	p.editing = false
	p.statusMsg = ""
	// Reload the providers so the list reflects saved state.
}

// TypeChar appends a character to the active field draft.
func (p *configPanel) TypeChar(ch string) {
	if !p.editing || p.fieldCursor >= len(p.inputs) {
		return
	}
	p.inputs[p.fieldCursor].draft += ch
	p.statusMsg = ""
}

// DeleteChar removes the last character from the active field draft.
func (p *configPanel) DeleteChar() {
	if !p.editing || p.fieldCursor >= len(p.inputs) {
		return
	}
	d := p.inputs[p.fieldCursor].draft
	if len(d) > 0 {
		p.inputs[p.fieldCursor].draft = d[:len(d)-1]
	}
	p.statusMsg = ""
}

// ToggleReveal shows/hides the secret value in the active field.
func (p *configPanel) ToggleReveal() { p.showSecret = !p.showSecret }

// CurrentFieldDraft returns the current draft text and whether it's a secret field.
func (p *configPanel) CurrentFieldDraft() (draft string, isSecret bool, fieldKey string) {
	if !p.editing || p.fieldCursor >= len(p.inputs) {
		return "", false, ""
	}
	inp := p.inputs[p.fieldCursor]
	return inp.draft, inp.field.Secret, inp.field.Key
}

// SetSaved marks the current field as successfully saved and updates isSet.
func (p *configPanel) SetSaved() {
	if p.fieldCursor < len(p.inputs) {
		p.inputs[p.fieldCursor].field.IsSet = true
		p.inputs[p.fieldCursor].draft = ""
		p.editProvider.Fields[p.fieldCursor].IsSet = true
		// Also update in the main providers list.
		for i, pv := range p.providers {
			if pv.ID == p.editProvider.ID {
				p.providers[i].Fields[p.fieldCursor].IsSet = true
				break
			}
		}
	}
	p.statusMsg = "✓ Saved"
	p.applyFilter()
}

func (p *configPanel) SetError(msg string) { p.statusMsg = "✗ " + msg }
func (p *configPanel) ClearStatus()        { p.statusMsg = "" }

// ─── View ────────────────────────────────────────────────────────────────────

func (p *configPanel) View() string {
	w := clamp(p.width*4/5, 54, 90)
	innerW := w - 4

	if p.editing {
		return p.viewEdit(w, innerW)
	}
	return p.viewList(w, innerW)
}

func (p *configPanel) viewList(w, innerW int) string {
	title := p.styles.BrowserTitle.Render("  Provider Configuration")
	filterLine := p.styles.BrowserFilter.Width(innerW).Render("  / " + p.filter + "█")
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	var rows []string
	for i, pv := range p.filtered {
		statusStr := p.providerStatusTag(pv)
		nameStr := lipgloss.NewStyle().Foreground(colorText).Render(pv.DisplayName)
		descStr := ""
		if pv.Description != "" {
			maxDesc := innerW - lipgloss.Width(pv.DisplayName) - lipgloss.Width(statusStr) - 10
			desc := pv.Description
			if len(desc) > maxDesc && maxDesc > 4 {
				desc = desc[:maxDesc-1] + "…"
			}
			descStr = p.styles.MsgTimestamp.Render(desc)
		}

		var row string
		if i == p.cursor {
			indicator := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("▶ ")
			left := "  " + indicator + lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(pv.DisplayName)
			if descStr != "" {
				left += "  " + descStr
			}
			_ = nameStr
			pad := innerW - lipgloss.Width(left) - lipgloss.Width(statusStr) - 2
			if pad < 1 {
				pad = 1
			}
			row = p.styles.BrowserSelected.Width(innerW).Render(
				left + strings.Repeat(" ", pad) + statusStr,
			)
		} else {
			left := "    " + nameStr
			if descStr != "" {
				left += "  " + descStr
			}
			pad := innerW - lipgloss.Width(left) - lipgloss.Width(statusStr) - 2
			if pad < 1 {
				pad = 1
			}
			row = p.styles.BrowserItem.Width(innerW).Render(
				left + strings.Repeat(" ", pad) + statusStr,
			)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		rows = append(rows, p.styles.BrowserItem.Render("  no matches"))
	}

	hint := p.styles.Footer.Render("  enter: configure  ↑↓ navigate  esc: close")

	parts := []string{title, filterLine, sep, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep, hint)
	return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
}

func (p *configPanel) viewEdit(w, innerW int) string {
	providerTitle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(p.editProvider.DisplayName)
	title := p.styles.BrowserTitle.Render("  " + providerTitle)
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	if len(p.inputs) == 0 {
		hint := p.styles.Footer.Render("  ← back  esc: close")
		noFields := p.styles.MsgTimestamp.Render("  No credentials required for this provider.")
		parts := []string{title, sep, "", noFields, "", sep, hint}
		return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
	}

	var rows []string
	for i, inp := range p.inputs {
		selected := i == p.fieldCursor

		// Label line
		var labelStyle lipgloss.Style
		if selected {
			labelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
		} else {
			labelStyle = lipgloss.NewStyle().Foreground(colorText)
		}
		labelLine := "  " + labelStyle.Render(inp.field.Label)
		if inp.field.EnvVar != "" {
			labelLine += "  " + p.styles.MsgTimestamp.Render("("+inp.field.EnvVar+")")
		}
		if inp.field.IsSet && inp.draft == "" {
			labelLine += "  " + lipgloss.NewStyle().Foreground(colorGreen).Render("✓ set")
		}
		rows = append(rows, labelLine)

		// Value / input line
		var valLine string
		if selected {
			draft := inp.draft
			display := draft
			if inp.field.Secret && !p.showSecret && draft != "" {
				display = strings.Repeat("•", len(draft))
			}
			if display == "" && inp.field.IsSet {
				display = p.styles.MsgTimestamp.Render("(keep existing — type to replace)")
			}
			cursor := lipgloss.NewStyle().Foreground(colorPrimary).Render("█")
			valLine = "  ▶ " + lipgloss.NewStyle().Foreground(colorText).Render(display) + cursor
			if inp.field.Secret {
				revealHint := "ctrl+v: reveal"
				if p.showSecret {
					revealHint = "ctrl+v: hide"
				}
				valLine += "  " + p.styles.MsgTimestamp.Render(revealHint)
			}
		} else {
			if inp.field.IsSet && inp.draft == "" {
				valLine = "    " + p.styles.MsgTimestamp.Render("••••••••••••••••")
			} else if inp.draft != "" {
				display := inp.draft
				if inp.field.Secret {
					display = strings.Repeat("•", len(inp.draft))
				}
				valLine = "    " + lipgloss.NewStyle().Foreground(colorMuted).Render(display)
			} else {
				valLine = "    " + p.styles.MsgTimestamp.Render("(not set)")
			}
		}
		rows = append(rows, valLine)

		// Blank line between fields
		if i < len(p.inputs)-1 {
			rows = append(rows, "")
		}
	}

	var statusLine string
	if p.statusMsg != "" {
		var st lipgloss.Style
		if strings.HasPrefix(p.statusMsg, "✓") {
			st = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
		} else {
			st = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		}
		statusLine = "  " + st.Render(p.statusMsg)
	}

	hint := p.styles.Footer.Render("  enter: save  ↑↓ switch field  ← back  esc: close")

	parts := []string{title, sep, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", sep)
	if statusLine != "" {
		parts = append(parts, statusLine)
	}
	parts = append(parts, hint)
	return p.styles.BrowserBorder.Width(w).Render(strings.Join(parts, "\n"))
}

func (p *configPanel) providerStatusTag(pv tui.ProviderStatus) string {
	if !pv.NeedsKey {
		return p.styles.MsgTimestamp.Render("─ local")
	}
	allSet := true
	anySet := false
	for _, f := range pv.Fields {
		if f.Required && !f.IsSet {
			allSet = false
		}
		if f.IsSet {
			anySet = true
		}
	}
	_ = anySet
	if allSet {
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓ configured")
	}
	return lipgloss.NewStyle().Foreground(colorRed).Render("✗ not configured")
}

// centred returns the panel horizontally centred (vertical centring via overlayOn).
func (p *configPanel) centred() string {
	box := p.View()
	lines := strings.Split(box, "\n")
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
