package components

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wrap"
)

type contentFlavor int

const (
	contentFlavorPlain contentFlavor = iota
	contentFlavorMarkdown
	contentFlavorCode
)

func (t *toolItem) inlinePreview(c *Chat, width int) string {
	input := t.toolInput()
	switch t.name {
	case "read_file":
		body := stringFromMap(t.metadata, "content")
		if body == "" {
			body = stringFromMap(t.metadata, "result")
		}
		start, clean := parseReadContent(body)
		return renderCodePanel(c.styles, stringFromMap(input, "file_path"), clean, width-4, inlinePreviewLines, start)
	case "write_file":
		return renderFilePanel(c.styles, stringFromMap(input, "file_path"), stringFromMap(input, "content"), width-4, inlinePreviewLines)
	case "edit_file", "apply_patch":
		diff := t.unifiedDiff()
		if diff == "" {
			diff = stringFromMap(t.metadata, "diff")
		}
		if diff != "" {
			return renderColoredDiff(c.styles, diff, width-4, inlinePreviewLines)
		}
		return renderFilePanel(c.styles, stringFromMap(input, "file_path"), stringFromMap(input, "content"), width-4, inlinePreviewLines)
	case "bash":
		return renderBashInline(c.styles, stringFromMap(input, "command"), stringFromMap(t.metadata, "content"), width-4, inlinePreviewLines)
	case "web_search":
		return renderContentPanel(c.styles, "", stringFromMap(t.metadata, "content"), width-4, inlinePreviewLines, contentFlavorPlain)
	case "web_fetch":
		return renderContentPanel(c.styles, "", stringFromMap(t.metadata, "content"), width-4, inlinePreviewLines, contentFlavorPlain)
	}
	if res := t.resultContent(); res != "" {
		return renderPlainBody(res, width-4)
	}
	return ""
}

func (t *toolItem) detailView(c *Chat, width, height int) string {
	return c.renderToolDetail(t, width, height)
}

func (t *toolItem) metaSummary() string {
	input := t.toolInput()
	var parts []string
	if workDir := stringFromMap(t.metadata, "working_directory"); workDir != "" {
		parts = append(parts, "Directory: "+prettyPath(workDir))
	}
	switch t.name {
	case "read_file", "write_file", "edit_file":
		if path := stringFromMap(input, "file_path"); path != "" {
			parts = append(parts, "Path: "+prettyPath(path))
		}
	case "apply_patch":
		if path := stringFromMap(input, "file_path"); path != "" {
			parts = append(parts, "Path: "+prettyPath(path))
		}
	case "web_search":
		if q := stringFromMap(input, "query"); q != "" {
			parts = append(parts, "Query: "+q)
		}
	case "web_fetch":
		if url := stringFromMap(input, "url"); url != "" {
			parts = append(parts, "URL: "+url)
		}
	case "spawn_agent":
		if nickname := stringFromMap(input, "nickname"); nickname != "" {
			parts = append(parts, "Nickname: "+nickname)
		}
		if id := stringFromMap(t.metadata, "agent_id"); id != "" {
			parts = append(parts, "Agent ID: "+id)
		}
	case "wait_agent", "close_agent", "send_agent_message":
		if id := stringFromMap(input, "agent_id"); id != "" {
			parts = append(parts, "Agent ID: "+id)
		}
	}
	return strings.Join(parts, "\n")
}

func (t *toolItem) detailBody(c *Chat, width int) string {
	if t.isDone() && t.detailCacheW == width && t.detailCacheR != "" {
		return t.detailCacheR
	}
	var res string
	switch t.name {
	case "list_directory", "glob":
		res = renderDirListing(c.styles, t.resultContent(), width, 0)
	case "read_file":
		path := stringFromMap(t.toolInput(), "file_path")
		startLine, cleanBody := parseReadContent(t.resultContent())
		if flavorForPath(path) == contentFlavorCode {
			res = renderCodeBody(c.styles, path, cleanBody, width, 0, startLine)
		} else {
			res = renderContentBody(c.styles, cleanBody, width, flavorForPath(path))
		}
	case "write_file":
		path := stringFromMap(t.toolInput(), "file_path")
		content := t.writeContent()
		if flavorForPath(path) == contentFlavorCode {
			res = renderCodeBody(c.styles, path, content, width, 0, 0)
		} else {
			res = renderContentBody(c.styles, content, width, flavorForPath(path))
		}
	case "edit_file", "apply_patch":
		path := stringFromMap(t.toolInput(), "file_path")
		if diff := t.unifiedDiff(); diff != "" {
			label := "patch"
			if path != "" {
				label = prettyPath(path)
			}
			label = ansi.Truncate(label, width, "…")
			res = c.styles.Key.Render(label) + "\n\n" + renderColoredDiff(c.styles, diff, width, 0)
		}
	case "bash":
		res = renderBashDetails(c.styles, stringFromMap(t.toolInput(), "command"), t.commandOutput(), width)
	case "web_search":
		res = renderWebSearchDetails(c.styles, t.summaryText(), t.resultContent(), width)
	case "web_fetch":
		res = renderWebFetchDetails(c.styles, t.summaryText(), t.resultContent(), width)
	case "agent", "spawn_agent", "wait_agent", "close_agent", "send_agent_message":
		res = renderContentBody(c.styles, t.agentDetails(), width, contentFlavorMarkdown)
	default:
		res = renderContentBody(c.styles, t.resultContent(), width, contentFlavorPlain)
	}
	if t.isDone() {
		t.detailCacheW = width
		t.detailCacheR = res
	}
	return res
}

func flavorForPath(path string) contentFlavor {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return contentFlavorMarkdown
	case ".txt", ".log", "":
		return contentFlavorPlain
	default:
		return contentFlavorCode
	}
}

func (t *toolItem) detailCacheKey(width, height int, body string) string {
	return fmt.Sprintf("%d-%d-%d", width, height, len(body))
}

func (t *toolItem) changeStatsText() string {
	added, ok1 := intFromMap(t.metadata, "lines_added")
	deleted, ok2 := intFromMap(t.metadata, "lines_removed")
	if !ok1 && !ok2 {
		return ""
	}
	parts := make([]string, 0, 2)
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("-%d", deleted))
	}
	return strings.Join(parts, " ")
}

func renderDiffSections(styles common.Styles, title, body string, width int) string {
	out := styles.Key.Render(title)
	if strings.TrimSpace(body) != "" {
		out += "\n\n" + renderDiffBody(styles, body, width, 0)
	}
	return out
}

func renderBashDetails(styles common.Styles, cmd, output string, width int) string {
	var sections []string
	if cmd = strings.TrimSpace(cmd); cmd != "" {
		sections = append(sections, styles.Key.Render("Command")+"\n\n"+renderCodeBody(styles, "command.sh", cmd, width, 0, 0))
	}
	if output = strings.TrimSpace(output); output != "" {
		sections = append(sections, styles.Key.Render("Output")+"\n\n"+renderPlainBody(output, width))
	}
	return strings.Join(sections, "\n\n")
}

func renderWebSearchDetails(styles common.Styles, summary, body string, width int) string {
	brief, cards := parseWebSearchContent(body)
	if brief == "" {
		brief = summary
	}
	var out strings.Builder
	if brief != "" {
		out.WriteString(renderMarkdownBody(brief, width))
	}
	for i, card := range cards {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(renderSearchResultCard(styles, i+1, card, width))
	}
	if out.Len() == 0 {
		return renderPlainBody(body, width)
	}
	return out.String()
}

func renderWebFetchDetails(styles common.Styles, summary, body string, width int) string {
	brief, content := splitWebFetchContent(body)
	if brief == "" {
		brief = summary
	}
	var out strings.Builder
	if brief != "" {
		out.WriteString(renderMarkdownBody(brief, width))
	}
	if content != "" {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(styles.MsgTimestamp.Render(headerLine(styles.Footer, width)) + "\n\n")
		out.WriteString(renderMarkdownBody(content, width))
	}
	if out.Len() == 0 {
		return renderPlainBody(body, width)
	}
	return out.String()
}

type webSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func parseWebSearchContent(body string) (string, []webSearchResult) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	parts := strings.SplitN(body, "\n\nJSON Results:\n", 2)
	if len(parts) < 2 {
		return "", nil
	}
	var results []webSearchResult
	if err := json.Unmarshal([]byte(parts[1]), &results); err != nil {
		return parts[0], nil
	}
	return parts[0], results
}

func renderSearchResultCard(styles common.Styles, index int, result webSearchResult, width int) string {
	w := max(12, width-2)
	card := fmt.Sprintf("[%d] %s\n%s\n\n%s", index, result.Title, styles.HeaderID.Render(result.URL), result.Description)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(w).
		Render(card)
}

func leadingNumberedItem(line string) int {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return -1
	}
	idx := strings.Index(line, "]")
	if idx <= 1 {
		return -1
	}
	val, err := strconv.Atoi(line[1:idx])
	if err != nil {
		return -1
	}
	return val
}

func splitWebFetchContent(body string) (string, string) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	parts := strings.SplitN(body, "\n\nMarkdown Content:\n", 2)
	if len(parts) < 2 {
		return "", body
	}
	return parts[0], parts[1]
}

func (t *toolItem) resultContent() string {
	if res, ok := stringAny(t.metadata["result"]); ok && res != "" {
		return res
	}
	if content, ok := stringAny(t.metadata["content"]); ok && content != "" {
		return content
	}
	return ""
}

func (t *toolItem) diffPreview() string {
	var sb strings.Builder
	if path := stringFromMap(t.toolInput(), "file_path"); path != "" {
		sb.WriteString(compactPath(path) + " ")
	}
	if stats := t.changeStatsText(); stats != "" {
		sb.WriteString(stats)
	}
	return sb.String()
}

func (t *toolItem) writeContent() string {
	return stringFromMap(t.toolInput(), "content")
}

func (t *toolItem) unifiedDiff() string {
	meta := t.metadata
	if patch, ok := meta["patch"].(string); ok && patch != "" {
		return patch
	}
	if diff, ok := meta["diff"].(string); ok && diff != "" {
		return diff
	}
	hunksVal, ok := meta["hunks"]
	if !ok {
		return ""
	}
	b, err := json.Marshal(hunksVal)
	if err != nil {
		return ""
	}
	var hunks []struct {
		Header string `json:"header"`
		Lines  string `json:"lines"`
	}
	if err := json.Unmarshal(b, &hunks); err != nil {
		return ""
	}
	var sb strings.Builder
	for i, hunk := range hunks {
		sb.WriteString(hunk.Header + "\n" + hunk.Lines)
		if i < len(hunks)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (t *toolItem) commandOutput() string {
	var sb strings.Builder
	input := t.toolInput()
	if cmd := stringFromMap(input, "command"); cmd != "" {
		sb.WriteString(cmd + "\n")
	}
	if output := stringFromMap(t.metadata, "content"); output != "" {
		sb.WriteString(output)
	} else if result := stringFromMap(t.metadata, "result"); result != "" {
		sb.WriteString(result)
	}
	return sb.String()
}

func (t *toolItem) agentDetails() string {
	input := t.toolInput()
	var sb strings.Builder
	prompt := stringFromMap(input, "prompt")
	if prompt == "" {
		prompt = stringFromMap(input, "task")
	}
	if prompt != "" {
		sb.WriteString("### Prompt\n" + prompt)
	}
	if log := stringFromMap(t.metadata, "subagent_log"); log != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("### Activity\n" + log)
	}
	res := t.resultContent()
	if res != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("### Result\n" + res)
	}
	return sb.String()
}

func (c *Chat) DetailView(width, height int) string {
	tool := c.selectedToolItem()
	if tool == nil {
		return ""
	}
	return tool.detailView(c, width, height)
}

func (c *Chat) renderToolDetail(t *toolItem, width, height int) string {
	if width < 20 || height < 6 {
		return ""
	}
	innerW := max(10, width-4)
	clampLines := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, innerW, "…")
		}
		return strings.Join(lines, "\n")
	}
	sections := []string{
		ansi.Truncate(c.styles.AssistantLabel.Render(toolDisplayName(t.name)), innerW, "…"),
		ansi.Truncate(c.styles.MsgTimestamp.Render(strings.ToUpper(t.status)), innerW, "…"),
	}
	if summary := strings.TrimSpace(t.summaryText()); summary != "" {
		sections = append(sections, clampLines(wrap.String(summary, innerW)))
	}
	if meta := strings.TrimSpace(t.metaSummary()); meta != "" {
		sections = append(sections, clampLines(wrap.String(meta, innerW)))
	}
	header := strings.Join(sections, "\n\n")
	body := strings.TrimSpace(t.detailBody(c, innerW))
	if body == "" {
		body = c.styles.MsgTimestamp.Render("No output")
	}
	bodyH := max(3, height-lipgloss.Height(header)-4)
	key := t.detailCacheKey(innerW, bodyH, body)

	// Apply dimension changes first so SetContent lays out correctly.
	sizeChanged := c.detail.Width() != innerW || c.detail.Height() != bodyH
	if c.detail.Width() != innerW {
		c.detail.SetWidth(innerW)
	}
	if c.detail.Height() != bodyH {
		c.detail.SetHeight(bodyH)
	}

	switch {
	case c.detailToolID != t.id:
		// Different tool selected: reset to top.
		c.detail.SetContent(body)
		c.detail.GotoTop()
		c.detailKey = key
		c.detailToolID = t.id

	case c.detailKey != key:
		// Same tool, content grew (streaming) or size changed: preserve scroll position.
		yOffset := c.detail.YOffset()
		c.detail.SetContent(body)
		c.detail.SetYOffset(yOffset)
		c.detailKey = key

	case sizeChanged:
		// Dimensions changed but content is identical: re-layout without losing position.
		yOffset := c.detail.YOffset()
		c.detail.SetContent(body)
		c.detail.SetYOffset(yOffset)
	}

	content := header + "\n\n" + c.detail.View()
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width).
		Height(height).
		Render(content)
}

// ── Directory listing ─────────────────────────────────────────────────────────

type dirEntry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	IsFile      bool   `json:"is_file"`
	IsSymlink   bool   `json:"is_symlink"`
	SizeBytes   int64  `json:"size_bytes"`
	Mode        string `json:"mode"`
}

type dirListing struct {
	Path      string     `json:"path"`
	Entries   []dirEntry `json:"entries"`
	Count     int        `json:"count"`
	Truncated bool       `json:"truncated"`
}

func parseDirListing(content string) (dirListing, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "{") {
		return dirListing{}, false
	}
	var dl dirListing
	if err := json.Unmarshal([]byte(content), &dl); err != nil {
		return dirListing{}, false
	}
	return dl, true
}

func humanSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func shortHomePath(path string) string {
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return prettyPath(path)
}

// renderDirListing renders a list_directory JSON result as a human-readable tree.
// maxLines=0 means no truncation (detail sidebar).
func renderDirListing(styles common.Styles, content string, width, maxLines int) string {
	dl, ok := parseDirListing(content)
	if !ok {
		return renderPlainBody(content, width)
	}

	// Header: path + count
	headerPath := styles.Key.Render(shortHomePath(dl.Path))
	count := fmt.Sprintf("(%d entries)", dl.Count)
	if dl.Truncated {
		count += " [truncated]"
	}
	headerCount := styles.MsgTimestamp.Render(count)
	lines := []string{headerPath + "  " + headerCount, ""}

	// Compute max visible name width for alignment.
	maxNameW := 0
	for _, e := range dl.Entries {
		n := e.Name
		if e.IsDirectory {
			n += "/"
		}
		if w := len("/ ") + len(n); w > maxNameW {
			maxNameW = w
		}
	}
	sizeColW := 8
	nameColW := max(16, min(maxNameW, width-sizeColW-2))

	for _, e := range dl.Entries {
		displayName := e.Name
		if e.IsDirectory {
			displayName += "/"
		}

		var nameRendered string
		switch {
		case e.IsSymlink:
			nameRendered = lipgloss.NewStyle().Foreground(common.ColorYellow).Render("→ " + displayName)
		case e.IsDirectory:
			nameRendered = lipgloss.NewStyle().Foreground(common.ColorBlue).Bold(true).Render("/ " + displayName)
		default:
			nameRendered = styles.PermBody.Render("· " + displayName)
		}

		// Truncate very long names to stay within nameColW.
		visibleNameW := lipgloss.Width(nameRendered)
		if visibleNameW > nameColW {
			// Re-render with truncated display name.
			truncated := ansi.Truncate(displayName, nameColW-3, "…")
			switch {
			case e.IsSymlink:
				nameRendered = lipgloss.NewStyle().Foreground(common.ColorYellow).Render("→ " + truncated)
			case e.IsDirectory:
				nameRendered = lipgloss.NewStyle().Foreground(common.ColorBlue).Bold(true).Render("/ " + truncated)
			default:
				nameRendered = styles.PermBody.Render("· " + truncated)
			}
			visibleNameW = lipgloss.Width(nameRendered)
		}

		var sizeStr string
		if !e.IsDirectory {
			sizeStr = humanSize(e.SizeBytes)
		}
		sizeRendered := styles.MsgTimestamp.Render(fmt.Sprintf("%*s", sizeColW, sizeStr))

		gap := max(1, nameColW-visibleNameW+1)
		lines = append(lines, nameRendered+strings.Repeat(" ", gap)+sizeRendered)
	}

	if maxLines > 0 && len(lines) > maxLines {
		hidden := len(lines) - maxLines
		lines = lines[:maxLines]
		lines = append(lines, styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
	}

	return strings.Join(lines, "\n")
}

// renderCodeBody renders source code with line numbers and syntax highlighting.
// maxLines=0 means no truncation (used by the detail sidebar).
func renderCodeBody(styles common.Styles, path, body string, width, maxLines, offset int) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	allLines := strings.Split(body, "\n")

	display := allLines
	hidden := 0
	if maxLines > 0 && len(allLines) > maxLines {
		hidden = len(allLines) - maxLines
		display = allLines[:maxLines]
	}

	highlighted := common.SyntaxHighlight(strings.Join(display, "\n"), path)
	hlLines := strings.Split(highlighted, "\n")
	// Chroma may trim a trailing newline, causing one fewer output line when the
	// last display line is blank. Pad or truncate to always match len(display).
	for len(hlLines) < len(display) {
		hlLines = append(hlLines, "")
	}
	if len(hlLines) > len(display) {
		hlLines = hlLines[:len(display)]
	}

	maxNum := len(display) + offset
	numWidth := len(fmt.Sprintf("%d", maxNum))
	if numWidth < 1 {
		numWidth = 1
	}
	numFmt := fmt.Sprintf("%%%dd ", numWidth)

	numColW := numWidth + 1 // digits + trailing space
	contentW := max(1, width-numColW)
	indent := strings.Repeat(" ", numColW)

	out := make([]string, 0, len(hlLines)+1)
	for i, ln := range hlLines {
		lineNum := styles.ToolLineNumber.Render(fmt.Sprintf(numFmt, i+1+offset))
		if maxLines > 0 {
			// Inline preview: truncate to keep fixed height.
			out = append(out, lineNum+ansi.Truncate(ln, contentW, "…"))
		} else {
			// Detail sidebar: soft-wrap with continuation indent.
			parts := strings.Split(wrap.String(ln, contentW), "\n")
			out = append(out, lineNum+parts[0])
			for _, cont := range parts[1:] {
				out = append(out, indent+cont)
			}
		}
	}
	if hidden > 0 {
		out = append(out, styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
	}
	return strings.Join(out, "\n")
}

// renderDiffBody renders a unified diff with +/- line coloring.
// maxLines=0 means no truncation (used by the detail sidebar).
func renderDiffBody(styles common.Styles, body string, width, maxLines int) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	allLines := strings.Split(body, "\n")

	display := allLines
	hidden := 0
	if maxLines > 0 && len(allLines) > maxLines {
		hidden = len(allLines) - maxLines
		display = allLines[:maxLines]
	}

	out := make([]string, 0, len(display)+1)
	for _, ln := range display {
		switch {
		case strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "+++"):
			out = append(out, styles.ToolDiffAdd.Render(ln))
		case strings.HasPrefix(ln, "-") && !strings.HasPrefix(ln, "---"):
			out = append(out, styles.ToolDiffDel.Render(ln))
		case strings.HasPrefix(ln, "@@"):
			out = append(out, styles.ToolDiffHunk.Render(ln))
		default:
			out = append(out, common.Escape(ln))
		}
	}
	if hidden > 0 {
		out = append(out, styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
	}
	return strings.Join(out, "\n")
}

// renderColoredDiff renders a unified diff with full-row colored backgrounds
// (green for added, red for deleted). maxLines=0 means no truncation.
// Reuses renderPermDiffLines so both places stay visually identical.
func renderColoredDiff(styles common.Styles, diffBody string, width, maxLines int) string {
	lines := renderPermDiffLines(styles, diffBody, width)
	if maxLines > 0 && len(lines) > maxLines {
		hidden := len(lines) - maxLines
		lines = append(lines[:maxLines], styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
	}
	return strings.Join(lines, "\n")
}

// panelBox wraps rendered content in the standard rounded border box used for inline previews.
func panelBox(styles common.Styles, content string, width int) string {
	if strings.TrimSpace(content) == "" {
		content = styles.MsgTimestamp.Render("No output")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width).
		Render(content)
}

// renderCodePanel renders a file's content as a code block for inline preview (no border).
func renderCodePanel(styles common.Styles, path, body string, width, maxLines, offset int) string {
	return renderCodeBody(styles, path, body, width, maxLines, offset)
}

// renderDiffPanel renders a unified diff for inline preview (no border).
func renderDiffPanel(styles common.Styles, body string, width, maxLines int) string {
	return renderDiffBody(styles, body, width, maxLines)
}

// renderBashInline renders a bash inline preview: a dim `$ cmd` prompt line
// followed by the command output, all truncated to maxLines (no border).
func renderBashInline(styles common.Styles, cmd, output string, width, maxLines int) string {
	var lines []string
	if cmd = strings.TrimSpace(cmd); cmd != "" {
		cmdOneLine := strings.ReplaceAll(cmd, "\n", "; ")
		lines = append(lines, styles.MsgTimestamp.Render("$ "+ansi.Truncate(cmdOneLine, max(1, width-2), "…")))
	}
	if output = strings.TrimSpace(output); output != "" {
		for _, ln := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
			lines = append(lines, wrap.String(common.Escape(ln), width))
		}
	}
	hidden := 0
	if maxLines > 0 && len(lines) > maxLines {
		hidden = len(lines) - maxLines
		lines = lines[:maxLines]
	}
	if hidden > 0 {
		lines = append(lines, styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
	}
	return strings.Join(lines, "\n")
}

// renderFilePanel picks the right panel renderer based on the file extension.
func renderFilePanel(styles common.Styles, path, body string, width, maxLines int) string {
	switch flavorForPath(path) {
	case contentFlavorCode:
		return renderCodePanel(styles, path, body, width, maxLines, 0)
	default:
		return renderContentPanel(styles, "", body, width, maxLines, flavorForPath(path))
	}
}

func renderContentPanel(styles common.Styles, title, body string, width, maxLines int, flavor contentFlavor) string {
	panelBody := renderContentBody(styles, body, width, flavor)
	if maxLines > 0 {
		lines := strings.Split(panelBody, "\n")
		if len(lines) > maxLines {
			hidden := len(lines) - maxLines
			lines = append(lines[:maxLines], styles.ToolTruncation.Render(fmt.Sprintf(previewTruncFmt, hidden)))
		}
		panelBody = strings.Join(lines, "\n")
	}
	if title != "" {
		panelBody = styles.Key.Render(title) + "\n" + panelBody
	}
	return panelBody
}

// parseReadContent strips the "File:/Lines:" header and N→ line-number prefixes
// produced by the read_file tool's FormatTextWithLineNumbers, returning the
// 0-based start line and the clean file content ready for rendering.
func parseReadContent(body string) (startLine int, clean string) {
	body = strings.ReplaceAll(body, "\r\n", "\n")

	// Locate the blank line that separates the header block from the content.
	sep := strings.Index(body, "\n\n")
	if sep < 0 {
		return 0, body
	}

	// Parse "Lines: N-M of T" from the header to recover the start offset.
	for _, line := range strings.SplitN(body[:sep], "\n", -1) {
		if strings.HasPrefix(line, "Lines: ") {
			var start, end int
			if _, err := fmt.Sscanf(line, "Lines: %d-%d", &start, &end); err == nil {
				startLine = start
			}
			break
		}
	}

	// Strip the N→ prefix (possibly padded with spaces) from every content line.
	// The separator is the Unicode right arrow U+2192 used by AddLineNumbers.
	rawLines := strings.Split(body[sep+2:], "\n")
	out := make([]string, 0, len(rawLines))
	const arrow = "→"
	for _, line := range rawLines {
		if idx := strings.Index(line, arrow); idx >= 0 {
			prefix := line[:idx]
			if strings.TrimLeft(prefix, " \t0123456789") == "" {
				line = line[idx+len(arrow):]
			}
		}
		out = append(out, line)
	}
	clean = strings.TrimRight(strings.Join(out, "\n"), "\n")
	return
}

func renderContentBody(styles common.Styles, body string, width int, flavor contentFlavor) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	switch flavor {
	case contentFlavorMarkdown:
		if rendered := renderMarkdownBody(body, width); rendered != "" {
			return rendered
		}
	}
	return renderPlainBody(body, width)
}

func renderMarkdownBody(body string, width int) string {
	renderer := common.MarkdownRenderer(width)
	if renderer == nil {
		return ""
	}
	mu := common.LockMarkdownRenderer(renderer)
	mu.Lock()
	defer mu.Unlock()
	rendered, err := renderer.Render(strings.ReplaceAll(body, "\r\n", "\n"))
	if err != nil {
		return ""
	}
	return strings.TrimRight(rendered, "\n")
}

func renderPlainBody(body string, width int) string {
	rawLines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	wrapped := make([]string, 0, len(rawLines))
	innerW := max(16, width)
	for _, line := range rawLines {
		wrapped = append(wrapped, wrap.String(common.Escape(line), innerW))
	}
	return strings.Join(wrapped, "\n")
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case nil:
		return nil
	default:
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
}

func nestedString(v any, keys ...string) string {
	m := normalizeMap(v)
	for _, key := range keys {
		if s, ok := stringAny(m[key]); ok {
			return s
		}
	}
	return ""
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := stringAny(m[key])
	return s
}

func intFromMap(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func stringAny(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case fmt.Stringer:
		return x.String(), true
	case nil:
		return "", false
	default:
		return fmt.Sprintf("%v", x), true
	}
}

func prettyPath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	return strings.ReplaceAll(clean, "\\", "/")
}

func compactPath(path string) string {
	pretty := prettyPath(path)
	if pretty == "" {
		return ""
	}
	base := filepath.Base(pretty)
	if base == "." || base == "/" {
		return pretty
	}
	return base
}

func prettyJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func indentBlock(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func toolDisplayName(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	parts := strings.Fields(name)
	for i, part := range parts {
		if part == "api" || part == "mcp" {
			parts[i] = strings.ToUpper(part)
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
