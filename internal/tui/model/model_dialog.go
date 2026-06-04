package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
)

// modelDialog is the Ctrl+M model selection overlay.
// Models are grouped by provider, with providers shown as headers.
type modelDialog struct {
	styles   Styles
	models   []tui.ProviderModel
	filtered []tui.ProviderModel
	filter   string
	cursor   int // index into filtered[] (headers excluded)
	width    int
	height   int
}

func newModelDialog(styles Styles) *modelDialog {
	return &modelDialog{styles: styles}
}

func (d *modelDialog) SetModels(models []tui.ProviderModel) {
	d.models = models
	d.cursor = 0
	d.applyFilter()
}

func (d *modelDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *modelDialog) TypeFilter(ch string)  { d.filter += ch; d.cursor = 0; d.applyFilter() }
func (d *modelDialog) DeleteFilter() {
	if len(d.filter) > 0 {
		d.filter = d.filter[:len(d.filter)-1]
		d.cursor = 0
		d.applyFilter()
	}
}
func (d *modelDialog) ClearFilter() { d.filter = ""; d.cursor = 0; d.applyFilter() }

func (d *modelDialog) Up() {
	if d.cursor > 0 {
		d.cursor--
	}
}

func (d *modelDialog) Down() {
	if d.cursor < len(d.filtered)-1 {
		d.cursor++
	}
}

// Selected returns the currently highlighted model, or nil.
func (d *modelDialog) Selected() *tui.ProviderModel {
	if d.cursor >= 0 && d.cursor < len(d.filtered) {
		m := d.filtered[d.cursor]
		return &m
	}
	return nil
}

func (d *modelDialog) applyFilter() {
	if d.filter == "" {
		d.filtered = make([]tui.ProviderModel, len(d.models))
		copy(d.filtered, d.models)
		return
	}
	needle := strings.ToLower(d.filter)
	d.filtered = d.filtered[:0]
	for _, m := range d.models {
		if strings.Contains(strings.ToLower(m.DisplayName), needle) ||
			strings.Contains(strings.ToLower(m.Description), needle) ||
			strings.Contains(strings.ToLower(m.Provider), needle) {
			d.filtered = append(d.filtered, m)
		}
	}
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
func (d *modelDialog) buildVisualRows(innerW int) []visualRow {
	type group struct {
		provider string
		models   []tui.ProviderModel
	}

	var groups []group
	providerIdx := map[string]int{}
	for _, m := range d.filtered {
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
		Foreground(colorSecondary).
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

			selected := globalIdx == d.cursor

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
				nameStr := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render(name)
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
				nameStr := lipgloss.NewStyle().Foreground(colorText).Render(name)
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
func (d *modelDialog) View() string {
	// Width: 80% of terminal, capped at 90, minimum 54.
	w := clamp(d.width*4/5, 54, 90)
	innerW := w - 4 // account for border + padding

	title := d.styles.BrowserTitle.Render("  Switch Model")
	filterLine := d.styles.BrowserFilter.Width(innerW).Render("  / " + d.filter + "█")
	sep := d.styles.MsgTimestamp.Render(strings.Repeat("─", innerW))

	// Build all visual rows.
	allRows := d.buildVisualRows(innerW)

	// Find the visual position of the selected cursor row.
	selectedVR := 0
	for i, row := range allRows {
		if row.cursorIdx == d.cursor {
			selectedVR = i
			break
		}
	}

	// Determine the scroll window — max lines in the content area.
	// Available content height: terminal height minus chrome (title, filter, seps, hint = ~6 lines).
	maxVisible := clamp(d.height-10, 6, 18)

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
	if len(d.filtered) > 0 {
		visible := 0
		for _, row := range allRows[start:end] {
			if row.cursorIdx >= 0 {
				visible++
			}
		}
		if len(d.filtered) > visible {
			scrollNote = d.styles.MsgTimestamp.Render(
				fmt.Sprintf("  %d of %d models", d.cursor+1, len(d.filtered)),
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
func (d *modelDialog) centred() string {
	box := d.View()
	lines := strings.Split(box, "\n")
	boxW := lipgloss.Width(lines[0]) // use first line (top border) for true width
	left := max(0, (d.width-boxW)/2)
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
