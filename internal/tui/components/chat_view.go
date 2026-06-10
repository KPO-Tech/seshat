package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components/list"
	"github.com/charmbracelet/x/ansi"
)

func (c *Chat) ScrollUp(n int)   { c.follow = false; c.list.ScrollBy(-n) }
func (c *Chat) ScrollDown(n int) { c.list.ScrollBy(n); c.follow = c.list.AtBottom() }
func (c *Chat) PageUp()          { c.follow = false; c.list.ScrollBy(-c.height / 2) }
func (c *Chat) PageDown()        { c.list.ScrollBy(c.height / 2); c.follow = c.list.AtBottom() }
func (c *Chat) GotoTop()         { c.follow = false; c.list.ScrollToTop() }
func (c *Chat) GotoBottom()      { c.follow = true; c.list.ScrollToBottom() }

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

func (c *Chat) View() string {
	if c.selection.hasSelection() {
		return c.highlightedSelectionContent()
	}
	return c.list.Render()
}

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
	items := make([]list.Item, len(c.messages))
	for i, m := range c.messages {
		items[i] = m
	}
	c.list.SetItems(items...)
	if c.follow {
		c.list.ScrollToBottom()
	}
	c.recomputePlainAndRegions()
}

func (c *Chat) refreshSelection() {
	// Handled automatically because m.viewChat() calls c.View()
}

func (c *Chat) recomputePlainAndRegions() {
	var sb strings.Builder
	var plainSB strings.Builder
	var toolRegions []toolRegion
	var thinkingRegions []thinkingRegion
	var itemRegions []itemRegion

	line := 0
	wroteAny := false
	var lastWasTool bool

	for mi, item := range c.messages {
		var rendered string
		var isTool bool

		if tool, ok := item.(*toolItem); ok {
			rendered = tool.Render(c.width)
			isTool = true
		} else {
			rendered = item.Render(c.width)
			isTool = false
		}

		if rendered == "" {
			itemRegions = append(itemRegions, itemRegion{startLine: line, endLine: line})
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
				msgIndex:      mi,
				expanderStart: 0,
				expanderEnd:   1,
				detailStart:   0,
				detailEnd:     0,
			})
		}
		if assistant, ok := item.(*assistantItem); ok && assistant.thinkingBoxHeight > 0 {
			thinkingStart := startLine
			if assistant.showLabel {
				thinkingStart++
			}
			thinkingHeight := max(1, assistant.thinkingBoxHeight)
			thinkingRegions = append(thinkingRegions, thinkingRegion{startLine: thinkingStart, endLine: thinkingStart + thinkingHeight - 1, msgIndex: mi})
		}
		itemRegions = append(itemRegions, itemRegion{
			startLine: startLine,
			endLine:   startLine + height - 1,
		})
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
	c.itemRegions = itemRegions
}

func (c *Chat) highlightedSelectionContent() string {
	if len(c.renderedLines) == 0 {
		return c.renderedContent
	}
	startLn, startCo, endLn, endCo := c.selectionRange()
	if startLn < 0 || endLn < 0 {
		return c.renderedContent
	}

	var sb strings.Builder
	// Write all lines before startLn
	if startLn > 0 {
		sb.WriteString(strings.Join(c.renderedLines[:startLn], "\n"))
		sb.WriteByte('\n')
	}

	// Process and write only the selected lines
	for line := startLn; line <= endLn; line++ {
		renderedLine := c.renderedLines[line]
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
		sb.WriteString(before)
		sb.WriteString(applySelectionStyle(middle, c.styles.Selection))
		sb.WriteString(after)
		if line < len(c.renderedLines)-1 {
			sb.WriteByte('\n')
		}
	}

	// Write all lines after endLn
	if endLn < len(c.renderedLines)-1 {
		sb.WriteString(strings.Join(c.renderedLines[endLn+1:], "\n"))
	}

	return sb.String()
}
