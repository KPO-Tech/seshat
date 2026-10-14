package prompt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// gitCommandTimeout bounds each git call so a slow or hung repository cannot
// stall a turn.
const gitCommandTimeout = 2 * time.Second

// safeBranchName is the only shape of branch name that reaches the prompt. A
// branch name comes from the repository, which the user may not trust, and the
// runtime context is part of the system prompt: anything else is dropped rather
// than quoted.
var safeBranchName = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,80}$`)

// gitContextBlock describes the working tree in one short block: the branch
// and how many files are modified, staged or untracked. It names no file, for
// the same reason it filters the branch name: a file name is repository
// content, and the agent can list the files with one command when it needs them.
// It returns "" outside a git repository or when git is unavailable.
func gitContextBlock(workingDirectory string) string {
	if strings.TrimSpace(workingDirectory) == "" {
		return ""
	}
	branch, ok := gitOutput(workingDirectory, "rev-parse", "--abbrev-ref", "HEAD")
	if !ok {
		return ""
	}
	branch = strings.TrimSpace(branch)
	if branch == "HEAD" {
		branch = "(detached HEAD)"
	} else if !safeBranchName.MatchString(branch) {
		branch = "(unnamed)"
	}

	status, ok := gitOutput(workingDirectory, "status", "--porcelain")
	if !ok {
		return fmt.Sprintf("# Git\n\nbranch: %s", branch)
	}
	changed, untracked := 0, 0
	for _, line := range strings.Split(status, "\n") {
		switch {
		case strings.HasPrefix(line, "??"):
			untracked++
		case strings.TrimSpace(line) != "":
			changed++
		}
	}
	state := "clean"
	if changed+untracked > 0 {
		state = fmt.Sprintf("%d modified or staged, %d untracked", changed, untracked)
	}
	return fmt.Sprintf("# Git\n\nbranch: %s\nworking tree: %s", branch, state)
}

func gitOutput(dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}
