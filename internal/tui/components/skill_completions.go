package components

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// SkillCompletions is the /-triggered skill picker shown above the composer.
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

func (c *SkillCompletions) Selected() string {
	if c.cursor >= 0 && c.cursor < len(c.filtered) {
		return "/" + c.filtered[c.cursor].Name
	}
	return ""
}

func (c *SkillCompletions) View(inputWidth int) string {
	if !c.open {
		return ""
	}
	w := min(inputWidth, 72)
	const maxVisible = 6
	start := max(0, c.cursor-maxVisible+1)
	end := min(len(c.filtered), start+maxVisible)
	if len(c.filtered) == 0 {
		return c.styles.BrowserBorder.Width(w).Render(
			c.styles.MsgTimestamp.Render("  no skills matching /" + c.query),
		)
	}
	var rows []string
	for i := start; i < end; i++ {
		item := c.filtered[i]
		name := "/" + item.Name
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = strings.TrimSpace(item.WhenToUse)
		}
		line := name
		if desc != "" {
			line += c.styles.MsgTimestamp.Render("  " + desc)
		}
		if i == c.cursor {
			rows = append(rows, c.styles.BrowserSelected.Width(w-4).Render("▶ "+line))
		} else {
			rows = append(rows, c.styles.BrowserItem.Width(w-4).Render("  "+line))
		}
	}
	title := c.styles.MsgTimestamp.Render("/" + c.query + "█")
	sep := c.styles.MsgTimestamp.Render(strings.Repeat("─", w-4))
	content := title + "\n" + sep + "\n" + strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorPrimary).
		PaddingLeft(1).PaddingRight(1).
		Width(w).
		Render(content)
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
