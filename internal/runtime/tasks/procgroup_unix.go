//go:build !windows

package tasks

import (
	"errors"
	"syscall"
)

// newProcessGroupAttr starts a command in its own process group, so
// killProcessGroup can later terminate the whole tree rather than just the
// immediate child.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the entire process group led by pid.
func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// processExists reports whether pid refers to a live process, using the
// POSIX idiom of sending signal 0 (no-op, but still validated by the
// kernel): ESRCH means gone, EPERM means it exists but we can't signal it.
func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
