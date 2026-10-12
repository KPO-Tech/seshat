package goal

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbpkg "github.com/KPO-Tech/seshat-runtime/internal/db"
)

// countingBackend counts SaveGoal calls and optionally blocks on a channel
// before returning, so tests can prove (a) which Store methods actually
// trigger a persistence write, and (b) that the write happens without the
// Store's mutex held.
type countingBackend struct {
	saves int64
	block chan struct{} // if non-nil, SaveGoal waits on this before returning
	goals map[string]*Goal
	mu    sync.Mutex
}

func newCountingBackend() *countingBackend {
	return &countingBackend{goals: make(map[string]*Goal)}
}

func (b *countingBackend) SaveGoal(g *Goal) error {
	atomic.AddInt64(&b.saves, 1)
	if b.block != nil {
		<-b.block
	}
	b.mu.Lock()
	b.goals[g.SessionID] = g.clone()
	b.mu.Unlock()
	return nil
}

func (b *countingBackend) LoadGoal(sessionID string) (*Goal, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	g, ok := b.goals[sessionID]
	if !ok {
		return nil, ErrGoalNotFound
	}
	return g.clone(), nil
}

func (b *countingBackend) DeleteGoal(sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.goals, sessionID)
	return nil
}

func (b *countingBackend) saveCount() int64 { return atomic.LoadInt64(&b.saves) }

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

func TestStore_SQLiteBackendPersistsAcrossStores(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "goals.sqlite")
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	now := time.Now().Unix()
	if _, err := database.SQL().Exec(
		`INSERT INTO session_metadata (session_id, status, created_at_unix, updated_at_unix, metadata_json)
		 VALUES (?, ?, ?, ?, ?)`,
		"sess-sqlite", "active", now, now, "{}",
	); err != nil {
		t.Fatalf("insert session metadata: %v", err)
	}

	backend, err := NewSQLiteBackend(database)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	budget := int64(1000)
	first := NewStore()
	first.SetBackend(backend)
	first.Set("sess-sqlite", "persist this goal", &budget)
	first.RecordTokenUsage("sess-sqlite", 250)

	second := NewStore()
	second.SetBackend(backend)
	got, ok := second.Get("sess-sqlite")
	if !ok {
		t.Fatal("expected persisted goal")
	}
	if got.Objective != "persist this goal" {
		t.Errorf("objective = %q", got.Objective)
	}
	if got.TokensUsed != 250 {
		t.Errorf("tokens_used = %d, want 250", got.TokensUsed)
	}
	if got.TokenBudget == nil || *got.TokenBudget != 1000 {
		t.Fatalf("token budget = %v, want 1000", got.TokenBudget)
	}

	complete := StatusComplete
	if _, ok := second.Update("sess-sqlite", &complete, nil); !ok {
		t.Fatal("expected update to succeed")
	}

	third := NewStore()
	third.SetBackend(backend)
	got, ok = third.Get("sess-sqlite")
	if !ok {
		t.Fatal("expected updated goal")
	}
	if got.Status != StatusComplete {
		t.Errorf("status = %q, want complete", got.Status)
	}

	third.Clear("sess-sqlite")
	fourth := NewStore()
	fourth.SetBackend(backend)
	if _, ok := fourth.Get("sess-sqlite"); ok {
		t.Fatal("expected goal to be deleted")
	}
}

// TestStore_Get_DoesNotWriteToBackend is a regression test: Get used to
// re-save the goal on every call just to refresh the already-derived
// TimeUsedSeconds field, turning every read into a synchronous write once a
// persistent backend was wired in. Get is called multiple times per agent
// turn whenever a goal is active (see runner.go's goal-continuation
// checks), so this mattered for real, not just in theory.
func TestStore_Get_DoesNotWriteToBackend(t *testing.T) {
	backend := newCountingBackend()
	s := NewStore()
	s.SetBackend(backend)
	s.Set("sess-1", "task", nil)

	before := backend.saveCount()
	for i := 0; i < 5; i++ {
		if _, ok := s.Get("sess-1"); !ok {
			t.Fatal("expected goal to be found")
		}
	}
	if got := backend.saveCount(); got != before {
		t.Errorf("Get triggered %d backend save(s), want 0 (before=%d, after=%d)", got-before, before, got)
	}
}

// TestStore_MutatingOperations_StillWriteToBackend guards against the fix
// above being too aggressive - Set/Update/RecordTokenUsage/Clear must keep
// persisting, only Get should stop.
func TestStore_MutatingOperations_StillWriteToBackend(t *testing.T) {
	backend := newCountingBackend()
	s := NewStore()
	s.SetBackend(backend)

	s.Set("sess-1", "task", nil)
	if backend.saveCount() != 1 {
		t.Fatalf("Set: expected 1 save, got %d", backend.saveCount())
	}

	s.RecordTokenUsage("sess-1", 10)
	if backend.saveCount() != 2 {
		t.Fatalf("RecordTokenUsage: expected 2 saves, got %d", backend.saveCount())
	}

	paused := StatusPaused
	s.Update("sess-1", &paused, nil)
	if backend.saveCount() != 3 {
		t.Fatalf("Update: expected 3 saves, got %d", backend.saveCount())
	}
}

// TestStore_Get_ReturnsSnapshotNotSharedPointer proves a caller mutating a
// Goal returned by Get cannot corrupt the Store's internal state - it must
// go through Update/RecordTokenUsage instead.
func TestStore_Get_ReturnsSnapshotNotSharedPointer(t *testing.T) {
	s := NewStore()
	s.Set("sess-1", "task", nil)

	got, _ := s.Get("sess-1")
	got.Objective = "mutated from outside"
	got.TokensUsed = 999999

	fresh, _ := s.Get("sess-1")
	if fresh.Objective != "task" {
		t.Errorf("Store state was corrupted by mutating a Get() result: objective = %q", fresh.Objective)
	}
	if fresh.TokensUsed != 0 {
		t.Errorf("Store state was corrupted by mutating a Get() result: tokens_used = %d", fresh.TokensUsed)
	}
}

// TestStore_Set_ReturnsSnapshotNotSharedPointer is the same check for Set's
// return value.
func TestStore_Set_ReturnsSnapshotNotSharedPointer(t *testing.T) {
	s := NewStore()
	g := s.Set("sess-1", "task", nil)
	g.Objective = "mutated from outside"

	fresh, _ := s.Get("sess-1")
	if fresh.Objective != "task" {
		t.Errorf("Store state was corrupted by mutating a Set() result: objective = %q", fresh.Objective)
	}
}

// TestStore_Update_BackendIOHappensOutsideLock proves Update releases s.mu
// before calling into the backend: a slow/blocked SaveGoal for one session
// must not stall a concurrent Get for a different session. Before the fix,
// Update held s.mu for the full duration of the backend call.
func TestStore_Update_BackendIOHappensOutsideLock(t *testing.T) {
	backend := newCountingBackend()
	s := NewStore()
	s.SetBackend(backend)
	s.Set("sess-slow", "slow task", nil)
	s.Set("sess-other", "other task", nil)
	backend.block = make(chan struct{}) // block SaveGoal from here on

	done := make(chan struct{})
	go func() {
		paused := StatusPaused
		s.Update("sess-slow", &paused, nil)
		close(done)
	}()

	// Give the goroutine a moment to enter Update and start the (blocked)
	// backend call.
	time.Sleep(20 * time.Millisecond)

	getDone := make(chan struct{})
	go func() {
		if _, ok := s.Get("sess-other"); !ok {
			t.Error("expected sess-other to be found")
		}
		close(getDone)
	}()

	select {
	case <-getDone:
		// Get completed while Update's backend call is still blocked - lock
		// was released before the I/O, as intended.
	case <-time.After(2 * time.Second):
		t.Fatal("Get for an unrelated session was blocked by Update's in-flight backend write - the Store's mutex is being held during backend I/O")
	}

	close(backend.block)
	<-done
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
