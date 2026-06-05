package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// configPanel is the provider configuration overlay (ctrl+, or via commands palette).
// It has two modes:
//   - list mode: shows all providers with their API key status
//   - edit mode: shows fields for a selected provider with text inputs
type ConfigPanel struct {
	styles common.Styles

	// ── list mode ──────────────────────────────────────────────────────────
	providers []tui.ProviderStatus
	list      common.ListState[tui.ProviderStatus]

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

func NewConfigPanel(styles common.Styles) *ConfigPanel {
	return &ConfigPanel{
		styles: styles,
		list: common.NewListState(func(pv tui.ProviderStatus, needle string) bool {
			return strings.Contains(strings.ToLower(pv.DisplayName), needle) ||
				strings.Contains(strings.ToLower(pv.Description), needle)
		}),
	}
}

func (p *ConfigPanel) SetSize(w, h int) { p.width = w; p.height = h }

// SetProviders refreshes the provider list.
func (p *ConfigPanel) SetProviders(providers []tui.ProviderStatus) {
	p.providers = providers
	p.list.SetItems(providers)
}

func (p *ConfigPanel) TypeFilter(ch string) { p.list.TypeFilter(ch) }
func (p *ConfigPanel) DeleteFilter()        { p.list.DeleteFilter() }

func (p *ConfigPanel) Up() {
	if p.editing {
		if p.fieldCursor > 0 {
			p.fieldCursor--
		}
		return
	}
	p.list.Up()
}

func (p *ConfigPanel) Down() {
	if p.editing {
		if p.fieldCursor < len(p.inputs)-1 {
			p.fieldCursor++
		}
		return
	}
	p.list.Down()
}

// EnterEdit switches to edit mode for the currently selected provider.
func (p *ConfigPanel) EnterEdit() {
	selected, ok := p.list.Selected()
	if !ok {
		return
	}
	p.editProvider = selected
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
func (p *ConfigPanel) ExitEdit() {
	p.editing = false
	p.statusMsg = ""
	// Reload the providers so the list reflects saved state.
}

// TypeChar appends a character to the active field draft.
func (p *ConfigPanel) TypeChar(ch string) {
	if !p.editing || p.fieldCursor >= len(p.inputs) {
		return
	}
	p.inputs[p.fieldCursor].draft += ch
	p.statusMsg = ""
}

// DeleteChar removes the last character from the active field draft.
func (p *ConfigPanel) DeleteChar() {
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
func (p *ConfigPanel) ToggleReveal() { p.showSecret = !p.showSecret }

// CurrentFieldDraft returns the current draft text and whether it's a secret field.
func (p *ConfigPanel) CurrentFieldDraft() (draft string, isSecret bool, fieldKey string) {
	if !p.editing || p.fieldCursor >= len(p.inputs) {
		return "", false, ""
	}
	inp := p.inputs[p.fieldCursor]
	return inp.draft, inp.field.Secret, inp.field.Key
}

// SetSaved marks the current field as successfully saved and updates isSet.
func (p *ConfigPanel) SetSaved() {
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
	p.list.ResetItems(p.providers, true)
}

func (p *ConfigPanel) SetError(msg string) { p.statusMsg = "✗ " + msg }
func (p *ConfigPanel) ClearStatus()        { p.statusMsg = "" }

// ─── View ────────────────────────────────────────────────────────────────────

func (p *ConfigPanel) View() string {
	w := common.Clamp(p.width*4/5, 54, 90)
	innerW := w - 4

	if p.editing {
		return p.viewEdit(w, innerW)
	}
	return p.viewList(w, innerW)
}

func (p *ConfigPanel) viewList(w, innerW int) string {
	title := p.styles.BrowserTitle.Render("  Provider Configuration")
	filterLine := p.styles.BrowserFilter.Width(innerW).Render("  / " + p.list.Filter() + "█")
	sep := p.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	filtered := p.list.FilteredItems()
	cursor := p.list.Cursor()
	var rows []string
	for i, pv := range filtered {
		statusStr := p.providerStatusTag(pv)
		nameStr := lipgloss.NewStyle().Foreground(common.ColorText).Render(pv.DisplayName)
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
		if i == cursor {
			indicator := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render("▶ ")
			left := "  " + indicator + lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(pv.DisplayName)
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

func (p *ConfigPanel) viewEdit(w, innerW int) string {
	providerTitle := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(p.editProvider.DisplayName)
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
			labelStyle = lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary)
		} else {
			labelStyle = lipgloss.NewStyle().Foreground(common.ColorText)
		}
		labelLine := "  " + labelStyle.Render(inp.field.Label)
		if inp.field.EnvVar != "" {
			labelLine += "  " + p.styles.MsgTimestamp.Render("("+inp.field.EnvVar+")")
		}
		if inp.field.IsSet && inp.draft == "" {
			labelLine += "  " + lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓ set")
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
			cursor := lipgloss.NewStyle().Foreground(common.ColorPrimary).Render("█")
			valLine = "  ▶ " + lipgloss.NewStyle().Foreground(common.ColorText).Render(display) + cursor
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
				valLine = "    " + lipgloss.NewStyle().Foreground(common.ColorMuted).Render(display)
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
			st = lipgloss.NewStyle().Foreground(common.ColorGreen).Bold(true)
		} else {
			st = lipgloss.NewStyle().Foreground(common.ColorRed).Bold(true)
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

func (p *ConfigPanel) providerStatusTag(pv tui.ProviderStatus) string {
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
		return lipgloss.NewStyle().Foreground(common.ColorGreen).Render("✓ configured")
	}
	return lipgloss.NewStyle().Foreground(common.ColorRed).Render("✗ not configured")
}

// centred returns the panel horizontally centred (vertical centring via overlayOn).
func (p *ConfigPanel) Centered() string {
	return common.CenterHorizontally(p.View(), p.width)
}

func (p *ConfigPanel) IsEditing() bool          { return p.editing }
func (p *ConfigPanel) EditedProviderID() string { return p.editProvider.ID }
