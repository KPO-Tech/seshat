package components

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

const skillPopupVisible = 3

// SkillCompletions is the /-triggered skill picker shown just above the composer.
type SkillCompletions struct {
	styles   common.Styles
	items    []tui.SkillInfo
	filtered []tui.SkillInfo
	query    string
	cursor   int
	open     bool
}

func NewSkillCompletions(styles common.Styles) *SkillCompletions {
	return &SkillCompletions{styles: styles}
}

func (c *SkillCompletions) Sync(items []tui.SkillInfo, input string) {
	query, ok := skillQuery(input)
	if !ok {
		c.Close()
		return
	}
	c.open = true
	c.query = query
	c.items = append(c.items[:0], items...)
	c.filter()
}

func (c *SkillCompletions) Close() {
	c.open = false
	c.query = ""
	c.cursor = 0
	c.filtered = c.filtered[:0]
}

func (c *SkillCompletions) IsOpen() bool { return c.open }

func (c *SkillCompletions) Up() {
	if c.cursor > 0 {
		c.cursor--
	}
}

func (c *SkillCompletions) Down() {
	if c.cursor < len(c.filtered)-1 {
		c.cursor++
	}
}

func (c *SkillCompletions) Scroll(delta int) {
	if delta < 0 {
		c.Up()
		return
	}
	if delta > 0 {
		c.Down()
	}
}

func (c *SkillCompletions) Selected() string {
	if c.cursor >= 0 && c.cursor < len(c.filtered) {
		return "/" + c.filtered[c.cursor].Name
	}
	return ""
}

func (c *SkillCompletions) Width(inputWidth int) int {
	return min(max(36, inputWidth-10), 84)
}

func (c *SkillCompletions) Height(inputWidth int) int {
	if !c.open {
		return 0
	}
	visible := min(len(c.filtered), skillPopupVisible)
	if visible == 0 {
		visible = 1
	}
	return visible + 2
}

func (c *SkillCompletions) ClickRow(row int) string {
	start, end := c.visibleRange()
	idx := start + row
	if idx < start || idx >= end || idx >= len(c.filtered) {
		return ""
	}
	c.cursor = idx
	return "/" + c.filtered[idx].Name
}

func (c *SkillCompletions) View(inputWidth int) string {
	if !c.open {
		return ""
	}
	w := c.Width(inputWidth)
	start, end := c.visibleRange()
	if len(c.filtered) == 0 {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(common.ColorPrimary).
			PaddingLeft(1).PaddingRight(1).
			Width(w).
			Render(c.styles.MsgTimestamp.Render("no skills matching /" + c.query))
	}

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := c.filtered[i]
		name := "/" + item.Name
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = strings.TrimSpace(item.WhenToUse)
		}
		desc = truncateText(desc, max(12, w-lipgloss.Width(name)-10))
		left := name
		if desc != "" {
			left += c.styles.MsgTimestamp.Render("  " + desc)
		}
		if i == c.cursor {
			rows = append(rows, c.styles.BrowserSelected.Width(w-4).Render("> "+left))
		} else {
			rows = append(rows, c.styles.BrowserItem.Width(w-4).Render("  "+left))
		}
	}
	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorPrimary).
		PaddingLeft(1).PaddingRight(1).
		Width(w).
		Render(content)
}

func (c *SkillCompletions) visibleRange() (int, int) {
	visible := skillPopupVisible
	if len(c.filtered) < visible {
		visible = len(c.filtered)
	}
	if visible <= 0 {
		return 0, 0
	}
	start := c.cursor - (visible - 1)
	if start < 0 {
		start = 0
	}
	maxStart := len(c.filtered) - visible
	if start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start, start + visible
}

func (c *SkillCompletions) filter() {
	c.filtered = c.filtered[:0]
	q := strings.ToLower(strings.TrimSpace(c.query))
	for _, item := range c.items {
		name := strings.ToLower(item.Name)
		if q == "" || strings.HasPrefix(name, q) {
			c.filtered = append(c.filtered, item)
		}
	}
	if len(c.filtered) == 0 && q != "" {
		for _, item := range c.items {
			if strings.Contains(strings.ToLower(item.Name), q) {
				c.filtered = append(c.filtered, item)
			}
		}
	}
	sort.SliceStable(c.filtered, func(i, j int) bool {
		return c.filtered[i].Name < c.filtered[j].Name
	})
	if c.cursor >= len(c.filtered) {
		c.cursor = max(0, len(c.filtered)-1)
	}
}

func skillQuery(input string) (string, bool) {
	if input == "" || !strings.HasPrefix(input, "/") {
		return "", false
	}
	if strings.ContainsAny(input, " \t\n") {
		return "", false
	}
	return strings.TrimPrefix(input, "/"), true
}

func truncateText(s string, limit int) string {
	if limit <= 0 || lipgloss.Width(s) <= limit {
		return s
	}
	runes := []rune(s)
	if limit <= 1 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}
