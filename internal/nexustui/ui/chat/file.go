package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
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
	cappedWidth := width
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

	// On error show the error; success is silent (content is for the AI, not the UI).
	if opts.Result.IsError {
		return joinToolParts(header, toolErrorContent(sty, opts.Result, cappedWidth))
	}
	return header
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
	cappedWidth := width
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

	// Render content: interpreted markdown for .md files, syntax-highlighted code otherwise.
	if params.Content != "" {
		bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
		var body string
		if isMarkdownPath(params.FilePath) {
			body = sty.Tool.Body.Render(toolOutputMarkdownContent(sty, params.Content, bodyWidth, opts.ExpandedContent))
		} else {
			body = toolOutputCodeContent(sty, params.FilePath, params.Content, 0, cappedWidth, opts.ExpandedContent)
		}
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
	oldContent, newContent := extractEditDiffContent(opts.Result.Metadata)
	if oldContent == "" && newContent == "" {
		bodyWidth := width - toolBodyLeftPaddingTotal
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
		return joinToolParts(header, body)
	}

	diff := toolOutputDiffContent(sty, file, oldContent, newContent, width, opts.ExpandedContent)

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
// Remove File Tool
// -----------------------------------------------------------------------------

// RemoveFileToolMessageItem is a message item that represents a remove_file tool call.
type RemoveFileToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*RemoveFileToolMessageItem)(nil)

// NewRemoveFileToolMessageItem creates a new [RemoveFileToolMessageItem].
func NewRemoveFileToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &RemoveFileToolRenderContext{}, canceled)
}

// RemoveFileToolRenderContext renders remove_file tool messages.
type RemoveFileToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (r *RemoveFileToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Remove File", opts.Anim, opts.Compact)
	}

	var params tools.RemoveFileParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Remove File", cappedWidth)
	}

	path := fsext.PrettyPath(params.Path)
	header := toolHeader(sty, opts.Status, "Remove File", cappedWidth, opts.Compact, path)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}
	return joinToolParts(header, toolErrorContent(sty, opts.Result, cappedWidth))
}

// -----------------------------------------------------------------------------
// Create Directory Tool
// -----------------------------------------------------------------------------

// CreateDirectoryToolMessageItem is a message item that represents a create_directory tool call.
type CreateDirectoryToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*CreateDirectoryToolMessageItem)(nil)

// NewCreateDirectoryToolMessageItem creates a new [CreateDirectoryToolMessageItem].
func NewCreateDirectoryToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &CreateDirectoryToolRenderContext{}, canceled)
}

// CreateDirectoryToolRenderContext renders create_directory tool messages.
type CreateDirectoryToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (c *CreateDirectoryToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Create Directory", opts.Anim, opts.Compact)
	}

	var params tools.CreateDirectoryParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Create Directory", cappedWidth)
	}

	path := fsext.PrettyPath(params.Path)
	header := toolHeader(sty, opts.Status, "Create Directory", cappedWidth, opts.Compact, path)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}
	return joinToolParts(header, toolErrorContent(sty, opts.Result, cappedWidth))
}

// -----------------------------------------------------------------------------
// Get File Metadata Tool
// -----------------------------------------------------------------------------

// GetFileMetadataToolMessageItem is a message item that represents a get_file_metadata tool call.
type GetFileMetadataToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*GetFileMetadataToolMessageItem)(nil)

// NewGetFileMetadataToolMessageItem creates a new [GetFileMetadataToolMessageItem].
func NewGetFileMetadataToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &GetFileMetadataToolRenderContext{}, canceled)
}

// GetFileMetadataToolRenderContext renders get_file_metadata tool messages.
type GetFileMetadataToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (g *GetFileMetadataToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "File Metadata", opts.Anim, opts.Compact)
	}

	var params tools.GetFileMetadataParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "File Metadata", cappedWidth)
	}

	path := fsext.PrettyPath(params.Path)
	header := toolHeader(sty, opts.Status, "File Metadata", cappedWidth, opts.Compact, path)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}
	return joinToolParts(header, toolErrorContent(sty, opts.Result, cappedWidth))
}

// -----------------------------------------------------------------------------
// Apply Patch Tool
// -----------------------------------------------------------------------------

// ApplyPatchToolMessageItem is a message item that represents an apply_patch tool call.
type ApplyPatchToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ApplyPatchToolMessageItem)(nil)

// NewApplyPatchToolMessageItem creates a new [ApplyPatchToolMessageItem].
func NewApplyPatchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ApplyPatchToolRenderContext{}, canceled)
}

// ApplyPatchToolRenderContext renders apply_patch tool messages.
type ApplyPatchToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (a *ApplyPatchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Apply Patch", opts.Anim, opts.Compact)
	}

	var fileLines []string
	var added, updated, deleted, moved int
	if opts.HasResult() && opts.Result.Content != "" {
		added, updated, deleted, moved, fileLines = parseApplyPatchResult(opts.Result.Content)
	}

	total := added + updated + deleted + moved
	headerParams := []string{}
	if opts.HasResult() {
		if total > 0 {
			s := "files"
			if total == 1 {
				s = "file"
			}
			headerParams = append(headerParams, fmt.Sprintf("%d %s", total, s))
		} else if opts.Result.IsError {
			headerParams = append(headerParams, "failed")
		}
	}

	header := toolHeader(sty, opts.Status, "Apply Patch", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if len(fileLines) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := renderApplyPatchFileList(sty, fileLines, bodyWidth)
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

// parseApplyPatchResult parses the text returned by formatSummary into counts and lines.
// Lines follow "Added: path", "Updated: path", "Deleted: path", "Moved: path" format.
func parseApplyPatchResult(content string) (added, updated, deleted, moved int, lines []string) {
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "Patch applied (no files changed)." {
			continue
		}
		lines = append(lines, line)
		switch {
		case strings.HasPrefix(line, "Added:"):
			added++
		case strings.HasPrefix(line, "Updated:"):
			updated++
		case strings.HasPrefix(line, "Deleted:"):
			deleted++
		case strings.HasPrefix(line, "Moved:"):
			moved++
		}
	}
	return
}

// renderApplyPatchFileList renders each file change line with semantic color.
func renderApplyPatchFileList(sty *styles.Styles, lines []string, width int) string {
	var out []string
	for _, line := range lines {
		var sigil string
		var labelStyle lipgloss.Style
		var path string
		switch {
		case strings.HasPrefix(line, "Added: "):
			sigil = "+"
			labelStyle = sty.Tool.ResultAdded
			path = fsext.PrettyPath(strings.TrimPrefix(line, "Added: "))
		case strings.HasPrefix(line, "Updated: "):
			sigil = "~"
			labelStyle = sty.Tool.ContentText
			path = fsext.PrettyPath(strings.TrimPrefix(line, "Updated: "))
		case strings.HasPrefix(line, "Deleted: "):
			sigil = "-"
			labelStyle = sty.Tool.ResultDeleted
			path = fsext.PrettyPath(strings.TrimPrefix(line, "Deleted: "))
		case strings.HasPrefix(line, "Moved: "):
			sigil = "→"
			labelStyle = sty.Tool.ResultMoved
			path = strings.TrimPrefix(line, "Moved: ")
		default:
			out = append(out, sty.Tool.ContentText.Render(ansi.Truncate(line, width, "…")))
			continue
		}
		sigilStr := labelStyle.Render(sigil)
		pathStr := sty.Tool.ContentText.Render(ansi.Truncate(path, width-len(sigil)-1, "…"))
		out = append(out, sigilStr+" "+pathStr)
	}
	return strings.Join(out, "\n")
}

// extractEditDiffContent reads the raw metadata JSON from an edit_file result and
// returns the full file content before the edit (oldContent) and after (newContent).
// The edit tool stores the original file under "original_file" and the replaced
// snippet under "old_string"/"new_string", so newContent is computed by applying
// the replacement to the original file.
func extractEditDiffContent(metadataJSON string) (oldContent, newContent string) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &raw); err != nil {
		return "", ""
	}
	oldContent, _ = raw["original_file"].(string)
	oldString, _ := raw["old_string"].(string)
	newString, _ := raw["new_string"].(string)
	replaceAll, _ := raw["replace_all"].(bool)

	if oldContent != "" && oldString != "" {
		count := 1
		if replaceAll {
			count = -1
		}
		newContent = strings.Replace(oldContent, oldString, newString, count)
	}
	return
}

// isMarkdownPath reports whether the file path has a markdown extension.
func isMarkdownPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
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
	cappedWidth := width
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
