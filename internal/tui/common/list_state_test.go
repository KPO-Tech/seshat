package common

import (
	"strings"
	"testing"
)

func TestStateFiltersAndNavigates(t *testing.T) {
	state := NewListState(func(item string, needle string) bool {
		return strings.Contains(strings.ToLower(item), needle)
	})
	state.SetItems([]string{"alpha", "beta", "betamax"})

	state.TypeFilter("b")
	state.TypeFilter("e")
	state.TypeFilter("t")

	if got := len(state.FilteredItems()); got != 2 {
		t.Fatalf("expected 2 filtered items, got %d", got)
	}
	if got := state.Cursor(); got != 0 {
		t.Fatalf("expected cursor to reset to 0, got %d", got)
	}

	state.Down()
	selected, ok := state.Selected()
	if !ok || selected != "betamax" {
		t.Fatalf("expected betamax to be selected, got %q ok=%v", selected, ok)
	}
}

func TestStateResetItemsPreservesCursorWhenPossible(t *testing.T) {
	state := NewListState(func(item string, needle string) bool {
		return strings.Contains(strings.ToLower(item), needle)
	})
	state.SetItems([]string{"alpha", "beta", "betamax"})
	state.SetFilter("beta")
	state.Down()

	state.ResetItems([]string{"alpha", "beta"}, true)

	selected, ok := state.Selected()
	if !ok || selected != "beta" {
		t.Fatalf("expected cursor to clamp to remaining item beta, got %q ok=%v", selected, ok)
	}
}

func TestStateClearFilterRestoresAllItems(t *testing.T) {
	state := NewListState(func(item string, needle string) bool {
		return strings.Contains(strings.ToLower(item), needle)
	})
	state.SetItems([]string{"alpha", "beta"})
	state.SetFilter("z")
	if got := len(state.FilteredItems()); got != 0 {
		t.Fatalf("expected 0 filtered items, got %d", got)
	}

	state.ClearFilter()
	if got := len(state.FilteredItems()); got != 2 {
		t.Fatalf("expected all items after clearing filter, got %d", got)
	}
}
