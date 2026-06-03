package memory

import (
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// MicroCompactor performs micro-compaction of tool results
type MicroCompactor struct {
	// maxToolResultSize is the maximum size for a tool result (in characters)
	maxToolResultSize int

	// trimStrategy determines how to trim results
	trimStrategy TrimStrategy
}

// TrimStrategy represents how to trim tool results
type TrimStrategy string

const (
	TrimStrategyTruncate TrimStrategy = "truncate"
	TrimStrategySnip     TrimStrategy = "snip"
	TrimStrategyPreview  TrimStrategy = "preview"
)

// NewMicroCompactor creates a new micro compactor
func NewMicroCompactor() *MicroCompactor {
	return &MicroCompactor{
		maxToolResultSize: 10000, // 10k characters
		trimStrategy:      TrimStrategyPreview,
	}
}

// SetMaxToolResultSize sets the maximum tool result size
func (m *MicroCompactor) SetMaxToolResultSize(size int) {
	m.maxToolResultSize = size
}

// SetTrimStrategy sets the trim strategy
func (m *MicroCompactor) SetTrimStrategy(strategy TrimStrategy) {
	m.trimStrategy = strategy
}

// TrimToolResult trims a tool result if it's too large
func (m *MicroCompactor) TrimToolResult(result types.ToolResultContent) types.ToolResultContent {
	content := result.Content

	if len(content) <= m.maxToolResultSize {
		return result
	}

	// Content is too large, trim it
	trimmed := m.trimContent(content)

	result.Content = trimmed

	// Add metadata about the replacement
	if result.Metadata == nil {
		result.Metadata = &map[string]any{}
	}

	(*result.Metadata)["content_replacement"] = types.ContentReplacementState{
		OriginalSize:    int64(len(content)),
		ReplacedSize:    int64(len(trimmed)),
		ReplacementType: types.ContentReplacementType(m.trimStrategy),
		Preview:         m.getPreview(content),
	}

	return result
}

// TrimToolResults trims multiple tool results
func (m *MicroCompactor) TrimToolResults(results []types.ToolResultContent) []types.ToolResultContent {
	trimmed := make([]types.ToolResultContent, len(results))

	for i, result := range results {
		trimmed[i] = m.TrimToolResult(result)
	}

	return trimmed
}

// trimContent trims content based on the strategy
func (m *MicroCompactor) trimContent(content string) string {
	switch m.trimStrategy {
	case TrimStrategyTruncate:
		return m.truncateContent(content)
	case TrimStrategySnip:
		return m.snipContent(content)
	case TrimStrategyPreview:
		return m.previewContent(content)
	default:
		return m.previewContent(content)
	}
}

// truncateContent truncates content to max size
func (m *MicroCompactor) truncateContent(content string) string {
	if len(content) <= m.maxToolResultSize {
		return content
	}

	return content[:m.maxToolResultSize]
}

// snipContent removes middle sections to keep important parts
func (m *MicroCompactor) snipContent(content string) string {
	if len(content) <= m.maxToolResultSize {
		return content
	}

	// Keep beginning and end
	keepSize := m.maxToolResultSize / 2
	beginning := content[:keepSize]
	ending := content[len(content)-keepSize:]

	snipMarker := "\n\n... [content snipped] ...\n\n"

	return beginning + snipMarker + ending
}

// previewContent returns a preview of the content
func (m *MicroCompactor) previewContent(content string) string {
	// Return first 1000 characters as preview
	previewSize := 1000
	if len(content) < previewSize {
		previewSize = len(content)
	}

	preview := content[:previewSize]

	// Add ellipsis if content is larger
	if len(content) > previewSize {
		preview += "\n\n... [" + formatSize(len(content)) + " total, preview truncated] ...\n\n"
	}

	return preview
}

// getPreview gets a short preview of content
func (m *MicroCompactor) getPreview(content string) string {
	previewSize := 200
	if len(content) < previewSize {
		previewSize = len(content)
	}

	return content[:previewSize] + "..."
}

// ShouldTrim returns true if content should be trimmed
func (m *MicroCompactor) ShouldTrim(content string) bool {
	return len(content) > m.maxToolResultSize
}

// CalculateTrimSize calculates how much would be trimmed
func (m *MicroCompactor) CalculateTrimSize(content string) int {
	if len(content) <= m.maxToolResultSize {
		return 0
	}

	return len(content) - m.maxToolResultSize
}

// EstimateToolResultTokens estimates tokens in a tool result
func (m *MicroCompactor) EstimateToolResultTokens(result string) int {
	// Rough estimate: ~4 characters per token
	return len(result) / 4
}

// CalculateToolResultSummary generates a summary of a tool result
func (m *MicroCompactor) CalculateToolResultSummary(result types.ToolResultContent) string {
	content := result.Content

	summary := formatSize(len(content))

	if m.ShouldTrim(content) {
		summary += " (trimmed)"
	}

	return summary
}

// formatSize formats a size in human-readable format
func formatSize(size int) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// TrimmedContent represents trimmed content with metadata
type TrimmedContent struct {
	// Content is the trimmed content
	Content string `json:"content"`

	// OriginalSize is the original size
	OriginalSize int `json:"original_size"`

	// TrimmedSize is the trimmed size
	TrimmedSize int `json:"trimmed_size"`

	// Strategy is the trim strategy used
	Strategy TrimStrategy `json:"strategy"`

	// Preview is a preview of the original content
	Preview string `json:"preview,omitempty"`
}

// TrimContentWithMeta trims content and returns metadata
func (m *MicroCompactor) TrimContentWithMeta(content string) TrimmedContent {
	trimmed := m.trimContent(content)

	return TrimmedContent{
		Content:      trimmed,
		OriginalSize: len(content),
		TrimmedSize:  len(trimmed),
		Strategy:     m.trimStrategy,
		Preview:      m.getPreview(content),
	}
}

// CountLines counts the number of lines in content
func CountLines(content string) int {
	if content == "" {
		return 0
	}

	return strings.Count(content, "\n") + 1
}

// CountTokensInContent estimates tokens in content
func CountTokensInContent(content string) int {
	// Rough estimate: ~4 characters per token
	return len(content) / 4
}
