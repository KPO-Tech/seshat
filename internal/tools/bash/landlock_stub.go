//go:build !linux

package bash

func commandWithLandlock(shell string, args []string, workspaceRoot string) (string, []string, []string, bool) {
	return shell, args, nil, false
}

func landlockAvailable() bool {
	return false
}
