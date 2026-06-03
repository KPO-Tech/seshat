package shared

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// blockedDevicePaths is the list of device paths that must never be written to or edited.
var blockedDevicePaths = []string{
	"/dev/zero",
	"/dev/random",
	"/dev/urandom",
	"/dev/full",
	"/dev/stdin",
	"/dev/stdout",
	"/dev/stderr",
	"/dev/tty",
	"/dev/console",
	"/dev/null",
}

// IsBlockedDevicePath reports whether filePath is (or is under) a blocked device node.
func IsBlockedDevicePath(filePath string) bool {
	for _, device := range blockedDevicePaths {
		if strings.HasPrefix(filePath, device) || strings.HasPrefix(filePath, device+"/") {
			return true
		}
	}
	return false
}

// ValidateFilePath rejects device paths and path-traversal sequences.
// operation is a short label used in error messages (e.g. "writing", "editing").
func ValidateFilePath(filePath, operation string) error {
	if IsBlockedDevicePath(filePath) {
		return fmt.Errorf("%s device paths is not allowed: %s", operation, filePath)
	}
	if strings.Contains(filePath, "../") {
		return fmt.Errorf("paths with '..' are not allowed for security reasons")
	}
	return nil
}

// ValidateSensitivePath rejects edits to credential / settings files that would
// leave them in an unsafe state. toolName is used in error messages.
func ValidateSensitivePath(absolutePath, toolName, content string) error {
	if strings.HasSuffix(absolutePath, ".nexus/settings.local.json") ||
		strings.HasSuffix(absolutePath, ".nexus/settings.json") {
		if !json.Valid([]byte(content)) {
			return fmt.Errorf("settings file must remain valid JSON: %s", absolutePath)
		}
	}
	baseName := strings.ToLower(filepath.Base(absolutePath))
	if baseName == ".env" || strings.HasPrefix(baseName, ".env.") ||
		strings.Contains(strings.ToLower(absolutePath), "/.ssh/") {
		return fmt.Errorf("%s sensitive credential files is not allowed through %s: %s",
			toolName, toolName, absolutePath)
	}
	return nil
}

// GitDiff holds a minimal summary of git changes for a file.
type GitDiff struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch"`
}

// ComputeGitDiff computes a git diff summary for absolutePath.
// Returns (diff, true) when inside a git repo; (nil, false) otherwise.
func ComputeGitDiff(absolutePath string) (*GitDiff, bool) {
	repoRootBytes, err := exec.Command("git", "-C", filepath.Dir(absolutePath), "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, false
	}
	repoRoot := strings.TrimSpace(string(repoRootBytes))
	if repoRoot == "" {
		return nil, false
	}

	relPath, err := filepath.Rel(repoRoot, absolutePath)
	if err != nil {
		relPath = absolutePath
	}

	statusBytes, err := exec.Command("git", "-C", repoRoot, "status", "--short", "--", relPath).Output()
	if err != nil {
		return nil, false
	}
	statusText := strings.TrimSpace(string(statusBytes))
	status := "modified"
	if strings.HasPrefix(statusText, "??") || strings.HasPrefix(statusText, "A") {
		status = "added"
	}

	patchBytes, err := exec.Command("git", "-C", repoRoot, "diff", "--no-ext-diff", "--", relPath).Output()
	if err != nil {
		return nil, false
	}
	patch := string(patchBytes)

	additions, deletions := 0, 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	return &GitDiff{
		Filename:  filepath.Base(absolutePath),
		Status:    status,
		Additions: additions,
		Deletions: deletions,
		Changes:   additions + deletions,
		Patch:     patch,
	}, true
}
