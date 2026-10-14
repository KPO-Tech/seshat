package engine

import (
	"context"
	"sort"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/prompt"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// namedStubTool is loopTestTool under another name, to put several tools in a registry.
type namedStubTool struct {
	loopTestTool
	name string
}

func (t namedStubTool) Definition() registry.Definition {
	return registry.Definition{Name: t.name, Description: t.name}
}

func sessionToolNames(t *testing.T, cfg *Config, profile string) []string {
	t.Helper()
	reg := registry.NewRegistry()
	for _, name := range []string{"read_file", "bash", "browser_open"} {
		if err := reg.Register(namedStubTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	e := NewEngine(nil, nil, nil, prompt.NewAssembler(), nil, reg, nil, cfg, nil, nil)
	var metadata *types.SessionMetadata
	if profile != "" {
		metadata = &types.SessionMetadata{Additional: map[string]any{"tool_surface_profile": profile}}
	}
	session, err := e.NewSessionFromState(context.Background(), types.NewSessionID("s1"), metadata, nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	names := make([]string, 0)
	for name := range session.state.ToolSurface() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func equalNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMainAgentToolsLimitsTheMainAgentSurface(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MainAgentTools = []string{"read_file", "bash"}
	if got := sessionToolNames(t, cfg, ""); !equalNames(got, []string{"bash", "read_file"}) {
		t.Fatalf("main agent should see only its list, got %v", got)
	}
}

func TestMainAgentToolsLeavesSubagentsAlone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MainAgentTools = []string{"read_file"}
	want := []string{"bash", "browser_open", "read_file"}
	if got := sessionToolNames(t, cfg, "subagent"); !equalNames(got, want) {
		t.Fatalf("a sub-agent keeps the whole registry, got %v", got)
	}
}

func TestMainAgentToolsEmptyKeepsEveryTool(t *testing.T) {
	want := []string{"bash", "browser_open", "read_file"}
	if got := sessionToolNames(t, DefaultConfig(), ""); !equalNames(got, want) {
		t.Fatalf("no list means every tool, got %v", got)
	}
}
