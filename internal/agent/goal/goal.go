// Package goal implements the persistent goal system.
// Mirrors Codex's ThreadGoal / ThreadGoalStatus from protocol.rs and the
// goal-management tools (create_goal, get_goal, update_goal).
package goal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	dbpkg "github.com/KPO-Tech/seshat-runtime/internal/db"
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

func (g *Goal) clone() *Goal {
	if g == nil {
		return nil
	}
	cp := *g
	if g.TokenBudget != nil {
		b := *g.TokenBudget
		cp.TokenBudget = &b
	}
	return &cp
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
	mu      sync.RWMutex
	goals   map[string]*Goal
	backend Backend
}

func NewStore() *Store {
	return &Store{goals: make(map[string]*Goal)}
}

// Backend persists goals behind Store. Implementations must be safe for
// repeated writes of the same session ID.
type Backend interface {
	SaveGoal(*Goal) error
	LoadGoal(sessionID string) (*Goal, error)
	DeleteGoal(sessionID string) error
}

// ErrGoalNotFound is returned by persistent backends when a session has no goal.
var ErrGoalNotFound = sql.ErrNoRows

// SetBackend installs optional persistent storage for this goal store.
func (s *Store) SetBackend(backend Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backend = backend
}

// Set creates or replaces the goal for sessionID.
// Always sets status=active and resets usage counters.
// Mirrors Codex's ThreadGoalSetParams (new objective → create).
//
// Returns a clone, not the Store's internal pointer - a caller mutating the
// returned Goal's exported fields directly (instead of going through
// Update/RecordTokenUsage) would otherwise corrupt Store state without
// holding s.mu, a data race independent of whether it actually happens
// today. Same reasoning applies to Get and Update below.
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
	backend := s.backend
	s.mu.Unlock()
	if backend != nil {
		_ = backend.SaveGoal(g)
	}
	return g.clone()
}

// Get returns the current goal for sessionID, or false if none.
//
// This is a pure read: it does not write to the backend. TimeUsedSeconds is
// always derived from CreatedAt/startedAt at read time (LoadGoal recomputes
// it the same way on a fresh load), so there is nothing to persist here -
// it never was a stored column. Get used to re-save the goal on every call
// purely to refresh this already-derived field; with a persistent backend
// wired in, that turned every read into a synchronous SQLite write, and
// this is called multiple times per agent turn whenever a goal is active
// (see internal/agent/runner.go's goal-continuation checks).
func (s *Store) Get(sessionID string) (*Goal, bool) {
	s.mu.RLock()
	g, ok := s.goals[sessionID]
	backend := s.backend
	s.mu.RUnlock()
	if !ok && backend != nil {
		loaded, err := backend.LoadGoal(sessionID)
		if err != nil {
			return nil, false
		}
		s.mu.Lock()
		if existing, already := s.goals[sessionID]; already {
			// Lost the race to another goroutine loading the same session -
			// use its copy so every reader converges on one backing pointer.
			g = existing
		} else {
			s.goals[sessionID] = loaded
			g = loaded
		}
		ok = true
		s.mu.Unlock()
	}
	if !ok {
		return nil, false
	}
	s.mu.RLock()
	snapshot := g.clone()
	s.mu.RUnlock()
	snapshot.TimeUsedSeconds = int64(time.Since(snapshot.startedAt).Seconds())
	return snapshot, true
}

// Update mutates the goal's status and/or objective.
// Returns the updated goal, or false if no goal exists.
// Mirrors Codex's ThreadGoalSetParams (update existing goal).
func (s *Store) Update(sessionID string, newStatus *Status, newObjective *string) (*Goal, bool) {
	s.mu.Lock()
	g, ok := s.goals[sessionID]
	if !ok {
		s.mu.Unlock()
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
	snapshot := g.clone()
	backend := s.backend
	s.mu.Unlock()
	// Backend I/O happens after releasing s.mu (like Set/Get above) so one
	// session's disk write can't block every other session's goal
	// operations - the Store's mutex is process-wide, not per-session.
	if backend != nil {
		_ = backend.SaveGoal(snapshot)
	}
	return snapshot, true
}

// RecordTokenUsage adds tokens to the running counter and
// auto-transitions to budgetLimited when the budget is exceeded.
// Called by the runner after each turn.
func (s *Store) RecordTokenUsage(sessionID string, tokens int64) {
	s.mu.Lock()
	g, ok := s.goals[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	g.TokensUsed += tokens
	if g.TokenBudget != nil && g.TokensUsed >= *g.TokenBudget && g.Status == StatusActive {
		g.Status = StatusBudgetLimited
		g.UpdatedAt = time.Now().UnixMilli()
	}
	snapshot := g.clone()
	backend := s.backend
	s.mu.Unlock()
	// See Update's comment: backend I/O deliberately happens outside s.mu.
	if backend != nil {
		_ = backend.SaveGoal(snapshot)
	}
}

// Clear removes the goal for sessionID.
// Mirrors Codex's ThreadGoalClearParams.
func (s *Store) Clear(sessionID string) {
	s.mu.Lock()
	delete(s.goals, sessionID)
	backend := s.backend
	s.mu.Unlock()
	if backend != nil {
		_ = backend.DeleteGoal(sessionID)
	}
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

// ConfigureDefaultStoreBackend wires persistent storage into the process-level
// goal store used by the built-in goal tools.
func ConfigureDefaultStoreBackend(backend Backend) {
	GetDefaultStore().SetBackend(backend)
}

// SQLiteBackend stores goals in the shared runtime SQLite database.
type SQLiteBackend struct {
	db *dbpkg.DB
}

// Close releases the owned database handle.
func (b *SQLiteBackend) Close() error {
	if b == nil || b.db == nil {
		return nil
	}
	return b.db.Close()
}

// NewSQLiteBackend creates a goal backend on top of the shared DB module.
func NewSQLiteBackend(database *dbpkg.DB) (*SQLiteBackend, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if database.Driver() != dbpkg.DriverSQLite {
		return nil, fmt.Errorf("goal sqlite backend requires sqlite database, got %q", database.Driver())
	}
	return &SQLiteBackend{db: database}, nil
}

// OpenSQLiteBackend opens a SQLite-backed goal store.
func OpenSQLiteBackend(path string) (*SQLiteBackend, error) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(path))
	if err != nil {
		return nil, err
	}
	return NewSQLiteBackend(database)
}

// SaveGoal upserts the goal row for a session.
func (b *SQLiteBackend) SaveGoal(g *Goal) error {
	if b == nil || b.db == nil {
		return fmt.Errorf("goal sqlite backend is closed")
	}
	if g == nil {
		return fmt.Errorf("goal is required")
	}
	var tokenBudget any
	if g.TokenBudget != nil {
		tokenBudget = *g.TokenBudget
	}
	_, err := b.db.SQL().Exec(
		`INSERT INTO session_goals (
			session_id, objective, status, token_budget, tokens_used,
			created_at_unix_ms, updated_at_unix_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			objective = excluded.objective,
			status = excluded.status,
			token_budget = excluded.token_budget,
			tokens_used = excluded.tokens_used,
			created_at_unix_ms = excluded.created_at_unix_ms,
			updated_at_unix_ms = excluded.updated_at_unix_ms`,
		g.SessionID,
		g.Objective,
		string(g.Status),
		tokenBudget,
		g.TokensUsed,
		g.CreatedAt,
		g.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save goal %s: %w", g.SessionID, err)
	}
	return nil
}

// LoadGoal returns the persisted goal for a session.
func (b *SQLiteBackend) LoadGoal(sessionID string) (*Goal, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("goal sqlite backend is closed")
	}
	var (
		objective string
		status    string
		budget    sql.NullInt64
		tokens    int64
		createdAt int64
		updatedAt int64
	)
	err := b.db.SQL().QueryRow(
		`SELECT objective, status, token_budget, tokens_used, created_at_unix_ms, updated_at_unix_ms
		 FROM session_goals WHERE session_id = ?`,
		sessionID,
	).Scan(&objective, &status, &budget, &tokens, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrGoalNotFound
		}
		return nil, fmt.Errorf("load goal %s: %w", sessionID, err)
	}
	var tokenBudget *int64
	if budget.Valid {
		b := budget.Int64
		tokenBudget = &b
	}
	startedAt := time.UnixMilli(createdAt)
	g := &Goal{
		SessionID:       sessionID,
		Objective:       objective,
		Status:          Status(status),
		TokenBudget:     tokenBudget,
		TokensUsed:      tokens,
		TimeUsedSeconds: int64(time.Since(startedAt).Seconds()),
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		startedAt:       startedAt,
	}
	return g, nil
}

// DeleteGoal removes a persisted goal for a session.
func (b *SQLiteBackend) DeleteGoal(sessionID string) error {
	if b == nil || b.db == nil {
		return fmt.Errorf("goal sqlite backend is closed")
	}
	_, err := b.db.SQL().Exec(`DELETE FROM session_goals WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("delete goal %s: %w", sessionID, err)
	}
	return nil
}
