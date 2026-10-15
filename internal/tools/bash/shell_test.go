package bash

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHost describes a machine: the programs the PATH finds and the files that exist.
type fakeHost struct {
	path  map[string]string // name -> full path found on the PATH
	files map[string]bool
	env   map[string]string
}

func (h fakeHost) detect(goos string) string {
	return detectShell(goos,
		func(k string) string { return h.env[k] },
		func(name string) (string, error) {
			if p, ok := h.path[name]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		func(p string) bool { return h.files[p] })
}

func TestDetectShellNeverTakesTheWSLLauncherOnWindows(t *testing.T) {
	gitBash := filepath.Clean(filepath.Join("C:", "Program Files", "Git", "cmd", "..", "bin", "bash.exe"))
	host := fakeHost{
		path: map[string]string{
			"bash": `C:\Windows\System32\bash.exe`, // WSL launcher: fails with no distribution
			"git":  filepath.Join("C:", "Program Files", "Git", "cmd", "git.exe"),
		},
		files: map[string]bool{gitBash: true},
	}
	if got := host.detect("windows"); got != gitBash {
		t.Fatalf("shell = %q, want Git Bash %q", got, gitBash)
	}
}

func TestDetectShellWindowsFallbacks(t *testing.T) {
	// Git Bash in the usual folder, git not on the PATH.
	pf := filepath.Join("C:", "Program Files")
	gitBash := filepath.Join(pf, "Git", "bin", "bash.exe")
	host := fakeHost{env: map[string]string{"ProgramFiles": pf}, files: map[string]bool{gitBash: true}}
	if got := host.detect("windows"); got != gitBash {
		t.Fatalf("usual folder: shell = %q, want %q", got, gitBash)
	}

	// No Git Bash: a bash that is not the WSL launcher is fine, the launcher is not.
	other := `D:\tools\msys64\usr\bin\bash.exe`
	if got := (fakeHost{path: map[string]string{"bash": other}}).detect("windows"); got != other {
		t.Fatalf("a real bash on the PATH must be used, got %q", got)
	}
	for _, launcher := range []string{`C:\Windows\System32\bash.exe`, `C:\Users\me\AppData\Local\Microsoft\WindowsApps\bash.exe`} {
		got := (fakeHost{path: map[string]string{"bash": launcher, "powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`}}).detect("windows")
		if !isPowerShell(got) {
			t.Fatalf("with only the WSL launcher %q the tool must fall back on PowerShell, got %q", launcher, got)
		}
	}

	// pwsh is preferred to Windows PowerShell; with nothing at all there is still "sh".
	both := fakeHost{path: map[string]string{"pwsh": `C:\Program Files\PowerShell\7\pwsh.exe`, "powershell": `C:\powershell.exe`}}
	if got := both.detect("windows"); !strings.Contains(got, "pwsh") {
		t.Fatalf("pwsh must come first, got %q", got)
	}
	if got := (fakeHost{}).detect("windows"); got != "sh" {
		t.Fatalf("nothing found: shell = %q, want sh", got)
	}
}

func TestDetectShellUnixKeepsItsOrderAndHonorsTheOverride(t *testing.T) {
	if got := (fakeHost{path: map[string]string{"bash": "/bin/bash", "sh": "/bin/sh"}}).detect("linux"); got != "bash" {
		t.Fatalf("shell = %q, want bash", got)
	}
	if got := (fakeHost{path: map[string]string{"zsh": "/bin/zsh"}}).detect("darwin"); got != "zsh" {
		t.Fatalf("shell = %q, want zsh", got)
	}
	override := fakeHost{
		env:   map[string]string{"SESHAT_BASH_SHELL": "fish"},
		path:  map[string]string{"bash": "/bin/bash", "fish": "/usr/bin/fish"},
		files: map[string]bool{},
	}
	if got := override.detect("linux"); got != "/usr/bin/fish" {
		t.Fatalf("SESHAT_BASH_SHELL ignored, got %q", got)
	}
	ignored := fakeHost{env: map[string]string{"SESHAT_BASH_SHELL": "nope"}, path: map[string]string{"bash": "/bin/bash"}}
	if got := ignored.detect("linux"); got != "bash" {
		t.Fatalf("an override that does not exist must be ignored, got %q", got)
	}
}

func TestShellArgsAndNote(t *testing.T) {
	ps := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if got := shellArgs(ps, "Get-ChildItem"); len(got) != 4 || got[2] != "-Command" || got[3] != "Get-ChildItem" {
		t.Fatalf("PowerShell args = %v", got)
	}
	if got := shellArgs("/bin/bash", "ls -la"); len(got) != 2 || got[0] != "-c" || got[1] != "ls -la" {
		t.Fatalf("bash args = %v", got)
	}
	if !isPowerShell("pwsh") || !isPowerShell(`C:\Program Files\PowerShell\7\pwsh.exe`) || isPowerShell("/bin/bash") {
		t.Fatal("isPowerShell misjudged a shell")
	}

	if note := shellNote("windows", ps); !strings.Contains(note, "PowerShell") || !strings.Contains(note, "Get-ChildItem") {
		t.Fatalf("PowerShell note = %q", note)
	}
	if note := shellNote("windows", `C:\Program Files\Git\bin\bash.exe`); !strings.Contains(note, "Git Bash") {
		t.Fatalf("Git Bash note = %q", note)
	}
	if note := shellNote("linux", "bash"); note != "" {
		t.Fatalf("Linux needs no note, got %q", note)
	}
}
