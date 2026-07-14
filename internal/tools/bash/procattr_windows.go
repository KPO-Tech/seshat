//go:build windows

package bash

import (
	"fmt"
	"os/exec"
	"syscall"
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
