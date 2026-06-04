package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// sessionList is the session browser overlay.
type SessionList struct {
	styles   common.Styles
	sessions []tui.SessionInfo
	list     common.ListState[tui.SessionInfo]
	width    int
	height   int
	editing  bool // whether the filter input has focus
}

func NewSessionList(styles common.Styles) *SessionList {
	return &SessionList{
		styles: styles,
		list: common.NewListState(func(sess tui.SessionInfo, needle string) bool {
			return strings.Contains(strings.ToLower(sess.ShortID), needle)
		}),
		editing: true,
	}
}

func (s *SessionList) SetSessions(sessions []tui.SessionInfo) {
	s.sessions = sessions
	s.list.SetItems(sessions)
}

func (s *SessionList) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *SessionList) TypeFilter(ch string) { s.list.TypeFilter(ch) }
func (s *SessionList) DeleteFilter()        { s.list.DeleteFilter() }
func (s *SessionList) ClearFilter()         { s.list.ClearFilter() }
func (s *SessionList) Up()                  { s.list.Up() }
func (s *SessionList) Down()                { s.list.Down() }

// Selected returns the session ID at the current cursor position, or "".
func (s *SessionList) Selected() string {
	sess, ok := s.list.Selected()
	if !ok {
		return ""
	}
	return sess.ID
}

// DeleteSelected returns the session ID to delete, if any.
func (s *SessionList) DeleteSelected() string {
	id := s.Selected()
	if id == "" {
		return ""
	}

	for i, sess := range s.sessions {
		if sess.ID == id {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			break
		}
	}
	s.list.ResetItems(s.sessions, true)
	return id
}

// View renders the session browser in a box centred on (width, height).
func (s *SessionList) View() string {
	const boxWidth = 60
	const maxItems = 10

	w := min(boxWidth, s.width-4)
	filtered := s.list.FilteredItems()
	cursor := s.list.Cursor()

	// Title
	title := s.styles.BrowserTitle.Render("  Sessions")

	// Filter line
	filterContent := s.list.Filter()
	if s.editing {
		filterContent += "█" // cursor
	}
	filterLine := s.styles.BrowserFilter.Width(w - 4).Render("/ " + filterContent)

	// Separator — use w-4 to guarantee no overflow regardless of lipgloss v2 Width semantics.
	sep := strings.Repeat("─", w-4)

	// Items
	start := max(0, cursor-maxItems+1)
	end := min(len(filtered), start+maxItems)

	var rows []string
	for i := start; i < end; i++ {
		sess := filtered[i]
		age := formatAge(sess.UpdatedAt)
		info := fmt.Sprintf("%s · %s · %d turns", sess.ShortID, age, sess.Turns)
		if len(info) > w-4 {
			info = info[:w-4]
		}
		if i == cursor {
			rows = append(rows, s.styles.BrowserSelected.Width(w-2).Render("▶ "+info))
		} else {
			rows = append(rows, s.styles.BrowserItem.Width(w-2).Render("  "+info))
		}
	}

	if len(rows) == 0 {
		if s.list.Filter() != "" {
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
func (s *SessionList) Centered() string {
	return common.CenterHorizontally(s.View(), s.width)
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

func (s *SessionList) Size() (int, int) { return s.width, s.height }
