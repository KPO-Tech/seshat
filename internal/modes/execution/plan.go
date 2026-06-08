// Package execution provides execution mode state management.
//
// Plan mode: the agent explores the codebase and writes a plan, which is then
// presented to the user for approval before any source files are modified.
package execution

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// DefaultPlansDirectory is the default location for plan files (~/.config/nexus/plans/).
var DefaultPlansDirectory = runtimepath.PlansDir("")

// State represents the current plan mode state for a session.
type State struct {
	HasExitedPlanMode           bool
	NeedsPlanModeExitAttachment bool
	HasPlanFile                 bool
	PlanWasEdited               bool
}

var planCache = NewModeCache(DefaultPlansDirectory, func() *State { return &State{} })

// GetState returns the plan mode state for a session, creating it if needed.
func GetState(sessionID types.SessionID) *State { return planCache.GetState(sessionID) }

// SetState explicitly sets the state for a session (useful for testing).
func SetState(sessionID types.SessionID, state *State) { planCache.SetState(sessionID, state) }

// ClearState removes the state for a session.
func ClearState(sessionID types.SessionID) { planCache.ClearState(sessionID) }

// ClearAllStates removes all session states (useful for testing).
func ClearAllStates() { planCache.ClearAllStates() }

// GetPlansDirectory returns the directory where plan files are stored.
func GetPlansDirectory() string { return planCache.GetDirectory() }

// SetPlansDirectory sets a custom directory for plan files.
func SetPlansDirectory(dir string) error { return planCache.SetDirectory(dir) }

// GetPlanSlug returns the slug for a session's plan file.
func GetPlanSlug(sessionID types.SessionID) string { return planCache.GetSlug(sessionID) }

// SetPlanSlug sets the plan slug for a session (useful for testing).
func SetPlanSlug(sessionID types.SessionID, slug string) { planCache.SetSlug(sessionID, slug) }

// ClearPlanSlug removes the plan slug for a session.
func ClearPlanSlug(sessionID types.SessionID) { planCache.ClearSlug(sessionID) }

// ClearAllPlanSlugs removes all plan slugs (useful for testing).
func ClearAllPlanSlugs() { planCache.ClearAllSlugs() }

// GetPlanFilePath returns the full path to the plan file for a session.
// Plans are stored under sessions/{sessionID}/plans/ so that deleting a session
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

// DeletePlan deletes the plan file for a session.
func DeletePlan(sessionID types.SessionID, agentID *types.AgentID) error {
	if err := os.Remove(GetPlanFilePath(sessionID, agentID)); err != nil {
		return fmt.Errorf("failed to delete plan file: %w", err)
	}
	SetHasPlanFile(sessionID, false)
	return nil
}

// ============================================================================
// State Accessors
// ============================================================================

func HasExitedPlanMode(sessionID types.SessionID) bool {
	return planCache.GetState(sessionID).HasExitedPlanMode
}
func SetHasExitedPlanMode(sessionID types.SessionID, v bool) {
	planCache.GetState(sessionID).HasExitedPlanMode = v
}

func NeedsPlanModeExitAttachment(sessionID types.SessionID) bool {
	return planCache.GetState(sessionID).NeedsPlanModeExitAttachment
}
func SetNeedsPlanModeExitAttachment(sessionID types.SessionID, v bool) {
	planCache.GetState(sessionID).NeedsPlanModeExitAttachment = v
}

func GetHasPlanFile(sessionID types.SessionID) bool {
	return planCache.GetState(sessionID).HasPlanFile
}
func SetHasPlanFile(sessionID types.SessionID, v bool) {
	planCache.GetState(sessionID).HasPlanFile = v
}

func GetPlanWasEdited(sessionID types.SessionID) bool {
	return planCache.GetState(sessionID).PlanWasEdited
}
func SetPlanWasEdited(sessionID types.SessionID, v bool) {
	planCache.GetState(sessionID).PlanWasEdited = v
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
}

// ExitPlanMode applies state changes when leaving plan mode.
func ExitPlanMode(sessionID types.SessionID) {
	s := planCache.GetState(sessionID)
	s.HasExitedPlanMode = true
	s.NeedsPlanModeExitAttachment = true
}

// CloneState returns a copy of the current plan state.
func CloneState(sessionID types.SessionID) *State {
	s := planCache.GetState(sessionID)
	return &State{
		HasExitedPlanMode:           s.HasExitedPlanMode,
		NeedsPlanModeExitAttachment: s.NeedsPlanModeExitAttachment,
		HasPlanFile:                 s.HasPlanFile,
		PlanWasEdited:               s.PlanWasEdited,
	}
}
