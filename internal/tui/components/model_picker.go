package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// modelDialog is the Ctrl+M model selection overlay.
// Models are grouped by provider, with providers shown as headers.
type ModelPicker struct {
	styles common.Styles
	models []tui.ProviderModel
	list   common.ListState[tui.ProviderModel]
	width  int
	height int
}

func NewModelPicker(styles common.Styles) *ModelPicker {
	return &ModelPicker{
		styles: styles,
		list: common.NewListState(func(m tui.ProviderModel, needle string) bool {
			return strings.Contains(strings.ToLower(m.DisplayName), needle) ||
				strings.Contains(strings.ToLower(m.Description), needle) ||
				strings.Contains(strings.ToLower(m.Provider), needle)
		}),
	}
}

func (d *ModelPicker) SetModels(models []tui.ProviderModel) {
	d.models = models
	d.list.SetItems(models)
}

func (d *ModelPicker) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *ModelPicker) TypeFilter(ch string) { d.list.TypeFilter(ch) }
func (d *ModelPicker) DeleteFilter()        { d.list.DeleteFilter() }
func (d *ModelPicker) ClearFilter()         { d.list.ClearFilter() }
func (d *ModelPicker) Up()                  { d.list.Up() }
func (d *ModelPicker) Down()                { d.list.Down() }

// Selected returns the currently highlighted model, or nil.
func (d *ModelPicker) Selected() *tui.ProviderModel {
	m, ok := d.list.Selected()
	if !ok {
		return nil
	}
	return &m
}

// prettyProvider returns a display name for a provider identifier.
func prettyProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "google":
		return "Google"
	case "mistral":
		return "Mistral"
	case "groq":
		return "Groq"
	case "ollama":
		return "Ollama"
	case "cohere":
		return "Cohere"
	case "deepseek":
		return "DeepSeek"
	case "":
		return "Other"
	default:
		if len(p) > 0 {
			return strings.ToUpper(p[:1]) + p[1:]
		}
		return "Other"
	}
}

// visualRow represents one rendered line in the dialog list.
// cursorIdx == -1 for provider headers (not selectable).
type visualRow struct {
	cursorIdx int
	text      string
}

// buildVisualRows groups filtered models by provider and returns visual rows.
func (d *ModelPicker) buildVisualRows(innerW int) []visualRow {
	type group struct {
		provider string
		models   []tui.ProviderModel
	}

	var groups []group
	providerIdx := map[string]int{}
	for _, m := range d.list.FilteredItems() {
		key := strings.ToLower(strings.TrimSpace(m.Provider))
		if key == "" {
			key = "other"
		}
		if i, ok := providerIdx[key]; ok {
			groups[i].models = append(groups[i].models, m)
		} else {
			providerIdx[key] = len(groups)
			groups = append(groups, group{provider: m.Provider, models: []tui.ProviderModel{m}})
		}
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(common.ColorSecondary).
		Padding(0, 1)

	var rows []visualRow
	globalIdx := 0
	for gi, g := range groups {
		// Blank line between provider groups (not before the first).
		if gi > 0 {
			rows = append(rows, visualRow{cursorIdx: -1, text: ""})
		}
		// Provider header.
		rows = append(rows, visualRow{
			cursorIdx: -1,
			text:      headerStyle.Render(prettyProvider(g.provider)),
		})

		for _, m := range g.models {
			ctx := ""
			if m.Context > 0 {
				ctx = fmt.Sprintf("  %dk ctx", m.Context/1000)
			}
			// Extract just the model identifier part (strip "Provider / " prefix from DisplayName if present).
			name := m.Identifier
			if name == "" {
				name = m.DisplayName
			}

			selected := globalIdx == d.list.Cursor()

			// Build the right-side info (description + context).
			info := ""
			if m.Description != "" {
				maxInfo := innerW - len(name) - len(ctx) - 12
				if maxInfo > 8 {
					desc := m.Description
					if len(desc) > maxInfo {
						desc = desc[:maxInfo-1] + "…"
					}
					info = d.styles.MsgTimestamp.Render(desc)
				}
			}
			ctxStr := d.styles.MsgTimestamp.Render(ctx)

			var text string
			if selected {
				nameStr := lipgloss.NewStyle().Bold(true).Foreground(common.ColorPrimary).Render(name)
				left := "    ▶ " + nameStr
				if info != "" {
					left += "  " + info
				}
				pad := innerW - lipgloss.Width(left) - lipgloss.Width(ctxStr) - 2
				if pad < 1 {
					pad = 1
				}
				text = d.styles.BrowserSelected.Width(innerW).Render(
					left + strings.Repeat(" ", pad) + ctxStr,
				)
			} else {
				nameStr := lipgloss.NewStyle().Foreground(common.ColorText).Render(name)
				left := "      " + nameStr
				if info != "" {
					left += "  " + info
				}
				pad := innerW - lipgloss.Width(left) - lipgloss.Width(ctxStr) - 2
				if pad < 1 {
					pad = 1
				}
				text = d.styles.BrowserItem.Width(innerW).Render(
					left + strings.Repeat(" ", pad) + ctxStr,
				)
			}

			rows = append(rows, visualRow{cursorIdx: globalIdx, text: text})
			globalIdx++
		}
	}
	return rows
}

// View renders the model selection panel.
func (d *ModelPicker) View() string {
	// Width: 80% of terminal, capped at 90, minimum 54.
	w := common.Clamp(d.width*4/5, 54, 90)
	innerW := w - 4 // account for border + padding

	title := d.styles.BrowserTitle.Render("  Switch Model")
	filterLine := d.styles.BrowserFilter.Width(innerW).Render("  > " + d.list.Filter() + "█")
	sep := d.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	// Build all visual rows.
	allRows := d.buildVisualRows(innerW)

	// Find the visual position of the selected cursor row.
	selectedVR := 0
	for i, row := range allRows {
		if row.cursorIdx == d.list.Cursor() {
			selectedVR = i
			break
		}
	}

	// Determine the scroll window — max lines in the content area.
	// Available content height: terminal height minus chrome (title, filter, seps, hint = ~6 lines).
	maxVisible := common.Clamp(d.height-10, 6, 18)

	start := 0
	if selectedVR >= maxVisible {
		start = selectedVR - maxVisible + 1
	}
	end := min(len(allRows), start+maxVisible)

	var rowTexts []string
	for _, row := range allRows[start:end] {
		rowTexts = append(rowTexts, row.text)
	}
	if len(rowTexts) == 0 {
		rowTexts = append(rowTexts, d.styles.BrowserItem.Render("  no matches"))
	}

	scrollNote := ""
	filtered := d.list.FilteredItems()
	if len(filtered) > 0 {
		visible := 0
		for _, row := range allRows[start:end] {
			if row.cursorIdx >= 0 {
				visible++
			}
		}
		if len(filtered) > visible {
			scrollNote = d.styles.MsgTimestamp.Render(
				fmt.Sprintf("  %d of %d models", d.list.Cursor()+1, len(filtered)),
			)
		}
	}

	hint := d.styles.Footer.Render("  ↑↓ navigate  enter select  ← back  esc close")

	parts := []string{title, filterLine, sep}
	parts = append(parts, rowTexts...)
	parts = append(parts, sep)
	if scrollNote != "" {
		parts = append(parts, scrollNote)
	}
	parts = append(parts, hint)

	content := strings.Join(parts, "\n")
	return d.styles.BrowserBorder.Width(w).Render(content)
}

// centred returns the panel positioned horizontally centred.
// Vertical centering is handled by overlayOn().
func (d *ModelPicker) Centered() string {
	return common.CenterHorizontally(d.View(), d.width)
}

func (d *ModelPicker) SetCursor(idx int) {
	d.list.SetCursor(idx)
}

func (d *ModelPicker) ClickRow(localY int) (selected bool, activated bool) {
	w := common.Clamp(d.width*4/5, 54, 90)
	innerW := w - 4
	allRows := d.buildVisualRows(innerW)

	// Find the visual position of the selected cursor row.
	selectedVR := 0
	for i, row := range allRows {
		if row.cursorIdx == d.list.Cursor() {
			selectedVR = i
			break
		}
	}

	maxVisible := common.Clamp(d.height-10, 6, 18)
	start := 0
	if selectedVR >= maxVisible {
		start = selectedVR - maxVisible + 1
	}

	// Items start at line 4 (after title, filter line, sep)
	visibleRowsCount := min(len(allRows), start+maxVisible) - start
	if localY >= 4 && localY < 4+visibleRowsCount {
		allRowsIdx := start + localY - 4
		if allRowsIdx >= 0 && allRowsIdx < len(allRows) {
			vr := allRows[allRowsIdx]
			if vr.cursorIdx >= 0 {
				if vr.cursorIdx == d.list.Cursor() {
					return true, true
				}
				d.list.SetCursor(vr.cursorIdx)
				return true, false
			}
		}
	}
	return false, false
}
