package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (c *Chat) ScrollUp(n int)   { c.follow = false; c.viewport.ScrollUp(n) }
func (c *Chat) ScrollDown(n int) { c.viewport.ScrollDown(n); c.follow = c.viewport.AtBottom() }
func (c *Chat) PageUp()          { c.follow = false; c.viewport.HalfPageUp() }
func (c *Chat) PageDown()        { c.viewport.HalfPageDown(); c.follow = c.viewport.AtBottom() }
func (c *Chat) GotoTop()         { c.follow = false; c.viewport.GotoTop() }
func (c *Chat) GotoBottom()      { c.follow = true; c.viewport.GotoBottom() }

func (c *Chat) DetailScrollUp(n int) {
	if c.detail != nil {
		c.detail.ScrollUp(n)
	}
}

func (c *Chat) DetailScrollDown(n int) {
	if c.detail != nil {
		c.detail.ScrollDown(n)
	}
}

func (c *Chat) DetailPageUp() {
	if c.detail != nil {
		c.detail.HalfPageUp()
	}
}

func (c *Chat) DetailPageDown() {
	if c.detail != nil {
		c.detail.HalfPageDown()
	}
}

func (c *Chat) DetailGotoTop() {
	if c.detail != nil {
		c.detail.GotoTop()
	}
}

func (c *Chat) DetailGotoBottom() {
	if c.detail != nil {
		c.detail.GotoBottom()
	}
}

func (c *Chat) View() string { return c.viewport.View() }

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
	i := 0
	for i < len(c.messages) {
		tool, ok := c.messages[i].(*toolItem)
		if !ok {
			i++
			continue
		}
		if isGroupableTool(tool.name) && tool.isDone() {
			// Scan ahead for the full group.
			j := i + 1
			for j < len(c.messages) {
				next, ok2 := c.messages[j].(*toolItem)
				if !ok2 || next.name != tool.name || !next.isDone() {
					break
				}
				j++
			}
			// The selection point for the whole group is its last item.
			indices = append(indices, j-1)
			i = j
		} else {
			indices = append(indices, i)
			i++
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
	mi := 0
	for mi < len(c.messages) {
		item := c.messages[mi]

		// ── Group detection: consecutive completed same-name groupable tools ──
		var rendered string
		var isTool bool
		var regionMsgIndex int

		if tool, ok := item.(*toolItem); ok && isGroupableTool(tool.name) && tool.isDone() {
			j := mi + 1
			for j < len(c.messages) {
				next, ok2 := c.messages[j].(*toolItem)
				if !ok2 || next.name != tool.name || !next.isDone() {
					break
				}
				j++
			}
			if j-mi >= 2 {
				// Render the group as one summary row.
				groupItems := make([]*toolItem, j-mi)
				for k := mi; k < j; k++ {
					groupItems[k-mi] = c.messages[k].(*toolItem)
				}
				lastIdx := j - 1
				selectedInGroup := c.selectedTool >= mi && c.selectedTool <= lastIdx
				rendered = renderToolGroup(c, groupItems, c.width, selectedInGroup)
				isTool = true
				regionMsgIndex = lastIdx // selection lands on last item in the group
				mi = j
			} else {
				// Only one item — render normally.
				rendered = tool.renderSelected(c, c.width, mi == c.selectedToolIndex())
				isTool = true
				regionMsgIndex = mi
				mi++
			}
		} else if tool, ok := item.(*toolItem); ok {
			rendered = tool.renderSelected(c, c.width, mi == c.selectedToolIndex())
			isTool = true
			regionMsgIndex = mi
			mi++
		} else {
			rendered = item.render(c, c.width)
			isTool = false
			regionMsgIndex = mi
			mi++
		}

		if rendered == "" {
			continue
		}
		plainRendered := ansi.Strip(rendered)
		if wroteAny {
			if lastWasTool && isTool {
				sb.WriteString("\n")
				plainSB.WriteString("\n")
				line++
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
		if isTool {
			// expanderStart/End at col 0-1 (the ▸/▾ symbol). Detail click disabled
			// (right panel is opened with keyboard or by selecting the tool).
			toolRegions = append(toolRegions, toolRegion{
				startLine:     startLine,
				endLine:       startLine + height - 1,
				msgIndex:      regionMsgIndex,
				expanderStart: 0,
				expanderEnd:   1,
				detailStart:   0,
				detailEnd:     0,
			})
		}
		if assistant, ok := item.(*assistantItem); ok && assistant.thinking != nil && strings.TrimSpace(assistant.thinking.content) != "" {
			thinkingStart := startLine
			if assistant.showLabel {
				thinkingStart++
			}
			thinkingRendered := assistant.thinking.render(c.styles, c.width)
			thinkingHeight := max(1, lipgloss.Height(ansi.Strip(thinkingRendered)))
			thinkingRegions = append(thinkingRegions, thinkingRegion{startLine: thinkingStart, endLine: thinkingStart + thinkingHeight - 1, msgIndex: regionMsgIndex})
		}
		line += height
		lastWasTool = isTool
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
