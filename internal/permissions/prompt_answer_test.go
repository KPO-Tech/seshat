package permissions

import (
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

// What the user answered to a permission prompt: a boolean is the decision, the text "always" approves and asks to remember it for the
// session, and anything else is a refusal.
func TestPromptApproval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		value        any
		approved     bool
		alwaysRemind bool
	}{
		{"true", true, true, false},
		{"false", false, false, false},
		{"always", "always", true, true},
		{"another text", "yes", false, false},
		{"empty text", "", false, false},
		{"always in capitals is not always", "ALWAYS", false, false},
		{"nil", nil, false, false},
		{"a number", 1, false, false},
	}
	for _, tc := range cases {
		approved, always := promptApproval(tc.value)
		if approved != tc.approved || always != tc.alwaysRemind {
			t.Errorf("%s: promptApproval(%#v) = (%v, %v), want (%v, %v)", tc.name, tc.value, approved, always, tc.approved, tc.alwaysRemind)
		}
	}
}

func TestPromptDenyReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		response types.PromptResponse
		want     string
	}{
		{"no metadata", types.PromptResponse{}, "user denied"},
		{"a reason", types.PromptResponse{Metadata: map[string]any{"reason": "not this file"}}, "not this file"},
		{"an empty reason", types.PromptResponse{Metadata: map[string]any{"reason": ""}}, "user denied"},
		{"a reason that is not text", types.PromptResponse{Metadata: map[string]any{"reason": 3}}, "user denied"},
		{"other metadata", types.PromptResponse{Metadata: map[string]any{"other": "x"}}, "user denied"},
	}
	for _, tc := range cases {
		if got := promptDenyReason(tc.response); got != tc.want {
			t.Errorf("%s: promptDenyReason = %q, want %q", tc.name, got, tc.want)
		}
	}
}
