package components

import (
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

func TestConfigPanelFilterAndEnterEdit(t *testing.T) {
	p := NewConfigPanel(common.DefaultStyles())
	p.SetProviders([]tui.ProviderStatus{
		{
			ID:          "anthropic",
			DisplayName: "Anthropic",
			Description: "Claude models",
			NeedsKey:    true,
			Fields:      []tui.ProviderFieldStatus{{Key: "api_key", Label: "API Key", Secret: true, Required: true}},
		},
		{
			ID:          "openai",
			DisplayName: "OpenAI",
			Description: "GPT models",
			NeedsKey:    true,
			Fields:      []tui.ProviderFieldStatus{{Key: "api_key", Label: "API Key", Secret: true, Required: true}},
		},
	})

	p.TypeFilter("o")
	p.TypeFilter("p")
	p.TypeFilter("e")
	p.TypeFilter("n")

	if got := len(p.list.FilteredItems()); got != 1 {
		t.Fatalf("expected 1 filtered provider, got %d", got)
	}

	p.EnterEdit()
	if !p.editing {
		t.Fatalf("expected config panel to enter edit mode")
	}
	if got := p.editProvider.ID; got != "openai" {
		t.Fatalf("expected openai provider in edit mode, got %q", got)
	}
	if got := len(p.inputs); got != 1 {
		t.Fatalf("expected 1 input field, got %d", got)
	}
}

func TestConfigPanelSetSavedRefreshesProviderState(t *testing.T) {
	p := NewConfigPanel(common.DefaultStyles())
	p.SetProviders([]tui.ProviderStatus{
		{
			ID:          "openai",
			DisplayName: "OpenAI",
			Description: "GPT models",
			NeedsKey:    true,
			Fields:      []tui.ProviderFieldStatus{{Key: "api_key", Label: "API Key", Secret: true, Required: true}},
		},
	})

	p.EnterEdit()
	p.inputs[0].draft = "super-secret"
	p.SetSaved()

	if got := p.statusMsg; got != "✓ Saved" {
		t.Fatalf("expected saved status message, got %q", got)
	}
	if !p.inputs[0].field.IsSet {
		t.Fatalf("expected current input field to be marked as set")
	}
	if !p.providers[0].Fields[0].IsSet {
		t.Fatalf("expected backing provider state to be marked as set")
	}
	if got := len(p.list.FilteredItems()); got != 1 {
		t.Fatalf("expected provider list to stay in sync, got %d items", got)
	}
}
