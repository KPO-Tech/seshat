package config

import "testing"

// TestProvidersKeepRegistryOrderAndDefaults guards the contract of the model
// registry: the first model of a provider is its default, the last its small
// model. The catalog view in pkg/config sorts models by name, and the dialog used
// to take its defaults from that sorted list.
func TestProvidersKeepRegistryOrderAndDefaults(t *testing.T) {
	list, err := Providers(nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string][]string{}
	large := map[string]string{}
	small := map[string]string{}
	for _, p := range list {
		var ids []string
		for _, m := range p.Models {
			ids = append(ids, m.ID)
		}
		byID[string(p.ID)] = ids
		large[string(p.ID)] = p.DefaultLargeModelID
		small[string(p.ID)] = p.DefaultSmallModelID
	}

	want := map[string][2]string{
		"anthropic": {"claude-sonnet-5-5", "claude-haiku-5-5"},
		"openai":    {"gpt-6.1-sol", "gpt-6-luna"},
		"z-ai":      {"glm-5.3", "glm-5.3-flash"},
		"gemini":    {"gemini-4-argon", "gemini-3.5-flash-lite"},
		// Two models only: the same one for both, not the dearer one as the small model.
		"deepseek": {"deepseek-flash", "deepseek-flash"},
	}
	for id, w := range want {
		if large[id] != w[0] || small[id] != w[1] {
			t.Errorf("%s defaults = large %q small %q, want %q / %q", id, large[id], small[id], w[0], w[1])
		}
		if ids := byID[id]; len(ids) == 0 || ids[0] != w[0] {
			t.Errorf("%s list must start with its default %q, got %v", id, w[0], ids)
		}
	}
}
