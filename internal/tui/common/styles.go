package common

import (
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Palette — orange / grey accent matching the Nexus logo.
// lipgloss/v2 Color returns an interface, so these must be var not const.
var (
	ColorPrimary   = lipgloss.Color("#E8630A") // orange
	ColorSecondary = lipgloss.Color("#FF8C42") // lighter orange
	ColorMuted     = lipgloss.Color("#6B7280") // grey
	ColorBorder    = lipgloss.Color("#374151") // dark border
	ColorText      = lipgloss.Color("#F9FAFB") // near-white text
	ColorGreen     = lipgloss.Color("#10B981")
	ColorRed       = lipgloss.Color("#EF4444")
	ColorYellow    = lipgloss.Color("#F59E0B")
	ColorBlue      = lipgloss.Color("#3B82F6")
	ColorUserMsg   = lipgloss.Color("#A5B4FC") // lavender for user messages
)

// Styles groups all lipgloss styles used by the TUI.
type Styles struct {
	// Header
	Logo             lipgloss.Style
	HeaderBar        lipgloss.Style
	HeaderModel      lipgloss.Style
	HeaderSep        lipgloss.Style
	HeaderID         lipgloss.Style
	HeaderBusy       lipgloss.Style
	HeaderReady      lipgloss.Style
	HeaderPill       lipgloss.Style
	HeaderPillActive lipgloss.Style
	HeaderPillBusy   lipgloss.Style
	HeaderPillReady  lipgloss.Style

	// Chat
	UserLabel        lipgloss.Style
	AssistantLabel   lipgloss.Style
	UserMarker       lipgloss.Style
	AssistantMarker  lipgloss.Style
	TurnMeta         lipgloss.Style
	UserMsg          lipgloss.Style
	InterimAssistant lipgloss.Style
	MsgTimestamp     lipgloss.Style
	ToolProgress     lipgloss.Style
	ToolDone         lipgloss.Style
	ToolError        lipgloss.Style
	ErrorMsg         lipgloss.Style
	Selection        lipgloss.Style

	// Input
	InputBorder      lipgloss.Style
	InputPrompt      lipgloss.Style
	InputPlaceholder lipgloss.Style
	InputHint        lipgloss.Style
	InputBadge       lipgloss.Style
	Textarea         textarea.Styles

	// Session browser
	BrowserBorder   lipgloss.Style
	BrowserTitle    lipgloss.Style
	BrowserItem     lipgloss.Style
	BrowserSelected lipgloss.Style
	BrowserFilter   lipgloss.Style

	// Permission dialog
	PermBorder lipgloss.Style
	PermTitle  lipgloss.Style
	PermBody   lipgloss.Style
	PermYes    lipgloss.Style
	PermNo     lipgloss.Style
	PermAlways lipgloss.Style

	// Footer
	Footer lipgloss.Style
	Key    lipgloss.Style
	Desc   lipgloss.Style

	// Tool inline rendering
	ToolLineNumber lipgloss.Style
	ToolTruncation lipgloss.Style
	ToolDiffAdd    lipgloss.Style
	ToolDiffDel    lipgloss.Style
	ToolDiffHunk   lipgloss.Style
}

// DefaultStyles returns the theme used by the TUI.
func DefaultStyles() Styles {
	s := Styles{}

	// Header
	s.Logo = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)
	s.HeaderBar = lipgloss.NewStyle().
		Padding(0, 1)
	s.HeaderModel = lipgloss.NewStyle().
		Foreground(ColorMuted)
	s.HeaderSep = lipgloss.NewStyle().
		Foreground(ColorBorder)
	s.HeaderID = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Faint(true)
	s.HeaderBusy = lipgloss.NewStyle().
		Foreground(ColorYellow)
	s.HeaderReady = lipgloss.NewStyle().
		Foreground(ColorGreen)
	s.HeaderPill = lipgloss.NewStyle().
		Foreground(ColorText).
		Background(lipgloss.Color("#151A21")).
		Padding(0, 1)
	s.HeaderPillActive = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#151A21")).
		Bold(true).
		Padding(0, 1)
	s.HeaderPillBusy = lipgloss.NewStyle().
		Foreground(ColorYellow).
		Background(lipgloss.Color("#151A21")).
		Bold(true).
		Padding(0, 1)
	s.HeaderPillReady = lipgloss.NewStyle().
		Foreground(ColorGreen).
		Background(lipgloss.Color("#151A21")).
		Bold(true).
		Padding(0, 1)

	// Chat
	s.UserLabel = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorUserMsg)
	s.AssistantLabel = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary)
	s.UserMarker = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBlue)
	s.AssistantMarker = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)
	s.TurnMeta = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Faint(true)
	s.UserMsg = lipgloss.NewStyle().
		Foreground(ColorText)
	s.InterimAssistant = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#B2BCCB")).
		Faint(true)
	s.MsgTimestamp = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Faint(true)
	s.ToolProgress = lipgloss.NewStyle().
		Foreground(ColorYellow)
	s.ToolDone = lipgloss.NewStyle().
		Foreground(ColorGreen)
	s.ToolError = lipgloss.NewStyle().
		Foreground(ColorRed)
	s.ErrorMsg = lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true)
	s.Selection = lipgloss.NewStyle().
		Background(lipgloss.Color("#5A3418"))

	// Input
	s.InputBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)
	s.InputPrompt = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	s.InputPlaceholder = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Faint(true)
	s.InputHint = lipgloss.NewStyle().
		Foreground(ColorMuted)
	s.InputBadge = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	s.Textarea = textarea.Styles{
		Focused: textarea.StyleState{
			Base:             lipgloss.NewStyle().Foreground(ColorText),
			Text:             lipgloss.NewStyle().Foreground(ColorText),
			LineNumber:       lipgloss.NewStyle().Foreground(ColorMuted),
			CursorLine:       lipgloss.NewStyle().Foreground(ColorText),
			CursorLineNumber: lipgloss.NewStyle().Foreground(ColorMuted),
			Placeholder:      lipgloss.NewStyle().Foreground(ColorMuted).Faint(true),
			Prompt:           lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true),
		},
		Blurred: textarea.StyleState{
			Base:             lipgloss.NewStyle().Foreground(ColorMuted),
			Text:             lipgloss.NewStyle().Foreground(ColorMuted),
			LineNumber:       lipgloss.NewStyle().Foreground(ColorMuted),
			CursorLine:       lipgloss.NewStyle().Foreground(ColorMuted),
			CursorLineNumber: lipgloss.NewStyle().Foreground(ColorMuted),
			Placeholder:      lipgloss.NewStyle().Foreground(ColorMuted).Faint(true),
			Prompt:           lipgloss.NewStyle().Foreground(ColorMuted),
		},
		Cursor: textarea.CursorStyle{
			Color:      ColorSecondary,
			Shape:      tea.CursorBar,
			Blink:      true,
			BlinkSpeed: 420 * time.Millisecond,
		},
	}

	// Session browser
	s.BrowserBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		PaddingLeft(1).PaddingRight(1)
	s.BrowserTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Padding(0, 1)
	s.BrowserItem = lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 1)
	s.BrowserSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1F2937")).
		Padding(0, 1)
	s.BrowserFilter = lipgloss.NewStyle().
		Foreground(ColorText).
		Padding(0, 1)

	// Permission dialog
	s.PermBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorYellow).
		PaddingLeft(2).PaddingRight(2).
		PaddingTop(1).PaddingBottom(1)
	s.PermTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorYellow)
	s.PermBody = lipgloss.NewStyle().
		Foreground(ColorText)
	s.PermYes = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorGreen)
	s.PermNo = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorRed)
	s.PermAlways = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBlue)

	// Footer
	s.Footer = lipgloss.NewStyle().
		Foreground(ColorMuted)
	s.Key = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	s.Desc = lipgloss.NewStyle().
		Foreground(ColorMuted)

	// Tool inline rendering
	s.ToolLineNumber = lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)
	s.ToolTruncation = lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)
	s.ToolDiffAdd = lipgloss.NewStyle().Foreground(ColorGreen)
	s.ToolDiffDel = lipgloss.NewStyle().Foreground(ColorRed)
	s.ToolDiffHunk = lipgloss.NewStyle().Foreground(ColorBlue).Faint(true)

	return s
}
