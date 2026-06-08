package components

import (
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
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

type Chat struct {
	styles   common.Styles
	viewport *viewport.Model
	renderer *glamour.TermRenderer
	detail   *viewport.Model
	messages []msgItem
	width    int
	height   int
	follow   bool

	selectedTool int
	detailOpen   bool
	planDepth    int // incremented by enter_plan_mode, decremented by exit_plan_mode
	pairDepth    int // incremented by enter_pair_programming_mode, decremented by exit_pair_programming_mode

	renderedContent string
	renderedLines   []string
	plainContent    string
	plainLines      []string
	toolRegions     []toolRegion
	thinkingRegions []thinkingRegion
	selection       mouseSelection
	verboseInterim  bool
	detailKey       string
	detailToolID    string // ID of tool currently rendered in the detail sidebar
}

func NewChat(styles common.Styles, width, height int) *Chat {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	vp.SetContent("")
	detail := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	detail.SetContent("")
	r := common.MarkdownRenderer(width)
	return &Chat{
		styles:       styles,
		viewport:     &vp,
		renderer:     r,
		detail:       &detail,
		follow:       true,
		width:        width,
		height:       height,
		selectedTool: -1,
	}
}

func (c *Chat) SetSize(width, height int) {
	if c.width == width && c.height == height {
		return
	}
	c.width = width
	c.height = height
	c.viewport.SetWidth(width)
	c.viewport.SetHeight(height)
	if r := common.MarkdownRenderer(max(10, width-2)); r != nil {
		c.renderer = r
	}
	for _, m := range c.messages {
		m.invalidate()
	}
	c.refresh()
}

func (c *Chat) SetVerboseInterim(v bool) {
	if c.verboseInterim == v {
		return
	}
	c.verboseInterim = v
	for _, m := range c.messages {
		m.invalidate()
	}
	c.refresh()
}

func (c *Chat) VerboseInterim() bool {
	return c.verboseInterim
}

func (c *Chat) PlanMode() bool { return c.planDepth > 0 }
func (c *Chat) PairMode() bool { return c.pairDepth > 0 }

// ExecutionMode returns "plan", "pair_programming", or "execute" (default).
func (c *Chat) ExecutionMode() string {
	if c.planDepth > 0 {
		return "plan"
	}
	if c.pairDepth > 0 {
		return "pair_programming"
	}
	return "execute"
}

func (c *Chat) AddUserMessage(text string) {
	c.sealActiveAssistant()
	c.selection.clear()
	c.messages = append(c.messages, &userItem{content: text, timestamp: time.Now()})
	c.refresh()
}

func (c *Chat) StartAssistantMessage() {
	c.sealActiveAssistant()
	c.selection.clear()

	showLabel := true
	for i := len(c.messages) - 1; i >= 0; i-- {
		if _, ok := c.messages[i].(*userItem); ok {
			break
		}
		if _, ok := c.messages[i].(*assistantItem); ok {
			showLabel = false
			break
		}
	}

	item := newAssistantItem()
	item.showLabel = showLabel
	c.messages = append(c.messages, item)
	c.refresh()
}

func (c *Chat) AppendChunk(text string, isThinking bool) {
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
	// Plan mode / Pair programming mode intercepts.
	if status == "completed" {
		switch toolName {
		case "enter_plan_mode":
			c.planDepth++
		case "exit_plan_mode":
			c.planDepth = max(0, c.planDepth-1)
		case "enter_pair_programming_mode":
			c.pairDepth++
		case "exit_pair_programming_mode":
			c.pairDepth = max(0, c.pairDepth-1)
		}
	}

	// Update existing tool item if we find it.
	for _, m := range c.messages {
		if t, ok := m.(*toolItem); ok && t.id == toolUseID {
			if !(t.isDone() && (status == "running" || status == "pending")) {
				t.status = status
			}
			t.label = label
			for k, v := range metadata {
				t.metadata[k] = v
			}
			if t.isDone() {
				t.finishedAt = time.Now()
				if isAutoExpandTool(t.name) && !t.expanded {
					t.expanded = true
				}
			}
			t.invalidate()
			c.refresh()
			return
		}
	}

	// Not found: seal active assistant item (if any) so the tool row starts cleanly.
	c.sealActiveAssistant()
	c.selection.clear()

	// Append as a new toolItem.
	tool := newToolItem(toolUseID, toolName, status, label, metadata)
	if tool.isDone() && isAutoExpandTool(toolName) {
		tool.expanded = true
	}
	c.messages = append(c.messages, tool)

	// If this new tool starts, automatically select it.
	c.selectedTool = len(c.messages) - 1
	if isAutoExpandTool(toolName) {
		c.detailOpen = true
	}

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
	c.sealActiveAssistant()
	c.selection.clear()
	c.messages = append(c.messages, &errorItem{content: err.Error()})
	c.refresh()
}

func (c *Chat) AddSystem(text string) {
	c.sealActiveAssistant()
	c.selection.clear()
	c.messages = append(c.messages, &systemItem{content: text})
	c.refresh()
}

func (c *Chat) Clear() {
	c.messages = nil
	c.selectedTool = -1
	c.detailOpen = false
	c.planDepth = 0
	c.pairDepth = 0
	c.detailKey = ""
	c.detailToolID = ""
	c.selection.clear()
	c.viewport.SetContent("")
	c.detail.SetContent("")
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
		if user, ok := c.messages[i].(*userItem); ok {
			return user.content
		}
	}
	return ""
}

func (c *Chat) ToggleThinking() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if assistant, ok := c.messages[i].(*assistantItem); ok {
			if assistant.thinking != nil {
				assistant.thinking.toggle()
				assistant.invalidate()
				c.refresh()
			}
			break
		}
	}
}

func (c *Chat) HasThinking() bool {
	for _, m := range c.messages {
		if assistant, ok := m.(*assistantItem); ok && assistant.thinking != nil {
			return true
		}
	}
	return false
}

func (c *Chat) HasTools() bool {
	for _, m := range c.messages {
		if _, ok := m.(*toolItem); ok {
			return true
		}
	}
	return false
}

func (c *Chat) HasSelectedTool() bool {
	return c.selectedToolIndex() >= 0
}

func (c *Chat) DetailsOpen() bool {
	return c.detailOpen
}

func (c *Chat) ToggleSelectedToolExpanded() bool {
	if tool := c.selectedToolItem(); tool != nil && tool.supportsPreview() {
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
	indices := c.toolIndices()
	for _, idx := range indices {
		if idx > c.selectedToolIndex() {
			c.selectedTool = idx
			c.refresh()
			return true
		}
	}
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
	case strings.HasPrefix(line, "  │ "):
		return strings.TrimPrefix(line, "  │ ")
	case strings.HasPrefix(line, "  "):
		return strings.TrimPrefix(line, "  ")
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

func (c *Chat) Size() (int, int) { return c.width, c.height }

const inlinePreviewLines = 10
const previewTruncFmt = "… (%d lines hidden) [enter for full view]"

func isAutoExpandTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "apply_patch", "bash", "spawn_agent", "agent":
		return true
	}
	return false
}

func isGroupableTool(name string) bool {
	switch name {
	case "read_file", "write_file", "list_directory", "glob":
		return true
	}
	return false
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
