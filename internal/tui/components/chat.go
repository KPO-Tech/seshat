package components

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/muesli/reflow/wrap"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var toolIcons = map[string]string{
	"bash":               "❯",
	"write_file":         "✏",
	"edit_file":          "✎",
	"apply_patch":        "⟠",
	"read_file":          "◻",
	"list_directory":     "◫",
	"glob":               "◈",
	"grep":               "◈",
	"web_fetch":          "◉",
	"web_search":         "◉",
	"job_output":         "◆",
	"job_kill":           "⊗",
	"write_stdin":        "❯",
	"create_directory":   "◫",
	"spawn_agent":        "◎",
	"wait_agent":         "◎",
	"send_agent_message": "◎",
	"close_agent":        "◎",
}

func toolIconFor(name string) string {
	if icon, ok := toolIcons[name]; ok {
		return icon
	}
	return "◆"
}

type msgItem interface {
	render(c *Chat, width int) string
	isFinished() bool
	invalidate()
}

const thinkTailLines = 4

type thinkingBlock struct {
	content    string
	streaming  bool
	startedAt  time.Time
	finishedAt time.Time
	collapsed  bool

	cacheWidth  int
	cacheRender string
}

func newThinkingBlock() *thinkingBlock {
	return &thinkingBlock{
		streaming: true,
		collapsed: true,
		startedAt: time.Now(),
	}
}

func (tb *thinkingBlock) append(text string) {
	tb.content += text
	tb.cacheWidth = 0
}

func (tb *thinkingBlock) finish() {
	tb.streaming = false
	tb.finishedAt = time.Now()
	tb.cacheWidth = 0
}

func (tb *thinkingBlock) toggle() {
	tb.collapsed = !tb.collapsed
	tb.cacheWidth = 0
}

func (tb *thinkingBlock) render(styles common.Styles, width int) string {
	if !tb.streaming && tb.cacheWidth == width && tb.cacheRender != "" {
		return tb.cacheRender
	}

	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}

	lines := strings.Split(strings.TrimRight(tb.content, "\n"), "\n")
	var shownLines []string
	var hiddenCount int

	if tb.collapsed && len(lines) > thinkTailLines {
		hiddenCount = len(lines) - thinkTailLines
		shownLines = lines[len(lines)-thinkTailLines:]
	} else {
		shownLines = lines
	}

	var inner strings.Builder
	if hiddenCount > 0 {
		inner.WriteString(styles.MsgTimestamp.Render(fmt.Sprintf("… %d lines hidden", hiddenCount)))
		inner.WriteString("\n")
	}
	for i, line := range shownLines {
		inner.WriteString(styles.MsgTimestamp.Render(wrap.String(line, innerW)))
		if i < len(shownLines)-1 {
			inner.WriteString("\n")
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width - 2)

	box := boxStyle.Render(inner.String())

	var footParts []string
	if tb.streaming {
		footParts = append(footParts, styles.MsgTimestamp.Render("thinking…"))
	} else {
		dur := tb.finishedAt.Sub(tb.startedAt).Round(100 * time.Millisecond)
		footParts = append(footParts,
			styles.MsgTimestamp.Render(fmt.Sprintf("Thought for %.1fs", dur.Seconds())))
		if tb.collapsed {
			footParts = append(footParts, styles.Desc.Render("click to expand"))
		} else {
			footParts = append(footParts, styles.Desc.Render("click to collapse"))
		}
	}
	foot := "  " + strings.Join(footParts, "  ")

	result := box + "\n" + foot
	if !tb.streaming {
		tb.cacheWidth = width
		tb.cacheRender = result
	}
	return result
}

type assistantItem struct {
	thinking     *thinkingBlock
	content      string
	streaming    bool
	startedAt    time.Time
	finishedAt   time.Time
	showLabel    bool
	showMeta     bool
	inputTokens  int
	outputTokens int
	stopReason   string

	contentCacheWidth  int
	contentCacheRender string
}

func newAssistantItem() *assistantItem {
	return &assistantItem{streaming: true, showLabel: true, startedAt: time.Now()}
}

func newContinuationItem(startedAt time.Time) *assistantItem {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &assistantItem{streaming: true, showLabel: false, startedAt: startedAt}
}

func (a *assistantItem) appendThinking(text string) {
	if text == "" {
		return
	}
	if a.thinking == nil {
		a.thinking = newThinkingBlock()
	}
	a.thinking.append(text)
	a.contentCacheWidth = 0
}

func (a *assistantItem) appendContent(text string) {
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
	}
	a.content += text
	a.contentCacheWidth = 0
}

func (a *assistantItem) finish(inputTokens, outputTokens int, stopReason string, showMeta bool) {
	a.streaming = false
	a.finishedAt = time.Now()
	a.showMeta = showMeta
	a.inputTokens = inputTokens
	a.outputTokens = outputTokens
	a.stopReason = stopReason
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
	}
	a.contentCacheWidth = 0
}

func (a *assistantItem) isFinished() bool { return !a.streaming }
func (a *assistantItem) invalidate()      { a.contentCacheWidth = 0 }

func (a *assistantItem) render(c *Chat, width int) string {
	var sb strings.Builder
	if a.showLabel {
		sb.WriteString(c.styles.AssistantMarker.Render("●"))
		sb.WriteString("\n")
	}
	if a.thinking != nil && strings.TrimSpace(a.thinking.content) != "" {
		sb.WriteString(a.thinking.render(c.styles, width))
		sb.WriteString("\n")
	}
	if a.content != "" {
		var rendered string
		if !a.streaming && a.contentCacheWidth == width && a.contentCacheRender != "" {
			rendered = a.contentCacheRender
		} else {
			var err error
			mu := common.LockMarkdownRenderer(c.renderer)
			mu.Lock()
			rendered, err = c.renderer.Render(a.content)
			mu.Unlock()
			if err != nil {
				rendered = a.content
			}
			rendered = strings.TrimRight(rendered, "\n")
			if !a.streaming {
				a.contentCacheWidth = width
				a.contentCacheRender = rendered
			}
		}
		sb.WriteString(rendered)
	} else if a.streaming {
		sb.WriteString(c.styles.MsgTimestamp.Render("…"))
	}
	if meta := a.metaLine(c.styles, width); meta != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(meta)
	}
	return sb.String()
}

func (a *assistantItem) metaLine(styles common.Styles, width int) string {
	if a.streaming || a.finishedAt.IsZero() || !a.showMeta {
		return ""
	}
	left := styles.ToolDone.Render("done")
	if !a.startedAt.IsZero() {
		left += styles.TurnMeta.Render(" · " + formatDuration(a.finishedAt.Sub(a.startedAt)))
	}
	turnTokens := a.inputTokens + a.outputTokens
	if turnTokens <= 0 {
		return left
	}
	right := styles.TurnMeta.Render(compactTokenCount(turnTokens) + " tok")
	sepLen := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if sepLen < 3 {
		sepLen = 3
	}
	sep := styles.TurnMeta.Render(strings.Repeat("·", sepLen))
	return left + " " + sep + " " + right
}

type userItem struct {
	content   string
	timestamp time.Time
	cacheW    int
	cacheR    string
}

func (u *userItem) isFinished() bool { return true }
func (u *userItem) invalidate()      { u.cacheW = 0 }

func (u *userItem) render(c *Chat, width int) string {
	if u.cacheW == width && u.cacheR != "" {
		return u.cacheR
	}
	_ = u.timestamp
	prefix := "● > "
	bodyWidth := max(12, width-lipgloss.Width(prefix))
	wrapped := strings.Split(wrap.String(u.content, bodyWidth), "\n")
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	wrapped[0] = c.styles.UserMarker.Render(prefix) + c.styles.UserMsg.Render(wrapped[0])
	for i := 1; i < len(wrapped); i++ {
		wrapped[i] = strings.Repeat(" ", lipgloss.Width(prefix)) + c.styles.UserMsg.Render(wrapped[i])
	}
	r := strings.Join(wrapped, "\n")
	u.cacheW = width
	u.cacheR = r
	return r
}

type toolItem struct {
	id         string
	name       string
	status     string
	label      string
	metadata   map[string]any
	expanded   bool
	startedAt  time.Time
	finishedAt time.Time

	cacheW int
	cacheR string
}

func newToolItem(id, name, status, label string, metadata map[string]any) *toolItem {
	return &toolItem{
		id:        id,
		name:      name,
		status:    status,
		label:     label,
		metadata:  cloneMap(metadata),
		startedAt: time.Now(),
	}
}

func (t *toolItem) isDone() bool {
	return t.status == "completed" || t.status == "failed" || t.status == "done" || t.status == "error"
}

func (t *toolItem) isFinished() bool { return t.isDone() }
func (t *toolItem) invalidate()      { t.cacheW = 0; t.cacheR = "" }

func (t *toolItem) render(c *Chat, width int) string {
	return t.renderSelected(c, width, false)
}

func (t *toolItem) expanderSymbol() string {
	if !t.supportsPreview() {
		return " "
	}
	if t.expanded {
		return "▾"
	}
	return "▸"
}

func (t *toolItem) detailsSymbol(selected, detailsOpen bool) string {
	if selected && detailsOpen {
		return "⊟"
	}
	return "⊞"
}

func (t *toolItem) renderSelected(c *Chat, width int, selected bool) string {
	if t.isDone() && !selected && !t.expanded && t.cacheW == width && t.cacheR != "" {
		return t.cacheR
	}

	icon := t.renderIcon(c.styles)
	nameStyle := t.renderNameStyle(c.styles)
	summary := truncate(t.summaryText(), max(12, width-34))
	expander := c.styles.MsgTimestamp.Render(t.expanderSymbol())
	details := c.styles.MsgTimestamp.Render(t.detailsSymbol(selected, c.detailOpen && selected))
	status := c.styles.MsgTimestamp.Render(t.statusLabel())

	parts := []string{expander, details, icon, nameStyle.Render(toolDisplayName(t.name)), status}
	if summary != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render(summary))
	}
	if dur := t.durationText(); dur != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render("("+dur+")"))
	}

	line := strings.Join(parts, " ")
	if selected {
		line = lipgloss.NewStyle().Foreground(common.ColorText).Background(lipgloss.Color("#1F2937")).Render(line)
	}

	if !t.expanded {
		if t.isDone() && !selected {
			t.cacheW = width
			t.cacheR = line
		}
		return line
	}

	preview := t.inlinePreview(c, width)
	if preview == "" {
		preview = c.styles.MsgTimestamp.Render("No preview available.")
	}
	result := line + "\n" + indentBlock(preview, "    ")
	if t.isDone() && !selected {
		t.cacheW = width
		t.cacheR = result
	}
	return result
}

func (t *toolItem) renderIcon(styles common.Styles) string {
	switch {
	case t.status == "completed" || t.status == "done":
		return styles.MsgTimestamp.Render("✓")
	case t.status == "failed" || t.status == "error":
		return styles.ToolError.Render("✗")
	default:
		return styles.ToolProgress.Render(toolIconFor(t.name))
	}
}

func (t *toolItem) renderNameStyle(styles common.Styles) lipgloss.Style {
	switch {
	case t.status == "completed" || t.status == "done":
		return styles.UserMsg
	case t.status == "failed" || t.status == "error":
		return styles.ToolError
	default:
		return styles.ToolProgress
	}
}

func (t *toolItem) durationText() string {
	if !t.isDone() || t.finishedAt.IsZero() {
		if ms, ok := intFromMap(t.metadata, "execution_duration_ms"); ok && ms > 0 {
			return formatDuration(time.Duration(ms) * time.Millisecond)
		}
		return ""
	}
	return formatDuration(t.finishedAt.Sub(t.startedAt))
}

func (t *toolItem) toolInput() map[string]any {
	return normalizeMap(t.metadata["tool_input"])
}

func (t *toolItem) supportsPreview() bool {
	switch t.name {
	case "read_file", "write_file", "edit_file", "apply_patch", "bash", "spawn_agent", "wait_agent", "close_agent", "send_agent_message":
		return true
	default:
		return strings.TrimSpace(t.resultContent()) != ""
	}
}

func (t *toolItem) statusLabel() string {
	switch t.status {
	case "completed", "done":
		return "done"
	case "failed", "error":
		return "failed"
	case "running", "started":
		return "running"
	default:
		return t.status
	}
}

func (t *toolItem) summaryText() string {
	input := t.toolInput()
	switch t.name {
	case "read_file":
		path := compactPath(stringFromMap(input, "file_path"))
		if path == "" {
			path = t.label
		}
		return path
	case "write_file", "edit_file":
		path := compactPath(stringFromMap(input, "file_path"))
		kind := stringFromMap(t.metadata, "type")
		if kind != "" {
			return path + " · " + kind
		}
		return path
	case "apply_patch":
		if content := stringFromMap(t.metadata, "content"); content != "" {
			return firstLine(content)
		}
		if patch := stringFromMap(input, "patch"); patch != "" {
			return firstLine(strings.TrimSpace(patch))
		}
	case "bash":
		cmd := strings.TrimSpace(stringFromMap(input, "command"))
		if cmd == "" {
			cmd = strings.TrimSpace(stringFromMap(t.metadata, "description"))
		}
		return cmd
	case "spawn_agent":
		prompt := strings.TrimSpace(stringFromMap(input, "prompt"))
		nickname := strings.TrimSpace(stringFromMap(input, "nickname"))
		if nickname != "" {
			return nickname + " · " + prompt
		}
		return prompt
	case "wait_agent", "close_agent", "send_agent_message":
		agentID := strings.TrimSpace(stringFromMap(input, "agent_id"))
		if agentID != "" {
			return agentID
		}
	}
	if t.label != "" && t.label != t.status {
		return strings.TrimSpace(t.label)
	}
	if msg := strings.TrimSpace(stringFromMap(t.metadata, "content")); msg != "" {
		return firstLine(msg)
	}
	return ""
}

func (t *toolItem) inlinePreview(c *Chat, width int) string {
	bodyWidth := max(20, width-8)
	switch t.name {
	case "read_file":
		input := t.toolInput()
		path := prettyPath(stringFromMap(input, "file_path"))
		content := t.resultContent()
		return renderContentPanel(c.styles, path, content, bodyWidth, 8)
	case "write_file", "edit_file":
		path := prettyPath(stringFromMap(t.toolInput(), "file_path"))
		if diff := t.diffPreview(); diff != "" {
			return renderContentPanel(c.styles, path, diff, bodyWidth, 10)
		}
		return renderContentPanel(c.styles, path, t.resultContent(), bodyWidth, 8)
	case "apply_patch":
		if diff := stringFromMap(t.toolInput(), "patch"); diff != "" {
			return renderContentPanel(c.styles, "patch", diff, bodyWidth, 10)
		}
		return renderContentPanel(c.styles, "patch", t.resultContent(), bodyWidth, 8)
	case "bash":
		return renderContentPanel(c.styles, "output", t.commandOutput(), bodyWidth, 8)
	case "spawn_agent", "wait_agent", "close_agent", "send_agent_message":
		return renderContentPanel(c.styles, "agent", t.agentDetails(), bodyWidth, 8)
	default:
		return renderContentPanel(c.styles, toolDisplayName(t.name), t.resultContent(), bodyWidth, 8)
	}
}

func (t *toolItem) detailView(c *Chat, width, height int) string {
	innerW := max(24, width-4)
	sections := []string{
		c.styles.AssistantLabel.Render(toolDisplayName(t.name)),
		c.styles.MsgTimestamp.Render(strings.ToUpper(t.status)),
	}
	if summary := t.summaryText(); summary != "" {
		sections = append(sections, wrap.String(summary, innerW))
	}
	if meta := t.metaSummary(); meta != "" {
		sections = append(sections, meta)
	}
	if body := t.detailBody(c, innerW); body != "" {
		sections = append(sections, body)
	}
	content := strings.Join(sections, "\n\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width).
		MaxHeight(height).
		Render(content)
	return box
}

func (t *toolItem) metaSummary() string {
	var lines []string
	if dur := t.durationText(); dur != "" {
		lines = append(lines, "duration: "+dur)
	}
	if code, ok := intFromMap(t.metadata, "exit_code"); ok {
		lines = append(lines, fmt.Sprintf("exit code: %d", code))
	}
	if cwd := stringFromMap(t.metadata, "cwd"); cwd != "" {
		lines = append(lines, "cwd: "+cwd)
	}
	if taskID := stringFromMap(t.metadata, "task_id"); taskID != "" {
		lines = append(lines, "task: "+taskID)
	}
	if count, ok := intFromMap(t.metadata, "lines_added"); ok {
		removed, _ := intFromMap(t.metadata, "lines_removed")
		lines = append(lines, fmt.Sprintf("changes: +%d -%d", count, removed))
	}
	return strings.Join(lines, "\n")
}

func (t *toolItem) detailBody(c *Chat, width int) string {
	switch t.name {
	case "read_file":
		return renderContentPanel(c.styles, prettyPath(stringFromMap(t.toolInput(), "file_path")), t.resultContent(), width, 200)
	case "write_file", "edit_file":
		if diff := t.diffPreview(); diff != "" {
			return renderContentPanel(c.styles, prettyPath(stringFromMap(t.toolInput(), "file_path")), diff, width, 200)
		}
		return renderContentPanel(c.styles, prettyPath(stringFromMap(t.toolInput(), "file_path")), t.resultContent(), width, 200)
	case "apply_patch":
		body := t.resultContent()
		if patch := stringFromMap(t.toolInput(), "patch"); patch != "" {
			body = body + "\n\n" + patch
		}
		return renderContentPanel(c.styles, "patch", body, width, 220)
	case "bash":
		cmd := stringFromMap(t.toolInput(), "command")
		body := t.commandOutput()
		if cmd != "" {
			body = "$ " + cmd + "\n\n" + body
		}
		return renderContentPanel(c.styles, "bash", body, width, 180)
	case "spawn_agent", "wait_agent", "close_agent", "send_agent_message":
		return renderContentPanel(c.styles, "agent", t.agentDetails(), width, 160)
	default:
		return renderContentPanel(c.styles, toolDisplayName(t.name), t.resultContent(), width, 140)
	}
}

func (t *toolItem) resultContent() string {
	if content := stringFromMap(t.metadata, "content"); content != "" {
		return content
	}
	return ""
}

func (t *toolItem) diffPreview() string {
	if patch := nestedString(t.metadata["git_diff"], "patch", "Patch"); patch != "" {
		return patch
	}
	if structured := prettyJSON(t.metadata["structured_patch"]); structured != "" {
		return structured
	}
	if original := stringFromMap(t.metadata, "original_file"); original != "" {
		if content := stringFromMap(t.metadata, "content"); content != "" {
			return "--- before\n" + original + "\n\n+++ after\n" + content
		}
	}
	return ""
}

func (t *toolItem) commandOutput() string {
	stdout := stringFromMap(t.metadata, "stdout")
	stderr := stringFromMap(t.metadata, "stderr")
	if stdout == "" && stderr == "" {
		if content := t.resultContent(); content != "" {
			return content
		}
		return t.label
	}
	if stdout != "" && stderr != "" {
		return stdout + "\n\n[stderr]\n" + stderr
	}
	if stdout != "" {
		return stdout
	}
	return stderr
}

func (t *toolItem) agentDetails() string {
	input := t.toolInput()
	var parts []string
	if nickname := stringFromMap(input, "nickname"); nickname != "" {
		parts = append(parts, "nickname: "+nickname)
	}
	if role := stringFromMap(input, "role"); role != "" {
		parts = append(parts, "role: "+role)
	}
	if agentID := stringFromMap(input, "agent_id"); agentID != "" {
		parts = append(parts, "agent: "+agentID)
	}
	if prompt := stringFromMap(input, "prompt"); prompt != "" {
		parts = append(parts, "prompt:\n"+prompt)
	}
	if msg := stringFromMap(input, "message"); msg != "" {
		parts = append(parts, "message:\n"+msg)
	}
	if content := t.resultContent(); content != "" {
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

type systemItem struct{ content string }

func (s *systemItem) isFinished() bool { return true }
func (s *systemItem) invalidate()      {}
func (s *systemItem) render(c *Chat, _ int) string {
	return c.styles.MsgTimestamp.Render("─ " + s.content)
}

type errorItem struct{ content string }

func (e *errorItem) isFinished() bool { return true }
func (e *errorItem) invalidate()      {}
func (e *errorItem) render(c *Chat, _ int) string {
	return c.styles.ToolError.Render("✗ " + e.content)
}

type toolRegion struct {
	startLine     int
	endLine       int
	msgIndex      int
	expanderStart int
	expanderEnd   int
	detailStart   int
	detailEnd     int
}

type thinkingRegion struct {
	startLine int
	endLine   int
	msgIndex  int
}

type Chat struct {
	styles   common.Styles
	viewport *viewport.Model
	renderer *glamour.TermRenderer
	messages []msgItem
	width    int
	height   int
	follow   bool

	selectedTool int
	detailOpen   bool

	renderedContent string
	renderedLines   []string
	plainContent    string
	plainLines      []string
	toolRegions     []toolRegion
	thinkingRegions []thinkingRegion
	selection       mouseSelection
}

func NewChat(styles common.Styles, width, height int) *Chat {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	vp.SetContent("")
	r := common.MarkdownRenderer(width)
	return &Chat{
		styles:       styles,
		viewport:     &vp,
		renderer:     r,
		follow:       true,
		width:        width,
		height:       height,
		selectedTool: -1,
	}
}

func (c *Chat) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.viewport.SetWidth(width)
	c.viewport.SetHeight(height)
	if r := common.MarkdownRenderer(width); r != nil {
		c.renderer = r
	}
	for _, m := range c.messages {
		m.invalidate()
	}
	c.refresh()
}

func (c *Chat) AddUserMessage(text string) {
	c.messages = append(c.messages, &userItem{content: text, timestamp: time.Now()})
	c.refresh()
}

func (c *Chat) StartAssistantMessage() {
	c.messages = append(c.messages, newAssistantItem())
	c.refresh()
}

func (c *Chat) AppendChunk(text string, isThinking bool) {
	if text == "" {
		return
	}
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok && a.streaming {
			if isThinking {
				a.appendThinking(text)
			} else {
				a.appendContent(text)
			}
			c.refresh()
			return
		}
	}
	isContinuation := false
	continuationStart := time.Time{}
	for i := len(c.messages) - 1; i >= 0; i-- {
		if _, ok := c.messages[i].(*userItem); ok {
			break
		}
		if a, ok := c.messages[i].(*assistantItem); ok {
			isContinuation = true
			continuationStart = a.startedAt
			break
		}
	}
	if isContinuation {
		c.messages = append(c.messages, newContinuationItem(continuationStart))
	} else {
		c.messages = append(c.messages, newAssistantItem())
	}
	c.AppendChunk(text, isThinking)
}

func (c *Chat) FinishAssistantMessage(inputTokens, outputTokens int, stopReason string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok && a.streaming {
			a.finish(inputTokens, outputTokens, stopReason, true)
			c.refresh()
			return
		}
	}
}

func (c *Chat) AddToolProgress(toolUseID, toolName, status, label string, metadata map[string]any) {
	if toolUseID != "" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if t, ok := c.messages[i].(*toolItem); ok && t.id == toolUseID {
				t.status = status
				t.label = label
				if len(metadata) > 0 {
					t.metadata = cloneMap(metadata)
				}
				if t.isDone() {
					t.finishedAt = time.Now()
				}
				t.invalidate()
				c.refresh()
				return
			}
		}
	} else {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if t, ok := c.messages[i].(*toolItem); ok && t.name == toolName && !t.isDone() {
				t.status = status
				t.label = label
				if len(metadata) > 0 {
					t.metadata = cloneMap(metadata)
				}
				if t.isDone() {
					t.finishedAt = time.Now()
				}
				t.invalidate()
				c.refresh()
				return
			}
		}
	}

	c.sealActiveAssistant()
	c.messages = append(c.messages, newToolItem(toolUseID, toolName, status, label, metadata))
	c.selectedTool = len(c.messages) - 1
	c.refresh()
}

func (c *Chat) sealActiveAssistant() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		a, ok := c.messages[i].(*assistantItem)
		if !ok || !a.streaming {
			continue
		}
		hasThinking := a.thinking != nil && strings.TrimSpace(a.thinking.content) != ""
		if a.content == "" && !hasThinking {
			c.messages = append(c.messages[:i], c.messages[i+1:]...)
		} else {
			a.finish(0, 0, "", false)
		}
		return
	}
}

func (c *Chat) AddError(err error) {
	c.messages = append(c.messages, &errorItem{content: err.Error()})
	c.refresh()
}

func (c *Chat) AddSystem(text string) {
	c.messages = append(c.messages, &systemItem{content: text})
	c.refresh()
}

func (c *Chat) Clear() {
	c.messages = c.messages[:0]
	c.selectedTool = -1
	c.detailOpen = false
	c.refresh()
}

func (c *Chat) GetLastAssistantText() string {
	var parts []string
	inCurrentTurn := false
	for i := len(c.messages) - 1; i >= 0; i-- {
		switch m := c.messages[i].(type) {
		case *userItem:
			if inCurrentTurn {
				goto done
			}
		case *assistantItem:
			inCurrentTurn = true
			if m.content != "" {
				parts = append([]string{m.content}, parts...)
			}
		}
	}
done:
	return strings.Join(parts, "\n\n")
}

func (c *Chat) GetLastUserText() string {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if u, ok := c.messages[i].(*userItem); ok {
			return u.content
		}
	}
	return ""
}

func (c *Chat) ToggleThinking() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok {
			if a.thinking != nil {
				a.thinking.toggle()
				c.refresh()
			}
			return
		}
	}
}

func (c *Chat) HasThinking() bool {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok {
			return a.thinking != nil
		}
	}
	return false
}

func (c *Chat) HasTools() bool {
	for _, item := range c.messages {
		if _, ok := item.(*toolItem); ok {
			return true
		}
	}
	return false
}

func (c *Chat) HasSelectedTool() bool {
	return c.selectedToolIndex() >= 0
}

func (c *Chat) DetailsOpen() bool {
	return c.detailOpen && c.HasSelectedTool()
}

func (c *Chat) ToggleSelectedToolExpanded() bool {
	if tool := c.selectedToolItem(); tool != nil {
		tool.expanded = !tool.expanded
		tool.invalidate()
		c.refresh()
		return true
	}
	return false
}

func (c *Chat) ToggleDetails() bool {
	if !c.HasSelectedTool() {
		return false
	}
	c.detailOpen = !c.detailOpen
	c.refresh()
	return true
}

func (c *Chat) CloseDetails() {
	if c.detailOpen {
		c.detailOpen = false
		c.refresh()
	}
}

func (c *Chat) SelectNextTool() bool {
	for i, start := range c.toolIndices() {
		if start > c.selectedToolIndex() {
			c.selectedTool = start
			c.refresh()
			_ = i
			return true
		}
	}
	indices := c.toolIndices()
	if len(indices) > 0 && c.selectedToolIndex() < 0 {
		c.selectedTool = indices[0]
		c.refresh()
		return true
	}
	return false
}

func (c *Chat) SelectPrevTool() bool {
	indices := c.toolIndices()
	for i := len(indices) - 1; i >= 0; i-- {
		if indices[i] < c.selectedToolIndex() {
			c.selectedTool = indices[i]
			c.refresh()
			return true
		}
	}
	if len(indices) > 0 && c.selectedToolIndex() < 0 {
		c.selectedTool = indices[len(indices)-1]
		c.refresh()
		return true
	}
	return false
}

func (c *Chat) HandleMouseDown(x, y int) bool {
	line := c.viewport.YOffset() + clampInt(y, 0, max(0, c.height-1))
	if line < 0 || line >= len(c.plainLines) {
		return false
	}
	clicks := c.selection.begin(line, max(0, x), time.Now())
	switch clicks {
	case 2:
		c.selectWordAt(line, max(0, x))
	case 3:
		c.selectLineAt(line)
	}
	c.refresh()
	return true
}

func (c *Chat) HandleMouseDrag(x, y int) bool {
	if !c.selection.dragging {
		return false
	}
	if len(c.plainLines) == 0 {
		return false
	}
	if y < 0 {
		c.ScrollUp(1)
		y = 0
	} else if y >= c.height {
		c.ScrollDown(1)
		y = max(0, c.height-1)
	}
	line := c.viewport.YOffset() + clampInt(y, 0, max(0, c.height-1))
	line = clampInt(line, 0, len(c.plainLines)-1)
	c.selection.update(line, max(0, x))
	c.refresh()
	return true
}

func (c *Chat) HandleMouseUp(x, y int) string {
	if !c.selection.dragging {
		return ""
	}
	_ = c.HandleMouseDrag(x, y)
	wasMoved := c.selection.finish()
	text := ""
	if wasMoved {
		text = c.selectedText()
		c.refresh()
	} else {
		line := c.selection.startLine
		c.selection.clear()
		if idx := c.thinkingIndexAtLine(line); idx >= 0 {
			c.handleThinkingLineClick(idx)
		} else if idx := c.toolIndexAtLine(line); idx >= 0 {
			c.handleToolLineClick(idx, max(0, x), line)
		} else {
			c.refresh()
		}
	}
	return text
}

func (c *Chat) HasMouseCapture() bool {
	return c.selection.dragging
}

func (c *Chat) handleToolLineClick(msgIndex, x, line int) {
	if msgIndex < 0 || msgIndex >= len(c.messages) {
		return
	}
	tool, ok := c.messages[msgIndex].(*toolItem)
	if !ok {
		return
	}
	for _, region := range c.toolRegions {
		if region.msgIndex != msgIndex {
			continue
		}
		if line == region.startLine && x >= region.expanderStart && x < region.expanderEnd && tool.supportsPreview() {
			tool.expanded = !tool.expanded
			tool.invalidate()
			c.selectedTool = msgIndex
			c.refresh()
			return
		}
		if line == region.startLine && x >= region.detailStart && x < region.detailEnd {
			c.selectedTool = msgIndex
			c.detailOpen = !(c.selectedTool == msgIndex && c.detailOpen)
			c.refresh()
			return
		}
		break
	}
	c.selectedTool = msgIndex
	c.refresh()
}

func (c *Chat) handleThinkingLineClick(msgIndex int) {
	if msgIndex < 0 || msgIndex >= len(c.messages) {
		return
	}
	assistant, ok := c.messages[msgIndex].(*assistantItem)
	if !ok || assistant.thinking == nil {
		return
	}
	assistant.thinking.toggle()
	assistant.invalidate()
	c.refresh()
}

func (c *Chat) thinkingIndexAtLine(line int) int {
	for _, region := range c.thinkingRegions {
		if line >= region.startLine && line <= region.endLine {
			return region.msgIndex
		}
	}
	return -1
}

func (c *Chat) toolIndexAtLine(line int) int {
	for _, region := range c.toolRegions {
		if line >= region.startLine && line <= region.endLine {
			return region.msgIndex
		}
	}
	return -1
}

func (c *Chat) selectWordAt(line, col int) {
	if line < 0 || line >= len(c.plainLines) {
		return
	}
	runes := []rune(c.plainLines[line])
	if len(runes) == 0 {
		c.selection.setRange(line, 0, line, 0)
		return
	}
	col = clampInt(col, 0, len(runes)-1)
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	if !isWord(runes[col]) {
		for col < len(runes) && !isWord(runes[col]) {
			col++
		}
		if col >= len(runes) {
			c.selection.setRange(line, 0, line, len(runes))
			return
		}
	}
	start := col
	for start > 0 && isWord(runes[start-1]) {
		start--
	}
	end := col
	for end < len(runes) && isWord(runes[end]) {
		end++
	}
	c.selection.setRange(line, start, line, end)
}

func (c *Chat) selectLineAt(line int) {
	if line < 0 || line >= len(c.plainLines) {
		return
	}
	runes := []rune(c.plainLines[line])
	c.selection.setRange(line, 0, line, len(runes))
}

func (c *Chat) HasSelection() bool {
	return c.selection.hasSelection()
}

func (c *Chat) SelectedText() string {
	return c.selectedText()
}

func (c *Chat) selectedText() string {
	if len(c.plainLines) == 0 {
		return ""
	}
	startLn, startCo, endLn, endCo := c.selectionRange()
	if startLn < 0 || endLn < 0 {
		return ""
	}
	var parts []string
	for line := startLn; line <= endLn; line++ {
		current := c.plainLines[line]
		runes := []rune(current)
		lineStart := 0
		lineEnd := len(runes)
		if line == startLn {
			lineStart = clampInt(startCo, 0, len(runes))
		}
		if line == endLn {
			lineEnd = clampInt(endCo, 0, len(runes))
		}
		if line == startLn && line == endLn && lineEnd < lineStart {
			lineStart, lineEnd = lineEnd, lineStart
		}
		if lineEnd < lineStart {
			lineEnd = lineStart
		}
		parts = append(parts, normalizeCopiedLine(string(runes[lineStart:lineEnd])))
	}
	joined := strings.Join(parts, "\n")
	joined = strings.Trim(joined, "\n")
	return joined
}

func normalizeCopiedLine(line string) string {
	switch {
	case strings.HasPrefix(line, "● > "):
		return strings.TrimPrefix(line, "● > ")
	case strings.HasPrefix(line, "● "):
		return strings.TrimPrefix(line, "● ")
	case line == "●":
		return ""
	case strings.HasPrefix(line, "─ "):
		return strings.TrimPrefix(line, "─ ")
	case strings.HasPrefix(line, "✗ "):
		return strings.TrimPrefix(line, "✗ ")
	default:
		return line
	}
}

func (c *Chat) selectionRange() (int, int, int, int) {
	return c.selection.rangeOrInvalid()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c *Chat) DetailView(width, height int) string {
	tool := c.selectedToolItem()
	if tool == nil {
		return ""
	}
	return tool.detailView(c, width, height)
}

func (c *Chat) ScrollUp(n int)   { c.follow = false; c.viewport.ScrollUp(n) }
func (c *Chat) ScrollDown(n int) { c.viewport.ScrollDown(n); c.follow = c.viewport.AtBottom() }
func (c *Chat) PageUp()          { c.follow = false; c.viewport.HalfPageUp() }
func (c *Chat) PageDown()        { c.viewport.HalfPageDown(); c.follow = c.viewport.AtBottom() }
func (c *Chat) GotoTop()         { c.follow = false; c.viewport.GotoTop() }
func (c *Chat) GotoBottom()      { c.follow = true; c.viewport.GotoBottom() }
func (c *Chat) View() string     { return c.viewport.View() }

func (c *Chat) selectedToolIndex() int {
	if c.selectedTool < 0 || c.selectedTool >= len(c.messages) {
		return -1
	}
	if _, ok := c.messages[c.selectedTool].(*toolItem); !ok {
		return -1
	}
	return c.selectedTool
}

func (c *Chat) selectedToolItem() *toolItem {
	if idx := c.selectedToolIndex(); idx >= 0 {
		if tool, ok := c.messages[idx].(*toolItem); ok {
			return tool
		}
	}
	return nil
}

func (c *Chat) toolIndices() []int {
	indices := make([]int, 0)
	for i, item := range c.messages {
		if _, ok := item.(*toolItem); ok {
			indices = append(indices, i)
		}
	}
	return indices
}

func (c *Chat) refresh() {
	var sb strings.Builder
	var plainSB strings.Builder
	lastWasTool := false
	wroteAny := false
	line := 0
	toolRegions := make([]toolRegion, 0)
	thinkingRegions := make([]thinkingRegion, 0)
	for i, item := range c.messages {
		var rendered string
		if tool, ok := item.(*toolItem); ok {
			rendered = tool.renderSelected(c, c.width, i == c.selectedToolIndex())
		} else {
			rendered = item.render(c, c.width)
		}
		if rendered == "" {
			continue
		}
		plainRendered := ansi.Strip(rendered)
		if wroteAny {
			_, currIsTool := item.(*toolItem)
			if lastWasTool && currIsTool {
				sb.WriteString("\n")
				plainSB.WriteString("\n")
				line += 1
			} else {
				sb.WriteString("\n\n")
				plainSB.WriteString("\n\n")
				line += 2
			}
		}
		startLine := line
		sb.WriteString(rendered)
		plainSB.WriteString(plainRendered)
		height := max(1, lipgloss.Height(plainRendered))
		if _, ok := item.(*toolItem); ok {
			toolRegions = append(toolRegions, toolRegion{startLine: startLine, endLine: startLine + height - 1, msgIndex: i, expanderStart: 0, expanderEnd: 1, detailStart: 2, detailEnd: 3})
		}
		if assistant, ok := item.(*assistantItem); ok && assistant.thinking != nil && strings.TrimSpace(assistant.thinking.content) != "" {
			thinkingStart := startLine
			if assistant.showLabel {
				thinkingStart++
			}
			thinkingRendered := assistant.thinking.render(c.styles, c.width)
			thinkingHeight := max(1, lipgloss.Height(ansi.Strip(thinkingRendered)))
			thinkingRegions = append(thinkingRegions, thinkingRegion{startLine: thinkingStart, endLine: thinkingStart + thinkingHeight - 1, msgIndex: i})
		}
		line += height
		_, lastWasTool = item.(*toolItem)
		wroteAny = true
	}
	content := sb.String()
	plain := plainSB.String()
	c.renderedContent = content
	if content == "" {
		c.renderedLines = nil
	} else {
		c.renderedLines = strings.Split(content, "\n")
	}
	c.plainContent = plain
	if plain == "" {
		c.plainLines = nil
	} else {
		c.plainLines = strings.Split(plain, "\n")
	}
	c.toolRegions = toolRegions
	c.thinkingRegions = thinkingRegions
	if c.selection.hasSelection() {
		c.viewport.SetContent(c.highlightedSelectionContent())
	} else {
		c.viewport.SetContent(content)
	}
	if c.follow {
		c.viewport.GotoBottom()
	}
}
func (c *Chat) highlightedSelectionContent() string {
	if len(c.renderedLines) == 0 {
		return c.renderedContent
	}
	startLn, startCo, endLn, endCo := c.selectionRange()
	if startLn < 0 || endLn < 0 {
		return c.renderedContent
	}
	lines := make([]string, len(c.renderedLines))
	copy(lines, c.renderedLines)
	for line := startLn; line <= endLn; line++ {
		renderedLine := lines[line]
		lineWidth := ansi.StringWidth(renderedLine)
		lineStart := 0
		lineEnd := lineWidth
		if line == startLn {
			lineStart = clampInt(startCo, 0, lineWidth)
		}
		if line == endLn {
			lineEnd = clampInt(endCo, 0, lineWidth)
		}
		if line == startLn && line == endLn && lineEnd < lineStart {
			lineStart, lineEnd = lineEnd, lineStart
		}
		if lineEnd < lineStart {
			lineEnd = lineStart
		}
		before := ansi.Cut(renderedLine, 0, lineStart)
		middle := ansi.Cut(renderedLine, lineStart, lineEnd)
		after := ansi.Cut(renderedLine, lineEnd, lineWidth)
		if middle == "" && lineStart < lineWidth {
			middle = ansi.Cut(renderedLine, lineStart, lineStart+1)
			after = ansi.Cut(renderedLine, lineStart+1, lineWidth)
		}
		lines[line] = before + applySelectionStyle(middle, c.styles.Selection) + after
	}
	return strings.Join(lines, "\n")
}

func headerLine(style lipgloss.Style, width int) string {
	return style.Render(strings.Repeat("─", max(0, width)))
}

func compactTokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000_000), ".0"), ".") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000), ".0"), ".") + "k"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len([]rune(s)) <= maxLen {
		return s
	}
	r := []rune(s)
	return string(r[:maxLen-1]) + "…"
}

func (c *Chat) Size() (int, int) { return c.width, c.height }

func renderContentPanel(styles common.Styles, title, body string, width, maxLines int) string {
	if strings.TrimSpace(body) == "" {
		return styles.MsgTimestamp.Render("No output")
	}
	clean := common.Escape(strings.ReplaceAll(body, "\r\n", "\n"))
	lines := strings.Split(clean, "\n")
	hidden := 0
	if maxLines > 0 && len(lines) > maxLines {
		hidden = len(lines) - maxLines
		lines = lines[:maxLines]
	}
	wrapped := make([]string, 0, len(lines)+1)
	innerW := max(16, width-4)
	for _, line := range lines {
		wrapped = append(wrapped, wrap.String(line, innerW))
	}
	if hidden > 0 {
		wrapped = append(wrapped, styles.MsgTimestamp.Render(fmt.Sprintf("… %d more lines", hidden)))
	}
	panelBody := strings.Join(wrapped, "\n")
	if title != "" {
		panelBody = styles.Key.Render(title) + "\n" + panelBody
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width).
		Render(panelBody)
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
