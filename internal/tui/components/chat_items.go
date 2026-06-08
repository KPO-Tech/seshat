package components

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wrap"
)

type msgItem interface {
	render(c *Chat, width int) string
	isFinished() bool
	invalidate()
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

	timeStr := ""
	if !u.timestamp.IsZero() {
		timeStr = u.timestamp.Format("15:04:05")
	}
	left := "👤 You"
	leftStyled := c.styles.UserLabel.Render(left)
	rightStyled := ""
	if timeStr != "" {
		rightStyled = c.styles.MsgTimestamp.Render(timeStr)
	}

	header := leftStyled
	if rightStyled != "" {
		padding := width - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
		if padding > 0 {
			header += strings.Repeat(" ", padding) + rightStyled
		} else {
			header += " " + rightStyled
		}
	}

	bar := c.styles.UserMarker.Render("│")
	prefix := "  " + bar + " "

	bodyWidth := max(12, width-4)
	wrapped := strings.Split(wrap.String(u.content, bodyWidth), "\n")
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	for i := 0; i < len(wrapped); i++ {
		wrapped[i] = prefix + c.styles.UserMsg.Render(wrapped[i])
	}
	body := strings.Join(wrapped, "\n")

	r := header + "\n" + body
	u.cacheW = width
	u.cacheR = r
	return r
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
