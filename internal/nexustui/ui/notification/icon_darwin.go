//go:build darwin

package notification

import _ "embed"

// Icon is the PNG data for the Nexus icon, used for OSC 99 notifications.
// Native macOS notifications don't support custom icons via beeep, but OSC 99 does.
//
//go:embed nexus-icon.png
var Icon []byte
