//go:build !windows

package titlemodel

import "os/exec"

func hideWindow(*exec.Cmd) {}
