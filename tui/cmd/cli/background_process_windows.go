//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func newBackgroundSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000200}
}

func isProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").CombinedOutput()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), strconv.Itoa(pid)), nil
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
