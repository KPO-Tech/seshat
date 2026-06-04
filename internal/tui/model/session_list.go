package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
)

// sessionList is the session browser overlay.
type sessionList struct {
	styles   Styles
	sessions []tui.SessionInfo
	filtered []tui.SessionInfo
	filter   string
	cursor   int
	width    int
	height   int
	editing  bool // whether the filter input has focus
}

func newSessionList(styles Styles) *sessionList {
	return &sessionList{
		styles:  styles,
		editing: true,
	}
}

func (s *sessionList) SetSessions(sessions []tui.SessionInfo) {
	s.sessions = sessions
	s.cursor = 0
	s.applyFilter()
}

func (s *sessionList) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *sessionList) TypeFilter(ch string) {
	s.filter += ch
	s.cursor = 0
	s.applyFilter()
}

func (s *sessionList) DeleteFilter() {
	if len(s.filter) > 0 {
		s.filter = s.filter[:len(s.filter)-1]
		s.cursor = 0
		s.applyFilter()
	}
}

func (s *sessionList) ClearFilter() {
	s.filter = ""
	s.cursor = 0
	s.applyFilter()
}

func (s *sessionList) Up() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *sessionList) Down() {
	if s.cursor < len(s.filtered)-1 {
		s.cursor++
	}
}

// Selected returns the session ID at the current cursor position, or "".
func (s *sessionList) Selected() string {
	if s.cursor >= 0 && s.cursor < len(s.filtered) {
		return s.filtered[s.cursor].ID
	}
	return ""
}

// DeleteSelected returns the session ID to delete, if any.
func (s *sessionList) DeleteSelected() string {
	id := s.Selected()
	if id == "" {
		return ""
	}
	// Remove from sessions slice.
	for i, sess := range s.sessions {
		if sess.ID == id {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			break
		}
	}
	if s.cursor >= len(s.filtered)-1 {
		s.cursor = max(0, len(s.filtered)-2)
	}
	s.applyFilter()
	return id
}

func (s *sessionList) applyFilter() {
	if s.filter == "" {
		s.filtered = make([]tui.SessionInfo, len(s.sessions))
		copy(s.filtered, s.sessions)
		return
	}
	needle := strings.ToLower(s.filter)
	s.filtered = s.filtered[:0]
	for _, sess := range s.sessions {
		if strings.Contains(strings.ToLower(sess.ShortID), needle) {
			s.filtered = append(s.filtered, sess)
		}
	}
}

// View renders the session browser in a box centred on (width, height).
func (s *sessionList) View() string {
	const boxWidth = 60
	const maxItems = 10

	w := min(boxWidth, s.width-4)

	// Title
	title := s.styles.BrowserTitle.Render("  Sessions")

	// Filter line
	filterContent := s.filter
	if s.editing {
		filterContent += "█" // cursor
	}
	filterLine := s.styles.BrowserFilter.Width(w - 4).Render("/ " + filterContent)

	// Separator — use w-4 to guarantee no overflow regardless of lipgloss v2 Width semantics.
	sep := strings.Repeat("─", w-4)

	// Items
	start := max(0, s.cursor-maxItems+1)
	end := min(len(s.filtered), start+maxItems)

	var rows []string
	for i := start; i < end; i++ {
		sess := s.filtered[i]
		age := formatAge(sess.UpdatedAt)
		info := fmt.Sprintf("%s · %s · %d turns", sess.ShortID, age, sess.Turns)
		if len(info) > w-4 {
			info = info[:w-4]
		}
		if i == s.cursor {
			rows = append(rows, s.styles.BrowserSelected.Width(w-2).Render("▶ "+info))
		} else {
			rows = append(rows, s.styles.BrowserItem.Width(w-2).Render("  "+info))
		}
	}

	if len(rows) == 0 {
		if s.filter != "" {
			rows = append(rows, s.styles.BrowserItem.Render("  no matches"))
		} else {
			rows = append(rows, s.styles.BrowserItem.Render("  no sessions yet"))
		}
	}

	// Footer hint
	hint := s.styles.Footer.Render("enter: open  d: delete  esc: close")

	content := strings.Join([]string{
		title,
		filterLine,
		s.styles.MsgTimestamp.Render(sep),
		strings.Join(rows, "\n"),
		s.styles.MsgTimestamp.Render(sep),
		hint,
	}, "\n")

	return s.styles.BrowserBorder.Width(w).Render(content)
}

// centred returns the box horizontally centred.
// Vertical centering is handled by overlayOn().
func (s *sessionList) centred() string {
	box := s.View()
	boxLines := strings.Split(box, "\n")
	boxW := lipgloss.Width(boxLines[0])
	leftPad := max(0, (s.width-boxW)/2)
	pad := strings.Repeat(" ", leftPad)
	var sb strings.Builder
	for i, line := range boxLines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pad + line)
	}
	return sb.String()
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}
