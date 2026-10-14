package builtin

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/prompt"
)

// registeredBySDK are the general agent's tools that the SDK registers after
// the engine is built, so they are not in the builtin registry.
var registeredBySDK = map[string]bool{"agent": true, "spawn_agent": true}

func TestGeneralAgentToolNamesAreRegistered(t *testing.T) {
	reg, err := NewBuiltinRegistryWithConfig(&Config{})
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	seen := map[string]bool{}
	for _, name := range GeneralAgentToolNames() {
		if seen[name] {
			t.Errorf("%q is listed twice", name)
		}
		seen[name] = true
		if registeredBySDK[name] {
			continue
		}
		if !reg.HasTool(name) {
			t.Errorf("%q is in the general agent list but no tool has that name (renamed or removed?)", name)
		}
	}
}

func TestGeneralAgentToolNamesIsASmallSurface(t *testing.T) {
	// The point of the list is a short surface. A new entry should be a decision,
	// not an accident: raise this bound on purpose.
	if n := len(GeneralAgentToolNames()); n > 30 {
		t.Errorf("general agent offers %d tools; keep the core small", n)
	}
}

func TestGeneralAgentPromptMentionsOnlyToolsTheAgentHas(t *testing.T) {
	known := map[string]bool{}
	for _, name := range GeneralAgentToolNames() {
		known[name] = true
	}

	// Backticked words that are not tools: agent types, shell commands and tool parameters.
	allowedNonTool := map[string]bool{
		"browse":          true,
		"explore":         true,
		"general-purpose": true,
		"plan":            true,
		"verify":          true,
		"rm":              true,
		"tasks":           true,
		"updates":         true,
	}

	re := regexp.MustCompile("`([^`]+)`")
	var unknown []string
	seen := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(prompt.SeshatCoreStablePrompt(), -1) {
		name := strings.TrimSpace(match[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if !strings.ContainsAny(name, "abcdefghijklmnopqrstuvwxyz") || strings.ContainsAny(name, "/.:") {
			continue // "#", "##", paths, URLs
		}
		if known[name] || allowedNonTool[name] {
			continue
		}
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Fatalf("the general agent's prompt mentions names that are not in its tool list: %v", unknown)
	}
}

func TestGeneralAgentPromptDescribesNoBrowserTool(t *testing.T) {
	if strings.Contains(prompt.SeshatCoreStablePrompt(), "browser_") {
		t.Fatal("the general agent has no browser tools; its prompt must not describe them")
	}
}
