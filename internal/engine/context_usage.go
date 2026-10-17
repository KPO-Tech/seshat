package engine

import (
	"context"
	"encoding/json"

	compact "github.com/KPO-Tech/seshat-runtime/internal/runtime/memory"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const (
	contextUsageSourceEstimate = "estimate"
	contextUsageSourceReported = "reported"

	// bytesPerToken is the rough ratio used to size tool definitions, which have no provider-side count before the
	// first response.
	bytesPerToken = 4
)

// usageReporter is what a compaction strategy offers to report where the window stands. The default Engine has it; a
// strategy that does not simply reports nothing.
type usageReporter interface {
	UsedTokens(systemPrompt string, messages []types.Message, tracking *compact.TrackingState) int
	Budget(model types.ModelIdentifier) compact.ContextBudget
}

// toolDefinitionTokens estimates what the tool definitions of a request cost, to be added to the messages' size.
func toolDefinitionTokens(tools []types.APIToolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return len(raw) / bytesPerToken
}

// compactionTracking is what the compaction strategy needs to size the next request: the failure count, the cost of
// the tool definitions, and the last size the provider reported.
func compactionTracking(state *MutableState, req RunRequest) *compact.TrackingState {
	return &compact.TrackingState{
		ConsecutiveFailures: state.AutoCompactFailureCount,
		ExtraTokens:         toolDefinitionTokens(req.ProviderTools),
		Anchor:              state.UsageAnchor,
	}
}

// recordUsageAnchor remembers the size the provider reported for the request just answered, plus the answer itself,
// and how many messages that covers. Not every provider reports usage; a zero figure leaves the anchor unset.
func recordUsageAnchor(state *MutableState, usage *types.TokenUsage) {
	state.UsageAnchor = nil
	if usage == nil {
		return
	}
	// Anthropic reports cache reads and writes apart from input tokens; for the other providers those are zero.
	tokens := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens + usage.OutputTokens
	if tokens <= 0 {
		return
	}
	state.UsageAnchor = &compact.UsageAnchor{Tokens: tokens, MessageCount: len(state.Messages)}
}

// emitContextUsage tells the host how full the window is. It does nothing without an emitter or a strategy that can
// report.
func (l *Loop) emitContextUsage(ctx context.Context, state *MutableState, req RunRequest) {
	emit, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent))
	if !ok || emit == nil {
		return
	}
	reporter, ok := l.compactor.(usageReporter)
	if !ok {
		return
	}
	source := contextUsageSourceEstimate
	if state.UsageAnchor != nil {
		source = contextUsageSourceReported
	}
	budget := reporter.Budget(req.Model)
	if budget.WindowTokens <= 0 {
		return
	}
	emit(types.RuntimeEvent{
		Type:      types.RuntimeEventTypeContextUsage,
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		ContextUsageEvent: &types.ContextUsageRuntimeEvent{
			UsedTokens:          reporter.UsedTokens(req.SystemPrompt, state.Messages, compactionTracking(state, req)),
			WindowTokens:        budget.WindowTokens,
			AutoCompactAtTokens: budget.AutoCompactAtTokens,
			BlockingLimitTokens: budget.BlockingLimitTokens,
			Source:              source,
		},
	})
}
