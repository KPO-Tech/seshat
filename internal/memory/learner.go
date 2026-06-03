package memory

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// Auto-Learner: Learns patterns from runtime
// ============================================================================

// Learner learns patterns from tool usage and results
type Learner struct {
	memory      *Manager
	projectPath string
	sessionID   string
	toolStats   map[string]*ToolStats // tool_name -> stats
}

// ToolStats tracks tool usage statistics
type ToolStats struct {
	ToolName    string    `json:"tool_name"`
	Attempts    int       `json:"attempts"`
	Successes   int       `json:"successes"`
	Failures    int       `json:"failures"`
	LastUsedAt  time.Time `json:"last_used_at"`
	AvgDuration Duration  `json:"avg_duration_ms"`
}

// Duration is a simple duration wrapper for JSON
type Duration struct {
	MS int64 `json:"ms"`
}

// NewLearner creates a new auto-learner
func NewLearner(projectPath, sessionID string) (*Learner, error) {
	mem, err := NewManager()
	if err != nil {
		return nil, err
	}

	// Load existing memory
	_ = mem.LoadUser()
	_ = mem.LoadCrossSession()
	_ = mem.LoadProject(projectPath)

	return &Learner{
		memory:      mem,
		projectPath: projectPath,
		sessionID:   sessionID,
		toolStats:   make(map[string]*ToolStats),
	}, nil
}

// OnToolUse records a tool usage attempt
func (l *Learner) OnToolUse(toolName string, success bool, durationMs int, errMsg string) {
	stats, exists := l.toolStats[toolName]
	if !exists {
		stats = &ToolStats{ToolName: toolName}
		l.toolStats[toolName] = stats
	}

	stats.Attempts++
	if success {
		stats.Successes++
	} else {
		stats.Failures++
	}
	stats.LastUsedAt = time.Now()

	// Update average duration
	totalMs := stats.AvgDuration.MS*int64(stats.Attempts-1) + int64(durationMs)
	stats.AvgDuration.MS = totalMs / int64(stats.Attempts)

	// Learn pattern if significant
	if stats.Attempts >= 3 {
		l.learnToolPattern(toolName, stats, errMsg)
	}
}

// learnToolPattern learns a pattern from tool statistics
func (l *Learner) learnToolPattern(toolName string, stats *ToolStats, lastError string) {
	successRate := float64(stats.Successes) / float64(stats.Attempts)

	cross := l.memory.GetCrossSession()
	if cross == nil {
		return
	}

	patternKey := fmt.Sprintf("tool:%s", toolName)

	// Update or create pattern
	if pattern, exists := cross.GlobalPatterns[patternKey]; exists {
		pattern.Frequency = stats.Attempts
		pattern.SuccessRate = successRate
		pattern.LastSeenAt = time.Now()

		// Add error example if this was a failure
		if lastError != "" && successRate < 0.5 {
			if len(pattern.Examples) < 5 {
				pattern.Examples = append(pattern.Examples, lastError)
			}
		}
	} else {
		description := fmt.Sprintf("used %d times, %.0f%% success rate", stats.Attempts, successRate*100)
		if lastError != "" && successRate < 0.5 {
			description += ", errors: " + lastError
		}

		cross.GlobalPatterns[patternKey] = &PatternEntry{
			Key:         patternKey,
			Pattern:     toolName,
			Description: description,
			Frequency:   stats.Attempts,
			SuccessRate: successRate,
			LastSeenAt:  time.Now(),
			Examples:    []string{},
		}
		if lastError != "" && successRate < 0.5 {
			cross.GlobalPatterns[patternKey].Examples = []string{lastError}
		}
	}

	// Also learn to project memory
	l.learnProjectPatternToStore(toolName, stats)
}

// learnProjectPatternToStore learns a pattern to project memory
func (l *Learner) learnProjectPatternToStore(toolName string, stats *ToolStats) error {
	project := l.memory.GetProject()
	if project == nil {
		return nil
	}

	key := fmt.Sprintf("tool:%s", toolName)
	entry, exists := project.Entries[key]
	if !exists {
		entry = NewEntry(MemoryScopeProject, MemoryTypePattern, key, "", "auto_learn")
	}

	successRate := float64(stats.Successes) / float64(stats.Attempts)
	entry.Value = fmt.Sprintf("used %d times, %.0f%% success rate", stats.Attempts, successRate*100)
	entry.UpdatedAt = time.Now()

	if entry.Metadata == nil {
		entry.Metadata = &EntryMetadata{}
	}
	entry.Metadata.Frequency = stats.Attempts
	entry.Metadata.SuccessRate = successRate
	entry.Metadata.LastUsedAt = &stats.LastUsedAt

	project.Entries[key] = entry

	return nil
}

// AddUserPreference learns a user preference
func (l *Learner) AddUserPreference(key, value, source string) error {
	return l.memory.LearnPreference(MemoryScopeUser, key, value, source)
}

// AddInstruction learns a persistent instruction
func (l *Learner) AddInstruction(key, instruction, source string) error {
	project := l.memory.GetProject()
	if project == nil {
		return fmt.Errorf("project memory not loaded")
	}

	entry := NewEntry(MemoryScopeProject, MemoryTypeInstruction, key, instruction, source)
	project.Entries[key] = entry

	return l.memory.SaveProject()
}

// Flush saves learned patterns to storage
func (l *Learner) Flush() error {
	if err := l.memory.SaveUser(); err != nil {
		return err
	}
	if err := l.memory.SaveCrossSession(); err != nil {
		return err
	}
	if err := l.memory.SaveProject(); err != nil {
		return err
	}
	return nil
}

// ============================================================================
// Error Pattern Learning
// ============================================================================

// ErrorLearner learns from errors
type ErrorLearner struct {
	memory *Manager
	errors map[string]*ErrorPattern
}

// ErrorPattern represents a learned error pattern
type ErrorPattern struct {
	ErrorType  string    `json:"error_type"`
	Message    string    `json:"message"`
	Suggestion string    `json:"suggestion"`
	Frequency  int       `json:"frequency"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

// NewErrorLearner creates a new error learner
func NewErrorLearner(projectPath string) (*ErrorLearner, error) {
	mem, err := NewManager()
	if err != nil {
		return nil, err
	}

	_ = mem.LoadProject(projectPath)

	return &ErrorLearner{
		memory: mem,
		errors: make(map[string]*ErrorPattern),
	}, nil
}

// OnError learns from an error
func (e *ErrorLearner) OnError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Categorize error
	errType := categorizeError(errMsg)
	key := fmt.Sprintf("error:%s", errType)

	pattern, exists := e.errors[key]
	if !exists {
		pattern = &ErrorPattern{
			ErrorType:  errType,
			Message:    errMsg,
			Suggestion: getSuggestion(errType),
			FirstSeen:  time.Now(),
		}
		e.errors[key] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()

	// Learn to memory if frequent
	if pattern.Frequency >= 2 {
		project := e.memory.GetProject()
		if project != nil {
			entryKey := fmt.Sprintf("error:%s", errType)
			entry := NewEntry(MemoryScopeProject, MemoryTypeKnowledge, entryKey, pattern.Suggestion, "error_learner")
			entry.Confidence = float64(pattern.Frequency) / float64(pattern.Frequency+1)
			project.Entries[entryKey] = entry

			return e.memory.SaveProject()
		}
	}

	return nil
}

// categorizeError categorizes an error message
func categorizeError(msg string) string {
	msg = strings.ToLower(msg)

	switch {
	case strings.Contains(msg, "permission denied"):
		return "permission"
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no such file"):
		return "not_found"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "connection"):
		return "connection"
	case strings.Contains(msg, "invalid"):
		return "invalid"
	case strings.Contains(msg, "failed"), strings.Contains(msg, "error"):
		return "generic"
	default:
		return "unknown"
	}
}

// getSuggestion returns a suggestion for an error type
func getSuggestion(errType string) string {
	switch errType {
	case "permission":
		return "Check file permissions or run with appropriate access"
	case "not_found":
		return "Verify the file path exists before operation"
	case "timeout":
		return "Check network connectivity or increase timeout"
	case "connection":
		return "Verify the service is running and accessible"
	case "invalid":
		return "Check input format and parameters"
	default:
		return "Review error details and try again"
	}
}

// ============================================================================
// Context Builder
// ============================================================================

// ContextBuilder builds context from learned patterns
type ContextBuilder struct {
	memory *Manager
}

// NewContextBuilder creates a new context builder
func NewContextBuilder(projectPath string) (*ContextBuilder, error) {
	mem, err := NewManager()
	if err != nil {
		return nil, err
	}

	_ = mem.LoadProject(projectPath)
	_ = mem.LoadUser()
	_ = mem.LoadCrossSession()

	return &ContextBuilder{memory: mem}, nil
}

// Build returns learned context for prompts
func (b *ContextBuilder) Build() string {
	var context string

	// User preferences (high confidence only)
	user := b.memory.GetUser()
	if user != nil {
		context += "## User Preferences\n"
		count := 0
		for _, e := range user.Entries {
			if e.Type == MemoryTypePreference && e.Confidence >= 0.8 && count < 5 {
				context += fmt.Sprintf("- %s: %s (%.0f%% confidence)\n", e.Key, e.Value, e.Confidence*100)
				count++
			}
		}
		if count == 0 {
			context += "(none learned yet)\n"
		}
	}

	// Project-specific patterns
	project := b.memory.GetProject()
	if project != nil {
		context += "\n## Project Patterns\n"
		count := 0
		for _, e := range project.Entries {
			if e.Type == MemoryTypePattern && count < 5 {
				context += fmt.Sprintf("- %s: %s\n", e.Key, e.Value)
				count++
			}
		}
		if count == 0 {
			context += "(none learned yet)\n"
		}
	}

	// Cross-session patterns
	cross := b.memory.GetCrossSession()
	if cross != nil && len(cross.GlobalPatterns) > 0 {
		context += "\n## Learned Tools\n"
		count := 0
		for _, p := range cross.GlobalPatterns {
			if p.Frequency >= 3 && count < 5 {
				context += fmt.Sprintf("- %s: %s (%.0f%% success)\n", p.Key, p.Description, p.SuccessRate*100)
				count++
			}
		}
	}

	return context
}

// ============================================================================
// Project Memory - moved to integration.go
// ============================================================================
