package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/muesli/reflow/wrap"
)

const thinkTailLines = 4
const interimNarrationLines = 2

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
			footParts = append(footParts, styles.Desc.Render("ctrl+t to expand"))
		} else {
			footParts = append(footParts, styles.Desc.Render("ctrl+t to collapse"))
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
		if a.content != "" {
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}
	if a.content != "" {
		var rendered string
		if !a.streaming && a.contentCacheWidth == width && a.contentCacheRender != "" {
			rendered = a.contentCacheRender
		} else {
			if !a.streaming && !a.showMeta && !c.verboseInterim {
				rendered = renderCompactAssistantNarration(c.styles, a.content, width)
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
			}
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

func renderCompactAssistantNarration(styles common.Styles, content string, width int) string {
	innerW := max(20, width-2)
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if normalized == "" {
		return ""
	}
	wrapped := strings.TrimSpace(wrap.String(normalized, innerW))
	lines := strings.Split(wrapped, "\n")
	if len(lines) > interimNarrationLines {
		lines = lines[:interimNarrationLines]
		last := []rune(strings.TrimRight(lines[len(lines)-1], " "))
		if len(last) >= innerW {
			last = last[:innerW-1]
		}
		lines[len(lines)-1] = strings.TrimRight(string(last), " ") + "…"
	}
	for i, line := range lines {
		lines[i] = styles.InterimAssistant.Render(line)
	}
	return strings.Join(lines, "\n")
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
