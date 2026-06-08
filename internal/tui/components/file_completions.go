package components

import (
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// fileCompletions is the @-triggered file picker popup shown above the input.
// When the user types @ the popup opens; typing more chars filters the list.
type FileCompletions struct {
	styles   common.Styles
	workDir  string
	items    []string // full relative paths from workDir
	filtered []string
	query    string
	cursor   int
	open     bool
	width    int
}

func NewFileCompletions(styles common.Styles, workDir string) *FileCompletions {
	return &FileCompletions{
		styles:  styles,
		workDir: workDir,
	}
}

func (c *FileCompletions) Open(workDir string) {
	c.workDir = workDir
	c.query = ""
	c.cursor = 0
	c.open = true
	c.load()
}

func (c *FileCompletions) Close() {
	c.open = false
	c.query = ""
	c.cursor = 0
}

func (c *FileCompletions) IsOpen() bool { return c.open }

func (c *FileCompletions) TypeChar(ch string) {
	c.query += ch
	c.cursor = 0
	c.filter()
}

func (c *FileCompletions) Backspace() {
	if len(c.query) > 0 {
		c.query = c.query[:len(c.query)-1]
		c.cursor = 0
		c.filter()
	} else {
		// Empty query on backspace = close
		c.Close()
	}
}

func (c *FileCompletions) Up() {
	if c.cursor > 0 {
		c.cursor--
	}
}
func (c *FileCompletions) Down() {
	if c.cursor < len(c.filtered)-1 {
		c.cursor++
	}
}

// Selected returns the currently highlighted item, or "".
func (c *FileCompletions) Selected() string {
	if c.cursor >= 0 && c.cursor < len(c.filtered) {
		return c.filtered[c.cursor]
	}
	return ""
}

// Query returns the current filter text (what was typed after @).
func (c *FileCompletions) Query() string { return c.query }

func (c *FileCompletions) SetSize(width int) { c.width = width }

func (c *FileCompletions) load() {
	c.items = c.items[:0]
	c.walkDir(c.workDir, "", 0)
	c.filter()
}

func (c *FileCompletions) walkDir(base, rel string, depth int) {
	if depth > 3 {
		return
	}
	entries, err := os.ReadDir(filepath.Join(base, rel))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		var path string
		if rel == "" {
			path = name
		} else {
			path = rel + "/" + name
		}
		if e.IsDir() {
			c.walkDir(base, path, depth+1)
		} else {
			c.items = append(c.items, path)
		}
	}
}

func (c *FileCompletions) filter() {
	c.filtered = c.filtered[:0]
	if c.query == "" {
		// Show recent/common files first
		limit := 15
		for i, item := range c.items {
			if i >= limit {
				break
			}
			c.filtered = append(c.filtered, item)
		}
		return
	}
	q := strings.ToLower(c.query)
	// Exact prefix matches first
	for _, item := range c.items {
		base := strings.ToLower(filepath.Base(item))
		if strings.HasPrefix(base, q) {
			c.filtered = append(c.filtered, item)
		}
	}
	// Then substring matches
	for _, item := range c.items {
		lower := strings.ToLower(item)
		if strings.Contains(lower, q) && !func() bool {
			base := strings.ToLower(filepath.Base(item))
			return strings.HasPrefix(base, q)
		}() {
			c.filtered = append(c.filtered, item)
		}
	}
	// Cap at 15 results
	if len(c.filtered) > 15 {
		c.filtered = c.filtered[:15]
	}
}

// View renders the completions popup (rendered as a string, placed above the input).
func (c *FileCompletions) View(inputWidth int) string {
	if !c.open {
		return ""
	}

	w := min(inputWidth, 60)
	const maxVisible = 8

	start := max(0, c.cursor-maxVisible+1)
	end := min(len(c.filtered), start+maxVisible)

	if len(c.filtered) == 0 {
		return c.styles.BrowserBorder.Width(w).Render(
			c.styles.MsgTimestamp.Render("  no files matching " + c.query),
		)
	}

	var rows []string
	for i := start; i < end; i++ {
		item := c.filtered[i]
		name := filepath.Base(item)
		dir := filepath.Dir(item)
		if dir == "." {
			dir = ""
		}

		var line string
		if dir != "" {
			line = name + c.styles.MsgTimestamp.Render("  "+dir)
		} else {
			line = name
		}

		if i == c.cursor {
			rows = append(rows, c.styles.BrowserSelected.Width(w-4).Render("▶ "+line))
		} else {
			rows = append(rows, c.styles.BrowserItem.Width(w-4).Render("  "+line))
		}
	}

	title := c.styles.MsgTimestamp.Render("@" + c.query + "█")
	sep := c.styles.MsgTimestamp.Render(strings.Repeat("─", w-4))

	content := title + "\n" + sep + "\n" + strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorPrimary).
		PaddingLeft(1).PaddingRight(1).
		Width(w).
		Render(content)
}

func (c *FileCompletions) Width(inputWidth int) int {
	return min(inputWidth, 60)
}

func (c *FileCompletions) Height(inputWidth int) int {
	if !c.open {
		return 0
	}
	if len(c.filtered) == 0 {
		return 3
	}
	visible := min(len(c.filtered), 8)
	return visible + 4 // border (2) + title (1) + sep (1)
}

func (c *FileCompletions) Scroll(delta int) {
	if delta < 0 {
		c.Up()
	} else if delta > 0 {
		c.Down()
	}
}

func (c *FileCompletions) ClickRow(row int) string {
	if len(c.filtered) == 0 {
		return ""
	}
	const maxVisible = 8
	start := max(0, c.cursor-maxVisible+1)
	idx := start + row
	if idx < start || idx >= start+maxVisible || idx >= len(c.filtered) {
		return ""
	}
	c.cursor = idx
	return c.filtered[idx]
}
