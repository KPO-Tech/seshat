// Package auto - Configuration types for auto mode.
//
// This file defines configuration structures used throughout the auto mode
// package, including mode-level configuration, denial limits, and defaults.
package auto

import "github.com/EngineerProjects/nexus-engine/internal/types"

// Config holds general configuration for auto mode.
// Note: ModeConfig in mode.go is used for the Mode struct itself.
// This Config is a separate type kept for potential future use.
type Config struct {
	FailClosed bool // When true, classifier errors result in deny (fail-closed)
}

// DefaultConfig returns the default auto mode configuration.
// FailClosed is true for security (deny on classifier errors).
func DefaultConfig() *Config {
	return &Config{
		FailClosed: true,
	}
}

// DenialLimitConfig defines thresholds for auto mode fallback behavior.
// When these limits are exceeded, the system falls back from auto-approval
// to prompting the user for permission.
// Aligned with OpenClaude's denial tracking configuration.
type DenialLimitConfig struct {
	MaxConsecutiveDenials int // Maximum consecutive denials before fallback
	MaxTotalDenials       int // Maximum total denials before fallback
	WindowMinutes         int // Time window for tracking (currently unused)
}

// DefaultDenialLimitConfig returns the default denial limit configuration.
// These values are aligned with OpenClaude:
//   - MaxConsecutiveDenials: 3 - After 3 consecutive denials, fallback to prompting
//   - MaxTotalDenials: 10 - After 10 total denials in session, fallback to prompting
func DefaultDenialLimitConfig() DenialLimitConfig {
	return DenialLimitConfig{
		MaxConsecutiveDenials: 3,
		MaxTotalDenials:       10,
		WindowMinutes:         60,
	}
}

// ShouldFallback checks if the denial limits have been exceeded.
// Returns true if the system should fall back from auto-approval to prompting.
func (c *DenialLimitConfig) ShouldFallback(state *types.DenialTrackingState) bool {
	if state == nil {
		return false
	}
	// Check consecutive denials limit
	if c.MaxConsecutiveDenials > 0 && state.GetConsecutiveDenials() >= c.MaxConsecutiveDenials {
		return true
	}
	// Check total denials limit
	if c.MaxTotalDenials > 0 && state.GetTotalDenials() >= c.MaxTotalDenials {
		return true
	}
	return false
}
