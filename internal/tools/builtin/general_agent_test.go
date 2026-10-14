package builtin

import "testing"

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
