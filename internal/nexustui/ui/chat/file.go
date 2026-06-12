package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

// -----------------------------------------------------------------------------
// View Tool
// -----------------------------------------------------------------------------

// ViewToolMessageItem is a message item that represents a view tool call.
type ViewToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ViewToolMessageItem)(nil)

// NewViewToolMessageItem creates a new [ViewToolMessageItem].
func NewViewToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ViewToolRenderContext{}, canceled)
}

// ViewToolRenderContext renders view tool messages.
type ViewToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (v *ViewToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "Read File", opts.Anim, opts.Compact)
	}

	var params tools.ViewParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Read File", cappedWidth)
	}

	file := fsext.PrettyPath(params.FilePath)
	toolParams := []string{file}
	if params.Limit != 0 {
		toolParams = append(toolParams, "limit", fmt.Sprintf("%d", params.Limit))
	}
	if params.Offset != 0 {
		toolParams = append(toolParams, "offset", fmt.Sprintf("%d", params.Offset))
	}

	header := toolHeader(sty, opts.Status, "Read File", cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() {
		return header
	}

	// Handle image content.
	if opts.Result.Data != "" && strings.HasPrefix(opts.Result.MIMEType, "image/") {
		body := toolOutputImageContent(sty, opts.Result.Data, opts.Result.MIMEType)
		return joinToolParts(header, body)
	}

	var meta tools.ViewResponseMetadata
	if err := json.Unmarshal([]byte(opts.Result.Metadata), &meta); err == nil && meta.ResourceType == tools.ViewResourceSkill {
		body := toolOutputSkillContent(sty, meta.ResourceName, meta.ResourceDescription)
		return joinToolParts(header, body)
	}

	summary := buildViewToolSummary(params, opts.Result.Content)
	if summary == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, summary, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

func buildViewToolSummary(params tools.ViewParams, content string) string {
	var lines []string

	if summary := extractReadFileLinesSummary(content); summary != "" {
		lines = append(lines, summary)
	} else {
		if params.Offset > 0 && params.Limit > 0 {
			lines = append(lines, fmt.Sprintf("Lines requested: %d-%d", params.Offset, params.Offset+params.Limit-1))
		} else if params.Offset > 0 {
			lines = append(lines, fmt.Sprintf("Starting from line %d", params.Offset))
		} else if params.Limit > 0 {
			lines = append(lines, fmt.Sprintf("Requested up to %d lines", params.Limit))
		}
	}

	lines = append(lines, "Content hidden in transcript")
	return strings.Join(lines, "\n")
}

func extractReadFileLinesSummary(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Lines:") {
			return trimmed
		}
	}
	return ""
}

// -----------------------------------------------------------------------------
// Write Tool
// -----------------------------------------------------------------------------

// WriteToolMessageItem is a message item that represents a write tool call.
type WriteToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*WriteToolMessageItem)(nil)

// NewWriteToolMessageItem creates a new [WriteToolMessageItem].
func NewWriteToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &WriteToolRenderContext{}, canceled)
}

// WriteToolRenderContext renders write tool messages.
type WriteToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (w *WriteToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "Write File", opts.Anim, opts.Compact)
	}

	var params tools.WriteParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Write File", cappedWidth)
	}

	file := fsext.PrettyPath(params.FilePath)
	header := toolHeader(sty, opts.Status, "Write File", cappedWidth, opts.Compact, file)
	if opts.Compact {
		return header
	}

	if !opts.HasResult() {
		if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
			return joinToolParts(header, earlyState)
		}
		return header
	}

	// On error with diff metadata (e.g. denied permission), show error + diff.
	if opts.Result.IsError {
		var meta tools.WriteResponseMetadata
		if err := json.Unmarshal([]byte(opts.Result.Metadata), &meta); err == nil && meta.Diff != "" {
			errLine := toolErrorContent(sty, opts.Result, cappedWidth)
			diff := toolOutputDiffContentFromUnified(sty, meta.Diff, cappedWidth, opts.ExpandedContent)
			return strings.Join([]string{header, "", errLine, "", diff}, "\n")
		}
		return joinToolParts(header, toolErrorContent(sty, opts.Result, cappedWidth))
	}

	// Render code content with syntax highlighting.
	if params.Content != "" {
		body := toolOutputCodeContent(sty, params.FilePath, params.Content, 0, cappedWidth, opts.ExpandedContent)
		return joinToolParts(header, body)
	}

	return header
}

// -----------------------------------------------------------------------------
// Edit Tool
// -----------------------------------------------------------------------------

// EditToolMessageItem is a message item that represents an edit tool call.
type EditToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*EditToolMessageItem)(nil)

// NewEditToolMessageItem creates a new [EditToolMessageItem].
func NewEditToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &EditToolRenderContext{}, canceled)
}

// EditToolRenderContext renders edit tool messages.
type EditToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (e *EditToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	// Edit tool uses full width for diffs.
	if opts.IsPending() {
		return pendingTool(sty, "Edit File", opts.Anim, opts.Compact)
	}

	var params tools.EditParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Edit File", width)
	}

	file := fsext.PrettyPath(params.FilePath)
	header := toolHeader(sty, opts.Status, "Edit File", width, opts.Compact, file)
	if opts.Compact {
		return header
	}

	if !opts.HasResult() {
		if earlyState, ok := toolEarlyStateContent(sty, opts, width); ok {
			return joinToolParts(header, earlyState)
		}
		return header
	}

	// Get diff content from metadata.
	var meta tools.EditResponseMetadata
	if err := json.Unmarshal([]byte(opts.Result.Metadata), &meta); err != nil {
		bodyWidth := width - toolBodyLeftPaddingTotal
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
		return joinToolParts(header, body)
	}

	diff := toolOutputDiffContent(sty, file, meta.OldContent, meta.NewContent, width, opts.ExpandedContent)

	// On error (e.g. denied permission), show error above the diff.
	if opts.Result.IsError {
		errLine := toolErrorContent(sty, opts.Result, width)
		return strings.Join([]string{header, "", errLine, "", diff}, "\n")
	}

	return joinToolParts(header, diff)
}

// -----------------------------------------------------------------------------
// MultiEdit Tool
// -----------------------------------------------------------------------------

// MultiEditToolMessageItem is a message item that represents a multi-edit tool call.
type MultiEditToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MultiEditToolMessageItem)(nil)

// NewMultiEditToolMessageItem creates a new [MultiEditToolMessageItem].
func NewMultiEditToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MultiEditToolRenderContext{}, canceled)
}

// MultiEditToolRenderContext renders multi-edit tool messages.
type MultiEditToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (m *MultiEditToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	// MultiEdit tool uses full width for diffs.
	if opts.IsPending() {
		return pendingTool(sty, "Multi-Edit", opts.Anim, opts.Compact)
	}

	var params tools.MultiEditParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Multi-Edit", width)
	}

	file := fsext.PrettyPath(params.FilePath)
	toolParams := []string{file}
	if len(params.Edits) > 0 {
		toolParams = append(toolParams, "edits", fmt.Sprintf("%d", len(params.Edits)))
	}

	header := toolHeader(sty, opts.Status, "Multi-Edit", width, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if !opts.HasResult() {
		if earlyState, ok := toolEarlyStateContent(sty, opts, width); ok {
			return joinToolParts(header, earlyState)
		}
		return header
	}

	// Get diff content from metadata.
	var meta tools.MultiEditResponseMetadata
	if err := json.Unmarshal([]byte(opts.Result.Metadata), &meta); err != nil {
		bodyWidth := width - toolBodyLeftPaddingTotal
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
		return joinToolParts(header, body)
	}

	// Render diff with optional failed edits note.
	diff := toolOutputMultiEditDiffContent(sty, file, meta, len(params.Edits), width, opts.ExpandedContent)

	// On error (e.g. denied permission), show error above the diff.
	if opts.Result.IsError {
		errLine := toolErrorContent(sty, opts.Result, width)
		return strings.Join([]string{header, "", errLine, "", diff}, "\n")
	}

	return joinToolParts(header, diff)
}

// -----------------------------------------------------------------------------
// Download Tool
// -----------------------------------------------------------------------------

// DownloadToolMessageItem is a message item that represents a download tool call.
type DownloadToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*DownloadToolMessageItem)(nil)

// NewDownloadToolMessageItem creates a new [DownloadToolMessageItem].
func NewDownloadToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &DownloadToolRenderContext{}, canceled)
}

// DownloadToolRenderContext renders download tool messages.
type DownloadToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (d *DownloadToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "Download", opts.Anim, opts.Compact)
	}

	var params tools.DownloadParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Download", cappedWidth)
	}

	toolParams := []string{params.URL}
	if params.FilePath != "" {
		toolParams = append(toolParams, "file_path", fsext.PrettyPath(params.FilePath))
	}
	if params.Timeout != 0 {
		toolParams = append(toolParams, "timeout", formatTimeout(params.Timeout))
	}

	header := toolHeader(sty, opts.Status, "Download", cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
