//go:build !windows

package bash

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// newProcessGroupAttr starts a command in its own session/process group, so
// killProcessGroup can later terminate the whole tree (the shell plus any
// children it spawned) rather than just the shell itself.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// killProcessGroup sends SIGKILL to the entire process group led by pid.
func killProcessGroup(pid int) error {
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		return err
	}
	return unix.Kill(-pgid, unix.SIGKILL)
}
