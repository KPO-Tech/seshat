package dialog

// settingsCardMaxWidth and settingsCardMaxHeight are the shared outer bounds
// for all settings-family dialogs (Settings, Commands, Models, Notifications,
// Reasoning). Every dialog in this family uses the same footprint so that
// navigating between them never reveals the background behind a narrower card.
const (
	settingsCardMaxWidth  = 86
	settingsCardMaxHeight = 32
)

// settingsCardSelectedBg is the soft dark-orange used for selected rows in
// all settings-family dialogs. It is warmer and less harsh than the default
// bright orange, making highlighted text readable without straining the eyes.
const settingsCardSelectedBg = "#3D1E00"
