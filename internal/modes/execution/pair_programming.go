// Package execution provides execution mode state management.
//
// Pair programming mode: collaborative AI-human coding with real-time feedback.
// All tools execute normally in this mode; the collaboration logic is handled
// at the prompt and suggestion level rather than through tool blocking.
package execution

import (
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const DefaultSuggestionsDirectory = ".nexus/suggestions"

// PairProgrammingState represents the pair programming mode state for a session.
type PairProgrammingState struct {
	HasActiveCollaboration  bool
	NeedsUserConfirmation   bool
	HasSuggestionsFile      bool
	SuggestionsWereReviewed bool
	CollaborationTurns      int
	SuggestionsCount        int
	AcceptedSuggestions     int
}

var ppCache = NewModeCache(DefaultSuggestionsDirectory, func() *PairProgrammingState {
	return &PairProgrammingState{}
})

// GetPairProgrammingState returns the pair programming state for a session.
func GetPairProgrammingState(sessionID types.SessionID) *PairProgrammingState {
	return ppCache.GetState(sessionID)
}

// SetPairProgrammingState explicitly sets the state for a session (useful for testing).
func SetPairProgrammingState(sessionID types.SessionID, state *PairProgrammingState) {
	ppCache.SetState(sessionID, state)
}

// ClearPairProgrammingState removes all state for a session.
func ClearPairProgrammingState(sessionID types.SessionID) { ppCache.ClearState(sessionID) }

// ClearAllPairProgrammingStates removes all session states (useful for testing).
func ClearAllPairProgrammingStates() { ppCache.ClearAllStates() }

// GetSuggestionsDirectory returns the directory where suggestion files are stored.
func GetSuggestionsDirectory() string { return ppCache.GetDirectory() }

// SetSuggestionsDirectory sets a custom directory for suggestion files.
func SetSuggestionsDirectory(dir string) error { return ppCache.SetDirectory(dir) }

// GetSuggestionSlug returns the suggestion slug for a session.
func GetSuggestionSlug(sessionID types.SessionID) string { return ppCache.GetSlug(sessionID) }

// SetSuggestionSlug sets the suggestion slug for a session (useful for testing).
func SetSuggestionSlug(sessionID types.SessionID, slug string) { ppCache.SetSlug(sessionID, slug) }

// ClearSuggestionSlug removes the suggestion slug for a session.
func ClearSuggestionSlug(sessionID types.SessionID) { ppCache.ClearSlug(sessionID) }

// ClearAllSuggestionSlugs removes all suggestion slugs (useful for testing).
func ClearAllSuggestionSlugs() { ppCache.ClearAllSlugs() }

// GetSuggestionFilePath returns the full path to the suggestions file for a session.
func GetSuggestionFilePath(sessionID types.SessionID) string { return ppCache.GetFilePath(sessionID) }

// GetSuggestionsDisplayPath returns a user-friendly relative path to the suggestions file.
func GetSuggestionsDisplayPath(suggestionFilePath string) string {
	return ppCache.GetDisplayPath(suggestionFilePath)
}

// SuggestionsExist checks if a suggestions file exists for the session.
func SuggestionsExist(sessionID types.SessionID) bool { return ppCache.FileExists(sessionID) }

// GetSuggestions reads the suggestions content from disk.
func GetSuggestions(sessionID types.SessionID) (string, error) { return ppCache.ReadFile(sessionID) }

// SetSuggestions writes the suggestions content to disk.
func SetSuggestions(sessionID types.SessionID, content string) error {
	if err := ppCache.WriteFile(sessionID, content); err != nil {
		return err
	}
	SetHasSuggestionsFile(sessionID, true)
	return nil
}

// DeleteSuggestions deletes the suggestions file for a session.
func DeleteSuggestions(sessionID types.SessionID) error {
	if err := ppCache.DeleteFile(sessionID); err != nil {
		return err
	}
	SetHasSuggestionsFile(sessionID, false)
	return nil
}

// ============================================================================
// State Accessors
// ============================================================================

func HasActiveCollaboration(sessionID types.SessionID) bool {
	return ppCache.GetState(sessionID).HasActiveCollaboration
}
func SetHasActiveCollaboration(sessionID types.SessionID, v bool) {
	ppCache.GetState(sessionID).HasActiveCollaboration = v
}

func NeedsUserConfirmation(sessionID types.SessionID) bool {
	return ppCache.GetState(sessionID).NeedsUserConfirmation
}
func SetNeedsUserConfirmation(sessionID types.SessionID, v bool) {
	ppCache.GetState(sessionID).NeedsUserConfirmation = v
}

func GetHasSuggestionsFile(sessionID types.SessionID) bool {
	return ppCache.GetState(sessionID).HasSuggestionsFile
}
func SetHasSuggestionsFile(sessionID types.SessionID, v bool) {
	ppCache.GetState(sessionID).HasSuggestionsFile = v
}

func GetSuggestionsWereReviewed(sessionID types.SessionID) bool {
	return ppCache.GetState(sessionID).SuggestionsWereReviewed
}
func SetSuggestionsWereReviewed(sessionID types.SessionID, v bool) {
	ppCache.GetState(sessionID).SuggestionsWereReviewed = v
}

func IncrementCollaborationTurns(sessionID types.SessionID) {
	ppCache.GetState(sessionID).CollaborationTurns++
}
func GetCollaborationTurns(sessionID types.SessionID) int {
	return ppCache.GetState(sessionID).CollaborationTurns
}

func IncrementSuggestionsCount(sessionID types.SessionID) {
	ppCache.GetState(sessionID).SuggestionsCount++
}
func GetSuggestionsCount(sessionID types.SessionID) int {
	return ppCache.GetState(sessionID).SuggestionsCount
}

func IncrementAcceptedSuggestions(sessionID types.SessionID) {
	ppCache.GetState(sessionID).AcceptedSuggestions++
}
func GetAcceptedSuggestions(sessionID types.SessionID) int {
	return ppCache.GetState(sessionID).AcceptedSuggestions
}

// ============================================================================
// Mode Transitions
// ============================================================================

// EnterPairProgrammingMode initializes pair programming mode state for a session.
func EnterPairProgrammingMode(sessionID types.SessionID) {
	s := ppCache.GetState(sessionID)
	s.HasActiveCollaboration = true
	s.NeedsUserConfirmation = true
	s.HasSuggestionsFile = SuggestionsExist(sessionID)
}

// ExitPairProgrammingMode applies state changes when leaving pair programming mode.
func ExitPairProgrammingMode(sessionID types.SessionID) {
	s := ppCache.GetState(sessionID)
	s.HasActiveCollaboration = false
	s.NeedsUserConfirmation = false
}

// ClonePairProgrammingState returns a copy of the current state.
func ClonePairProgrammingState(sessionID types.SessionID) *PairProgrammingState {
	s := ppCache.GetState(sessionID)
	return &PairProgrammingState{
		HasActiveCollaboration:  s.HasActiveCollaboration,
		NeedsUserConfirmation:   s.NeedsUserConfirmation,
		HasSuggestionsFile:      s.HasSuggestionsFile,
		SuggestionsWereReviewed: s.SuggestionsWereReviewed,
		CollaborationTurns:      s.CollaborationTurns,
		SuggestionsCount:        s.SuggestionsCount,
		AcceptedSuggestions:     s.AcceptedSuggestions,
	}
}
