package common

import "charm.land/bubbles/v2/key"

// KeyMap groups all key bindings for the TUI.
type KeyMap struct {
	// Input / editor
	SendMessage key.Binding
	Newline     key.Binding

	// Chat navigation
	ScrollUp   key.Binding
	ScrollDown key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	ScrollHome key.Binding
	ScrollEnd  key.Binding

	// Global
	NewSession key.Binding
	Sessions   key.Binding
	Cancel     key.Binding
	Quit       key.Binding
	Help       key.Binding

	// Session browser
	BrowseUp    key.Binding
	BrowseDown  key.Binding
	BrowseOpen  key.Binding
	BrowseClose key.Binding
	BrowseDel   key.Binding

	// Permission dialog
	PermYes    key.Binding
	PermNo     key.Binding
	PermAlways key.Binding
	PermClose  key.Binding
}

// DefaultKeys returns the default key map.
func DefaultKeys() KeyMap {
	return KeyMap{
		SendMessage: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send"),
		),
		Newline: key.NewBinding(
			key.WithKeys("shift+enter", "alt+enter"),
			key.WithHelp("shift+enter", "newline"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		ScrollHome: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		ScrollEnd: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
		NewSession: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "new session"),
		),
		Sessions: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "sessions"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "cancel/quit"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+q"),
			key.WithHelp("ctrl+q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "help"),
		),
		BrowseUp: key.NewBinding(
			key.WithKeys("up", "k"),
		),
		BrowseDown: key.NewBinding(
			key.WithKeys("down", "j"),
		),
		BrowseOpen: key.NewBinding(
			key.WithKeys("enter"),
		),
		BrowseClose: key.NewBinding(
			key.WithKeys("esc", "ctrl+s"),
		),
		BrowseDel: key.NewBinding(
			key.WithKeys("d", "delete"),
		),
		PermYes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "yes"),
		),
		PermNo: key.NewBinding(
			key.WithKeys("n", "esc"),
			key.WithHelp("n/esc", "no"),
		),
		PermAlways: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "always"),
		),
		PermClose: key.NewBinding(
			key.WithKeys("esc"),
		),
	}
}
