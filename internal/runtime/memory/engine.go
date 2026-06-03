package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/internal/utils"
)

const (
	defaultAutoCompactBufferTokens      = 13_000
	defaultManualCompactBufferTokens    = 3_000
	defaultMaxSummaryTokens             = 2_000
	defaultMaxConsecutiveCompactFailure = 3
	compactSummaryMessageID             = "compact_summary"
	compactMarkerMessageID              = "compact_marker"
)

// Engine performs runtime-driven conversation compaction.
type Engine struct {
	apiClient      *providers.Client
	preparer       *MessagePreparer
	microCompactor *MicroCompactor
	contextManager *utils.ContextManager
	config         *Config
}

// NewEngine creates a new compaction engine.
func NewEngine(apiClient *providers.Client, config *Config) *Engine {
	if config == nil {
		config = DefaultConfig()
	}
	if config.AutoCompactThreshold <= 0 {
		config.AutoCompactThreshold = DefaultConfig().AutoCompactThreshold
	}
	if config.CompactTargetPercentage <= 0 {
		config.CompactTargetPercentage = DefaultConfig().CompactTargetPercentage
	}
	if config.MaxSummaryTokens <= 0 {
		config.MaxSummaryTokens = defaultMaxSummaryTokens
	}
	if config.AutoCompactBufferTokens <= 0 {
		config.AutoCompactBufferTokens = defaultAutoCompactBufferTokens
	}
	if config.ManualCompactBufferTokens <= 0 {
		config.ManualCompactBufferTokens = defaultManualCompactBufferTokens
	}
	if config.MaxConsecutiveFailures <= 0 {
		config.MaxConsecutiveFailures = defaultMaxConsecutiveCompactFailure
	}

	return &Engine{
		apiClient:      apiClient,
		preparer:       NewMessagePreparer(),
		microCompactor: NewMicroCompactor(),
		contextManager: utils.NewContextManager(),
		config:         config,
	}
}

// EffectiveContextWindow returns the effective context window after reserving summary output budget.
func (e *Engine) EffectiveContextWindow(model types.ModelIdentifier) int {
	window := types.GetContextWindow(model)
	reserved := e.config.MaxSummaryTokens
	if window.MaxOutputTokens > 0 && window.MaxOutputTokens < reserved {
		reserved = window.MaxOutputTokens
	}
	effective := window.MaxTokens - reserved
	minimum := reserved + e.config.AutoCompactBufferTokens
	if effective < minimum {
		return minimum
	}
	return effective
}

// AutoCompactThresholdTokens returns the absolute token threshold for auto-compaction.
func (e *Engine) AutoCompactThresholdTokens(model types.ModelIdentifier) int {
	effective := e.EffectiveContextWindow(model)
	threshold := int(float64(effective) * e.config.AutoCompactThreshold)
	hardCap := effective - e.config.AutoCompactBufferTokens
	if threshold > hardCap {
		return hardCap
	}
	return threshold
}

// BlockingLimitTokens returns the hard blocking limit for the current model.
func (e *Engine) BlockingLimitTokens(model types.ModelIdentifier) int {
	effective := e.EffectiveContextWindow(model)
	return effective - e.config.ManualCompactBufferTokens
}

// TargetTokens returns the post-compaction target token count.
func (e *Engine) TargetTokens(model types.ModelIdentifier) int {
	effective := e.EffectiveContextWindow(model)
	return int(float64(effective) * e.config.CompactTargetPercentage)
}

// CalculateUsage calculates total token usage for the current request.
func (e *Engine) CalculateUsage(systemPrompt string, messages []types.Message) int {
	return e.contextManager.EstimateRequestTokens(systemPrompt, messages)
}

// ShouldCompact returns true if compaction should be attempted.
func (e *Engine) ShouldCompact(systemPrompt string, messages []types.Message, model types.ModelIdentifier) bool {
	used := e.CalculateUsage(systemPrompt, messages)
	return used >= e.AutoCompactThresholdTokens(model)
}

// Compact performs compaction and returns a canonical result.
func (e *Engine) Compact(
	ctx context.Context,
	systemPrompt string,
	messages []types.Message,
	model types.ModelIdentifier,
	sessionID types.SessionID,
	turnID types.TurnID,
) (CompactionResult, error) {
	preCompactTokens := e.CalculateUsage(systemPrompt, messages)
	targetTokens := e.TargetTokens(model)

	microCompacted := e.microCompact(messages)
	postMicroTokens := e.CalculateUsage(systemPrompt, microCompacted)
	if postMicroTokens <= targetTokens {
		return CompactionResult{
			Messages:            microCompacted,
			DidCompact:          true,
			UsedMicroCompact:    true,
			PreCompactTokens:    preCompactTokens,
			PostCompactTokens:   postMicroTokens,
			TargetTokens:        targetTokens,
			ConsecutiveFailures: 0,
		}, nil
	}

	summary, err := e.generateSummary(ctx, microCompacted, sessionID, turnID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("failed to generate summary: %w", err)
	}

	compactedMessages := e.buildPostCompactMessages(microCompacted, summary, targetTokens)
	postCompactTokens := e.CalculateUsage(systemPrompt, compactedMessages)
	metadata, err := e.buildCompactionMetadata("summary", preCompactTokens, postCompactTokens, targetTokens, compactedMessages)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("failed to build compaction metadata: %w", err)
	}
	compactedMessages = attachCompactionMetadata(compactedMessages, metadata)

	return CompactionResult{
		Messages:            compactedMessages,
		DidCompact:          true,
		UsedMicroCompact:    true,
		UsedSummaryCompact:  true,
		PreCompactTokens:    preCompactTokens,
		PostCompactTokens:   postCompactTokens,
		TargetTokens:        targetTokens,
		Summary:             summary,
		ConsecutiveFailures: 0,
		RuntimeMetadata:     metadata,
	}, nil
}

// AutoCompact performs compaction if needed and respects the circuit breaker.
func (e *Engine) AutoCompact(
	ctx context.Context,
	systemPrompt string,
	messages []types.Message,
	model types.ModelIdentifier,
	sessionID types.SessionID,
	turnID types.TurnID,
	tracking *TrackingState,
) (CompactionResult, error) {
	current := TrackingState{}
	if tracking != nil {
		current = *tracking
	}
	if current.ConsecutiveFailures >= e.config.MaxConsecutiveFailures {
		return CompactionResult{
			Messages:            messages,
			DidCompact:          false,
			PreCompactTokens:    e.CalculateUsage(systemPrompt, messages),
			PostCompactTokens:   e.CalculateUsage(systemPrompt, messages),
			TargetTokens:        e.TargetTokens(model),
			ConsecutiveFailures: current.ConsecutiveFailures,
		}, nil
	}
	if !e.ShouldCompact(systemPrompt, messages, model) {
		return CompactionResult{
			Messages:            messages,
			DidCompact:          false,
			PreCompactTokens:    e.CalculateUsage(systemPrompt, messages),
			PostCompactTokens:   e.CalculateUsage(systemPrompt, messages),
			TargetTokens:        e.TargetTokens(model),
			ConsecutiveFailures: current.ConsecutiveFailures,
		}, nil
	}

	result, err := e.Compact(ctx, systemPrompt, messages, model, sessionID, turnID)
	if err != nil {
		current.ConsecutiveFailures++
		return CompactionResult{
			Messages:            messages,
			DidCompact:          false,
			PreCompactTokens:    e.CalculateUsage(systemPrompt, messages),
			PostCompactTokens:   e.CalculateUsage(systemPrompt, messages),
			TargetTokens:        e.TargetTokens(model),
			ConsecutiveFailures: current.ConsecutiveFailures,
		}, err
	}
	result.ConsecutiveFailures = 0
	return result, nil
}

func (e *Engine) microCompact(messages []types.Message) []types.Message {
	compacted := make([]types.Message, len(messages))
	for i, msg := range messages {
		compacted[i] = e.preparer.PrepareMessageForAPI(msg)
	}
	return compacted
}

func (e *Engine) generateSummary(
	ctx context.Context,
	messages []types.Message,
	sessionID types.SessionID,
	turnID types.TurnID,
) (string, error) {
	prompt := e.buildSummaryPrompt(messages)
	resp, err := e.apiClient.CreateMessage(ctx, types.APIRequest{
		Model:     e.config.SummaryModel,
		Messages:  []types.Message{types.UserMessage("compact_summary_request", prompt)},
		MaxTokens: e.config.MaxSummaryTokens,
	})
	if err != nil {
		return "", err
	}
	for _, block := range resp.Content {
		if text, ok := block.(types.TextContent); ok && strings.TrimSpace(text.Text) != "" {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("no summary generated for session %s turn %s", sessionID, turnID)
}

func (e *Engine) buildSummaryPrompt(messages []types.Message) string {
	var builder strings.Builder
	builder.WriteString("Summarize the conversation so the runtime can continue work after compaction. Preserve:\n")
	builder.WriteString("- active user requests\n")
	builder.WriteString("- decisions already made\n")
	builder.WriteString("- tool uses and outcomes that still matter\n")
	builder.WriteString("- unresolved work and constraints\n\n")
	builder.WriteString("Conversation:\n\n")
	for _, msg := range messages {
		builder.WriteString(fmt.Sprintf("[%s] ", msg.Role))
		for _, block := range msg.Content {
			switch content := block.(type) {
			case types.TextContent:
				builder.WriteString(content.Text)
			case types.ToolUseContent:
				builder.WriteString(fmt.Sprintf("Tool use: %s ", content.Name))
			case types.ToolResultContent:
				builder.WriteString(fmt.Sprintf("Tool result: %s ", e.microCompactor.CalculateToolResultSummary(content)))
			}
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func (e *Engine) buildPostCompactMessages(messages []types.Message, summary string, targetTokens int) []types.Message {
	preservedStart := e.findPreservedStart(messages, targetTokens)
	compacted := make([]types.Message, 0, len(messages)-preservedStart+2)

	summaryMessage := types.SystemMessage(compactSummaryMessageID, fmt.Sprintf("Previous conversation summary:\n\n%s", summary))
	summaryMessage.Metadata = &types.MessageMetadata{TurnID: "compact"}
	compacted = append(compacted, summaryMessage)

	// The marker must sit before the preserved tail so downstream transcript and
	// resume logic can see where compaction cut the history before it reads the
	// still-live messages that follow.
	marker := types.SystemMessage(compactMarkerMessageID, "[Context compacted for token pressure]")
	marker.Metadata = &types.MessageMetadata{TurnID: "compact"}
	compacted = append(compacted, marker)
	compacted = append(compacted, types.CloneMessages(messages[preservedStart:])...)
	return compacted
}

func (e *Engine) findPreservedStart(messages []types.Message, targetTokens int) int {
	if len(messages) == 0 {
		return 0
	}

	preservedTokens := 0
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		preservedTokens += e.preparer.CalculateMessageTokens(messages[i])
		start = i
		if preservedTokens >= targetTokens {
			break
		}
	}
	return adjustPreservedStartForToolPairs(messages, start)
}

// adjustPreservedStartForToolPairs widens the preserved tail when the naive
// token-based start would keep a tool_result without the assistant tool_use it
// answers. Preserving the pair keeps the post-compact transcript structurally
// valid for both the runtime and future replay/resume logic.
func adjustPreservedStartForToolPairs(messages []types.Message, start int) int {
	if start <= 0 || start >= len(messages) {
		return start
	}

	adjustedStart := start
	for {
		neededToolUseIDs := make(map[string]bool)
		for i := adjustedStart; i < len(messages); i++ {
			for _, toolUseID := range collectToolResultIDs(messages[i]) {
				neededToolUseIDs[toolUseID] = true
			}
		}
		if len(neededToolUseIDs) == 0 {
			return adjustedStart
		}

		updated := adjustedStart
		for i := adjustedStart - 1; i >= 0; i-- {
			toolUseIDs := collectToolUseIDs(messages[i])
			if len(toolUseIDs) == 0 {
				continue
			}
			matched := false
			for _, toolUseID := range toolUseIDs {
				if neededToolUseIDs[toolUseID] {
					matched = true
					delete(neededToolUseIDs, toolUseID)
				}
			}
			if matched {
				updated = i
				if len(neededToolUseIDs) == 0 {
					break
				}
			}
		}
		if updated == adjustedStart {
			return adjustedStart
		}
		adjustedStart = updated
	}
}

func collectToolUseIDs(message types.Message) []string {
	ids := make([]string, 0)
	for _, block := range message.Content {
		if toolUse, ok := block.(types.ToolUseContent); ok {
			ids = append(ids, toolUse.ID)
		}
	}
	return ids
}

func collectToolResultIDs(message types.Message) []string {
	ids := make([]string, 0)
	for _, block := range message.Content {
		if toolResult, ok := block.(types.ToolResultContent); ok {
			ids = append(ids, toolResult.ToolUseID)
		}
	}
	return ids
}

func attachCompactionMetadata(messages []types.Message, metadata *types.CompactionMetadata) []types.Message {
	if metadata == nil || len(messages) == 0 {
		return messages
	}
	compacted := types.CloneMessages(messages)
	for i := range compacted {
		if compacted[i].ID == types.MessageID(compactSummaryMessageID) || compacted[i].ID == types.MessageID(compactMarkerMessageID) {
			compacted[i] = types.WithCompactionMetadata(compacted[i], metadata)
		}
	}
	return compacted
}

func (e *Engine) buildCompactionMetadata(kind string, preCompactTokens int, postCompactTokens int, targetTokens int, messages []types.Message) (*types.CompactionMetadata, error) {
	preservedTailStart := 0
	for i, message := range messages {
		if message.ID == types.MessageID(compactMarkerMessageID) {
			preservedTailStart = i + 1
			break
		}
	}
	preserved := messages[preservedTailStart:]
	preservedTailHash, err := types.CanonicalTranscriptHash(preserved)
	if err != nil {
		return nil, err
	}
	metadata := &types.CompactionMetadata{
		Kind:               kind,
		PreCompactTokens:   preCompactTokens,
		PostCompactTokens:  postCompactTokens,
		TargetTokens:       targetTokens,
		PreservedMessages:  len(preserved),
		PreservedTurns:     countPreservedTurns(preserved),
		PreservedToolPairs: countPreservedToolPairs(preserved),
	}
	if len(preserved) > 0 {
		metadata.BoundaryVersion = types.CompactionBoundaryVersionV1
		metadata.FirstPreservedMessageID = preserved[0].ID
		metadata.LastPreservedMessageID = preserved[len(preserved)-1].ID
		metadata.PreservedTailHash = preservedTailHash
	}
	if len(preserved) == 0 {
		metadata.BoundaryVersion = types.CompactionBoundaryVersionV1
		metadata.PreservedTailHash = preservedTailHash
	}
	return metadata, nil
}

func countPreservedTurns(messages []types.Message) int {
	return types.CountDistinctTurnIDs(messages)
}

func countPreservedToolPairs(messages []types.Message) int {
	toolUseIDs := make(map[string]bool)
	for _, message := range messages {
		for _, toolUseID := range collectToolUseIDs(message) {
			toolUseIDs[toolUseID] = true
		}
	}
	count := 0
	for _, message := range messages {
		for _, toolUseID := range collectToolResultIDs(message) {
			if toolUseIDs[toolUseID] {
				count++
			}
		}
	}
	return count
}

// CalculateCompactionImpact calculates the impact of compaction.
func (e *Engine) CalculateCompactionImpact(messages []types.Message) CompactionImpact {
	originalTokens := e.preparer.CalculateMessagesTokens(messages)
	microCompacted := e.microCompact(messages)
	microTokens := e.preparer.CalculateMessagesTokens(microCompacted)
	estimatedTokens := int(float64(e.TargetTokens(e.config.SummaryModel)))
	if estimatedTokens <= 0 {
		estimatedTokens = microTokens
	}
	return CompactionImpact{
		OriginalTokens:              originalTokens,
		MicroCompactTokens:          microTokens,
		EstimatedFullCompactTokens:  estimatedTokens,
		MicroCompactSavings:         originalTokens - microTokens,
		EstimatedFullCompactSavings: originalTokens - estimatedTokens,
	}
}

// CompactionImpact represents the impact of compaction.
type CompactionImpact struct {
	OriginalTokens              int `json:"original_tokens"`
	MicroCompactTokens          int `json:"micro_compact_tokens"`
	EstimatedFullCompactTokens  int `json:"estimated_full_compact_tokens"`
	MicroCompactSavings         int `json:"micro_compact_savings"`
	EstimatedFullCompactSavings int `json:"estimated_full_compact_savings"`
}
