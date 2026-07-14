//go:build windows

package tasks

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// newProcessGroupAttr starts a command in a new process group
// (CREATE_NEW_PROCESS_GROUP), so killProcessGroup can later terminate the
// whole tree via taskkill.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessGroup terminates pid and its full descendant tree. Windows has
// no direct equivalent to POSIX process-group signaling, so this shells out
// to taskkill /T, the standard way to reliably kill a process tree there.
func killProcessGroup(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
}

// stillActive is Win32's STILL_ACTIVE constant (259) — the exit code
// GetExitCodeProcess reports while a process hasn't exited yet. Not exposed
// by golang.org/x/sys/windows, but a stable, documented Win32 API value.
const stillActive = 259

// processExists reports whether pid refers to a live process. Windows has
// no signal-0 equivalent, so this opens a handle and checks the exit code.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
