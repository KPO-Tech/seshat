package utils

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TokenEstimator estimates token counts for text
type TokenEstimator struct {
	// CharactersPerToken is the average characters per token
	CharactersPerToken float64
}

// NewTokenEstimator creates a new token estimator
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{
		// Rough estimate: ~4 characters per token for English text
		CharactersPerToken: 4.0,
	}
}

// EstimateTokens estimates the number of tokens in a string
func (e *TokenEstimator) EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Use character count as a rough estimate
	charCount := utf8.RuneCountInString(text)
	estimated := int(float64(charCount) / e.CharactersPerToken)

	// Ensure at least 1 token for non-empty text
	if estimated < 1 && charCount > 0 {
		return 1
	}

	return estimated
}

// EstimateMessageTokens estimates tokens in a message
func (e *TokenEstimator) EstimateMessageTokens(role string, content string) int {
	// Add overhead for role and formatting
	overhead := len(role) + 10 // ~10 tokens for formatting
	return e.EstimateTokens(content) + overhead
}

// EstimateToolUseTokens estimates tokens for a tool use
func (e *TokenEstimator) EstimateToolUseTokens(toolName string, input map[string]any) int {
	// Estimate tool name and formatting
	overhead := e.EstimateTokens(toolName) + 20

	// Estimate input JSON
	inputStr := formatMapForEstimation(input)
	return overhead + e.EstimateTokens(inputStr)
}

// EstimateToolResultTokens estimates tokens for a tool result
func (e *TokenEstimator) EstimateToolResultTokens(result string) int {
	// Add overhead for result formatting
	return e.EstimateTokens(result) + 10
}

// formatMapForEstimation formats a map for token estimation
func formatMapForEstimation(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}

	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// CountTokensInText counts tokens in text (alias for EstimateTokens)
func CountTokensInText(text string) int {
	estimator := NewTokenEstimator()
	return estimator.EstimateTokens(text)
}

// EstimateImageTokens estimates tokens for an image
// Based on Anthropic's pricing: each image costs a fixed number of tokens
func EstimateImageTokens() int {
	// Anthropic charges ~1500 tokens per image
	return 1500
}

// CalculateTotalTokens calculates total tokens from usage
func CalculateTotalTokens(inputTokens, outputTokens int) int {
	return inputTokens + outputTokens
}
