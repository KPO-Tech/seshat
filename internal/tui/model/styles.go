package model

import "charm.land/lipgloss/v2"

// Palette — orange / grey accent matching the Nexus logo.
// lipgloss/v2 Color returns an interface, so these must be var not const.
var (
	colorPrimary   = lipgloss.Color("#E8630A") // orange
	colorSecondary = lipgloss.Color("#FF8C42") // lighter orange
	colorMuted     = lipgloss.Color("#6B7280") // grey
	colorBorder    = lipgloss.Color("#374151") // dark border
	colorText      = lipgloss.Color("#F9FAFB") // near-white text
	colorGreen     = lipgloss.Color("#10B981")
	colorRed       = lipgloss.Color("#EF4444")
	colorYellow    = lipgloss.Color("#F59E0B")
	colorBlue      = lipgloss.Color("#3B82F6")
	colorUserMsg   = lipgloss.Color("#A5B4FC") // lavender for user messages
)

// Styles groups all lipgloss styles used by the TUI.
type Styles struct {
	// Header
	Logo        lipgloss.Style
	HeaderModel lipgloss.Style
	HeaderSep   lipgloss.Style
	HeaderID    lipgloss.Style
	HeaderBusy  lipgloss.Style
	HeaderReady lipgloss.Style

	// Chat
	UserLabel      lipgloss.Style
	AssistantLabel lipgloss.Style
	UserMsg        lipgloss.Style
	MsgTimestamp   lipgloss.Style
	ToolProgress   lipgloss.Style
	ToolDone       lipgloss.Style
	ToolError      lipgloss.Style
	ErrorMsg       lipgloss.Style

	// Input
	InputBorder      lipgloss.Style
	InputPrompt      lipgloss.Style
	InputPlaceholder lipgloss.Style

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
}

// DefaultStyles returns the theme used by the TUI.
func DefaultStyles() Styles {
	s := Styles{}

	// Header
	s.Logo = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary)
	s.HeaderModel = lipgloss.NewStyle().
		Foreground(colorMuted)
	s.HeaderSep = lipgloss.NewStyle().
		Foreground(colorBorder)
	s.HeaderID = lipgloss.NewStyle().
		Foreground(colorMuted).
		Faint(true)
	s.HeaderBusy = lipgloss.NewStyle().
		Foreground(colorYellow)
	s.HeaderReady = lipgloss.NewStyle().
		Foreground(colorGreen)

	// Chat
	s.UserLabel = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorUserMsg)
	s.AssistantLabel = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorSecondary)
	s.UserMsg = lipgloss.NewStyle().
		Foreground(colorText)
	s.MsgTimestamp = lipgloss.NewStyle().
		Foreground(colorMuted).
		Faint(true)
	s.ToolProgress = lipgloss.NewStyle().
		Foreground(colorYellow)
	s.ToolDone = lipgloss.NewStyle().
		Foreground(colorGreen)
	s.ToolError = lipgloss.NewStyle().
		Foreground(colorRed)
	s.ErrorMsg = lipgloss.NewStyle().
		Foreground(colorRed).
		Bold(true)

	// Input
	s.InputBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		PaddingLeft(1).PaddingRight(1)
	s.InputPrompt = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true)

	// Session browser
	s.BrowserBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		PaddingLeft(1).PaddingRight(1)
	s.BrowserTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary).
		Padding(0, 1)
	s.BrowserItem = lipgloss.NewStyle().
		Foreground(colorText).
		Padding(0, 1)
	s.BrowserSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary).
		Background(lipgloss.Color("#1F2937")).
		Padding(0, 1)
	s.BrowserFilter = lipgloss.NewStyle().
		Foreground(colorText).
		Padding(0, 1)

	// Permission dialog
	s.PermBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorYellow).
		PaddingLeft(2).PaddingRight(2).
		PaddingTop(1).PaddingBottom(1)
	s.PermTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorYellow)
	s.PermBody = lipgloss.NewStyle().
		Foreground(colorText)
	s.PermYes = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorGreen)
	s.PermNo = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorRed)
	s.PermAlways = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBlue)

	// Footer
	s.Footer = lipgloss.NewStyle().
		Foreground(colorMuted)
	s.Key = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true)
	s.Desc = lipgloss.NewStyle().
		Foreground(colorMuted)

	return s
}
