package bash

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	shellOnce     sync.Once
	detectedShell string
)

// DetectShell picks the shell the tool runs commands in, once per process.
//
// On Linux and macOS that is the first of bash, sh and zsh on the PATH. On Windows
// a bare "bash" on the PATH is usually the WSL launcher (System32 or WindowsApps),
// which fails with "execvpe(/bin/bash) failed" when no Linux distribution is
// installed, so Git Bash is looked for first and the WSL launcher is never taken;
// without Git Bash the tool falls back on PowerShell. SESHAT_BASH_SHELL overrides
// all of it with a path or a name (a .env file may not set it: it decides what
// is run, see pkg/config/dotenv_guard.go).
func DetectShell() string {
	shellOnce.Do(func() {
		detectedShell = detectShell(runtime.GOOS, os.Getenv, exec.LookPath, fileExists)
	})
	return detectedShell
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// detectShell is DetectShell with its environment passed in, so each case can be
// tested without the host it runs on.
func detectShell(goos string, getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) string {
	if override := strings.TrimSpace(getenv("SESHAT_BASH_SHELL")); override != "" {
		if path, err := lookPath(override); err == nil {
			return path
		}
		if exists(override) {
			return override
		}
	}

	if goos != "windows" {
		for _, sh := range []string{"bash", "sh", "zsh"} {
			if _, err := lookPath(sh); err == nil {
				return sh
			}
		}
		return "sh"
	}

	if path := windowsGitBash(getenv, lookPath, exists); path != "" {
		return path
	}
	if path, err := lookPath("bash"); err == nil && !isWSLLauncher(path) {
		return path
	}
	for _, name := range []string{"pwsh", "powershell"} {
		if path, err := lookPath(name); err == nil {
			return path
		}
	}
	return "sh"
}

// windowsGitBash finds Git for Windows' bash: next to the git on the PATH, then
// in the usual install folders.
func windowsGitBash(getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) string {
	var candidates []string
	if gitPath, err := lookPath("git"); err == nil {
		dir := filepath.Dir(gitPath) // ...\Git\cmd or ...\Git\mingw64\bin
		candidates = append(candidates,
			filepath.Join(dir, "..", "bin", "bash.exe"),
			filepath.Join(dir, "..", "..", "bin", "bash.exe"),
		)
	}
	for _, root := range []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)")} {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "Git", "bin", "bash.exe"))
		}
	}
	if local := getenv("LocalAppData"); local != "" {
		candidates = append(candidates, filepath.Join(local, "Programs", "Git", "bin", "bash.exe"))
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

// isWSLLauncher reports whether a bash.exe is the Windows Subsystem for Linux
// launcher rather than a real bash.
func isWSLLauncher(path string) bool {
	lower := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	return strings.Contains(lower, "/windows/system32/") || strings.Contains(lower, "/windowsapps/")
}

// isPowerShell reports whether a shell path is PowerShell, which takes its
// command after -Command instead of -c.
func isPowerShell(shell string) bool {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(shell, "\\", "/")))
	base = strings.TrimSuffix(base, ".exe")
	return base == "pwsh" || base == "powershell"
}

// shellArgs returns the arguments that make shell run command.
func shellArgs(shell, command string) []string {
	if isPowerShell(shell) {
		return []string{"-NoProfile", "-NonInteractive", "-Command", command}
	}
	return []string{"-c", command}
}

// shellNote tells the model which shell the commands run in when that is not
// the bash or zsh the tool description assumes.
func shellNote(goos, shell string) string {
	switch {
	case isPowerShell(shell):
		return "\n\n## Shell on this host\n\nThis host is Windows and commands run in PowerShell, not bash. Use PowerShell syntax and commands (Get-ChildItem, Get-Content, Select-String, $env:NAME, `;` to chain commands), not Unix ones (ls -la, grep, cat, export). Prefer the dedicated tools for reading, searching and editing files."
	case goos == "windows":
		return "\n\n## Shell on this host\n\nThis host is Windows and commands run in Git Bash: POSIX commands (ls, grep, cat) and syntax work. Write paths with forward slashes (C:/Users/name/project or /c/Users/name/project), or quote them when they contain backslashes."
	default:
		return ""
	}
}
