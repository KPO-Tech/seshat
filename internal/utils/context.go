package utils

import "github.com/EngineerProjects/nexus-engine/internal/types"

// ContextManager manages context window limits
type ContextManager struct {
	estimator *TokenEstimator
}

// NewContextManager creates a new context manager
func NewContextManager() *ContextManager {
	return &ContextManager{
		estimator: NewTokenEstimator(),
	}
}

// CalculateMessageTokens calculates tokens for a message
func (m *ContextManager) CalculateMessageTokens(msg types.Message) int {
	total := 0

	// Add overhead for role
	total += len(string(msg.Role)) + 10

	// Add tokens for each content block
	for _, block := range msg.Content {
		total += m.calculateContentBlockTokens(block)
	}

	// Add overhead for message formatting
	total += 10

	return total
}

// calculateContentBlockTokens calculates tokens for a content block
func (m *ContextManager) calculateContentBlockTokens(block types.ContentBlock) int {
	switch block.ContentType() {
	case types.ContentTypeText:
		if text, ok := block.(types.TextContent); ok {
			return m.estimator.EstimateTokens(text.Text)
		}
	case types.ContentTypeImage:
		// Images cost a fixed amount
		return EstimateImageTokens()
	case types.ContentTypeToolUse:
		if toolUse, ok := block.(types.ToolUseContent); ok {
			return m.estimator.EstimateToolUseTokens(toolUse.Name, toolUse.Input)
		}
	case types.ContentTypeToolResult:
		if result, ok := block.(types.ToolResultContent); ok {
			return m.estimator.EstimateToolResultTokens(result.Content)
		}
	case types.ContentTypeThinking:
		if thinking, ok := block.(types.ThinkingContent); ok {
			return m.estimator.EstimateTokens(thinking.Thinking)
		}
	}
	return 0
}

// CalculateMessagesTokens calculates total tokens for a list of messages
func (m *ContextManager) CalculateMessagesTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		total += m.CalculateMessageTokens(msg)
	}
	return total
}

// CalculateSystemPromptTokens calculates tokens for the system prompt
func (m *ContextManager) CalculateSystemPromptTokens(systemPrompt string) int {
	// System prompt has overhead
	return m.estimator.EstimateTokens(systemPrompt) + 20
}

// EstimateRequestTokens estimates total tokens for an API request
func (m *ContextManager) EstimateRequestTokens(
	systemPrompt string,
	messages []types.Message,
) int {
	total := 0

	if systemPrompt != "" {
		total += m.CalculateSystemPromptTokens(systemPrompt)
	}

	total += m.CalculateMessagesTokens(messages)

	return total
}

// CalculateRemainingTokens calculates remaining tokens before hitting the limit
func (m *ContextManager) CalculateRemainingTokens(
	contextWindow types.ContextWindow,
	systemPrompt string,
	messages []types.Message,
) int {
	used := m.EstimateRequestTokens(systemPrompt, messages)
	return contextWindow.MaxTokens - used
}

// ShouldCompact returns true if compaction is needed
func (m *ContextManager) ShouldCompact(
	contextWindow types.ContextWindow,
	systemPrompt string,
	messages []types.Message,
	threshold float64,
) bool {
	used := m.EstimateRequestTokens(systemPrompt, messages)
	limit := contextWindow.MaxTokens
	thresholdTokens := int(float64(limit) * threshold)
	return used >= thresholdTokens
}

// CalculateCompactionTarget calculates how many tokens need to be removed
func (m *ContextManager) CalculateCompactionTarget(
	contextWindow types.ContextWindow,
	systemPrompt string,
	messages []types.Message,
	targetPercentage float64,
) int {
	used := m.EstimateRequestTokens(systemPrompt, messages)
	target := int(float64(contextWindow.MaxTokens) * targetPercentage)
	if used <= target {
		return 0
	}
	return used - target
}
