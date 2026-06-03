package types

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// DenialTrackingState tracks consecutive permission denials for a session.
type DenialTrackingState struct {
	mu sync.RWMutex

	// ConsecutiveDenials counts refusals in a row since the last success.
	ConsecutiveDenials int `json:"consecutive_denials"`

	// TotalDenials is the cumulative count of denials in this session.
	TotalDenials int `json:"total_denials"`

	// LastDeniedAt records when the most recent denial occurred.
	LastDeniedAt *time.Time `json:"last_denied_at,omitempty"`

	// LastAllowedAt records when the most recent allowance occurred.
	LastAllowedAt *time.Time `json:"last_allowed_at,omitempty"`
}

// RecordDenial increments the denial counters and updates the last denied timestamp.
func (d *DenialTrackingState) RecordDenial() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ConsecutiveDenials++
	d.TotalDenials++
	now := time.Now()
	d.LastDeniedAt = &now
}

// RecordSuccess resets the consecutive denial counter and updates the last allowed timestamp.
func (d *DenialTrackingState) RecordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ConsecutiveDenials = 0
	now := time.Now()
	d.LastAllowedAt = &now
}

// GetConsecutiveDenials returns the current consecutive denial count.
func (d *DenialTrackingState) GetConsecutiveDenials() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ConsecutiveDenials
}

// GetTotalDenials returns the total denial count.
func (d *DenialTrackingState) GetTotalDenials() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.TotalDenials
}

// DenialLimitConfig defines thresholds for denial-based fallback.
type DenialLimitConfig struct {
	// MaxConsecutiveDenials triggers fallback when reached.
	// A value of 0 means no limit.
	MaxConsecutiveDenials int `json:"max_consecutive_denials"`

	// MaxTotalDenials triggers fallback when reached.
	// A value of 0 means no limit.
	MaxTotalDenials int `json:"max_total_denials"`

	// ResetAfterSeconds without a denial resets consecutive counter.
	// A value of 0 means no time-based reset.
	ResetAfterSeconds int `json:"reset_after_seconds"`
}

// DefaultDenialLimitConfig returns sensible defaults.
func DefaultDenialLimitConfig() DenialLimitConfig {
	return DenialLimitConfig{
		MaxConsecutiveDenials: 3,
		MaxTotalDenials:       0,   // no limit
		ResetAfterSeconds:     300, // 5 minutes
	}
}

// ShouldFallback returns true if the denial state exceeds configured limits.
// When the time-based reset threshold is exceeded, consecutive denials are
// automatically reset before evaluating the limit.
func (c DenialLimitConfig) ShouldFallback(state *DenialTrackingState) bool {
	// Time-based reset of consecutive counter: if enough time has passed
	// since the last denial, reset consecutive denials.
	if c.ResetAfterSeconds > 0 && state.LastDeniedAt != nil {
		if time.Since(*state.LastDeniedAt).Seconds() > float64(c.ResetAfterSeconds) {
			state.mu.Lock()
			state.ConsecutiveDenials = 0
			state.mu.Unlock()
		}
	}

	if c.MaxConsecutiveDenials > 0 {
		if state.GetConsecutiveDenials() >= c.MaxConsecutiveDenials {
			return true
		}
	}

	if c.MaxTotalDenials > 0 {
		if state.GetTotalDenials() >= c.MaxTotalDenials {
			return true
		}
	}

	return false
}

// SafetyCheckResult is the outcome of a bypass-immune safety check.
type SafetyCheckResult struct {
	// IsDangerous indicates the pattern is unsafe.
	IsDangerous bool `json:"is_dangerous"`

	// Reason explains why the pattern is dangerous.
	Reason string `json:"reason"`

	// CheckType identifies which safety check produced this result.
	CheckType string `json:"check_type"`
}

// SafetyChecker performs bypass-immune safety validation.
type SafetyChecker interface {
	// CheckSafety evaluates whether a tool use is unsafe regardless of permission mode.
	CheckSafety(toolName string, input map[string]any) SafetyCheckResult
}

// DangerousPatternChecker is a simple pattern-based safety checker.
type DangerousPatternChecker struct {
	patterns map[string][]DangerousPattern
}

// DangerousPattern defines a safety rule.
type DangerousPattern struct {
	// ToolName is the tool this pattern applies to. Empty means all tools.
	ToolName string `json:"tool_name"`

	// Pattern is a substring match in the input (key or value).
	Pattern string `json:"pattern"`

	// Reason explains why this pattern is dangerous.
	Reason string `json:"reason"`

	// CheckType identifies the check source.
	CheckType string `json:"check_type"`
}

// NewDangerousPatternChecker creates a checker with default patterns.
func NewDangerousPatternChecker() *DangerousPatternChecker {
	return &DangerousPatternChecker{
		patterns: map[string][]DangerousPattern{
			"bash": {
				{Pattern: "rm -rf /", Reason: "destructive root filesystem deletion", CheckType: "destructive_command"},
				{Pattern: "rm -rf /*", Reason: "destructive root filesystem deletion", CheckType: "destructive_command"},
				{Pattern: "mkfs", Reason: "filesystem formatting", CheckType: "destructive_command"},
				{Pattern: "dd if=/dev/zero", Reason: "disk wiping", CheckType: "destructive_command"},
				{Pattern: "chmod 000", Reason: "permission stripping", CheckType: "destructive_command"},
				{Pattern: "shutdown", Reason: "system shutdown", CheckType: "system_command"},
				{Pattern: "reboot", Reason: "system reboot", CheckType: "system_command"},
			},
		},
	}
}

// AddPattern registers a dangerous pattern for a tool.
func (c *DangerousPatternChecker) AddPattern(toolName string, pattern DangerousPattern) {
	if c.patterns == nil {
		c.patterns = make(map[string][]DangerousPattern)
	}
	c.patterns[toolName] = append(c.patterns[toolName], pattern)
}

// CheckSafety evaluates input against registered patterns.
func (c *DangerousPatternChecker) CheckSafety(toolName string, input map[string]any) SafetyCheckResult {
	toolPatterns := c.patterns[toolName]
	globalPatterns := c.patterns[""] // applies to all tools

	allPatterns := append(toolPatterns, globalPatterns...)

	inputStr := serializeInput(input)
	for _, pattern := range allPatterns {
		if contains(inputStr, pattern.Pattern) {
			return SafetyCheckResult{
				IsDangerous: true,
				Reason:      pattern.Reason,
				CheckType:   pattern.CheckType,
			}
		}
	}

	return SafetyCheckResult{IsDangerous: false}
}

func serializeInput(input map[string]any) string {
	// Simple serialization: key=value pairs separated by spaces
	out := ""
	for k, v := range input {
		out += k + "=" + fmt.Sprintf("%v", v) + " "
	}
	return out
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
