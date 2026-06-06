package components

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// PermissionDialog is the modal overlay shown when the agent needs approval.
type PermissionDialog struct {
	styles  common.Styles
	pending *tui.PromptRequestMsg
	width   int
	height  int
}

func NewPermissionDialog(styles common.Styles) *PermissionDialog {
	return &PermissionDialog{styles: styles}
}

func (p *PermissionDialog) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *PermissionDialog) SetPending(msg *tui.PromptRequestMsg) {
	p.pending = msg
}

func (p *PermissionDialog) HasPending() bool {
	return p.pending != nil
}

// Resolve sends the user's response and clears the pending request.
func (p *PermissionDialog) Resolve(value any, cancelled bool) {
	if p.pending == nil {
		return
	}
	select {
	case p.pending.Response <- tui.PromptResponse{Value: value, Cancelled: cancelled}:
	default:
	}
	p.pending = nil
}

// View renders the full-width permission dialog.
func (p *PermissionDialog) View() string {
	if p.pending == nil {
		return ""
	}

	// Use most of the available width, capped for readability.
	w := max(60, min(p.width-4, 110))
	innerW := w - 4 // account for border + padding

	msg := p.pending
	meta := msg.Metadata

	toolName := stringMeta(meta, "tool_name")
	toolInput := normalizeMetaMap(meta["tool_input"])
	workDir := stringMeta(meta, "working_directory")

	var sections []string

	// ── Header ────────────────────────────────────────────────────────────
	sections = append(sections, p.styles.PermTitle.Render("  Permission Required"))

	// ── Tool context ──────────────────────────────────────────────────────
	if toolName != "" {
		sections = append(sections, p.renderToolContext(toolName, toolInput, workDir, innerW))
	} else if msg.Message != "" {
		sections = append(sections, p.styles.PermBody.Render(wordWrap(msg.Message, innerW)))
	}

	// ── Diff / content preview ────────────────────────────────────────────
	if preview := p.renderToolPreview(toolName, toolInput, innerW); preview != "" {
		sections = append(sections, preview)
	}

	// ── Choice options ────────────────────────────────────────────────────
	if msg.Type == "choice" && len(msg.Options) > 0 {
		var opts []string
		for i, opt := range msg.Options {
			opts = append(opts, p.styles.PermBody.Render(fmt.Sprintf("[%d] %s", i+1, opt.Label)))
		}
		sections = append(sections, strings.Join(opts, "  "))
	}

	// ── Action bar ────────────────────────────────────────────────────────
	sections = append(sections, p.renderActions(msg.Type))

	content := strings.Join(sections, "\n\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorYellow).
		Padding(1, 2).
		Width(w).
		Render(content)

	return centreOverlay(addDropShadow(box), p.width, p.height)
}

// renderToolContext builds the tool name + file path rows.
func (p *PermissionDialog) renderToolContext(toolName string, input map[string]any, workDir string, width int) string {
	labelW := 8
	label := func(s string) string {
		return p.styles.MsgTimestamp.Render(fmt.Sprintf("%-*s", labelW, s))
	}
	value := func(s string) string {
		return p.styles.PermBody.Render(s)
	}

	rows := []string{
		label("Tool") + value(toolDisplayName(toolName)),
	}

	if workDir != "" {
		rows = append(rows, label("Path")+value(tildePath(workDir)))
	}

	if filePath := stringFromInput(input, "file_path"); filePath != "" {
		rows = append(rows, label("File")+value(tildePath(filePath)))
	} else if url := stringFromInput(input, "url"); url != "" {
		rows = append(rows, label("URL")+value(truncate(url, width-labelW-2)))
	} else if cmd := stringFromInput(input, "command"); cmd != "" {
		cmdOneLine := strings.ReplaceAll(strings.TrimSpace(cmd), "\n", "; ")
		rows = append(rows, label("Command")+value(truncate(cmdOneLine, width-labelW-2)))
	}

	return strings.Join(rows, "\n")
}

// renderToolPreview shows a diff or file content preview for file-modifying tools.
func (p *PermissionDialog) renderToolPreview(toolName string, input map[string]any, width int) string {
	const maxPreviewLines = 15

	switch toolName {
	case "edit_file", "write_file", "apply_patch":
		// Try to get a diff from the input (patch field or old/new content).
		var diffBody string
		if patch := stringFromInput(input, "patch"); patch != "" {
			diffBody = patch
		} else if oldContent := stringFromInput(input, "old_content"); oldContent != "" {
			newContent := stringFromInput(input, "new_content")
			diffBody = buildSimpleDiff(oldContent, newContent)
		}
		if diffBody != "" {
			return renderDiffBody(p.styles, diffBody, width, maxPreviewLines)
		}
		// Fallback: show the file content for write.
		if content := stringFromInput(input, "content"); content != "" {
			path := stringFromInput(input, "file_path")
			return renderCodeBody(p.styles, path, content, width, maxPreviewLines, 0)
		}

	case "bash":
		if cmd := stringFromInput(input, "command"); cmd != "" {
			cmdBlock := strings.ReplaceAll(strings.TrimSpace(cmd), "\t", "    ")
			return renderCodeBody(p.styles, "command.sh", cmdBlock, width, maxPreviewLines, 0)
		}

	case "read_file":
		if path := stringFromInput(input, "file_path"); path != "" {
			return p.styles.MsgTimestamp.Render("Reading " + prettyPath(path))
		}
	}
	return ""
}

// renderActions renders the keyboard shortcut bar at the bottom, right-aligned.
func (p *PermissionDialog) renderActions(promptType string) string {
	w := max(60, min(p.width-4, 110))
	innerW := w - 4 // account for border + padding
	label := func(s string) string {
		return p.styles.PermBody.Render(s)
	}
	sep := p.styles.MsgTimestamp.Render("   ·   ")
	switch promptType {
	case "confirm":
		allow := p.styles.PermYes.Render("[y]") + label(" Allow")
		always := p.styles.PermAlways.Render("[a]") + label(" Allow for session")
		deny := p.styles.PermNo.Render("[n]") + label(" Deny")
		buttons := lipgloss.JoinHorizontal(lipgloss.Left, allow, sep, always, sep, deny)
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, buttons)
	case "choice":
		hint := p.styles.MsgTimestamp.Render("enter number · esc to cancel")
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, hint)
	default:
		hint := p.styles.MsgTimestamp.Render("type your response · enter to confirm · esc to cancel")
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, hint)
	}
}

// buildSimpleDiff produces a minimal unified-style diff from two strings.
func buildSimpleDiff(old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")
	var out strings.Builder
	maxLen := max(len(oldLines), len(newLines))
	for i := 0; i < maxLen; i++ {
		var o, n string
		if i < len(oldLines) {
			o = oldLines[i]
		}
		if i < len(newLines) {
			n = newLines[i]
		}
		if o == n {
			fmt.Fprintf(&out, " %s\n", o)
		} else {
			if o != "" {
				fmt.Fprintf(&out, "-%s\n", o)
			}
			if n != "" {
				fmt.Fprintf(&out, "+%s\n", n)
			}
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func stringMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func normalizeMetaMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func stringFromInput(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// tildePath replaces the home directory prefix with ~.
func tildePath(path string) string {
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return prettyPath(path)
}

func centreOverlay(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	boxH := len(lines)
	boxW := lipgloss.Width(content)

	topPad := max(0, (height-boxH)/2)
	leftPad := max(0, (width-boxW)/2)
	pad := strings.Repeat(" ", leftPad)

	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pad + line)
	}
	return sb.String()
}

// addDropShadow adds a subtle bottom-right shadow to give the dialog an elevated look.
func addDropShadow(box string) string {
	lines := strings.Split(box, "\n")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#2A2A2A")).Faint(true)
	sv := dim.Render("░")

	var out []string
	for i, line := range lines {
		if i == 0 {
			out = append(out, line) // no shadow on top edge
		} else {
			out = append(out, line+sv)
		}
	}
	// Bottom shadow row, shifted 1 right so it peeks out below the box.
	if len(lines) > 0 {
		boxW := lipgloss.Width(lines[0])
		out = append(out, " "+dim.Render(strings.Repeat("░", boxW)))
	}
	return strings.Join(out, "\n")
}

func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	var sb strings.Builder
	words := strings.Fields(text)
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			sb.WriteString(line + "\n")
			line = w
		}
	}
	if line != "" {
		sb.WriteString(line)
	}
	return sb.String()
}
