package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/modes"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	contract "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Transition represents a state transition in the query loop
// Based on OpenClaude's query.ts transitions
type Transition interface {
	IsContinue() bool
	IsTerminal() bool
}

// ContinueTransition indicates the loop should continue
type ContinueTransition struct {
	// Reason explains why we're continuing
	Reason string

	// RecoveryType indicates the type of recovery (if any)
	RecoveryType RecoveryType
}

// RecoveryType represents the type of recovery
type RecoveryType string

const (
	// RecoveryTypeNone means no recovery
	RecoveryTypeNone RecoveryType = "none"

	// RecoveryTypeMaxOutputTokens means max_output_tokens recovery
	RecoveryTypeMaxOutputTokens RecoveryType = "max_output_tokens"

	// RecoveryTypePromptTooLong means prompt_too_long recovery
	RecoveryTypePromptTooLong RecoveryType = "prompt_too_long"

	// RecoveryTypeContinuationNudge means continuation nudge
	RecoveryTypeContinuationNudge RecoveryType = "continuation_nudge"

	// RecoveryTypeAPIRetry means a narrow retry of a retryable API failure.
	RecoveryTypeAPIRetry RecoveryType = "api_retry"

	// RecoveryTypeTokenBudget means a budget-based continuation nudge fired.
	RecoveryTypeTokenBudget RecoveryType = "token_budget"

	// RecoveryTypeStopHook means a stop hook requested continuation.
	RecoveryTypeStopHook RecoveryType = "stop_hook"
)

const defaultBudgetContinuationLimit = 3

// IsContinue returns true
func (c ContinueTransition) IsContinue() bool {
	return true
}

// IsTerminal returns false
func (c ContinueTransition) IsTerminal() bool {
	return false
}

// TerminalTransition indicates the loop should stop
type TerminalTransition struct {
	// Reason explains why we're stopping
	Reason string

	// StopReason is the API stop reason
	StopReason string
}

// IsContinue returns false
func (t TerminalTransition) IsContinue() bool {
	return false
}

// IsTerminal returns true
func (t TerminalTransition) IsTerminal() bool {
	return true
}

// Continue creates a continue transition
func Continue(reason string) Transition {
	return ContinueTransition{
		Reason:       reason,
		RecoveryType: RecoveryTypeNone,
	}
}

// ContinueWithRecovery creates a continue transition with recovery
func ContinueWithRecovery(reason string, recoveryType RecoveryType) Transition {
	return ContinueTransition{
		Reason:       reason,
		RecoveryType: recoveryType,
	}
}

// Terminate creates a terminal transition
func Terminate(reason string, stopReason string) Transition {
	return TerminalTransition{
		Reason:     reason,
		StopReason: stopReason,
	}
}

// SessionState is the long-lived, session-owned conversation state.
//
// The query loop owns short-lived per-turn execution details in MutableState;
// this struct keeps only the canonical transcript, tool surface, and aggregate
// counters that must survive across turns and restores.
type SessionState struct {
	SessionID           types.SessionID
	TurnID              types.TurnID
	Messages            []types.Message
	Tools               map[string]tool.Tool
	DiscoveredDeferred  []string
	PermissionContext   *types.PermissionContext
	DenialTracking      *types.DenialTrackingState
	TurnNumber          int
	TotalTokens         int
	Metadata            *types.SessionMetadata
	LastCompaction      *types.CompactionMetadata
	CompactionCount     int
	CanonicalTranscript int
}

func NewSessionState(sessionID types.SessionID, turnID types.TurnID, messages []types.Message, tools map[string]tool.Tool, turnNumber int, totalTokens int, metadata *types.SessionMetadata) *SessionState {
	canonicalMessages := types.CanonicalTranscriptMessages(messages)
	compactionCount := types.CountCompactionMessages(canonicalMessages)
	discoveredDeferred := discoveredDeferredToolsFromMetadata(metadata)
	return &SessionState{
		SessionID:           sessionID,
		TurnID:              turnID,
		Messages:            append([]types.Message(nil), messages...),
		Tools:               tools,
		DiscoveredDeferred:  discoveredDeferred,
		PermissionContext:   permissionContextFromMetadata(metadata),
		DenialTracking:      &types.DenialTrackingState{},
		TurnNumber:          turnNumber,
		TotalTokens:         totalTokens,
		Metadata:            metadata,
		LastCompaction:      types.CompactionMetadataFromMessages(canonicalMessages),
		CompactionCount:     compactionCount,
		CanonicalTranscript: len(canonicalMessages),
	}
}

func (s *SessionState) CloneMessages() []types.Message {
	return append([]types.Message(nil), s.Messages...)
}

func (s *SessionState) ReplaceMessages(messages []types.Message) {
	s.Messages = append([]types.Message(nil), messages...)
	canonicalMessages := types.CanonicalTranscriptMessages(messages)
	s.CanonicalTranscript = len(canonicalMessages)
	s.LastCompaction = types.CompactionMetadataFromMessages(canonicalMessages)
	s.CompactionCount = types.CountCompactionMessages(canonicalMessages)
	if s.Metadata != nil {
		if s.Metadata.Additional == nil {
			s.Metadata.Additional = make(map[string]any)
		}
		s.Metadata.Additional["canonical_transcript"] = map[string]any{
			"message_count": s.CanonicalTranscript,
			"turn_count":    types.CountDistinctTurnIDs(canonicalMessages),
			"tool_results":  types.CountToolResultMessages(canonicalMessages),
		}
		s.Metadata.CompactCount = s.CompactionCount
		if s.LastCompaction != nil {
			now := GetCurrentTime()
			s.Metadata.LastCompactedAt = &now
		} else {
			s.Metadata.LastCompactedAt = nil
		}
	}
}

// AdvanceTurn folds a completed loop result back into the canonical session
// state. This is the handoff point between per-turn runtime execution and the
// long-lived conversation owner.
func (s *SessionState) AdvanceTurn(usage *types.TokenUsage, updatedMessages []types.Message) {
	s.ReplaceMessages(updatedMessages)
	s.TurnNumber++
	if s.Metadata != nil {
		s.Metadata.TotalTurns = s.TurnNumber
	}
	if usage != nil {
		s.TotalTokens += usage.InputTokens + usage.OutputTokens
		if s.Metadata != nil {
			s.Metadata.TotalTokens = s.TotalTokens
		}
	}
	if s.Metadata != nil {
		s.Metadata.UpdatedAt = GetCurrentTime()
	}
	s.TurnID = nextTurnID(s.Messages)
}

func (s *SessionState) MarkInterrupted() {
	if s.Metadata != nil {
		s.Metadata.Status = types.SessionStatusInterrupt
		s.Metadata.UpdatedAt = GetCurrentTime()
	}
}

// GetLastRecoveryContext returns the most recent recovery context for resume
func (s *SessionState) GetLastRecoveryContext() *RecoveryContext {
	if s.Metadata == nil || s.Metadata.Additional == nil {
		return nil
	}
	if rec, ok := s.Metadata.Additional["recovery_context"]; ok {
		if rc, ok := rec.(*RecoveryContext); ok {
			return rc
		}
	}
	return nil
}

// StoreRecoveryContext stores recovery context in metadata for persistence
func (s *SessionState) StoreRecoveryContext(ctx *RecoveryContext) {
	if s.Metadata == nil {
		return
	}
	if s.Metadata.Additional == nil {
		s.Metadata.Additional = make(map[string]any)
	}
	s.Metadata.Additional["recovery_context"] = ctx
}

func (s *SessionState) MarkClosed() {
	if s.Metadata != nil {
		s.Metadata.Status = types.SessionStatusClosed
		s.Metadata.UpdatedAt = GetCurrentTime()
	}
}

func (s *SessionState) MetadataSnapshot() *types.SessionMetadata {
	return s.Metadata
}

func (s *SessionState) PermissionContextSnapshot() *types.PermissionContext {
	return clonePermissionContext(s.PermissionContext)
}

func (s *SessionState) SetPermissionContext(permissionContext *types.PermissionContext) {
	s.PermissionContext = clonePermissionContext(permissionContext)
	if s.PermissionContext != nil {
		s.PermissionContext.NormalizeLegacyPlanMode()
	}
	if s.Metadata == nil || s.PermissionContext == nil {
		return
	}
	if s.Metadata.Additional == nil {
		s.Metadata.Additional = make(map[string]any)
	}
	s.Metadata.Additional["permission_context"] = clonePermissionContext(s.PermissionContext)
}

func (s *SessionState) CurrentPermissionMode() types.PermissionMode {
	if s.PermissionContext == nil || s.PermissionContext.Mode == "" {
		return types.PermissionModeOnRequest
	}
	return s.PermissionContext.Mode
}

func (s *SessionState) CurrentExecutionMode() string {
	if s.PermissionContext == nil || s.PermissionContext.ExecutionMode == "" {
		return string(modes.ExecutionModeExecute)
	}
	return s.PermissionContext.ExecutionMode
}

func (s *SessionState) DenialTrackingState() *types.DenialTrackingState {
	if s.DenialTracking == nil {
		s.DenialTracking = &types.DenialTrackingState{}
	}
	return s.DenialTracking
}

func (s *SessionState) ToolSurface() map[string]tool.Tool {
	return s.Tools
}

func (s *SessionState) EffectiveToolSurface(reg *tool.Registry) map[string]tool.Tool {
	surface := make(map[string]tool.Tool, len(s.Tools)+len(s.DiscoveredDeferred))
	for name, resolved := range s.Tools {
		surface[name] = resolved
	}
	if reg == nil {
		return surface
	}
	for _, name := range s.DiscoveredDeferred {
		resolved, ok := reg.Resolve(name)
		if !ok {
			continue
		}
		surface[name] = resolved
	}
	return surface
}

func (s *SessionState) PendingDeferredToolNames(reg *tool.Registry) []string {
	if reg == nil {
		return nil
	}
	profile := tool.ToolSurfaceProfileMonoRun
	if s.Metadata != nil && s.Metadata.Additional != nil {
		if rawProfile, ok := s.Metadata.Additional["tool_surface_profile"].(string); ok && strings.TrimSpace(rawProfile) != "" {
			profile = rawProfile
		}
	}
	discovered := make(map[string]bool, len(s.DiscoveredDeferred))
	for _, name := range s.DiscoveredDeferred {
		discovered[name] = true
	}
	deferred := reg.ListDeferred()
	names := make([]string, 0, len(deferred))
	seen := make(map[string]bool, len(deferred))
	for _, deferredTool := range deferred {
		if deferredTool == nil {
			continue
		}
		name := deferredTool.Definition().Name
		if seen[name] || discovered[name] {
			continue
		}
		if !tool.VisibleInSurfaceProfile(tool.ToolDefinition{
			Name:        name,
			Description: deferredTool.Definition().Description,
			Category:    deferredTool.Definition().Category,
			InputSchema: deferredTool.Definition().InputSchema,
			Metadata:    deferredTool.Definition().Metadata,
		}, profile) {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *SessionState) RegisterDiscoveredDeferredTools(names []string) {
	if len(names) == 0 {
		return
	}
	seen := make(map[string]bool, len(s.DiscoveredDeferred))
	for _, name := range s.DiscoveredDeferred {
		seen[name] = true
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		s.DiscoveredDeferred = append(s.DiscoveredDeferred, name)
	}
	sort.Strings(s.DiscoveredDeferred)
	if s.Metadata == nil {
		return
	}
	if s.Metadata.Additional == nil {
		s.Metadata.Additional = make(map[string]any)
	}
	values := make([]string, len(s.DiscoveredDeferred))
	copy(values, s.DiscoveredDeferred)
	s.Metadata.Additional["discovered_deferred_tools"] = values
}

func discoveredDeferredToolsFromMetadata(metadata *types.SessionMetadata) []string {
	if metadata == nil || metadata.Additional == nil {
		return nil
	}
	raw, ok := metadata.Additional["discovered_deferred_tools"]
	if !ok {
		return nil
	}

	seen := make(map[string]bool)
	names := make([]string, 0)
	switch values := raw.(type) {
	case []string:
		for _, name := range values {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	case []any:
		for _, rawName := range values {
			name, ok := rawName.(string)
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func permissionContextFromMetadata(metadata *types.SessionMetadata) *types.PermissionContext {
	if metadata == nil || metadata.Additional == nil {
		return nil
	}
	raw, ok := metadata.Additional["permission_context"]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var permissionContext types.PermissionContext
	if err := json.Unmarshal(data, &permissionContext); err != nil {
		return nil
	}
	permissionContext.NormalizeLegacyPlanMode()
	if permissionContext.Mode == "" {
		return nil
	}
	return &permissionContext
}

func clonePermissionContext(permissionContext *types.PermissionContext) *types.PermissionContext {
	if permissionContext == nil {
		return nil
	}
	cloned := *permissionContext
	if permissionContext.StrippedDangerousRules != nil {
		cloned.StrippedDangerousRules = make(map[string][]string, len(permissionContext.StrippedDangerousRules))
		for key, values := range permissionContext.StrippedDangerousRules {
			cloned.StrippedDangerousRules[key] = append([]string(nil), values...)
		}
	}
	return &cloned
}

func normalizePermissionContext(current *types.PermissionContext, fallback *types.PermissionContext) *types.PermissionContext {
	if current == nil {
		normalized := clonePermissionContext(fallback)
		if normalized != nil {
			normalized.NormalizeLegacyPlanMode()
		}
		return normalized
	}
	normalized := clonePermissionContext(current)
	normalized.NormalizeLegacyPlanMode()
	if normalized.Mode == "" && fallback != nil {
		normalized.Mode = fallback.Mode
	}
	if fallback != nil {
		if !normalized.IsBypassPermissionsModeAvailable {
			normalized.IsBypassPermissionsModeAvailable = fallback.IsBypassPermissionsModeAvailable
		}
		if !normalized.IsAutoModeAvailable {
			normalized.IsAutoModeAvailable = fallback.IsAutoModeAvailable
		}
	}
	if normalized.Mode == types.PermissionModeBypass {
		normalized.IsBypassPermissionsModeAvailable = true
	}
	return normalized
}

func permissionContextFromToolContext(base *types.PermissionContext, toolCtx contract.ToolUseContext) *types.PermissionContext {
	current := normalizePermissionContext(base, nil)
	if current == nil {
		current = &types.PermissionContext{}
	}
	current.Mode = toolCtx.PermissionMode
	current.ExecutionMode = toolCtx.ExecutionMode
	current.PrePlanMode = toolCtx.PrePlanMode
	current.IsBypassPermissionsModeAvailable = toolCtx.IsBypassPermissionsModeAvailable
	current.IsAutoModeAvailable = toolCtx.IsAutoModeAvailable
	current.NormalizeLegacyPlanMode()
	return current
}

// MutableState is the per-turn working set owned by the query loop.
// It is intentionally mutable and short-lived: each Run starts from canonical
// SessionState, mutates this structure across iterations, then folds the final
// result back into SessionState via AdvanceTurn.
type MutableState struct {
	// Messages are the conversation messages
	Messages []types.Message

	// ToolUses are all tool uses executed
	ToolUses []types.ToolUseContent

	// ToolResults are all tool results
	ToolResults []tool.CallResult

	// DiscoveredDeferred tracks deferred tools discovered during the current run.
	DiscoveredDeferred []string

	// Usage is token usage information
	Usage *types.TokenUsage

	// StopReason is why the loop stopped
	StopReason string

	// Compacted indicates if compaction occurred
	Compacted bool

	// Iterations is the number of iterations performed
	Iterations int

	// TurnCount is the turn count
	TurnCount int

	// MaxOutputTokensRecoveryCount is recovery attempt count
	MaxOutputTokensRecoveryCount int

	// HasAttemptedReactiveCompact indicates if reactive compact was tried
	HasAttemptedReactiveCompact bool

	// MaxOutputTokensOverride is the override for max tokens
	MaxOutputTokensOverride int

	// ContinuationNudgeCount is continuation nudge count
	ContinuationNudgeCount int

	// Transition is the previous transition (undefined on first iteration)
	Transition Transition

	// StopHookActive indicates if a stop hook is active
	StopHookActive *bool

	// AutoCompactFailureCount tracks consecutive auto-compact failures.
	AutoCompactFailureCount int

	// TotalTurnTokens accumulates API usage across iterations of the turn.
	TotalTurnTokens int

	// BudgetContinuationCount tracks how many budget-based continuations fired.
	BudgetContinuationCount int

	// LastBudgetCheckTokens is the previous total token count seen by budget logic.
	LastBudgetCheckTokens int

	// LastBudgetDelta stores the last token delta for diminishing-returns checks.
	LastBudgetDelta int

	// RecoveryContext tracks details needed to properly resume after interruption
	RecoveryContext *RecoveryContext

	// CurrentStage is the prompt.ExecutionStage detected at the start of the last
	// iteration. Used to avoid redundant system-prompt rebuilds.
	CurrentStage prompt.ExecutionStage

	// PermissionContext is the evolving permission context owned by the loop
	// during this turn. Initialized from RunRequest at turn start; updated after
	// each tool execution batch. Session reads it back via RunResult.
	PermissionContext *types.PermissionContext

	// PermissionMode mirrors PermissionContext.Mode for fast access without a
	// nil check on every iteration.
	PermissionMode types.PermissionMode
}

// NewMutableState creates the short-lived per-turn state for one loop run.
func NewMutableState(initialMessages []types.Message) *MutableState {
	return &MutableState{
		Messages:                     append([]types.Message(nil), initialMessages...),
		ToolUses:                     make([]types.ToolUseContent, 0),
		ToolResults:                  make([]tool.CallResult, 0),
		DiscoveredDeferred:           make([]string, 0),
		TurnCount:                    1,
		StopHookActive:               nil,
		Transition:                   nil,
		TotalTurnTokens:              0,
		BudgetContinuationCount:      0,
		LastBudgetCheckTokens:        0,
		LastBudgetDelta:              0,
		Iterations:                   0,
		MaxOutputTokensRecoveryCount: 0,
		HasAttemptedReactiveCompact:  false,
		MaxOutputTokensOverride:      0,
		ContinuationNudgeCount:       0,
		Compacted:                    false,
		Usage:                        nil,
		StopReason:                   "",
		RecoveryContext:              nil,
		PermissionContext:            nil,
		PermissionMode:               "",
	}
}

// Clone creates a deep copy of the state
func (s *MutableState) Clone() *MutableState {
	// Copy messages
	messages := make([]types.Message, len(s.Messages))
	copy(messages, s.Messages)

	// Copy tool uses
	toolUses := make([]types.ToolUseContent, len(s.ToolUses))
	copy(toolUses, s.ToolUses)

	// Copy tool results
	toolResults := make([]tool.CallResult, len(s.ToolResults))
	copy(toolResults, s.ToolResults)

	return &MutableState{
		Messages:                     messages,
		ToolUses:                     toolUses,
		ToolResults:                  toolResults,
		DiscoveredDeferred:           append([]string(nil), s.DiscoveredDeferred...),
		Usage:                        s.Usage,
		StopReason:                   s.StopReason,
		Compacted:                    s.Compacted,
		Iterations:                   s.Iterations,
		TurnCount:                    s.TurnCount,
		MaxOutputTokensRecoveryCount: s.MaxOutputTokensRecoveryCount,
		HasAttemptedReactiveCompact:  s.HasAttemptedReactiveCompact,
		MaxOutputTokensOverride:      s.MaxOutputTokensOverride,
		ContinuationNudgeCount:       s.ContinuationNudgeCount,
		Transition:                   s.Transition,
		StopHookActive:               s.StopHookActive,
		TotalTurnTokens:              s.TotalTurnTokens,
		BudgetContinuationCount:      s.BudgetContinuationCount,
		LastBudgetCheckTokens:        s.LastBudgetCheckTokens,
		LastBudgetDelta:              s.LastBudgetDelta,
		RecoveryContext:              s.RecoveryContext,
		CurrentStage:                 s.CurrentStage,
		PermissionContext:            clonePermissionContext(s.PermissionContext),
		PermissionMode:               s.PermissionMode,
	}
}

// ================================================================================
// Extended Session State Types (Slice 1: State Kernel + Prompt Caching)
// ================================================================================

// PromptSectionCache mémoïse les sections stables du prompt
// Based on OpenClaude's memoized system prompt sections
type PromptSectionCache struct {
	mu      sync.RWMutex
	cache   map[string]*CachedSection
	version CacheVersion
}

// CachedSection represents a cached prompt section
type CachedSection struct {
	Content       string
	ToolHash      string
	CachedAt      time.Time
	LastValidated time.Time
}

// CacheVersion represents versioning information for cache invalidation
type CacheVersion struct {
	ToolNames   []string
	ModelConfig string
	Environment map[string]string
}

// DenialTrackingState suit les refus et fallback-to-prompting
// Based on OpenClaude's denial tracking system
type DenialTrackingState struct {
	mu                     sync.RWMutex
	consecutiveToolDenials int
	totalToolDenials       int
	lastDenialTime         time.Time
	promptModeEngaged      bool
	fallbackTriggered      bool
}

// RecoveryContext tracks details needed to properly resume after interruption
// This enables richer session reconstruction post-compaction
type RecoveryContext struct {
	// LastTransitionReason explains why the loop stopped/continued
	LastTransitionReason string

	// LastRecoveryType is the type of recovery that occurred
	LastRecoveryType RecoveryType

	// LastStopReason is why the loop stopped (if terminal)
	LastStopReason string

	// CompactionSnapshot captures compaction state at interruption point
	CompactionSnapshot *CompactionSnapshot

	// TurnProgress captures turn execution state at interruption point
	TurnProgress *TurnProgress
}

// CompactionSnapshot captures the state of compaction at interruption
type CompactionSnapshot struct {
	// PreCompactionTokenCount tokens before last compaction
	PreCompactionTokenCount int

	// PostCompactionTokenCount tokens after last compaction
	PostCompactionTokenCount int

	// FirstPreservedMessageID marks the first message preserved in tail
	FirstPreservedMessageID types.MessageID

	// LastPreservedMessageID marks the last message preserved in tail
	LastPreservedMessageID types.MessageID

	// PreservedTailHash hash of preserved tail for integrity
	PreservedTailHash string

	// BoundaryVersion compaction boundary version
	BoundaryVersion int
}

// TurnProgress captures the turn execution state at interruption
type TurnProgress struct {
	// IterationsCompleted number of LLM calls completed in this turn
	IterationsCompleted int

	// LastAssistantMessageID the last assistant message before interruption
	LastAssistantMessageID types.MessageID

	// PendingToolUses tool uses that haven't been executed yet
	PendingToolUses []types.ToolUseContent

	// PendingToolResults tool results that haven't been processed yet
	PendingToolResults []tool.CallResult

	// TotalTokensUsed total tokens consumed in this turn so far
	TotalTokensUsed int
}

// ContentReplacementState suit les remplacements de contenu
// Based on OpenClaude's content replacement tracking
type ContentReplacementState struct {
	mu           sync.RWMutex
	replacements map[string]ContentReplacementRecord
}

// ContentReplacementRecord represents a single content replacement
type ContentReplacementRecord struct {
	ToolUseID   string
	Original    string
	Replacement string
	ReplacedAt  time.Time
}

// CompactBoundaryTracking suit les boundaries de compaction
// Based on OpenClaude's compact boundary tracking
type CompactBoundaryTracking struct {
	mu                sync.RWMutex
	boundaries        []CompactBoundary
	lastBoundaryIndex int
}

// CompactBoundary represents a single compaction boundary
type CompactBoundary struct {
	TurnNumber   int
	MessageIndex int
	Kind         string
	TargetTokens int
}

// ActivityTracking suit l'activité session
type ActivityTracking struct {
	mu             sync.RWMutex
	lastActiveTime time.Time
	totalTurns     int
	totalTokens    int
	activeDuration time.Duration
}

// NewPromptSectionCache creates a new prompt section cache
func NewPromptSectionCache() *PromptSectionCache {
	return &PromptSectionCache{
		cache: make(map[string]*CachedSection),
	}
}

// NewDenialTrackingState creates a new denial tracking state
func NewDenialTrackingState() *DenialTrackingState {
	return &DenialTrackingState{
		consecutiveToolDenials: 0,
		totalToolDenials:       0,
		promptModeEngaged:      false,
		fallbackTriggered:      false,
	}
}

// NewContentReplacementState creates a new content replacement state
func NewContentReplacementState() *ContentReplacementState {
	return &ContentReplacementState{
		replacements: make(map[string]ContentReplacementRecord),
	}
}

// NewCompactBoundaryTracking creates a new compact boundary tracking
func NewCompactBoundaryTracking() *CompactBoundaryTracking {
	return &CompactBoundaryTracking{
		boundaries:        make([]CompactBoundary, 0),
		lastBoundaryIndex: -1,
	}
}

// NewActivityTracking creates a new activity tracking
func NewActivityTracking() *ActivityTracking {
	return &ActivityTracking{
		lastActiveTime: time.Now(),
		totalTurns:     0,
		totalTokens:    0,
		activeDuration: 0,
	}
}

// ================================================================================
// Cache Key Generation (Deterministic)
// ================================================================================

// BuildCacheKey construit une clé de cache déterministe
// This ensures cache stability when inputs are identical
func BuildCacheKey(
	sectionNames []string,
	tools map[string]tool.Tool,
	model types.ModelIdentifier,
	env map[string]string,
) CacheKey {
	// Tri pour déterminisme
	sortedNames := make([]string, len(sectionNames))
	copy(sortedNames, sectionNames)
	sort.Strings(sortedNames)

	// Hash des outils
	toolHash := HashTools(tools)

	// Config modèle
	modelConfig := model.String()

	return CacheKey{
		SectionNames: sortedNames,
		ToolHash:     toolHash,
		ModelConfig:  modelConfig,
		Environment:  env,
	}
}

// HashTools construit un hash déterministe des outils
func HashTools(tools map[string]tool.Tool) string {
	toolNames := make([]string, 0, len(tools))
	for name := range tools {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	combined := strings.Join(toolNames, "|")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// ================================================================================
// Cache Key Type
// ================================================================================

// CacheKey represents a deterministic cache key
type CacheKey struct {
	SectionNames []string
	ToolHash     string
	ModelConfig  string
	Environment  map[string]string
}

// Hash returns a string hash of the cache key
func (k CacheKey) Hash() string {
	// Build a deterministic string representation
	var builder strings.Builder

	// Section names (already sorted)
	builder.WriteString("sections:")
	for _, name := range k.SectionNames {
		builder.WriteString(name)
		builder.WriteString(",")
	}
	builder.WriteString(";")

	// Tool hash
	builder.WriteString("tools:")
	builder.WriteString(k.ToolHash)
	builder.WriteString(";")

	// Model config
	builder.WriteString("model:")
	builder.WriteString(k.ModelConfig)
	builder.WriteString(";")

	// Environment (sorted)
	if k.Environment != nil {
		envKeys := make([]string, 0, len(k.Environment))
		for key := range k.Environment {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)

		builder.WriteString("env:")
		for _, key := range envKeys {
			builder.WriteString(fmt.Sprintf("%s=%s", key, k.Environment[key]))
			builder.WriteString(",")
		}
	}

	// Hash the final string
	hash := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}
