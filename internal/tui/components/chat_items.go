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
