package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wrap"
)

// ─── Tool icons ───────────────────────────────────────────────────────────────

var toolIcons = map[string]string{
	"bash":             "❯",
	"write_file":       "✏",
	"edit_file":        "✏",
	"apply_patch":      "✏",
	"read_file":        "◻",
	"list_directory":   "◫",
	"glob":             "◈",
	"grep":             "◈",
	"web_fetch":        "◉",
	"web_search":       "◉",
	"job_output":       "◆",
	"job_kill":         "⊗",
	"write_stdin":      "❯",
	"create_directory": "◫",
}

func toolIconFor(name string) string {
	if icon, ok := toolIcons[name]; ok {
		return icon
	}
	return "◆"
}

// ─── Message item interface ───────────────────────────────────────────────────

// msgItem is the renderable unit in the chat viewport.
type msgItem interface {
	render(c *chat, width int) string
	isFinished() bool
	invalidate()
}

// ─── Thinking block ───────────────────────────────────────────────────────────

const (
	thinkTailLines = 10 // lines shown when collapsed
)

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

func (tb *thinkingBlock) render(styles Styles, width int) string {
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
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	box := boxStyle.Render(inner.String())

	// Footer: duration + toggle hint
	var footParts []string
	if tb.streaming {
		footParts = append(footParts, styles.MsgTimestamp.Render("thinking…"))
	} else {
		dur := tb.finishedAt.Sub(tb.startedAt).Round(100 * time.Millisecond)
		footParts = append(footParts,
			styles.MsgTimestamp.Render(fmt.Sprintf("Thought for %.1fs", dur.Seconds())))
		if tb.collapsed {
			footParts = append(footParts,
				styles.Key.Render("ctrl+t")+" "+styles.Desc.Render("expand"))
		} else {
			footParts = append(footParts,
				styles.Key.Render("ctrl+t")+" "+styles.Desc.Render("collapse"))
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

// ─── assistantItem ────────────────────────────────────────────────────────────

type assistantItem struct {
	thinking   *thinkingBlock
	content    string
	streaming  bool
	finishedAt time.Time

	// showLabel is true only for the first assistantItem in a turn.
	// Subsequent items (post-tool text) omit the "Nexus" header so the
	// whole turn reads as one continuous agent response.
	showLabel bool

	contentCacheWidth  int
	contentCacheRender string
}

func newAssistantItem() *assistantItem {
	return &assistantItem{streaming: true, showLabel: true}
}

// newContinuationItem creates a follow-up assistant item within the same
// turn (text that arrives after a tool call). No "Nexus" label — it
// visually continues from the previous segment.
func newContinuationItem() *assistantItem {
	return &assistantItem{streaming: true, showLabel: false}
}

func (a *assistantItem) appendThinking(text string) {
	if text == "" {
		return // never create a thinkingBlock for empty deltas
	}
	if a.thinking == nil {
		a.thinking = newThinkingBlock()
	}
	a.thinking.append(text)
	a.contentCacheWidth = 0
}

func (a *assistantItem) appendContent(text string) {
	// Seal thinking block when content begins
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
	}
	a.content += text
	a.contentCacheWidth = 0
}

func (a *assistantItem) finish() {
	a.streaming = false
	a.finishedAt = time.Now()
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
	}
	a.contentCacheWidth = 0
}

func (a *assistantItem) isFinished() bool { return !a.streaming }
func (a *assistantItem) invalidate()      { a.contentCacheWidth = 0 }

func (a *assistantItem) render(c *chat, width int) string {
	var sb strings.Builder

	// Only the first item per turn shows the "Nexus" label.
	if a.showLabel {
		sb.WriteString(c.styles.AssistantLabel.Render("Nexus"))
		sb.WriteString("\n")
	}

	// Only render thinking if it has actual content.
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
			rendered, err = c.renderer.Render(a.content)
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

	return sb.String()
}

// ─── userItem ────────────────────────────────────────────────────────────────

type userItem struct {
	content   string
	timestamp time.Time
	cacheW    int
	cacheR    string
}

func (u *userItem) isFinished() bool { return true }
func (u *userItem) invalidate()      { u.cacheW = 0 }

func (u *userItem) render(c *chat, width int) string {
	if u.cacheW == width && u.cacheR != "" {
		return u.cacheR
	}
	header := c.styles.UserLabel.Render("You") + "  " +
		c.styles.MsgTimestamp.Render(u.timestamp.Format("15:04"))
	body := c.styles.UserMsg.Render(wrap.String(u.content, width-2))
	r := header + "\n" + body
	u.cacheW = width
	u.cacheR = r
	return r
}

// ─── toolItem ────────────────────────────────────────────────────────────────

type toolItem struct {
	id         string // ToolUseID — unique per call
	name       string
	status     string // "pending" | "running" | "completed" | "failed" | "done" | "error"
	label      string
	startedAt  time.Time
	finishedAt time.Time

	cacheW int
	cacheR string
}

func newToolItem(id, name, status, label string) *toolItem {
	return &toolItem{
		id:        id,
		name:      name,
		status:    status,
		label:     label,
		startedAt: time.Now(),
	}
}

func (t *toolItem) isDone() bool {
	return t.status == "completed" || t.status == "failed" ||
		t.status == "done" || t.status == "error"
}

func (t *toolItem) isFinished() bool { return t.isDone() }
func (t *toolItem) invalidate()      { t.cacheW = 0 }

func (t *toolItem) renderIcon(styles Styles) string {
	switch {
	case t.status == "completed" || t.status == "done":
		return styles.ToolDone.Render("✓")
	case t.status == "failed" || t.status == "error":
		return styles.ToolError.Render("✗")
	default:
		return styles.ToolProgress.Render(toolIconFor(t.name))
	}
}

func (t *toolItem) render(c *chat, width int) string {
	if t.isDone() && t.cacheW == width && t.cacheR != "" {
		return t.cacheR
	}

	icon := t.renderIcon(c.styles)

	var nameStyle lipgloss.Style
	switch {
	case t.status == "completed" || t.status == "done":
		nameStyle = c.styles.ToolDone
	case t.status == "failed" || t.status == "error":
		nameStyle = c.styles.ToolError
	default:
		nameStyle = c.styles.ToolProgress
	}

	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(icon)
	sb.WriteString(" ")
	sb.WriteString(nameStyle.Render(t.name))

	// Label (truncated to fit)
	if t.label != "" && t.label != t.status {
		maxLabelW := width - 30
		if maxLabelW < 10 {
			maxLabelW = 10
		}
		short := truncate(t.label, maxLabelW)
		sb.WriteString(c.styles.MsgTimestamp.Render("  " + short))
	}

	// Duration for finished tools
	if t.isDone() && !t.finishedAt.IsZero() {
		d := t.finishedAt.Sub(t.startedAt)
		var durStr string
		if d < time.Second {
			durStr = fmt.Sprintf("%dms", d.Milliseconds())
		} else {
			durStr = fmt.Sprintf("%.1fs", d.Seconds())
		}
		sb.WriteString(c.styles.MsgTimestamp.Render("  (" + durStr + ")"))
	}

	r := sb.String()
	if t.isDone() {
		t.cacheW = width
		t.cacheR = r
	}
	return r
}

// ─── systemItem ──────────────────────────────────────────────────────────────

type systemItem struct {
	content string
}

func (s *systemItem) isFinished() bool { return true }
func (s *systemItem) invalidate()      {}

func (s *systemItem) render(c *chat, _ int) string {
	return c.styles.MsgTimestamp.Render("─ " + s.content)
}

// ─── errorItem ───────────────────────────────────────────────────────────────

type errorItem struct {
	content string
}

func (e *errorItem) isFinished() bool { return true }
func (e *errorItem) invalidate()      {}

func (e *errorItem) render(c *chat, _ int) string {
	return c.styles.ToolError.Render("✗ " + e.content)
}

// ─── chat ────────────────────────────────────────────────────────────────────

type chat struct {
	styles   Styles
	viewport *viewport.Model
	renderer *glamour.TermRenderer
	messages []msgItem
	width    int
	height   int
	follow   bool
}

func newChat(styles Styles, width, height int) *chat {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	vp.SetContent("")
	r, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(clampInt(width-4, 1, 100)),
	)
	return &chat{
		styles:   styles,
		viewport: &vp,
		renderer: r,
		follow:   true,
		width:    width,
		height:   height,
	}
}

func (c *chat) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.viewport.SetWidth(width)
	c.viewport.SetHeight(height)
	if r, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(clampInt(width-4, 1, 100)),
	); err == nil {
		c.renderer = r
	}
	for _, m := range c.messages {
		m.invalidate()
	}
	c.refresh()
}

// ─── Public mutation API ──────────────────────────────────────────────────────

func (c *chat) AddUserMessage(text string) {
	c.messages = append(c.messages, &userItem{
		content:   text,
		timestamp: time.Now(),
	})
	c.refresh()
}

func (c *chat) StartAssistantMessage() {
	c.messages = append(c.messages, newAssistantItem())
	c.refresh()
}

func (c *chat) AppendChunk(text string, isThinking bool) {
	if text == "" {
		return // nothing to render; avoid creating empty items
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
	// No active streaming item — post-tool text within the same turn.
	// Search backwards; if we see an assistantItem before hitting a userItem
	// (or the start), this is a continuation — omit the "Nexus" label.
	isContinuation := false
	for i := len(c.messages) - 1; i >= 0; i-- {
		if _, ok := c.messages[i].(*userItem); ok {
			break
		}
		if _, ok := c.messages[i].(*assistantItem); ok {
			isContinuation = true
			break
		}
	}
	if isContinuation {
		c.messages = append(c.messages, newContinuationItem())
	} else {
		c.messages = append(c.messages, newAssistantItem())
	}
	c.AppendChunk(text, isThinking)
}

func (c *chat) FinishAssistantMessage() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok && a.streaming {
			a.finish()
			c.refresh()
			return
		}
	}
}

// AddToolProgress adds or updates a tool call entry.
// toolUseID is the unique per-call identifier; if empty, falls back to
// name-based matching on the most recent undone tool with that name.
func (c *chat) AddToolProgress(toolUseID, toolName, status, label string) {
	// Update existing tool item if found.
	if toolUseID != "" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if t, ok := c.messages[i].(*toolItem); ok && t.id == toolUseID {
				t.status = status
				t.label = label
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
				if t.isDone() {
					t.finishedAt = time.Now()
				}
				t.invalidate()
				c.refresh()
				return
			}
		}
	}

	// New tool: seal the current streaming assistant item so post-tool text
	// appears in a fresh item after this tool entry (crush interleave pattern).
	c.sealActiveAssistant()

	c.messages = append(c.messages, newToolItem(toolUseID, toolName, status, label))
	c.refresh()
}

// sealActiveAssistant closes the last streaming assistantItem so subsequent
// ChunkMsgs create a new one. An empty item (no content, no thinking) is
// removed rather than kept as a blank entry.
func (c *chat) sealActiveAssistant() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		a, ok := c.messages[i].(*assistantItem)
		if !ok || !a.streaming {
			continue
		}
		// Drop placeholder if it has no visible content at all.
		hasThinking := a.thinking != nil && strings.TrimSpace(a.thinking.content) != ""
		if a.content == "" && !hasThinking {
			c.messages = append(c.messages[:i], c.messages[i+1:]...)
		} else {
			a.finish()
		}
		return
	}
}

func (c *chat) AddError(err error) {
	c.messages = append(c.messages, &errorItem{content: err.Error()})
	c.refresh()
}

func (c *chat) AddSystem(text string) {
	c.messages = append(c.messages, &systemItem{content: text})
	c.refresh()
}

func (c *chat) Clear() {
	c.messages = c.messages[:0]
	c.refresh()
}

// GetLastAssistantText returns the plain-text content of the most recent
// completed assistant turn (all segments concatenated, no markdown symbols).
func (c *chat) GetLastAssistantText() string {
	var parts []string
	inCurrentTurn := false
	for i := len(c.messages) - 1; i >= 0; i-- {
		switch m := c.messages[i].(type) {
		case *userItem:
			if inCurrentTurn {
				// Reached the user message that started this turn — stop.
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

// GetLastUserText returns the text of the most recent user message.
func (c *chat) GetLastUserText() string {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if u, ok := c.messages[i].(*userItem); ok {
			return u.content
		}
	}
	return ""
}

// ToggleThinking toggles the collapse state of the most recent thinking block.
func (c *chat) ToggleThinking() {
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

// HasThinking reports whether the most recent assistant item has a thinking block.
func (c *chat) HasThinking() bool {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if a, ok := c.messages[i].(*assistantItem); ok {
			return a.thinking != nil
		}
	}
	return false
}

// ─── Scroll ──────────────────────────────────────────────────────────────────

func (c *chat) ScrollUp(n int)   { c.follow = false; c.viewport.ScrollUp(n) }
func (c *chat) ScrollDown(n int) { c.viewport.ScrollDown(n); c.follow = c.viewport.AtBottom() }
func (c *chat) PageUp()          { c.follow = false; c.viewport.HalfPageUp() }
func (c *chat) PageDown()        { c.viewport.HalfPageDown(); c.follow = c.viewport.AtBottom() }
func (c *chat) GotoTop()         { c.follow = false; c.viewport.GotoTop() }
func (c *chat) GotoBottom()      { c.follow = true; c.viewport.GotoBottom() }

func (c *chat) View() string { return c.viewport.View() }

// ─── Internal ────────────────────────────────────────────────────────────────

func (c *chat) refresh() {
	var sb strings.Builder
	lastWasTool := false
	wroteAny := false
	for _, item := range c.messages {
		rendered := item.render(c, c.width)
		if rendered == "" {
			continue // skip invisible items; no separator either
		}
		if wroteAny {
			_, currIsTool := item.(*toolItem)
			if lastWasTool && currIsTool {
				sb.WriteString("\n") // consecutive tools: no blank line
			} else {
				sb.WriteString("\n\n") // all other boundaries: blank line
			}
		}
		sb.WriteString(rendered)
		_, lastWasTool = item.(*toolItem)
		wroteAny = true
	}
	c.viewport.SetContent(sb.String())
	if c.follow {
		c.viewport.GotoBottom()
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func headerLine(style lipgloss.Style, width int) string {
	return style.Render(strings.Repeat("─", max(0, width)))
}

// clampInt clamps v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
