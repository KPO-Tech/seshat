// Package auto - State management for auto mode classifier.
//
// This module provides state tracking for the auto mode classifier,
// including session state, circuit breaking, and transcript caching.
// Aligned with OpenClaude's autoModeState.ts.
package auto

import (
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

var (
	autoModeActive   bool
	autoModeActiveMu sync.RWMutex

	autoModeFlagCli   bool
	autoModeFlagCliMu sync.RWMutex

	autoModeCircuitBroken   bool
	autoModeCircuitBrokenMu sync.RWMutex

	sessionID   types.SessionID
	sessionIDMu sync.RWMutex

	lastClassifierTranscript   string
	lastClassifierTranscriptMu sync.RWMutex
)

// SetAutoModeActive sets whether auto mode is currently active.
func SetAutoModeActive(active bool) {
	autoModeActiveMu.Lock()
	defer autoModeActiveMu.Unlock()
	autoModeActive = active
}

// IsAutoModeActive returns whether auto mode is currently active.
func IsAutoModeActive() bool {
	autoModeActiveMu.RLock()
	defer autoModeActiveMu.RUnlock()
	return autoModeActive
}

// SetAutoModeFlagCli sets whether auto mode was requested via CLI.
func SetAutoModeFlagCli(passed bool) {
	autoModeFlagCliMu.Lock()
	defer autoModeFlagCliMu.Unlock()
	autoModeFlagCli = passed
}

// GetAutoModeFlagCli returns whether auto mode was requested via CLI.
func GetAutoModeFlagCli() bool {
	autoModeFlagCliMu.RLock()
	defer autoModeFlagCliMu.RUnlock()
	return autoModeFlagCli
}

// SetAutoModeCircuitBroken sets whether auto mode circuit is broken.
// When circuit is broken, auto mode cannot be re-enabled.
func SetAutoModeCircuitBroken(broken bool) {
	autoModeCircuitBrokenMu.Lock()
	defer autoModeCircuitBrokenMu.Unlock()
	autoModeCircuitBroken = broken
}

// IsAutoModeCircuitBroken returns whether auto mode circuit is broken.
func IsAutoModeCircuitBroken() bool {
	autoModeCircuitBrokenMu.RLock()
	defer autoModeCircuitBrokenMu.RUnlock()
	return autoModeCircuitBroken
}

// SetSessionID sets the current session ID.
func SetSessionID(id types.SessionID) {
	sessionIDMu.Lock()
	defer sessionIDMu.Unlock()
	sessionID = id
}

// GetSessionID returns the current session ID.
func GetSessionID() types.SessionID {
	sessionIDMu.RLock()
	defer sessionIDMu.RUnlock()
	return sessionID
}

// SetLastClassifierTranscript stores the last classifier transcript for debugging.
func SetLastClassifierTranscript(transcript string) {
	lastClassifierTranscriptMu.Lock()
	defer lastClassifierTranscriptMu.Unlock()
	lastClassifierTranscript = transcript
}

// GetLastClassifierTranscript returns the last classifier transcript.
func GetLastClassifierTranscript() string {
	lastClassifierTranscriptMu.RLock()
	defer lastClassifierTranscriptMu.RUnlock()
	return lastClassifierTranscript
}

// ResetForTesting resets all state for testing.
func ResetForTesting() {
	autoModeActiveMu.Lock()
	autoModeActive = false
	autoModeActiveMu.Unlock()

	autoModeFlagCliMu.Lock()
	autoModeFlagCli = false
	autoModeFlagCliMu.Unlock()

	autoModeCircuitBrokenMu.Lock()
	autoModeCircuitBroken = false
	autoModeCircuitBrokenMu.Unlock()

	sessionIDMu.Lock()
	sessionID = ""
	sessionIDMu.Unlock()

	lastClassifierTranscriptMu.Lock()
	lastClassifierTranscript = ""
	lastClassifierTranscriptMu.Unlock()
}
