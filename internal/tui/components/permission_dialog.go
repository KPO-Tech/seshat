package components

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wrap"
)

const permPreviewH = 20

// PermissionDialog is the modal overlay shown when the agent needs approval.
type PermissionDialog struct {
	styles        common.Styles
	pending       *tui.PromptRequestMsg
	width         int
	height        int
	previewScroll int
	previewTotal  int
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
	p.previewScroll = 0
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

// HandleKey handles scroll keys within the preview. Returns true if consumed.
func (p *PermissionDialog) HandleKey(k string) bool {
	if p.previewTotal <= permPreviewH {
		return false
	}
	maxScroll := max(0, p.previewTotal-permPreviewH)
	switch k {
	case "up":
		if p.previewScroll > 0 {
			p.previewScroll--
			return true
		}
	case "down":
		if p.previewScroll < maxScroll {
			p.previewScroll++
			return true
		}
	case "pgup":
		p.previewScroll = max(0, p.previewScroll-permPreviewH)
		return true
	case "pgdown":
		p.previewScroll = min(maxScroll, p.previewScroll+permPreviewH)
		return true
	case "home":
		p.previewScroll = 0
		return true
	case "end":
		p.previewScroll = maxScroll
		return true
	}
	return false
}

// View renders the full-width permission dialog.
func (p *PermissionDialog) View() string {
	if p.pending == nil {
		return ""
	}

	w := max(60, min(p.width-4, 110))
	innerW := w - 4

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

	// ── Scrollable preview ────────────────────────────────────────────────
	previewLines := p.computePreviewLines(innerW)
	p.previewTotal = len(previewLines)
	if len(previewLines) > 0 {
		start := min(p.previewScroll, max(0, len(previewLines)-permPreviewH))
		end := min(start+permPreviewH, len(previewLines))
		previewStr := strings.Join(previewLines[start:end], "\n")
		if len(previewLines) > permPreviewH {
			hint := fmt.Sprintf("  %d / %d lines  ↑↓ to scroll", end, len(previewLines))
			previewStr += "\n" + p.styles.MsgTimestamp.Render(hint)
		}
		sections = append(sections, previewStr)
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

// computePreviewLines builds the full list of rendered lines for the scrollable
// preview area. Returns nil when there is nothing to show.
func (p *PermissionDialog) computePreviewLines(innerW int) []string {
	if p.pending == nil {
		return nil
	}
	meta := p.pending.Metadata
	toolName := stringMeta(meta, "tool_name")
	toolInput := normalizeMetaMap(meta["tool_input"])

	switch toolName {
	case "edit_file", "apply_patch":
		var diffBody string
		if patch := stringFromInput(toolInput, "patch"); patch != "" {
			diffBody = patch
		} else if old := stringFromInput(toolInput, "old_content"); old != "" {
			diffBody = buildSimpleDiff(old, stringFromInput(toolInput, "new_content"))
		}
		if diffBody != "" {
			return renderPermDiffLines(p.styles, diffBody, innerW)
		}
		if content := stringFromInput(toolInput, "content"); content != "" {
			return renderPermContentLines(p.styles, stringFromInput(toolInput, "file_path"), content, innerW)
		}

	case "write_file":
		content := stringFromInput(toolInput, "content")
		path := stringFromInput(toolInput, "file_path")
		if content != "" {
			return renderPermContentLines(p.styles, path, content, innerW)
		}

	case "bash":
		cmd := strings.TrimSpace(stringFromInput(toolInput, "command"))
		if cmd != "" {
			return renderPermCodeLines(p.styles, "command.sh", cmd, innerW)
		}

	case "read_file":
		if path := stringFromInput(toolInput, "file_path"); path != "" {
			return []string{p.styles.MsgTimestamp.Render("Reading " + prettyPath(path))}
		}
	}
	return nil
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

// renderActions renders the keyboard shortcut bar at the bottom, right-aligned.
func (p *PermissionDialog) renderActions(promptType string) string {
	w := max(60, min(p.width-4, 110))
	innerW := w - 4
	sep := p.styles.MsgTimestamp.Render("   ·   ")
	body := p.styles.PermBody.Render
	switch promptType {
	case "confirm":
		buttons := p.styles.PermYes.Render("[y]") + body(" Allow") +
			sep +
			p.styles.PermAlways.Render("[a]") + body(" Allow for session") +
			sep +
			p.styles.PermNo.Render("[n]") + body(" Deny")
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, buttons)
	case "choice":
		hint := p.styles.MsgTimestamp.Render("enter number · esc to cancel")
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, hint)
	default:
		hint := p.styles.MsgTimestamp.Render("type your response · enter to confirm · esc to cancel")
		return lipgloss.PlaceHorizontal(innerW, lipgloss.Right, hint)
	}
}

// ── Preview renderers ─────────────────────────────────────────────────────────

// renderPermContentLines dispatches to markdown or code renderer based on file type.
func renderPermContentLines(styles common.Styles, path, body string, width int) []string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown", ".txt", ".rst":
		return renderPermMarkdownLines(width, body)
	default:
		return renderPermCodeLines(styles, path, body, width)
	}
}

// renderPermMarkdownLines renders markdown body with glamour (full colors).
func renderPermMarkdownLines(width int, body string) []string {
	rendered, err := common.RenderMarkdown(width, body)
	if err != nil || rendered == "" {
		var lines []string
		for _, ln := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
			lines = append(lines, common.Escape(ln))
		}
		return lines
	}
	return strings.Split(rendered, "\n")
}

// renderPermCodeLines renders code with line numbers, syntax highlighting and
// soft-wrapping (continuation lines are indented to align with text start).
func renderPermCodeLines(styles common.Styles, path, body string, width int) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	allLines := strings.Split(body, "\n")

	highlighted := common.SyntaxHighlight(body, path)
	hlLines := strings.Split(highlighted, "\n")
	if len(hlLines) != len(allLines) {
		hlLines = allLines
	}

	numWidth := len(fmt.Sprintf("%d", len(allLines)))
	numFmt := fmt.Sprintf("%%%dd ", numWidth)
	numColW := numWidth + 1
	contentW := max(1, width-numColW)
	indent := strings.Repeat(" ", numColW)

	var result []string
	for i, ln := range hlLines {
		lineNum := styles.ToolLineNumber.Render(fmt.Sprintf(numFmt, i+1))
		wrapped := wrap.String(ln, contentW)
		parts := strings.Split(wrapped, "\n")
		result = append(result, lineNum+parts[0])
		for _, cont := range parts[1:] {
			result = append(result, indent+cont)
		}
	}
	return result
}

// renderPermDiffLines renders a unified diff with full-row colored backgrounds.
// Deleted lines get a red background; added lines get a green background.
func renderPermDiffLines(styles common.Styles, diffBody string, width int) []string {
	diffBody = strings.ReplaceAll(diffBody, "\r\n", "\n")

	delBg := lipgloss.Color("#3a0a0a")
	addBg := lipgloss.Color("#0a2a0a")
	delFg := lipgloss.Color("#ff9999")
	addFg := lipgloss.Color("#99ff99")
	hunkFg := lipgloss.Color("#6e88a1")
	dimFg := lipgloss.Color("#555566")

	numW := 4
	markerW := 2
	prefixW := numW + 1 + numW + 1 + markerW
	contentW := max(1, width-prefixW)

	var result []string
	oldN, newN := 1, 1

	for _, line := range strings.Split(diffBody, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			old, new := parseHunkHeader(line)
			if old > 0 {
				oldN = old
			}
			if new > 0 {
				newN = new
			}
			result = append(result, lipgloss.NewStyle().Foreground(hunkFg).Render(
				ansi.Truncate(line, width, "…"),
			))

		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			result = append(result, lipgloss.NewStyle().Foreground(dimFg).Render(
				ansi.Truncate(line, width, "…"),
			))

		case strings.HasPrefix(line, "-"):
			content := line[1:]
			nums := fmt.Sprintf("%*d %-*s ", numW, oldN, numW, "")
			text := nums + "- " + ansi.Truncate(content, contentW, "…")
			result = append(result, lipgloss.NewStyle().
				Background(delBg).Foreground(delFg).Width(width).Render(text))
			oldN++

		case strings.HasPrefix(line, "+"):
			content := line[1:]
			nums := fmt.Sprintf("%-*s %*d ", numW, "", numW, newN)
			text := nums + "+ " + ansi.Truncate(content, contentW, "…")
			result = append(result, lipgloss.NewStyle().
				Background(addBg).Foreground(addFg).Width(width).Render(text))
			newN++

		default:
			content := strings.TrimPrefix(line, " ")
			nums := fmt.Sprintf("%*d %*d ", numW, oldN, numW, newN)
			numStr := lipgloss.NewStyle().Foreground(dimFg).Render(nums + "  ")
			result = append(result, numStr+common.Escape(ansi.Truncate(content, contentW, "…")))
			oldN++
			newN++
		}
	}
	return result
}

// parseHunkHeader parses a unified diff @@ header and returns old/new start lines.
func parseHunkHeader(line string) (oldStart, newStart int) {
	var oS, oL, nS, nL int
	if _, err := fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oS, &oL, &nS, &nL); err == nil {
		return oS, nS
	}
	if _, err := fmt.Sscanf(line, "@@ -%d +%d @@", &oS, &nS); err == nil {
		return oS, nS
	}
	return 0, 0
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
			out = append(out, line)
		} else {
			out = append(out, line+sv)
		}
	}
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
