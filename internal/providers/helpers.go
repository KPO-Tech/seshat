package providers

import (
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// parseTokenUsage extracts token usage from a raw usage map.
// Handles both Anthropic (cache_read_input_tokens, cache_creation_input_tokens)
// and OpenAI (prompt_tokens_details.cached_tokens) response shapes.
func parseTokenUsage(raw map[string]any) *types.TokenUsage {
	usage := &types.TokenUsage{}
	if inputTokens, ok := intFromAny(raw["input_tokens"]); ok {
		usage.InputTokens = inputTokens
	}
	if outputTokens, ok := intFromAny(raw["output_tokens"]); ok {
		usage.OutputTokens = outputTokens
	}
	// Anthropic prompt cache fields (top-level in usage object).
	if cacheRead, ok := intFromAny(raw["cache_read_input_tokens"]); ok {
		usage.CacheReadInputTokens = cacheRead
	}
	if cacheCreation, ok := intFromAny(raw["cache_creation_input_tokens"]); ok {
		usage.CacheCreationInputTokens = cacheCreation
	}
	// OpenAI-style cached tokens (kept for OpenAI provider compat).
	if details, ok := raw["prompt_tokens_details"].(map[string]any); ok {
		if cachedTokens, ok := intFromAny(details["cached_tokens"]); ok {
			usage.CachedTokens = cachedTokens
		}
	}
	return usage
}

// intFromAny coerces numeric JSON values (float64, int, int32, int64, float32)
// to int. JSON numbers unmarshal as float64 by default.
func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func stringPtr(value string) *string {
	return &value
}

// joinTextBlocks concatenates all text content blocks in a message into a single string.
func joinTextBlocks(content []types.ContentBlock) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		text, ok := block.(types.TextContent)
		if !ok || text.Text == "" {
			continue
		}
		parts = append(parts, text.Text)
	}
	return strings.Join(parts, "\n")
}

// hasImageContent returns true if any block in content is an ImageContent.
func hasImageContent(content []types.ContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(types.ImageContent); ok {
			return true
		}
	}
	return false
}

// openAIVisionContent builds the OpenAI multi-part content array for a user
// message that contains images. Text and images are interleaved in order.
func openAIVisionContent(content []types.ContentBlock) []map[string]any {
	parts := make([]map[string]any, 0, len(content))
	for _, block := range content {
		switch typed := block.(type) {
		case types.TextContent:
			if typed.Text != "" {
				parts = append(parts, map[string]any{"type": "text", "text": typed.Text})
			}
		case types.ImageContent:
			url := fmt.Sprintf("data:%s;base64,%s", typed.Source.MediaType, typed.Source.Data)
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": url},
			})
		}
	}
	return parts
}

// toolResultsFromMessage extracts all tool result content blocks from a message.
func toolResultsFromMessage(message types.Message) []types.ToolResultContent {
	results := make([]types.ToolResultContent, 0)
	for _, block := range message.Content {
		result, ok := block.(types.ToolResultContent)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}
