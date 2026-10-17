//go:build windows

package titlemodel

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console window from opening for the server.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
