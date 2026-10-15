package builtin

import (
	"regexp"
	"testing"
)

// staleToolNames are names tools had in an earlier naming scheme. A model
// reading one of them in a tool description may call a tool that does not exist.
var staleToolNames = regexp.MustCompile(`\b(FileRead|FileWrite|FileEdit|WebFetch|WebSearch|AskUserQuestion|EnterPlanMode|ExitPlanMode|ToolSearch|TodoWrite|NotebookEdit|Bash tool)\b`)

func TestToolDescriptionsDoNotUseStaleToolNames(t *testing.T) {
	reg, err := NewBuiltinRegistryWithConfig(&Config{})
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	for _, tl := range reg.List() {
		def := tl.Definition()
		if m := staleToolNames.FindString(def.Description); m != "" {
			t.Errorf("%s: description mentions %q, which is not a tool name (use the registered name, e.g. read_file)", def.Name, m)
		}
	}
}

func TestBashDescriptionAndSchemaAgreeOnTimeoutUnit(t *testing.T) {
	reg, err := NewBuiltinRegistryWithConfig(&Config{})
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	tl, ok := reg.Get("bash")
	if !ok {
		t.Fatal("bash is not registered")
	}
	if regexp.MustCompile(`(?i)milliseconds|\b\d+ms\b`).MatchString(tl.Definition().Description) {
		t.Error("bash timeout is in seconds (see the input schema); its description must not say milliseconds")
	}
}
