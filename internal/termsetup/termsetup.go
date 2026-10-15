// Package termsetup makes shift+Enter and ctrl+Enter work in the terminal interface.
//
// A terminal sends the same byte for Enter, shift+Enter and ctrl+Enter, so the
// program cannot tell them apart; ctrl+j is the one newline key every terminal
// delivers. Terminals that support a keyboard protocol (kitty, WezTerm, Ghostty,
// recent Alacritty, iTerm2...) report the modifiers by themselves. Windows
// Terminal and the VS Code terminal do not, but they can be told to send the
// sequences the protocol uses for these two keys (ESC [ 13 ; 2 u and ESC [ 13 ; 5 u),
// which the interface already decodes. This package writes that setting.
//
// Both files it edits are JSON with comments, so they are patched as text: what
// the user wrote is kept as it is, a backup is made first, and nothing is written
// unless the result still parses.
package termsetup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// ShiftEnterSequence and CtrlEnterSequence are what the terminal is told to send.
	ShiftEnterSequence = "\x1b[13;2u"
	CtrlEnterSequence  = "\x1b[13;5u"

	// jsonShift and jsonCtrl are the same sequences as they are written in a JSON file.
	jsonShift = `\u001b[13;2u`
	jsonCtrl  = `\u001b[13;5u`

	backupSuffix = ".bak-before-seshat-keys"
)

// Status is what happened to one file.
type Status string

const (
	Added     Status = "added"     // the keys were written
	Already   Status = "already"   // the keys were already there
	Conflict  Status = "conflict"  // shift+enter is bound to something else: left alone
	WouldAdd  Status = "would-add" // dry run: the keys would be written
	NotFound  Status = "not-found" // no such terminal on this machine
	Failed    Status = "failed"
	Unchanged Status = "unchanged"
)

// Result describes one target file.
type Result struct {
	Target string // "Windows Terminal", "VS Code"
	Path   string
	Status Status
	Backup string // path of the backup, when one was made
	Detail string
}

// Options controls Run.
type Options struct {
	DryRun bool
	GOOS   string
	Getenv func(string) string
}

// Run sets up every supported terminal found on this machine.
func Run(opts Options) []Result {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	var results []Result
	for _, path := range windowsTerminalPaths(opts.GOOS, opts.Getenv) {
		results = append(results, patchFile("Windows Terminal", path, patchWindowsTerminal, opts))
	}
	for _, path := range vscodeKeybindingPaths(opts.GOOS, opts.Getenv) {
		results = append(results, patchFile("VS Code", path, patchVSCodeKeybindings, opts))
	}
	return results
}

func windowsTerminalPaths(goos string, getenv func(string) string) []string {
	if goos != "windows" {
		return nil
	}
	local := getenv("LOCALAPPDATA")
	if local == "" {
		return nil
	}
	return existing([]string{
		filepath.Join(local, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(local, "Packages", "Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(local, "Microsoft", "Windows Terminal", "settings.json"),
	})
}

func vscodeKeybindingPaths(goos string, getenv func(string) string) []string {
	var dirs []string
	switch goos {
	case "windows":
		if appData := getenv("APPDATA"); appData != "" {
			dirs = []string{filepath.Join(appData, "Code", "User"), filepath.Join(appData, "Code - Insiders", "User")}
		}
	case "darwin":
		if home := getenv("HOME"); home != "" {
			base := filepath.Join(home, "Library", "Application Support")
			dirs = []string{filepath.Join(base, "Code", "User"), filepath.Join(base, "Code - Insiders", "User")}
		}
	default:
		base := getenv("XDG_CONFIG_HOME")
		if base == "" && getenv("HOME") != "" {
			base = filepath.Join(getenv("HOME"), ".config")
		}
		if base != "" {
			dirs = []string{filepath.Join(base, "Code", "User"), filepath.Join(base, "Code - Insiders", "User")}
		}
	}
	var paths []string
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			paths = append(paths, filepath.Join(dir, "keybindings.json"))
		}
	}
	return paths
}

func existing(paths []string) []string {
	var out []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// patcher turns the text of a file into the patched text. changed is false when
// nothing needs to be written; the status then says why.
type patcher func(text string) (patched string, status Status, detail string, err error)

func patchFile(target, path string, patch patcher, opts Options) Result {
	res := Result{Target: target, Path: path}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		res.Status, res.Detail = Failed, err.Error()
		return res
	}
	text := string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	patched, status, detail, err := patch(text)
	if err != nil {
		res.Status, res.Detail = Failed, err.Error()
		return res
	}
	res.Status, res.Detail = status, detail
	if status != Added {
		return res
	}
	if opts.DryRun {
		res.Status = WouldAdd
		return res
	}
	if len(data) > 0 {
		backup := path + backupSuffix
		if _, statErr := os.Stat(backup); os.IsNotExist(statErr) {
			if err := os.WriteFile(backup, data, 0o600); err != nil {
				res.Status, res.Detail = Failed, "could not back the file up, nothing was changed: "+err.Error()
				return res
			}
		}
		res.Backup = backup
	}
	if err := writeAtomic(path, []byte(patched)); err != nil {
		res.Status, res.Detail = Failed, err.Error()
	}
	return res
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".seshat-termsetup-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// ── Windows Terminal ──────────────────────────────────────────────────────────

const (
	wtShiftID = "User.shiftEnter"
	wtCtrlID  = "User.ctrlEnter"
)

var (
	shiftEnterKey = regexp.MustCompile(`(?i)"keys"\s*:\s*"shift\+enter"`)
	ctrlEnterKey  = regexp.MustCompile(`(?i)"keys"\s*:\s*"ctrl\+enter"`)
)

func patchWindowsTerminal(text string) (string, Status, string, error) {
	code := maskComments(text) // a comment that mentions a key is not a binding
	if strings.Contains(code, jsonShift) || strings.Contains(code, wtShiftID) {
		return text, Already, "shift+enter is already set up", nil
	}
	if shiftEnterKey.MatchString(code) {
		return text, Conflict, "shift+enter is already bound to another action in this file; it was left alone", nil
	}
	if strings.TrimSpace(text) == "" {
		text = "{\n}\n"
	}

	actions := []string{
		`{ "command": { "action": "sendInput", "input": "` + jsonShift + `" }, "id": "` + wtShiftID + `" }`,
		`{ "command": { "action": "sendInput", "input": "` + jsonCtrl + `" }, "id": "` + wtCtrlID + `" }`,
	}
	keys := []string{`{ "id": "` + wtShiftID + `", "keys": "shift+enter" }`}
	// ctrl+enter is added only when the user has not bound it to something else.
	if !ctrlEnterKey.MatchString(code) {
		keys = append(keys, `{ "id": "`+wtCtrlID+`", "keys": "ctrl+enter" }`)
	} else {
		actions = actions[:1]
	}

	out, err := addToArray(text, "actions", actions)
	if err != nil {
		return "", Failed, "", err
	}
	out, err = addToArray(out, "keybindings", keys)
	if err != nil {
		return "", Failed, "", err
	}
	if err := mustParse(out); err != nil {
		return "", Failed, "", fmt.Errorf("the patched file would not be valid JSON, nothing was changed: %w", err)
	}
	return out, Added, "shift+enter and ctrl+enter now send a newline sequence", nil
}

// ── VS Code ───────────────────────────────────────────────────────────────────

func patchVSCodeKeybindings(text string) (string, Status, string, error) {
	if strings.Contains(maskComments(text), jsonShift) {
		return text, Already, "shift+enter is already set up", nil
	}
	if strings.TrimSpace(stripJSONC(text)) == "" {
		text = "[\n]\n"
	}
	entries := []string{
		`{ "key": "shift+enter", "command": "workbench.action.terminal.sendSequence", "args": { "text": "` + jsonShift + `" }, "when": "terminalFocus" }`,
		`{ "key": "ctrl+enter", "command": "workbench.action.terminal.sendSequence", "args": { "text": "` + jsonCtrl + `" }, "when": "terminalFocus" }`,
	}
	out, err := addToTopLevelArray(text, entries)
	if err != nil {
		return "", Failed, "", err
	}
	if err := mustParse(out); err != nil {
		return "", Failed, "", fmt.Errorf("the patched file would not be valid JSON, nothing was changed: %w", err)
	}
	return out, Added, "shift+enter and ctrl+enter now send a newline sequence in the integrated terminal", nil
}

// ── Text patching of JSON with comments ───────────────────────────────────────

// addToArray adds entries at the start of the top-level array "key" of an object,
// or creates the member when the object has none.
func addToArray(text, key string, entries []string) (string, error) {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*\[`)
	loc := re.FindStringIndex(maskComments(text))
	if loc != nil {
		return insertAfter(text, loc[1], entries), nil
	}
	return addMember(text, key, "[\n        "+strings.Join(entries, ",\n        ")+"\n    ]")
}

// addToTopLevelArray adds entries at the start of a file whose root is an array.
func addToTopLevelArray(text string, entries []string) (string, error) {
	masked := maskComments(text)
	open := strings.Index(masked, "[")
	if open < 0 {
		return "", fmt.Errorf("the file is not a JSON array")
	}
	return insertAfter(text, open+1, entries), nil
}

// insertAfter puts the entries right after position pos (just past an opening
// bracket), with a comma after them unless the array is empty.
func insertAfter(text string, pos int, entries []string) string {
	rest := text[pos:]
	trimmed := strings.TrimLeft(maskComments(rest), " \t\r\n")
	empty := strings.HasPrefix(trimmed, "]")
	body := "\n        " + strings.Join(entries, ",\n        ")
	if !empty {
		body += ","
	}
	return text[:pos] + body + text[pos:]
}

// addMember adds "key": value as the first member of the root object.
func addMember(text, key, value string) (string, error) {
	masked := maskComments(text)
	open := strings.Index(masked, "{")
	if open < 0 {
		return "", fmt.Errorf("the file is not a JSON object")
	}
	rest := strings.TrimLeft(masked[open+1:], " \t\r\n")
	member := "\n    \"" + key + "\": " + value
	if !strings.HasPrefix(rest, "}") {
		member += ","
	}
	return text[:open+1] + member + text[open+1:], nil
}

// maskComments returns text of the same length where the content of comments is
// replaced by spaces, so a bracket or a key inside a comment is not taken for
// structure. Strings are left as they are.
func maskComments(text string) string {
	b := []byte(text)
	out := make([]byte, len(b))
	copy(out, b)
	blank := func(i int) {
		if out[i] != '\n' && out[i] != '\r' {
			out[i] = ' '
		}
	}
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '"':
			j := i + 1
			for j < len(b) && b[j] != '"' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			i = j
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				blank(i)
				i++
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			for i < len(b) && !(b[i] == '*' && i+1 < len(b) && b[i+1] == '/') {
				blank(i)
				i++
			}
			if i < len(b) {
				blank(i)
				blank(i + 1)
				i++
			}
		}
	}
	return string(out)
}

// stripJSONC removes comments and trailing commas so encoding/json can read the
// file. Strings are left as they are.
func stripJSONC(text string) string {
	b := []byte(text)
	var out bytes.Buffer
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '"':
			j := i + 1
			for j < len(b) && b[j] != '"' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(b) {
				j = len(b) - 1
			}
			out.Write(b[i : j+1])
			i = j
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			i++
		default:
			out.WriteByte(b[i])
		}
	}
	return trailingComma.ReplaceAllString(out.String(), "$1")
}

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

func mustParse(text string) error {
	var v any
	return json.Unmarshal([]byte(stripJSONC(text)), &v)
}
