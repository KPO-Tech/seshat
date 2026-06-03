// Package goal implements the persistent goal system.
// Mirrors Codex's ThreadGoal / ThreadGoalStatus from protocol.rs and the
// goal-management tools (create_goal, get_goal, update_goal).
package goal

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MaxObjectiveChars mirrors Codex's MAX_THREAD_GOAL_OBJECTIVE_CHARS.
const MaxObjectiveChars = 4_000

// Status mirrors Codex's ThreadGoalStatus.
type Status string

const (
	StatusActive        Status = "active"
	StatusPaused        Status = "paused"
	StatusBlocked       Status = "blocked"
	StatusUsageLimited  Status = "usageLimited"
	StatusBudgetLimited Status = "budgetLimited"
	StatusComplete      Status = "complete"
)

// IsFinal returns true for terminal statuses where the goal is no longer actionable.
func IsFinal(s Status) bool {
	return s == StatusComplete || s == StatusBudgetLimited
}

// Goal mirrors Codex's ThreadGoal struct (protocol.rs:3651).
// Keyed by SessionID in the Store.
type Goal struct {
	SessionID       string `json:"session_id"`
	Objective       string `json:"objective"`
	Status          Status `json:"status"`
	TokenBudget     *int64 `json:"token_budget,omitempty"`
	TokensUsed      int64  `json:"tokens_used"`
	TimeUsedSeconds int64  `json:"time_used_seconds"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`

	// startedAt is the wall-clock time the goal was created, used to compute TimeUsedSeconds.
	startedAt time.Time
}

// RemainingTokens returns how many tokens are left in the budget, or -1 if unbounded.
func (g *Goal) RemainingTokens() int64 {
	if g.TokenBudget == nil {
		return -1
	}
	r := *g.TokenBudget - g.TokensUsed
	if r < 0 {
		return 0
	}
	return r
}

// IsOverBudget returns true when the token budget is exhausted.
func (g *Goal) IsOverBudget() bool {
	if g.TokenBudget == nil {
		return false
	}
	return g.TokensUsed >= *g.TokenBudget
}

// ValidateObjective returns an error when the objective violates the Codex constraints.
func ValidateObjective(objective string) error {
	if strings.TrimSpace(objective) == "" {
		return fmt.Errorf("goal objective must not be empty")
	}
	if len([]rune(objective)) > MaxObjectiveChars {
		return fmt.Errorf("goal objective must be at most %d characters", MaxObjectiveChars)
	}
	return nil
}

// ─── Store ────────────────────────────────────────────────────────────────────

// Store is the in-memory goal registry keyed by SessionID.
// Thread-safe. Mirrors Codex's server-side goal state per thread.
type Store struct {
	mu    sync.RWMutex
	goals map[string]*Goal
}

func NewStore() *Store {
	return &Store{goals: make(map[string]*Goal)}
}

// Set creates or replaces the goal for sessionID.
// Always sets status=active and resets usage counters.
// Mirrors Codex's ThreadGoalSetParams (new objective → create).
func (s *Store) Set(sessionID, objective string, tokenBudget *int64) *Goal {
	now := time.Now()
	g := &Goal{
		SessionID:   sessionID,
		Objective:   objective,
		Status:      StatusActive,
		TokenBudget: tokenBudget,
		TokensUsed:  0,
		CreatedAt:   now.UnixMilli(),
		UpdatedAt:   now.UnixMilli(),
		startedAt:   now,
	}
	s.mu.Lock()
	s.goals[sessionID] = g
	s.mu.Unlock()
	return g
}

// Get returns the current goal for sessionID, or false if none.
func (s *Store) Get(sessionID string) (*Goal, bool) {
	s.mu.RLock()
	g, ok := s.goals[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	// Update computed TimeUsedSeconds on read.
	s.mu.Lock()
	g.TimeUsedSeconds = int64(time.Since(g.startedAt).Seconds())
	s.mu.Unlock()
	return g, true
}

// Update mutates the goal's status and/or objective.
// Returns the updated goal, or false if no goal exists.
// Mirrors Codex's ThreadGoalSetParams (update existing goal).
func (s *Store) Update(sessionID string, newStatus *Status, newObjective *string) (*Goal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[sessionID]
	if !ok {
		return nil, false
	}
	if newStatus != nil {
		g.Status = *newStatus
	}
	if newObjective != nil && strings.TrimSpace(*newObjective) != "" {
		g.Objective = *newObjective
	}
	g.UpdatedAt = time.Now().UnixMilli()
	g.TimeUsedSeconds = int64(time.Since(g.startedAt).Seconds())
	return g, true
}

// RecordTokenUsage adds tokens to the running counter and
// auto-transitions to budgetLimited when the budget is exceeded.
// Called by the runner after each turn.
func (s *Store) RecordTokenUsage(sessionID string, tokens int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.goals[sessionID]
	if !ok {
		return
	}
	g.TokensUsed += tokens
	if g.TokenBudget != nil && g.TokensUsed >= *g.TokenBudget && g.Status == StatusActive {
		g.Status = StatusBudgetLimited
		g.UpdatedAt = time.Now().UnixMilli()
	}
}

// Clear removes the goal for sessionID.
// Mirrors Codex's ThreadGoalClearParams.
func (s *Store) Clear(sessionID string) {
	s.mu.Lock()
	delete(s.goals, sessionID)
	s.mu.Unlock()
}

// ─── Global default store ─────────────────────────────────────────────────────

var (
	defaultStore     *Store
	defaultStoreOnce sync.Once
)

// GetDefaultStore returns the process-level singleton GoalStore.
func GetDefaultStore() *Store {
	defaultStoreOnce.Do(func() {
		defaultStore = NewStore()
	})
	return defaultStore
}
