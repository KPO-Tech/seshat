// Package execution provides execution mode state management.
//
// Plan mode: the agent explores the codebase and writes a plan, which is then
// presented to the user for approval before any source files are modified.
package execution

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

// DefaultPlansDirectory is the default location for plan files (~/.config/seshat/plans/).
var DefaultPlansDirectory = runtimepath.PlansDir("")

// State represents the current plan mode state for a session.
type State struct {
	HasExitedPlanMode           bool
	NeedsPlanModeExitAttachment bool
	HasPlanFile                 bool

	// reviewMu guards the two fields below. They are written from the goroutine
	// that answers the plan review (the TUI, via submit_plan and the review
	// dialog) and read from the goroutine running the tool call (exit_plan_mode's
	// permission check), so they must not be plain shared booleans.
	reviewMu      sync.RWMutex
	planSubmitted bool
	planApproved  bool
}

var planCache = NewModeCache(DefaultPlansDirectory, func() *State { return &State{} })

// ClearAllStates removes all session states (useful for testing).
func ClearAllStates() { planCache.ClearAllStates() }

// GetPlanSlug returns the slug for a session's plan file.
func GetPlanSlug(sessionID types.SessionID) string { return planCache.GetSlug(sessionID) }

// SetPlanSlug sets the plan slug for a session (useful for testing).
func SetPlanSlug(sessionID types.SessionID, slug string) { planCache.SetSlug(sessionID, slug) }

// ClearAllPlanSlugs removes all plan slugs (useful for testing).
func ClearAllPlanSlugs() { planCache.ClearAllSlugs() }

// GetPlanFilePath returns the full path to the plan file for a session.
// Plans are stored under workspaces/{sessionID}/plans/ so that deleting a session
// directory removes all its plans in one shot.
// When agentID is non-nil the filename includes the agent identifier, allowing
// separate plan files per sub-agent within the same session.
func GetPlanFilePath(sessionID types.SessionID, agentID *types.AgentID) string {
	dir := runtimepath.SessionPlansDir("", string(sessionID))
	slug := planCache.GetSlug(sessionID)
	if agentID != nil {
		return filepath.Join(dir, slug+"-"+string(*agentID)+".md")
	}
	return filepath.Join(dir, slug+".md")
}

// GetDisplayPath returns a user-friendly tilde-prefixed path to the plan file.
func GetDisplayPath(planFilePath string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return planFilePath
	}
	if rel, err := filepath.Rel(home, planFilePath); err == nil && !filepath.IsAbs(rel) {
		return "~/" + rel
	}
	return planFilePath
}

// PlanExists checks if a plan file exists for the session.
func PlanExists(sessionID types.SessionID, agentID *types.AgentID) bool {
	_, err := os.Stat(GetPlanFilePath(sessionID, agentID))
	return err == nil
}

// GetPlan reads the plan content from disk.
func GetPlan(sessionID types.SessionID, agentID *types.AgentID) (string, error) {
	content, err := os.ReadFile(GetPlanFilePath(sessionID, agentID))
	if err != nil {
		return "", fmt.Errorf("failed to read plan file: %w", err)
	}
	return string(content), nil
}

// SetPlan writes the plan content to disk.
func SetPlan(sessionID types.SessionID, agentID *types.AgentID, content string) error {
	path := GetPlanFilePath(sessionID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create plans directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plan file: %w", err)
	}
	SetHasPlanFile(sessionID, true)
	return nil
}

// ============================================================================
// State Accessors
// ============================================================================

func SetHasPlanFile(sessionID types.SessionID, v bool) {
	planCache.GetState(sessionID).HasPlanFile = v
}

// ============================================================================
// Mode Transitions
// ============================================================================

// EnterPlanMode initializes plan mode state for a session.
func EnterPlanMode(sessionID types.SessionID, agentID *types.AgentID) {
	s := planCache.GetState(sessionID)
	s.HasExitedPlanMode = false
	s.NeedsPlanModeExitAttachment = false
	s.HasPlanFile = PlanExists(sessionID, agentID)
	// Entering starts a new plan cycle: an approval granted for an earlier plan
	// says nothing about the plan written now.
	ClearPlanReview(sessionID)
}

// ExitedPlanMode reports whether the session has left plan mode. It is how a
// caller tells "the plan was approved and the mode moved on" from "the exit was
// refused and the session is still in plan mode".
func ExitedPlanMode(sessionID types.SessionID) bool {
	return planCache.GetState(sessionID).HasExitedPlanMode
}

// ExitPlanMode applies state changes when leaving plan mode.
func ExitPlanMode(sessionID types.SessionID) {
	s := planCache.GetState(sessionID)
	s.HasExitedPlanMode = true
	s.NeedsPlanModeExitAttachment = true
}

// ============================================================================
// Plan Review
// ============================================================================
//
// submit_plan can hand a plan to the user for review, and the user answers in
// the review dialog. exit_plan_mode is gated on that answer: leaving plan mode
// is what grants write access, so a plan nobody has accepted yet must not be
// implemented. These accessors are the memory of that exchange.

// MarkPlanSubmitted records that a plan was handed to the user for review. A
// new submission also clears any earlier approval: a revised plan has to be
// reviewed on its own.
func MarkPlanSubmitted(sessionID types.SessionID) {
	s := planCache.GetState(sessionID)
	s.reviewMu.Lock()
	s.planSubmitted = true
	s.planApproved = false
	s.reviewMu.Unlock()
}

// MarkPlanApproved records the user's acceptance of the submitted plan.
func MarkPlanApproved(sessionID types.SessionID) {
	s := planCache.GetState(sessionID)
	s.reviewMu.Lock()
	s.planApproved = true
	s.reviewMu.Unlock()
}

// ClearPlanReview drops the review state so the next plan cycle starts clean.
func ClearPlanReview(sessionID types.SessionID) {
	s := planCache.GetState(sessionID)
	s.reviewMu.Lock()
	s.planSubmitted = false
	s.planApproved = false
	s.reviewMu.Unlock()
}

// PlanAwaitingApproval reports whether a plan was submitted and the user has not
// approved it yet - the window in which leaving plan mode would run ahead of the
// person reading the plan.
//
// A session that never submitted a plan is awaiting nothing. submit_plan is
// optional (it is disabled when the host provides no persistence hook), and
// without a submission there is no review surface to answer, so plan mode must
// keep its exit rather than become a room with no door.
func PlanAwaitingApproval(sessionID types.SessionID) bool {
	s := planCache.GetState(sessionID)
	s.reviewMu.RLock()
	defer s.reviewMu.RUnlock()
	return s.planSubmitted && !s.planApproved
}
