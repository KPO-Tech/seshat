package termsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// settings is the shape of a real Windows Terminal settings.json.
const settings = `{
    "$help": "https://aka.ms/terminal-documentation",
    "actions": [],
    "defaultProfile": "{61c54bbd}",
    "keybindings":
    [
        { "id": "Terminal.CopyToClipboard", "keys": "ctrl+c" },
        { "id": "Terminal.FindText", "keys": "ctrl+shift+f" }
    ],
    "profiles": { "list": [ { "name": "PowerShell" } ] }
}
`

func parse(t *testing.T, text string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(stripJSONC(text)), &v); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, text)
	}
	return v
}

func TestWindowsTerminalAddsBothKeysAndKeepsTheRest(t *testing.T) {
	out, status, _, err := patchWindowsTerminal(settings)
	if err != nil || status != Added {
		t.Fatalf("status = %s err = %v", status, err)
	}
	v := parse(t, out)

	actions := v["actions"].([]any)
	if len(actions) != 2 {
		t.Fatalf("actions = %v", actions)
	}
	first := actions[0].(map[string]any)
	if first["id"] != "User.shiftEnter" || first["command"].(map[string]any)["input"] != ShiftEnterSequence {
		t.Fatalf("first action = %v", first)
	}
	var shiftBound, ctrlBound, copyKept bool
	for _, k := range v["keybindings"].([]any) {
		m := k.(map[string]any)
		switch m["keys"] {
		case "shift+enter":
			shiftBound = m["id"] == "User.shiftEnter"
		case "ctrl+enter":
			ctrlBound = m["id"] == "User.ctrlEnter"
		case "ctrl+c":
			copyKept = true
		}
	}
	if !shiftBound || !ctrlBound || !copyKept {
		t.Fatalf("keybindings = %v", v["keybindings"])
	}
	if v["defaultProfile"] != "{61c54bbd}" || v["$help"] == nil {
		t.Fatal("the rest of the file must be kept")
	}
	if !strings.Contains(out, `"profiles": { "list": [ { "name": "PowerShell" } ] }`) {
		t.Fatal("the text outside the two arrays must be untouched")
	}
}

func TestWindowsTerminalIsIdempotent(t *testing.T) {
	once, _, _, _ := patchWindowsTerminal(settings)
	twice, status, _, err := patchWindowsTerminal(once)
	if err != nil || status != Already || twice != once {
		t.Fatalf("second run: status = %s err = %v changed = %v", status, err, twice != once)
	}
}

func TestWindowsTerminalLeavesAnotherShiftEnterBindingAlone(t *testing.T) {
	custom := strings.Replace(settings, `"keys": "ctrl+shift+f"`, `"keys": "shift+enter"`, 1)
	out, status, _, err := patchWindowsTerminal(custom)
	if err != nil || status != Conflict || out != custom {
		t.Fatalf("status = %s err = %v changed = %v", status, err, out != custom)
	}
}

func TestWindowsTerminalKeepsAnExistingCtrlEnterBinding(t *testing.T) {
	custom := strings.Replace(settings, `"keys": "ctrl+shift+f"`, `"keys": "ctrl+enter"`, 1)
	out, status, _, err := patchWindowsTerminal(custom)
	if err != nil || status != Added {
		t.Fatalf("status = %s err = %v", status, err)
	}
	if strings.Count(out, `"keys": "ctrl+enter"`) != 1 || strings.Contains(out, "User.ctrlEnter") {
		t.Fatal("the user's own ctrl+enter binding must not be duplicated or replaced")
	}
	parse(t, out)
}

func TestWindowsTerminalToleratesCommentsAndMissingKeys(t *testing.T) {
	withComments := `{
    // my settings
    "defaultProfile": "x", /* inline */
    "actions": [ // none yet
    ],
    "keybindings": [ /* "keys": "shift+enter" is only in this comment */ ],
}
`
	out, status, _, err := patchWindowsTerminal(withComments)
	if err != nil || status != Added {
		t.Fatalf("status = %s err = %v", status, err)
	}
	v := parse(t, out)
	if len(v["actions"].([]any)) != 2 || len(v["keybindings"].([]any)) != 2 {
		t.Fatalf("result = %v", v)
	}
	if !strings.Contains(out, "// my settings") || !strings.Contains(out, "/* inline */") {
		t.Fatal("the user's comments must be kept")
	}

	bare := `{ "defaultProfile": "x" }`
	out, status, _, err = patchWindowsTerminal(bare)
	if err != nil || status != Added {
		t.Fatalf("bare: status = %s err = %v", status, err)
	}
	v = parse(t, out)
	if len(v["actions"].([]any)) != 2 || len(v["keybindings"].([]any)) != 2 || v["defaultProfile"] != "x" {
		t.Fatalf("bare result = %v", v)
	}
}

func TestVSCodeKeybindings(t *testing.T) {
	for name, text := range map[string]string{
		"missing file":   "",
		"empty array":    "[]",
		"with a comment": "// Place your key bindings here\n[\n  { \"key\": \"ctrl+k\", \"command\": \"x\" } // mine\n]\n",
	} {
		out, status, _, err := patchVSCodeKeybindings(text)
		if err != nil || status != Added {
			t.Fatalf("%s: status = %s err = %v", name, status, err)
		}
		var list []map[string]any
		if err := json.Unmarshal([]byte(stripJSONC(out)), &list); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		var found bool
		for _, e := range list {
			if e["key"] == "shift+enter" && e["when"] == "terminalFocus" && e["args"].(map[string]any)["text"] == ShiftEnterSequence {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: shift+enter missing in %v", name, list)
		}
		if name == "with a comment" && !strings.Contains(out, "// mine") {
			t.Fatal("comments must be kept")
		}
		again, status, _, _ := patchVSCodeKeybindings(out)
		if status != Already || again != out {
			t.Fatalf("%s: not idempotent (%s)", name, status)
		}
	}
}

func TestRunWritesABackupOnceAndDryRunWritesNothing(t *testing.T) {
	local := t.TempDir()
	dir := filepath.Join(local, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf"+settings), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{GOOS: "windows", Getenv: func(k string) string {
		if k == "LOCALAPPDATA" {
			return local
		}
		return ""
	}}

	dry := opts
	dry.DryRun = true
	res := Run(dry)
	if len(res) != 1 || res[0].Status != WouldAdd {
		t.Fatalf("dry run = %+v", res)
	}
	if data, _ := os.ReadFile(path); string(data) != "\xef\xbb\xbf"+settings {
		t.Fatal("a dry run must not write")
	}

	res = Run(opts)
	if len(res) != 1 || res[0].Status != Added || res[0].Backup == "" {
		t.Fatalf("run = %+v", res)
	}
	if backup, _ := os.ReadFile(res[0].Backup); string(backup) != "\xef\xbb\xbf"+settings {
		t.Fatal("the backup must hold the original file")
	}
	patched, _ := os.ReadFile(path)
	parse(t, string(patched))

	res = Run(opts)
	if res[0].Status != Already {
		t.Fatalf("second run = %+v", res)
	}
	if backup, _ := os.ReadFile(path + backupSuffix); string(backup) != "\xef\xbb\xbf"+settings {
		t.Fatal("the backup must keep the very first original")
	}
}

func TestRunFindsNothingOnAMachineWithoutThoseTerminals(t *testing.T) {
	empty := t.TempDir()
	res := Run(Options{GOOS: "linux", Getenv: func(k string) string {
		if k == "HOME" {
			return empty
		}
		return ""
	}})
	if len(res) != 0 {
		t.Fatalf("results = %+v", res)
	}
}
