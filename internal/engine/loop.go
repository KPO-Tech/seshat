package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/hooks"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	providerretry "github.com/EngineerProjects/nexus-engine/internal/providers/retry"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	"github.com/EngineerProjects/nexus-engine/internal/schema"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	toolsearch "github.com/EngineerProjects/nexus-engine/internal/tools/special/tool_search"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Loop represents the main query loop state machine.
type Loop struct {
	apiClient              *providers.Client
	activeFallbackClient   *providers.Client
	activeBaseModel        *types.ModelIdentifier
	activeStreamingTools   *streamingToolCoordinator
	callModelFn            func(ctx context.Context, state *MutableState, req RunRequest) (*types.APIResponse, error)
	sendAPIStreamRequestFn func(ctx context.Context, apiReq types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIResponse, error)
	sendAPIRequestFn       func(ctx context.Context, apiReq types.APIRequest) (*types.APIResponse, error)
	transitionObserver     func(Transition)
	orchestrator           *execution.Orchestrator
	compactor              compact.CompactionStrategy
	promptAssembler        *prompt.Assembler
	permissionIntegrator   *permissions.Integrator
	hookExecutor           *hooks.Executor
	config                 *LoopConfig
	// providerConfig holds provider routing and fallback configuration
	providerConfig       *providers.Config
	currentFallbackIndex int
	// monitoring provides centralized metrics and logging
	monitoring *monitoring.System
}

// LoopConfig represents the loop configuration.
type LoopConfig struct {
	MaxIterations                int        `json:"max_iterations"`
	AutoCompact                  bool       `json:"auto_compact"`
	MaxTurns                     int        `json:"max_turns"`
	EnableStreaming              bool       `json:"enable_streaming"`
	MaxOutputTokensRecoveryLimit int        `json:"max_output_tokens_recovery_limit"`
	ContinuationNudgeLimit       int        `json:"continuation_nudge_limit"`
	TurnTokenBudget              int        `json:"turn_token_budget,omitempty"`
	BudgetContinuationLimit      int        `json:"budget_continuation_limit,omitempty"`
	BudgetCompletionThreshold    float64    `json:"budget_completion_threshold,omitempty"`
	BudgetDiminishingThreshold   int        `json:"budget_diminishing_threshold,omitempty"`
	StopHooks                    []StopHook `json:"-"`
	// StopHookConfig controls stop hook execution behavior
	StopHookTimeout         int    `json:"stop_hook_timeout,omitempty"`           // milliseconds, 0 = no timeout
	StopHookMode            string `json:"stop_hook_mode,omitempty"`              // "first" (default) or "all"
	StopHookContinueOnError bool   `json:"stop_hook_continue_on_error,omitempty"` // continue if hook errors
	// Additional recovery options
	MaxRecoveryAttempts       int `json:"max_recovery_attempts,omitempty"`       // total recovery attempts per turn
	RecoveryBackoffMultiplier int `json:"recovery_backoff_multiplier,omitempty"` // milliseconds
}

// DefaultLoopConfig returns default loop configuration.
//
// MaxIterations is the number of model-call rounds the engine executes within a
// single RunPrompt/StreamPrompt call. Each iteration is one LLM call plus any
// tool executions that follow. Increasing this allows complex autonomous tasks
// (many sequential tool-use rounds) without requiring multiple user messages.
//
// MaxTurns is the session-level turn counter limit across all RunPrompt calls.
func DefaultLoopConfig() *LoopConfig {
	return normalizeLoopConfig(&LoopConfig{
		MaxIterations:                100,
		AutoCompact:                  true,
		MaxTurns:                     200,
		EnableStreaming:              true,
		MaxOutputTokensRecoveryLimit: 3,
		ContinuationNudgeLimit:       3,
	})
}

func normalizeLoopConfig(config *LoopConfig) *LoopConfig {
	if config == nil {
		config = &LoopConfig{}
	}
	if config.MaxIterations <= 0 {
		config.MaxIterations = 100
	}
	if config.MaxTurns <= 0 {
		config.MaxTurns = 200
	}
	if !config.EnableStreaming {
		config.EnableStreaming = true
	}
	if config.MaxOutputTokensRecoveryLimit <= 0 {
		config.MaxOutputTokensRecoveryLimit = 3
	}
	if config.ContinuationNudgeLimit <= 0 {
		config.ContinuationNudgeLimit = 3
	}
	if config.BudgetContinuationLimit <= 0 {
		config.BudgetContinuationLimit = defaultBudgetContinuationLimit
	}
	if config.BudgetCompletionThreshold <= 0 {
		config.BudgetCompletionThreshold = defaultBudgetCompletionThreshold
	}
	if config.BudgetDiminishingThreshold <= 0 {
		config.BudgetDiminishingThreshold = defaultBudgetDiminishingTokens
	}
	return config
}

// NewLoop creates a new query loop.
func NewLoop(
	apiClient *providers.Client,
	orchestrator *execution.Orchestrator,
	compactor compact.CompactionStrategy,
	promptAssembler *prompt.Assembler,
	permissionIntegrator *permissions.Integrator,
	hookExecutor *hooks.Executor,
	config *LoopConfig,
	monitoringSys *monitoring.System,
	providerConfigs ...*providers.Config,
) *Loop {
	loop := &Loop{
		apiClient:            apiClient,
		orchestrator:         orchestrator,
		compactor:            compactor,
		promptAssembler:      promptAssembler,
		permissionIntegrator: permissionIntegrator,
		hookExecutor:         hookExecutor,
		config:               normalizeLoopConfig(config),
		monitoring:           monitoringSys,
		currentFallbackIndex: -1,
	}
	if len(providerConfigs) > 0 && providerConfigs[0] != nil {
		loop.providerConfig = providerConfigs[0]
	}
	return loop
}

// RunRequest represents a request to run the query loop.
type RunRequest struct {
	Messages           []types.Message           `json:"messages"`
	SystemPrompt       string                    `json:"system_prompt"`
	SystemPromptBlocks []types.SystemPromptBlock `json:"system_prompt_blocks,omitempty"`
	ProviderTools      []types.APIToolDefinition `json:"tools,omitempty"`
	Tools              map[string]tool.Tool      `json:"-"`
	// Toolsets are resolved dynamically at the start of each iteration.
	// Their tools are merged into Tools each iteration (Toolset wins on name collision).
	Toolsets            []tool.Toolset                                                                                                                      `json:"-"`
	ToolRegistry        *tool.Registry                                                                                                                      `json:"-"`
	RefreshSystemPrompt func(ctx context.Context, tools map[string]tool.Tool, pendingDeferred []string, stage prompt.ExecutionStage) (PromptRefresh, error) `json:"-"`
	// AutoDetectStage enables per-iteration stage detection. The engine sets this
	// when no static PromptStage is configured so that stage overlays fire
	// automatically without manual configuration.
	AutoDetectStage       bool                         `json:"-"`
	SessionID             types.SessionID              `json:"session_id"`
	TurnID                types.TurnID                 `json:"turn_id"`
	WorkingDirectory      string                       `json:"working_directory,omitempty"`
	PermissionMode        types.PermissionMode         `json:"permission_mode"`
	PermissionContext     *types.PermissionContext     `json:"-"`
	PermissionCheck       types.CanUseToolFn           `json:"-"`
	DenialTracking        *types.DenialTrackingState   `json:"-"`
	Model                 types.ModelIdentifier        `json:"model"`
	MaxTokens             int                          `json:"max_tokens"`
	ProgressCallback      func(types.ToolProgress)     `json:"-"`
	ResponseChunkCallback func(types.APIResponseChunk) `json:"-"`
	// EventQueue receives every streaming chunk emitted during this turn.
	// Callers read from EventQueue.Recv() in a separate goroutine to avoid
	// blocking the loop. Overflow is counted but never blocks.
	EventQueue      *execution.EventQueue `json:"-"`
	TurnTokenBudget int                   `json:"turn_token_budget,omitempty"`

	// OutputSchema, when set, constrains the model to return JSON matching the
	// schema. Passed through to types.APIRequest.OutputSchema.
	OutputSchema *schema.StructuredOutputInfo `json:"-"`
}

// RunResult represents the result of running the loop.
type RunResult struct {
	Messages           []types.Message          `json:"messages"`
	StopReason         string                   `json:"stop_reason"`
	ToolUses           []types.ToolUseContent   `json:"tool_uses"`
	ToolResults        []tool.CallResult        `json:"tool_results"`
	Usage              *types.TokenUsage        `json:"usage"`
	PermissionContext  *types.PermissionContext `json:"-"`
	Compacted          bool                     `json:"compacted"`
	Iterations         int                      `json:"iterations"`
	DiscoveredDeferred []string                 `json:"discovered_deferred,omitempty"`
	Error              error                    `json:"error,omitempty"`
	// RecoveryContext captures the turn execution state at loop exit.
	// Persisted by the engine into session metadata so that interrupted or
	// errored sessions can expose what was in-flight at the time of failure.
	RecoveryContext *RecoveryContext `json:"-"`
}

type PromptRefresh struct {
	SystemPrompt       string
	SystemPromptBlocks []types.SystemPromptBlock
}

type toolDiscoveryUpdate struct {
	HadToolSearch bool
	Discovered    []string
	Unresolved    []string
	Malformed     bool
	RefreshFailed error
}

// Run executes the main query loop.
func (l *Loop) Run(ctx context.Context, req RunRequest) RunResult {
	startTime := time.Now()
	l.currentFallbackIndex = -1
	l.activeFallbackClient = nil
	l.activeBaseModel = nil
	l.activeStreamingTools = nil
	req = normalizeRunRequestPermissionContext(req)
	state := l.initializeState(req)

	// Record query start in monitoring
	if l.monitoring != nil {
		l.monitoring.RecordQueryTurn()
	}

	// Execute query start hook
	l.executeHook(ctx, types.HookEventQueryStart, map[string]any{
		"session_id":      req.SessionID,
		"turn_id":         req.TurnID,
		"permission_mode": req.PermissionMode,
		"model":           req.Model,
		"max_tokens":      req.MaxTokens,
	})

	for state.Iterations < l.config.MaxIterations {
		select {
		case <-ctx.Done():
			return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: ctx.Err(), RecoveryContext: state.RecoveryContext}
		default:
		}

		state.Iterations++

		// Record iteration in monitoring
		if l.monitoring != nil {
			l.monitoring.RecordQueryIteration()
		}

		// When AutoDetectStage is enabled, detect the current stage from accumulated
		// messages and refresh the system prompt when it changes. This injects the
		// right stage overlay (tool_result, continuation, …) per iteration without
		// requiring callers to configure PromptStage manually.
		if req.AutoDetectStage && req.RefreshSystemPrompt != nil {
			detectedStage := DetectTurnStage(state.Messages, req.TurnID)
			if detectedStage != state.CurrentStage {
				state.CurrentStage = detectedStage
				refresh, refreshErr := req.RefreshSystemPrompt(ctx, req.Tools, remainingDeferredToolNames(req.ToolRegistry, state.DiscoveredDeferred), detectedStage)
				if refreshErr == nil {
					req.SystemPrompt = refresh.SystemPrompt
					req.SystemPromptBlocks = append([]types.SystemPromptBlock(nil), refresh.SystemPromptBlocks...)
				}
			}
		}

		// Execute iteration start hook
		if l.executeHook(ctx, types.HookEventIterationStart, map[string]any{
			"iteration":     state.Iterations,
			"session_id":    req.SessionID,
			"turn_id":       req.TurnID,
			"message_count": len(state.Messages),
			"total_tokens":  state.TotalTurnTokens,
		}) {
			l.setTransition(state, Terminate("hook_stop", types.StopReasonEndTurn))
			break
		}

		l.prepareMessagesForIteration(state)
		l.resolveToolsets(ctx, &req)

		if err := l.maybeAutoCompact(ctx, state, req); err != nil {
			// Record error in monitoring
			if l.monitoring != nil {
				l.monitoring.RecordQueryFailure(err, time.Since(startTime))
			}
			l.setTransition(state, Terminate("auto_compact_error", types.StopReasonEndTurn))
			return RunResult{
				Messages:        state.Messages,
				StopReason:      types.StopReasonEndTurn,
				Iterations:      state.Iterations,
				Error:           err,
				RecoveryContext: state.RecoveryContext,
			}
		}

		apiResp, err := l.callModel(ctx, state, req)
		if err != nil {
			// Record error in monitoring
			if l.monitoring != nil {
				l.monitoring.RecordQueryFailure(err, time.Since(startTime))
			}
			if l.isRecoverableError(err) {
				recovered, recoveryErr := l.tryRecovery(ctx, state, req, err)
				if recoveryErr != nil {
					l.setTransition(state, Terminate("recovery_error", types.StopReasonEndTurn))
					return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: fmt.Errorf("recovery failed: %w", recoveryErr), RecoveryContext: state.RecoveryContext}
				}
				if recovered {
					continue
				}
			}
			l.setTransition(state, Terminate("api_error_terminal", types.StopReasonEndTurn))
			return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: err, RecoveryContext: state.RecoveryContext}
		}

		l.updateUsage(state, &apiResp.Usage)
		assistantMessage := buildAssistantMessage(apiResp, req.TurnID, len(state.Messages))
		state.Messages = append(state.Messages, assistantMessage)

		toolUses := l.extractToolUses(apiResp.Content)
		if len(toolUses) > 0 {
			// Execute tool uses start hook
			if l.executeHook(ctx, types.HookEventToolUsesStart, map[string]any{
				"iteration":  state.Iterations,
				"tool_count": len(toolUses),
				"session_id": req.SessionID,
				"turn_id":    req.TurnID,
			}) {
				l.setTransition(state, Terminate("hook_stop", types.StopReasonEndTurn))
				break
			}

			state.ToolUses = append(state.ToolUses, toolUses...)

			transcriptBeforeCurrentAction := append([]types.Message(nil), state.Messages[:len(state.Messages)-1]...)
			execResult, execErr := l.executeToolsForResponse(ctx, toolUses, req, state, transcriptBeforeCurrentAction)

			if execErr != nil {
				l.setTransition(state, Terminate("tool_execution_error", types.StopReasonEndTurn))
				return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: fmt.Errorf("tool execution failed: %w", execErr), RecoveryContext: state.RecoveryContext}
			}

			state.PermissionContext = permissionContextFromToolContext(state.PermissionContext, execResult.FinalToolContext)
			if state.PermissionContext != nil {
				state.PermissionMode = state.PermissionContext.Mode
			}

			state.Messages = append(state.Messages, execResult.Messages...)
			state.ToolResults = append(state.ToolResults, execResult.Results...)

			// Execute tool uses complete hook
			l.executeHook(ctx, types.HookEventToolUsesComplete, map[string]any{
				"iteration":      state.Iterations,
				"tool_count":     len(toolUses),
				"success_count":  len(execResult.Results),
				"error_count":    len(execResult.Errors),
				"session_id":     req.SessionID,
				"turn_id":        req.TurnID,
				"total_duration": execResult.TotalDuration,
			})

			discovery := l.integrateDiscoveredDeferredTools(ctx, state, &req, toolUses, execResult.Results)
			if recoveryMessage := toolLoopRecoveryMessage(discovery); recoveryMessage != "" {
				state.Messages = append(state.Messages, newContinuationMessage(req.TurnID, recoveryMessage))
				state.ContinuationNudgeCount = 0
				state.MaxOutputTokensRecoveryCount = 0
				l.setTransition(state, ContinueWithRecovery("tool_loop_recovery", RecoveryTypeContinuationNudge))
				continue
			}
			state.ContinuationNudgeCount = 0
			state.MaxOutputTokensRecoveryCount = 0
			l.setTransition(state, Continue("tool_results_appended"))
			continue
		}

		if l.shouldStop(apiResp.StopReason) {
			// Execute iteration stop hook
			l.executeHook(ctx, types.HookEventIterationStop, map[string]any{
				"iteration":    state.Iterations,
				"stop_reason":  apiResp.StopReason,
				"session_id":   req.SessionID,
				"turn_id":      req.TurnID,
				"total_tokens": state.TotalTurnTokens,
			})

			continued, stopErr := l.handleTerminalStop(ctx, state, req, apiResp.StopReason, assistantMessage, toolUses)
			if stopErr != nil {
				l.setTransition(state, Terminate("stop_hook_error", types.StopReasonEndTurn))
				return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: stopErr, RecoveryContext: state.RecoveryContext}
			}
			if continued {
				continue
			}
			break
		}

		continued, noToolErr := l.handleNoToolUses(ctx, state, req, assistantMessage)
		if noToolErr != nil {
			l.setTransition(state, Terminate("stop_hook_error", types.StopReasonEndTurn))
			return RunResult{Messages: state.Messages, StopReason: types.StopReasonEndTurn, Iterations: state.Iterations, Error: noToolErr, RecoveryContext: state.RecoveryContext}
		}
		if continued {
			// Execute iteration continue hook
			l.executeHook(ctx, types.HookEventIterationContinue, map[string]any{
				"iteration":     state.Iterations,
				"continue_type": "no_tool_uses",
				"session_id":    req.SessionID,
				"turn_id":       req.TurnID,
			})
			continue
		}

		// Execute iteration complete hook
		l.executeHook(ctx, types.HookEventIterationComplete, map[string]any{
			"iteration":    state.Iterations,
			"session_id":   req.SessionID,
			"turn_id":      req.TurnID,
			"total_tokens": state.TotalTurnTokens,
		})

		state.StopReason = types.StopReasonEndTurn
		break
	}

	if state.StopReason == "" && state.Iterations >= l.config.MaxIterations {
		state.StopReason = types.StopReasonEndTurn
		l.setTransition(state, Terminate("max_iterations_reached", types.StopReasonEndTurn))
	}

	// Execute query complete hook
	l.executeHook(ctx, types.HookEventQueryComplete, map[string]any{
		"session_id":         req.SessionID,
		"turn_id":            req.TurnID,
		"total_iterations":   state.Iterations,
		"stop_reason":        state.StopReason,
		"total_tokens":       state.TotalTurnTokens,
		"compacted":          state.Compacted,
		"tool_uses_count":    len(state.ToolUses),
		"tool_results_count": len(state.ToolResults),
	})

	// Record query success in monitoring (assume success if we reach this point)
	if l.monitoring != nil {
		l.monitoring.RecordQuerySuccess(time.Since(startTime))
	}

	return RunResult{
		Messages:           state.Messages,
		StopReason:         terminalStopReason(state.StopReason),
		ToolUses:           state.ToolUses,
		ToolResults:        state.ToolResults,
		Usage:              state.Usage,
		PermissionContext:  clonePermissionContext(state.PermissionContext),
		Compacted:          state.Compacted,
		Iterations:         state.Iterations,
		DiscoveredDeferred: append([]string(nil), state.DiscoveredDeferred...),
		RecoveryContext:    state.RecoveryContext,
	}
}

func (l *Loop) setTransition(state *MutableState, transition Transition) {
	state.Transition = transition
	if l.transitionObserver != nil {
		l.transitionObserver(transition)
	}

	// Capture recovery context for richer resume
	state.RecoveryContext = l.captureRecoveryContext(state, transition)
}

func (l *Loop) captureRecoveryContext(state *MutableState, transition Transition) *RecoveryContext {
	ctx := &RecoveryContext{
		TurnProgress: &TurnProgress{
			IterationsCompleted: state.Iterations,
			TotalTokensUsed:     state.TotalTurnTokens,
		},
	}

	// Extract details from transition
	if cont, ok := transition.(ContinueTransition); ok {
		ctx.LastTransitionReason = cont.Reason
		if cont.RecoveryType != "" {
			ctx.LastRecoveryType = cont.RecoveryType
		}
	} else if term, ok := transition.(TerminalTransition); ok {
		ctx.LastTransitionReason = term.Reason
		ctx.LastStopReason = state.StopReason
	}

	// Capture compaction snapshot if present in metadata
	if len(state.Messages) > 0 {
		lastMsg := state.Messages[len(state.Messages)-1]
		if lastMsg.Metadata != nil && lastMsg.Metadata.Compaction != nil {
			comp := lastMsg.Metadata.Compaction
			ctx.CompactionSnapshot = &CompactionSnapshot{
				PreCompactionTokenCount:  comp.PreCompactTokens,
				PostCompactionTokenCount: comp.PostCompactTokens,
				FirstPreservedMessageID:  comp.FirstPreservedMessageID,
				LastPreservedMessageID:   comp.LastPreservedMessageID,
				PreservedTailHash:        comp.PreservedTailHash,
				BoundaryVersion:          comp.BoundaryVersion,
			}
		}
	}

	// Capture last assistant message for resume
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == types.RoleAssistant {
			ctx.TurnProgress.LastAssistantMessageID = state.Messages[i].ID
			break
		}
	}

	// Capture pending tool uses/results
	ctx.TurnProgress.PendingToolUses = make([]types.ToolUseContent, len(state.ToolUses))
	copy(ctx.TurnProgress.PendingToolUses, state.ToolUses)
	ctx.TurnProgress.PendingToolResults = make([]tool.CallResult, len(state.ToolResults))
	copy(ctx.TurnProgress.PendingToolResults, state.ToolResults)

	return ctx
}

// shouldNudgeContinuation reports whether the loop should inject a continuation
// message after a response that used no tools.
//
// The nudge is warranted only when:
//   - the model already executed at least one tool this turn (priorToolUseCount > 0),
//     meaning it was in the middle of active work when it stopped, AND
//   - the response contains an explicit forward-looking signal that confirms the
//     model considers itself mid-work.
//
// When priorToolUseCount == 0 the model gave a direct answer with no tool use;
// nudging in that case produces spurious continuation loops on plain responses
// like "Let me know if you have questions."
func (l *Loop) shouldNudgeContinuation(message types.Message, priorToolUseCount int) bool {
	if priorToolUseCount == 0 {
		return false
	}

	text := assistantTextContent(message)
	if text == "" {
		return false
	}

	// Explicit finish signals suppress the nudge even when the model had tool uses.
	for _, done := range []string{
		"done", "finished", "complete", "all set", "that's all",
		"there you go", "let me know", "feel free", "hope that",
		"happy to", "if you need", "summary", "in summary",
	} {
		if strings.Contains(text, done) {
			return false
		}
	}

	// Conservative forward-looking signals: only nudge when the model explicitly
	// signals it was mid-work and intended to continue.
	for _, signal := range []string{
		"let me continue", "i'll continue", "i will continue",
		"next i'll", "next i will", "next step",
		"moving on", "continuing with", "proceeding to",
		"i'll now", "i will now",
	} {
		if strings.Contains(text, signal) {
			return true
		}
	}

	return false
}

func assistantTextContent(message types.Message) string {
	if message.Role != types.RoleAssistant {
		return ""
	}
	var builder strings.Builder
	for _, block := range message.Content {
		if text, ok := block.(types.TextContent); ok {
			trimmed := strings.TrimSpace(text.Text)
			if trimmed == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(strings.ToLower(trimmed))
		}
	}
	return builder.String()
}

func (l *Loop) prepareMessagesForIteration(state *MutableState) {
	if state == nil || len(state.Messages) == 0 {
		return
	}
	state.Messages = normalizeLeadingContinuationArtifacts(state.Messages)
}

func normalizeLeadingContinuationArtifacts(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}
	trimmed := append([]types.Message(nil), messages...)
	for len(trimmed) > 1 && isEmptyContinuationArtifact(trimmed[len(trimmed)-1]) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

func isEmptyContinuationArtifact(message types.Message) bool {
	if message.Role != types.RoleUser {
		return false
	}
	if len(message.Content) == 0 {
		return true
	}
	for _, block := range message.Content {
		switch content := block.(type) {
		case types.TextContent:
			if strings.TrimSpace(content.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func flattenMessageText(message types.Message) string {
	var builder strings.Builder
	for _, block := range message.Content {
		if text, ok := block.(types.TextContent); ok {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text.Text)
		}
	}
	return builder.String()
}

// resolveToolsets evaluates every Toolset in req and merges the returned tools
// into req.Tools. Toolset-provided tools take precedence over statically
// registered ones, allowing context-aware overrides per iteration.
// Provider tool definitions are rebuilt when any toolset returns tools.
func (l *Loop) resolveToolsets(ctx context.Context, req *RunRequest) {
	if len(req.Toolsets) == 0 {
		return
	}
	added := false
	if req.Tools == nil {
		req.Tools = make(map[string]tool.Tool)
	}
	for _, ts := range req.Toolsets {
		for _, t := range ts.Tools(ctx) {
			if t == nil || !t.IsEnabled() {
				continue
			}
			req.Tools[t.Definition().Name] = t
			added = true
		}
	}
	if added {
		req.ProviderTools = prompt.BuildProviderToolDefinitions(req.Tools)
	}
}

func (l *Loop) initializeState(req RunRequest) *MutableState {
	state := NewMutableState(req.Messages)
	state.PermissionContext = clonePermissionContext(req.PermissionContext)
	state.PermissionMode = req.PermissionMode
	state.Compacted = false
	return state
}

func (l *Loop) maybeAutoCompact(ctx context.Context, state *MutableState, req RunRequest) error {
	if !l.config.AutoCompact {
		return nil
	}
	result, err := l.compactor.AutoCompact(
		ctx,
		req.SystemPrompt,
		state.Messages,
		req.Model,
		req.SessionID,
		req.TurnID,
		&compact.TrackingState{ConsecutiveFailures: state.AutoCompactFailureCount},
	)
	if err != nil {
		state.AutoCompactFailureCount = result.ConsecutiveFailures
		return fmt.Errorf("auto-compact failed: %w", err)
	}
	state.AutoCompactFailureCount = result.ConsecutiveFailures
	if result.DidCompact {
		state.Messages = result.Messages
		state.Compacted = true
	}
	return nil
}

// buildChunkCallback returns a combined callback that forwards each streaming
// chunk to both the host callback (if set) and the EventQueue (if set).
func buildChunkCallback(cb func(types.APIResponseChunk), queue *execution.EventQueue) func(types.APIResponseChunk) {
	if cb == nil && queue == nil {
		return nil
	}
	return func(chunk types.APIResponseChunk) {
		if cb != nil {
			cb(chunk)
		}
		if queue != nil {
			queue.Emit(chunk)
		}
	}
}

func (l *Loop) callModel(ctx context.Context, state *MutableState, req RunRequest) (*types.APIResponse, error) {
	if l.callModelFn != nil {
		return l.callModelFn(ctx, state, req)
	}
	model := l.getFallbackModel(req.Model)
	chunkFn := buildChunkCallback(req.ResponseChunkCallback, req.EventQueue)
	l.activeStreamingTools.discard() // release any goroutines from a previous iteration
	l.activeStreamingTools = nil
	if l.config.EnableStreaming {
		if coordinator := newStreamingToolCoordinator(ctx, l, req, state, append([]types.Message(nil), state.Messages...)); coordinator != nil {
			l.activeStreamingTools = coordinator
			baseChunkFn := chunkFn
			chunkFn = func(chunk types.APIResponseChunk) {
				if baseChunkFn != nil {
					baseChunkFn(chunk)
				}
				coordinator.observe(chunk)
			}
		}
	}
	resp, err := l.sendAPIRequestWithClient(ctx, l.effectiveAPIClient(), l.buildAPIRequest(state, req, model), chunkFn)
	if err != nil {
		// Discard any in-flight pre-streamed tool goroutines before falling back.
		l.activeStreamingTools.discard()
		l.activeStreamingTools = nil
		return l.tryFallbackModel(ctx, state, req, err, model)
	}
	if breaker := l.circuitBreaker(); breaker != nil {
		breaker.RecordSuccess()
	}
	return resp, nil
}

// getFallbackModel returns the current fallback model or the original model
func (l *Loop) getFallbackModel(original types.ModelIdentifier) types.ModelIdentifier {
	base := original
	if l.activeBaseModel != nil {
		base = *l.activeBaseModel
	}
	providerConfig := l.effectiveProviderConfig()
	if providerConfig == nil || providerConfig.Routing == nil {
		return base
	}
	if l.currentFallbackIndex >= 0 && l.currentFallbackIndex < len(providerConfig.Routing.FallbackModels) {
		return providerConfig.Routing.FallbackModels[l.currentFallbackIndex]
	}
	return base
}

// tryFallbackModel attempts to use a fallback model if available.
func (l *Loop) tryFallbackModel(
	ctx context.Context,
	state *MutableState,
	req RunRequest,
	err error,
	attemptedModel types.ModelIdentifier,
) (*types.APIResponse, error) {
	providerConfig := l.effectiveProviderConfig()
	if err == nil || providerConfig == nil || providerConfig.Routing == nil {
		return nil, err
	}

	breaker := l.circuitBreaker()
	if breaker != nil {
		breaker.RecordFailure()
		if !breaker.IsAvailable() {
			return nil, err
		}
	}

	if !l.isRecoverableError(err) {
		return nil, err
	}

	lastErr := err
	chunkFn := buildChunkCallback(req.ResponseChunkCallback, req.EventQueue)
	fallbacks := providerConfig.Routing.FallbackModels
	for idx, candidate := range fallbacks {
		if idx <= l.currentFallbackIndex || sameModelIdentifier(candidate, attemptedModel) {
			continue
		}

		resp, fallbackErr := l.sendAPIRequestWithClient(ctx, l.effectiveAPIClient(), l.buildAPIRequest(state, req, candidate), chunkFn)
		if fallbackErr == nil {
			l.currentFallbackIndex = idx
			if breaker != nil {
				breaker.RecordSuccess()
			}
			return resp, nil
		}

		lastErr = fallbackErr
		if breaker != nil {
			breaker.RecordFailure()
		}
		if !l.isRecoverableError(fallbackErr) {
			return nil, lastErr
		}
	}

	providerResp, providerErr := l.tryFallbackProvider(ctx, state, req, lastErr)
	if providerErr == nil {
		if breaker != nil {
			breaker.RecordSuccess()
		}
		return providerResp, nil
	}

	lastErr = providerErr
	return nil, lastErr
}

func (l *Loop) buildAPIRequest(state *MutableState, req RunRequest, model types.ModelIdentifier) types.APIRequest {
	return types.APIRequest{
		Model:              model,
		Messages:           state.Messages,
		MaxTokens:          req.MaxTokens,
		SystemPrompt:       req.SystemPrompt,
		SystemPromptBlocks: append([]types.SystemPromptBlock(nil), req.SystemPromptBlocks...),
		Tools:              append([]types.APIToolDefinition(nil), req.ProviderTools...),
		Stream:             l.config.EnableStreaming,
		OutputSchema:       req.OutputSchema,
	}
}

func (l *Loop) sendAPIRequest(ctx context.Context, apiReq types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIResponse, error) {
	return l.sendAPIRequestWithClient(ctx, l.effectiveAPIClient(), apiReq, onChunk)
}

func (l *Loop) sendAPIRequestWithClient(ctx context.Context, client *providers.Client, apiReq types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIResponse, error) {
	if l.config.EnableStreaming {
		if l.sendAPIStreamRequestFn != nil {
			return l.sendAPIStreamRequestFn(ctx, apiReq, onChunk)
		}
		if l.sendAPIRequestFn != nil {
			return l.sendAPIRequestFn(ctx, apiReq)
		}
		if client == nil {
			return nil, types.NewError(types.ErrCodeAPIRequest, "api client not configured")
		}
		streamResult, err := client.CreateMessageStreamResultWithCallback(ctx, apiReq, onChunk)
		if err != nil {
			return nil, err
		}
		return &streamResult.Response, nil
	}
	if l.sendAPIRequestFn != nil {
		return l.sendAPIRequestFn(ctx, apiReq)
	}
	if client == nil {
		return nil, types.NewError(types.ErrCodeAPIRequest, "api client not configured")
	}
	return client.CreateMessage(ctx, apiReq)
}

func (l *Loop) effectiveAPIClient() *providers.Client {
	if l.activeFallbackClient != nil {
		return l.activeFallbackClient
	}
	return l.apiClient
}

func (l *Loop) effectiveProviderConfig() *providers.Config {
	if l.activeFallbackClient != nil {
		return l.activeFallbackClient.Config()
	}
	return l.providerConfig
}

func (l *Loop) circuitBreaker() *providers.CircuitBreakerConfig {
	providerConfig := l.effectiveProviderConfig()
	if providerConfig == nil || providerConfig.Routing == nil {
		return nil
	}
	return providerConfig.Routing.CircuitBreaker
}

func sameModelIdentifier(left, right types.ModelIdentifier) bool {
	return left.Provider == right.Provider && left.Model == right.Model && left.Version == right.Version
}

func (l *Loop) tryFallbackProvider(
	ctx context.Context,
	state *MutableState,
	req RunRequest,
	lastErr error,
) (*types.APIResponse, error) {
	providerConfig := l.effectiveProviderConfig()
	if providerConfig == nil || providerConfig.Routing == nil || len(providerConfig.Routing.FallbackProviders) == 0 {
		return nil, lastErr
	}

	chunkFn := buildChunkCallback(req.ResponseChunkCallback, req.EventQueue)
	for _, fallbackProvider := range providerConfig.Routing.FallbackProviders {
		baseModel, ok := providers.DefaultModelIdentifier(fallbackProvider)
		if !ok {
			continue
		}

		fallbackClient, err := providers.NewFallbackClient(ctx, fallbackProvider)
		if err != nil {
			lastErr = err
			continue
		}
		if l.monitoring != nil {
			fallbackClient.SetMonitoring(l.monitoring)
		}

		resp, fallbackErr := l.sendAPIRequestWithClient(ctx, fallbackClient, l.buildAPIRequest(state, req, baseModel), chunkFn)
		if fallbackErr == nil {
			l.activeFallbackClient = fallbackClient
			l.activeBaseModel = &baseModel
			l.currentFallbackIndex = -1
			return resp, nil
		}

		lastErr = fallbackErr
		if !l.isRecoverableError(fallbackErr) {
			return nil, lastErr
		}
	}

	return nil, lastErr
}

func (l *Loop) shouldStop(stopReason string) bool {
	return stopReason == types.StopReasonEndTurn || stopReason == types.StopReasonMaxTokens || stopReason == types.StopReasonStopSequence
}

func (l *Loop) extractToolUses(content []types.ContentBlock) []types.ToolUseContent {
	toolUses := make([]types.ToolUseContent, 0)
	for _, block := range content {
		if toolUse, ok := block.(types.ToolUseContent); ok {
			toolUses = append(toolUses, toolUse)
		}
	}
	return toolUses
}

func (l *Loop) integrateDiscoveredDeferredTools(ctx context.Context, state *MutableState, req *RunRequest, toolUses []types.ToolUseContent, results []tool.CallResult) toolDiscoveryUpdate {
	update := toolDiscoveryUpdate{}
	if state == nil || req == nil || req.ToolRegistry == nil || len(toolUses) == 0 || len(results) == 0 {
		return update
	}

	discovery := extractDiscoveredDeferredToolNames(toolUses, results)
	update.HadToolSearch = discovery.HadToolSearch
	update.Malformed = discovery.Malformed
	discovered := discovery.Names
	if len(discovered) == 0 {
		return update
	}
	if req.Tools == nil {
		req.Tools = make(map[string]tool.Tool)
	}

	seen := make(map[string]bool, len(state.DiscoveredDeferred))
	for _, name := range state.DiscoveredDeferred {
		seen[name] = true
	}

	added := false
	for _, name := range discovered {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		resolved, ok := req.ToolRegistry.Resolve(name)
		if !ok {
			update.Unresolved = append(update.Unresolved, name)
			continue
		}
		req.Tools[name] = resolved
		seen[name] = true
		state.DiscoveredDeferred = append(state.DiscoveredDeferred, name)
		update.Discovered = append(update.Discovered, name)
		added = true
	}

	if !added {
		sort.Strings(update.Unresolved)
		return update
	}

	sort.Strings(state.DiscoveredDeferred)
	sort.Strings(update.Discovered)
	sort.Strings(update.Unresolved)
	req.ProviderTools = prompt.BuildProviderToolDefinitions(req.Tools)
	if req.RefreshSystemPrompt != nil {
		refresh, err := req.RefreshSystemPrompt(ctx, req.Tools, remainingDeferredToolNames(req.ToolRegistry, state.DiscoveredDeferred), state.CurrentStage)
		if err != nil {
			update.RefreshFailed = err
			return update
		}
		req.SystemPrompt = refresh.SystemPrompt
		req.SystemPromptBlocks = append([]types.SystemPromptBlock(nil), refresh.SystemPromptBlocks...)
	}
	return update
}

func remainingDeferredToolNames(reg *tool.Registry, discovered []string) []string {
	if reg == nil {
		return nil
	}
	seen := make(map[string]bool, len(discovered))
	for _, name := range discovered {
		seen[name] = true
	}
	deferred := reg.ListDeferred()
	names := make([]string, 0, len(deferred))
	unique := make(map[string]bool, len(deferred))
	for _, deferredTool := range deferred {
		if deferredTool == nil {
			continue
		}
		name := deferredTool.Definition().Name
		if name == "" || seen[name] || unique[name] {
			continue
		}
		unique[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type toolSearchDiscovery struct {
	HadToolSearch bool
	Names         []string
	Malformed     bool
}

func extractDiscoveredDeferredToolNames(toolUses []types.ToolUseContent, results []tool.CallResult) toolSearchDiscovery {
	discovery := toolSearchDiscovery{}
	if len(toolUses) == 0 || len(results) == 0 {
		return discovery
	}

	seen := make(map[string]bool)
	names := make([]string, 0)
	for i, toolUse := range toolUses {
		if toolUse.Name != toolsearch.ToolSearchToolName || i >= len(results) {
			continue
		}
		discovery.HadToolSearch = true
		resultNames, valid := toolSearchMatches(results[i])
		if !valid {
			discovery.Malformed = true
			continue
		}
		for _, name := range resultNames {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	discovery.Names = names
	return discovery
}

func toolSearchMatches(result tool.CallResult) ([]string, bool) {
	if result.IsError() {
		return nil, true
	}
	if result.ContentType == tool.ContentTypeText {
		return nil, true
	}
	switch data := result.Data.(type) {
	case toolsearch.ToolSearchOutput:
		return matchNames(data.Matches), true
	case *toolsearch.ToolSearchOutput:
		if data == nil {
			return nil, false
		}
		return matchNames(data.Matches), true
	case map[string]any:
		names, ok := toolSearchMatchesFromMap(data)
		return names, ok
	default:
		return nil, false
	}
}

// matchNames extracts tool names from a []SearchMatch slice.
func matchNames(matches []toolsearch.SearchMatch) []string {
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.Name
	}
	return names
}

func toolSearchMatchesFromMap(data map[string]any) ([]string, bool) {
	raw, ok := data["matches"]
	if !ok {
		return nil, false
	}

	switch matches := raw.(type) {
	case []string:
		return append([]string(nil), matches...), true
	case []any:
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			name, ok := match.(string)
			if !ok {
				continue
			}
			names = append(names, name)
		}
		return names, true
	default:
		return nil, false
	}
}

func toolLoopRecoveryMessage(update toolDiscoveryUpdate) string {
	if update.RefreshFailed != nil && len(update.Discovered) > 0 {
		return fmt.Sprintf(
			"ToolSearch completed and these tools are now callable: %s. The runtime could not refresh the deferred-tool prompt, so continue using the updated tool surface directly without re-running ToolSearch.",
			strings.Join(update.Discovered, ", "),
		)
	}
	if !update.HadToolSearch {
		return ""
	}
	if update.Malformed {
		return "ToolSearch returned a non-usable discovery payload. If you still need a deferred tool, retry ToolSearch with a narrower `select:<tool_name>` query or continue with the current callable tools."
	}
	if len(update.Unresolved) > 0 && len(update.Discovered) == 0 {
		return fmt.Sprintf(
			"ToolSearch returned tool names that are not callable in this runtime: %s. Retry ToolSearch with a narrower query or continue with the current callable tools.",
			strings.Join(update.Unresolved, ", "),
		)
	}
	if len(update.Unresolved) > 0 {
		return fmt.Sprintf(
			"Some ToolSearch matches were not callable in this runtime: %s. Continue with the discovered callable tools or retry ToolSearch if you still need the unresolved ones.",
			strings.Join(update.Unresolved, ", "),
		)
	}
	return ""
}

func (l *Loop) executeTools(ctx context.Context, toolUses []types.ToolUseContent, req RunRequest, state *MutableState, transcript []types.Message) (execution.ExecuteResult, error) {
	return l.orchestrator.Execute(ctx, l.buildExecuteRequest(toolUses, req, state, transcript))
}

func (l *Loop) isRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	if engineErr, ok := err.(*types.EngineError); ok {
		return engineErr.IsRetryable()
	}
	// For non-EngineErrors (network errors, etc.) use the structured classifier
	// rather than brittle string matching.
	switch providerretry.ClassifyHTTPError(err, 0) {
	case providerretry.RetryClassificationClientError,
		providerretry.RetryClassificationAuthError:
		return false
	default:
		return true
	}
}

func normalizeRunRequestPermissionContext(req RunRequest) RunRequest {
	if req.PermissionContext == nil {
		req.PermissionContext = &types.PermissionContext{
			Mode:                             req.PermissionMode,
			IsBypassPermissionsModeAvailable: req.PermissionMode == types.PermissionModeBypass,
			IsAutoModeAvailable:              req.PermissionMode == types.PermissionModeAuto,
		}
	}
	if req.PermissionContext.Mode == "" {
		req.PermissionContext.Mode = req.PermissionMode
	}
	if req.PermissionContext.Mode == "" {
		req.PermissionContext.Mode = types.PermissionModeOnRequest
	}
	if req.PermissionContext.Mode == types.PermissionModeBypass {
		req.PermissionContext.IsBypassPermissionsModeAvailable = true
	}
	req.PermissionContext.NormalizeLegacyPlanMode()
	req.PermissionMode = req.PermissionContext.Mode
	return req
}

func (l *Loop) tryRecovery(ctx context.Context, state *MutableState, req RunRequest, err error) (bool, error) {
	if state.MaxOutputTokensRecoveryCount >= l.config.MaxOutputTokensRecoveryLimit {
		return false, fmt.Errorf(
			"api retry recovery limit exceeded after %d attempts: %w",
			state.MaxOutputTokensRecoveryCount,
			err,
		)
	}

	nextAttempt := state.MaxOutputTokensRecoveryCount + 1
	if limit := l.config.MaxRecoveryAttempts; limit > 0 && nextAttempt > limit {
		return false, fmt.Errorf(
			"api retry recovery limit exceeded after %d attempts: %w",
			limit,
			err,
		)
	}

	if err := l.waitRecoveryBackoff(ctx, nextAttempt); err != nil {
		return false, err
	}

	state.MaxOutputTokensRecoveryCount++
	l.setTransition(state, ContinueWithRecovery(l.recoveryLabel(err), RecoveryTypeAPIRetry))
	return true, nil
}

// recoveryLabel returns a stable label for the recovery transition based on the
// structured error code. Falls back to a generic label for non-EngineErrors
// (network errors, context cancellation, etc.) so the transition log stays
// meaningful without relying on error message strings.
func (l *Loop) recoveryLabel(err error) string {
	if engineErr, ok := err.(*types.EngineError); ok {
		switch engineErr.Code {
		case types.ErrCodeAPIRateLimit:
			return "recoverable_rate_limit"
		case types.ErrCodeAPITimeout:
			return "recoverable_timeout"
		case types.ErrCodeAPIResponse:
			return "recoverable_api_response"
		default:
			return "recoverable_api_retry"
		}
	}
	return "recoverable_network"
}

func (l *Loop) waitRecoveryBackoff(ctx context.Context, attempt int) error {
	if attempt <= 0 {
		return nil
	}
	multiplier := l.config.RecoveryBackoffMultiplier
	if multiplier <= 0 {
		multiplier = 250
	}
	delay := time.Duration(multiplier*attempt) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *Loop) updateUsage(state *MutableState, usage *types.TokenUsage) {
	if usage == nil {
		return
	}
	state.Usage = usage
	state.TotalTurnTokens += usage.InputTokens + usage.OutputTokens
}

func (l *Loop) handleTerminalStop(ctx context.Context, state *MutableState, req RunRequest, stopReason string, assistantMessage types.Message, toolUses []types.ToolUseContent) (bool, error) {
	if stopReason == types.StopReasonMaxTokens && state.MaxOutputTokensRecoveryCount < l.config.MaxOutputTokensRecoveryLimit {
		state.Messages = append(state.Messages, newContinuationMessage(req.TurnID, "Output token limit hit. Resume directly without apology or recap. Continue the unfinished work in smaller pieces."))
		state.MaxOutputTokensRecoveryCount++
		l.setTransition(state, ContinueWithRecovery("max_output_tokens_recovery", RecoveryTypeMaxOutputTokens))
		return true, nil
	}
	if len(toolUses) == 0 {
		if decision := l.maybeContinueForBudget(state, req, assistantMessage); decision.continueLoop {
			state.Messages = append(state.Messages, newContinuationMessage(req.TurnID, decision.nudgeMessage))
			l.setTransition(state, ContinueWithRecovery("token_budget_continuation", RecoveryTypeTokenBudget))
			return true, nil
		}
	}
	stopHookResult, err := l.runStopHooks(ctx, state, req, terminalStopReason(stopReason))
	if err != nil {
		return false, err
	}
	appendHookMessages(state, stopHookResult, req.TurnID)
	if stopHookResult.Continue {
		l.setTransition(state, ContinueWithRecovery("stop_hook_continue", RecoveryTypeStopHook))
		return true, nil
	}
	state.StopReason = terminalStopReason(stopReason)
	l.setTransition(state, Terminate("stop_reason_terminal", state.StopReason))
	return false, nil
}

func (l *Loop) handleNoToolUses(ctx context.Context, state *MutableState, req RunRequest, assistantMessage types.Message) (bool, error) {
	if l.shouldNudgeContinuation(assistantMessage, len(state.ToolUses)) && state.ContinuationNudgeCount < l.config.ContinuationNudgeLimit {
		state.Messages = append(state.Messages, newContinuationMessage(req.TurnID, "Continue with the task. Use the appropriate tools to proceed."))
		state.ContinuationNudgeCount++
		l.setTransition(state, ContinueWithRecovery("continuation_nudge", RecoveryTypeContinuationNudge))
		return true, nil
	}
	if decision := l.maybeContinueForBudget(state, req, assistantMessage); decision.continueLoop {
		state.Messages = append(state.Messages, newContinuationMessage(req.TurnID, decision.nudgeMessage))
		l.setTransition(state, ContinueWithRecovery("token_budget_continuation", RecoveryTypeTokenBudget))
		return true, nil
	}
	stopHookResult, err := l.runStopHooks(ctx, state, req, types.StopReasonEndTurn)
	if err != nil {
		return false, err
	}
	appendHookMessages(state, stopHookResult, req.TurnID)
	if stopHookResult.Continue {
		l.setTransition(state, ContinueWithRecovery("stop_hook_continue", RecoveryTypeStopHook))
		return true, nil
	}
	state.StopReason = types.StopReasonEndTurn
	l.setTransition(state, Terminate("no_tool_uses", types.StopReasonEndTurn))
	return false, nil
}

func buildAssistantMessage(apiResp *types.APIResponse, turnID types.TurnID, msgIndex int) types.Message {
	msg := types.AssistantMessage(fmt.Sprintf("msg-%d", msgIndex+1), apiResp.Content)
	msg.Metadata = &types.MessageMetadata{
		TurnID:     turnID.String(),
		StopReason: apiResp.StopReason,
		Usage:      &apiResp.Usage,
	}
	return msg
}

func newContinuationMessage(turnID types.TurnID, content string) types.Message {
	msg := types.UserMessage(fmt.Sprintf("msg-%d", GetCurrentTime().UnixNano()), content)
	msg.Metadata = &types.MessageMetadata{TurnID: turnID.String()}
	return msg
}

func appendHookMessages(state *MutableState, result StopHookResult, turnID types.TurnID) {
	for _, msg := range result.Messages {
		if msg.Metadata == nil {
			msg.Metadata = &types.MessageMetadata{TurnID: turnID.String()}
		}
		state.Messages = append(state.Messages, msg)
	}
}

func terminalStopReason(stopReason string) string {
	if stopReason == "" {
		return types.StopReasonEndTurn
	}
	return stopReason
}

// GetCurrentTime returns the current time (overridable for testing).
var GetCurrentTime = func() time.Time {
	return time.Now()
}

// executeHook executes lifecycle hooks for an event.
// Returns true if any hook requested Stop or Deny (early termination).
// This method is safe to call even if hookExecutor is nil.
func (l *Loop) executeHook(ctx context.Context, event types.HookEvent, data map[string]any) bool {
	result := l.executeHookWithResult(ctx, event, data)
	return result != nil && (result.Action == types.HookActionStop || result.Action == types.HookActionDeny)
}

// executeHookWithResult executes hooks and returns the first actionable result.
// Returns nil if no hook requested a non-continue action or hookExecutor is nil.
// Callers that need Modify data (e.g. UpdatedInput) should use this method.
func (l *Loop) executeHookWithResult(ctx context.Context, event types.HookEvent, data map[string]any) *types.HookResult {
	if l.hookExecutor == nil {
		return nil
	}
	result := l.hookExecutor.ExecuteFirst(ctx, event, data)
	if result == nil || result.Action == types.HookActionContinue || result.Action == "" {
		return nil
	}
	switch result.Action {
	case types.HookActionStop, types.HookActionDeny:
		slog.Warn("hook blocked execution", "event", event, "action", result.Action, "message", result.Message)
	case types.HookActionModify:
		slog.Debug("hook modified execution data", "event", event, "keys", len(result.UpdatedInput))
	case types.HookActionRetry:
		slog.Debug("hook requested retry", "event", event, "message", result.Message)
	}
	return result
}
