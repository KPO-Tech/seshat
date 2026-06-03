package goal

import (
	"strings"
	"testing"
	"time"
)

// ─── Store tests ──────────────────────────────────────────────────────────────

func TestStore_SetAndGet(t *testing.T) {
	s := NewStore()
	g := s.Set("sess-1", "build a feature", nil)

	if g.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", g.SessionID)
	}
	if g.Status != StatusActive {
		t.Errorf("status = %q, want active", g.Status)
	}
	if g.TokensUsed != 0 {
		t.Errorf("tokens_used = %d, want 0", g.TokensUsed)
	}

	got, ok := s.Get("sess-1")
	if !ok {
		t.Fatal("Get returned false for existing session")
	}
	if got.Objective != "build a feature" {
		t.Errorf("objective = %q", got.Objective)
	}
}

func TestStore_Get_missing(t *testing.T) {
	s := NewStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing session")
	}
}

func TestStore_Update_status(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)

	newStatus := StatusComplete
	g, ok := s.Update("sess-1", &newStatus, nil)
	if !ok {
		t.Fatal("Update returned false")
	}
	if g.Status != StatusComplete {
		t.Errorf("status after update = %q, want complete", g.Status)
	}
}

func TestStore_Update_objective(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "old objective", nil)

	newObj := "new objective"
	g, ok := s.Update("sess-1", nil, &newObj)
	if !ok {
		t.Fatal("Update returned false")
	}
	if g.Objective != "new objective" {
		t.Errorf("objective after update = %q", g.Objective)
	}
}

func TestStore_Update_missing(t *testing.T) {
	s := NewStore()
	newStatus := StatusPaused
	_, ok := s.Update("nonexistent", &newStatus, nil)
	if ok {
		t.Fatal("expected false for missing session")
	}
}

func TestStore_RecordTokenUsage_no_budget(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)
	s.RecordTokenUsage("sess-1", 500)

	g, _ := s.Get("sess-1")
	if g.TokensUsed != 500 {
		t.Errorf("tokens_used = %d, want 500", g.TokensUsed)
	}
	if g.Status != StatusActive {
		t.Errorf("status = %q, should stay active with no budget", g.Status)
	}
}

func TestStore_RecordTokenUsage_budget_exceeded(t *testing.T) {
	s := NewStore()
	budget := int64(1000)
	s.Set("sess-1", "task", &budget)
	s.RecordTokenUsage("sess-1", 500)
	s.RecordTokenUsage("sess-1", 600) // total 1100 > 1000

	g, _ := s.Get("sess-1")
	if g.TokensUsed != 1100 {
		t.Errorf("tokens_used = %d, want 1100", g.TokensUsed)
	}
	if g.Status != StatusBudgetLimited {
		t.Errorf("status = %q, want budgetLimited", g.Status)
	}
}

func TestStore_Clear(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)
	s.Clear("sess-1")
	_, ok := s.Get("sess-1")
	if ok {
		t.Fatal("expected false after Clear")
	}
}

func TestStore_Set_replaces(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "first objective", nil)
	s.RecordTokenUsage("sess-1", 999)
	s.Set("sess-1", "second objective", nil) // replace

	g, _ := s.Get("sess-1")
	if g.Objective != "second objective" {
		t.Errorf("objective = %q, want 'second objective'", g.Objective)
	}
	if g.TokensUsed != 0 {
		t.Errorf("tokens_used should be reset, got %d", g.TokensUsed)
	}
}

func TestGoal_RemainingTokens(t *testing.T) {
	s := NewStore()
	budget := int64(1000)
	s.Set("sess-1", "task", &budget)
	s.RecordTokenUsage("sess-1", 300)

	g, _ := s.Get("sess-1")
	if got := g.RemainingTokens(); got != 700 {
		t.Errorf("RemainingTokens() = %d, want 700", got)
	}
}

func TestGoal_RemainingTokens_unbounded(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)
	g, _ := s.Get("sess-1")
	if got := g.RemainingTokens(); got != -1 {
		t.Errorf("RemainingTokens() for unbounded = %d, want -1", got)
	}
}

func TestGoal_IsOverBudget(t *testing.T) {
	s := NewStore()
	budget := int64(100)
	s.Set("sess-1", "task", &budget)
	s.RecordTokenUsage("sess-1", 50)

	g, _ := s.Get("sess-1")
	if g.IsOverBudget() {
		t.Error("IsOverBudget should be false when under budget")
	}

	s.RecordTokenUsage("sess-1", 60)
	g, _ = s.Get("sess-1")
	if !g.IsOverBudget() {
		t.Error("IsOverBudget should be true when budget exceeded")
	}
}

func TestStore_TimeUsedSeconds(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)
	time.Sleep(10 * time.Millisecond)

	g, _ := s.Get("sess-1")
	if g.TimeUsedSeconds < 0 {
		t.Errorf("TimeUsedSeconds should be non-negative, got %d", g.TimeUsedSeconds)
	}
}

// ─── ValidateObjective ────────────────────────────────────────────────────────

func TestValidateObjective_empty(t *testing.T) {
	if err := ValidateObjective(""); err == nil {
		t.Fatal("expected error for empty objective")
	}
	if err := ValidateObjective("   "); err == nil {
		t.Fatal("expected error for whitespace-only objective")
	}
}

func TestValidateObjective_tooLong(t *testing.T) {
	long := strings.Repeat("a", MaxObjectiveChars+1)
	if err := ValidateObjective(long); err == nil {
		t.Fatal("expected error for too-long objective")
	}
}

func TestValidateObjective_valid(t *testing.T) {
	if err := ValidateObjective("build a feature"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── IsFinal ──────────────────────────────────────────────────────────────────

func TestIsFinal(t *testing.T) {
	finals := []Status{StatusComplete, StatusBudgetLimited}
	nonFinals := []Status{StatusActive, StatusPaused, StatusBlocked, StatusUsageLimited}

	for _, s := range finals {
		if !IsFinal(s) {
			t.Errorf("IsFinal(%q) = false, want true", s)
		}
	}
	for _, s := range nonFinals {
		if IsFinal(s) {
			t.Errorf("IsFinal(%q) = true, want false", s)
		}
	}
}

// ─── Prompts ──────────────────────────────────────────────────────────────────

func TestContinuationPrompt_contains_objective(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "implement feature X", nil)
	g, _ := s.Get("sess-1")

	prompt := ContinuationPrompt(g)
	if !strings.Contains(prompt, "implement feature X") {
		t.Error("ContinuationPrompt missing objective")
	}
	if !strings.Contains(prompt, "Continue working toward the active goal") {
		t.Error("ContinuationPrompt missing continuation marker")
	}
}

func TestContinuationPrompt_with_budget(t *testing.T) {
	s := NewStore()
	budget := int64(5000)
	s.Set("sess-1", "task", &budget)
	s.RecordTokenUsage("sess-1", 1200)
	g, _ := s.Get("sess-1")

	prompt := ContinuationPrompt(g)
	if !strings.Contains(prompt, "1200") {
		t.Error("ContinuationPrompt missing tokens_used")
	}
	if !strings.Contains(prompt, "5000") {
		t.Error("ContinuationPrompt missing token_budget")
	}
	if !strings.Contains(prompt, "3800") {
		t.Error("ContinuationPrompt missing remaining_tokens")
	}
}

func TestBudgetLimitPrompt_contains_objective(t *testing.T) {
	s := NewStore()
	budget := int64(1000)
	s.Set("sess-1", "finish the report", &budget)
	g, _ := s.Get("sess-1")

	prompt := BudgetLimitPrompt(g)
	if !strings.Contains(prompt, "finish the report") {
		t.Error("BudgetLimitPrompt missing objective")
	}
	if !strings.Contains(prompt, "budget_limited") {
		t.Error("BudgetLimitPrompt missing budget_limited marker")
	}
}

func TestObjectiveUpdatedPrompt_contains_new_objective(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "new objective after update", nil)
	g, _ := s.Get("sess-1")

	prompt := ObjectiveUpdatedPrompt(g)
	if !strings.Contains(prompt, "new objective after update") {
		t.Error("ObjectiveUpdatedPrompt missing objective")
	}
	if !strings.Contains(prompt, "untrusted_objective") {
		t.Error("ObjectiveUpdatedPrompt missing untrusted_objective tag")
	}
}

func TestEscapeXML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{"safe text", "safe text"},
	}
	for _, tc := range cases {
		if got := escapeXML(tc.in); got != tc.want {
			t.Errorf("escapeXML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ─── Singleton ────────────────────────────────────────────────────────────────

func TestGetDefaultStore_singleton(t *testing.T) {
	a := GetDefaultStore()
	b := GetDefaultStore()
	if a != b {
		t.Error("GetDefaultStore should return the same instance")
	}
}
