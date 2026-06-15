package runtimepath

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultConfigDir returns the platform-appropriate config directory for the
// given application name.
//
//   - Linux / BSD: $XDG_CONFIG_HOME/<appName>  or  ~/.config/<appName>
//   - macOS:       ~/.config/<appName>  (XDG convention — familiar for CLI tools,
//     avoids the space in "Library/Application Support")
//   - Windows:     %APPDATA%\<appName>  (e.g. C:\Users\<user>\AppData\Roaming\<appName>)
//
// NEXUS_RUNTIME_ROOT overrides everything — callers should check that first.
func DefaultConfigDir(appName string) string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, appName)
		}
		// Fallback: construct from USERPROFILE
		if profile := os.Getenv("USERPROFILE"); profile != "" {
			return filepath.Join(profile, "AppData", "Roaming", appName)
		}
	default: // linux, darwin, freebsd, …
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, appName)
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".config", appName)
		}
		// Last resort: try $HOME directly
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, ".config", appName)
		}
	}
	return filepath.Join(os.TempDir(), appName)
}
