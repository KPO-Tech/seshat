package shared

import (
	"fmt"
	"runtime"
)

// RipgrepNotFoundError returns a clear, OS-specific error when ripgrep is missing.
func RipgrepNotFoundError() error {
	var hint string
	switch runtime.GOOS {
	case "darwin":
		hint = "brew install ripgrep"
	case "linux":
		hint = "sudo apt install ripgrep  # Debian/Ubuntu\n  sudo dnf install ripgrep  # Fedora\n  sudo pacman -S ripgrep    # Arch"
	case "windows":
		hint = "winget install BurntSushi.ripgrep.MSVC"
	default:
		hint = "see https://github.com/BurntSushi/ripgrep#installation"
	}
	return fmt.Errorf("ripgrep (rg) not found — run: %s\n  Or run: make install-deps", hint)
}
