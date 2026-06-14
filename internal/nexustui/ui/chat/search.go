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
// Glob Tool
// -----------------------------------------------------------------------------

const globMaxVisible = 8

// GlobToolMessageItem is a message item that represents a glob tool call.
type GlobToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*GlobToolMessageItem)(nil)

// NewGlobToolMessageItem creates a new [GlobToolMessageItem].
func NewGlobToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &GlobToolRenderContext{}, canceled)
}

// GlobToolRenderContext renders glob tool messages.
type GlobToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (g *GlobToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Glob", opts.Anim, opts.Compact)
	}

	var params tools.GlobParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Glob", cappedWidth)
	}

	var count int
	var filenames []string
	var truncated bool
	if opts.HasResult() && opts.Result.Content != "" {
		count, filenames, truncated = parseGlobContent(opts.Result.Content)
	}

	headerParams := []string{params.Pattern}
	if opts.HasResult() {
		if count > 0 {
			s := "files"
			if count == 1 {
				s = "file"
			}
			countStr := fmt.Sprintf("%d %s", count, s)
			if truncated {
				countStr += "+"
			}
			headerParams = append(headerParams, countStr)
		} else {
			headerParams = append(headerParams, "no results")
		}
	}

	header := toolHeader(sty, opts.Status, "Glob", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if len(filenames) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := renderFileList(sty, filenames, bodyWidth, opts.ExpandedContent)
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

// parseGlobContent splits the glob Content into count + filenames.
// Expected format (set by formatGlobResult):
//
//	"Found N files in Xms[\n(results limited to 100 files)]\nfile1\nfile2\n..."
//	or "No files found"
func parseGlobContent(content string) (count int, filenames []string, truncated bool) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return 0, nil, false
	}
	first := lines[0]
	if strings.Contains(first, "results limited") {
		truncated = true
	}
	if idx := strings.Index(first, "Found "); idx >= 0 {
		var n int
		if _, err := fmt.Sscanf(first[idx+6:], "%d", &n); err == nil {
			count = n
		}
	}
	for _, l := range lines[1:] {
		if l = strings.TrimSpace(l); l != "" {
			filenames = append(filenames, l)
		}
	}
	return count, filenames, truncated
}

func renderFileList(sty *styles.Styles, filenames []string, width int, expanded bool) string {
	maxVisible := globMaxVisible
	if expanded {
		maxVisible = len(filenames)
	}
	shown := min(maxVisible, len(filenames))

	var out []string
	for i := 0; i < shown; i++ {
		name := filenames[i]
		rendered := sty.Tool.ContentText.Render(ansi.Truncate(name, width, "…"))
		out = append(out, rendered)
	}

	if remaining := len(filenames) - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}

	return strings.Join(out, "\n")
}

// -----------------------------------------------------------------------------
// Grep Tool
// -----------------------------------------------------------------------------

const grepMaxVisible = 6

// GrepToolMessageItem is a message item that represents a grep tool call.
type GrepToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*GrepToolMessageItem)(nil)

// NewGrepToolMessageItem creates a new [GrepToolMessageItem].
func NewGrepToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &GrepToolRenderContext{}, canceled)
}

// GrepToolRenderContext renders grep tool messages.
type GrepToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (g *GrepToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Grep", opts.Anim, opts.Compact)
	}

	var params tools.GrepParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Grep", cappedWidth)
	}

	headerParams := []string{params.Pattern}
	if params.Include != "" {
		headerParams = append(headerParams, params.Include)
	}

	var fileCount, matchCount int
	if opts.HasResult() && opts.Result.Content != "" {
		fileCount, matchCount = parseGrepContent(opts.Result.Content)
		if fileCount > 0 {
			headerParams = append(headerParams, fmt.Sprintf("%d files", fileCount))
		}
		if matchCount > 0 {
			headerParams = append(headerParams, fmt.Sprintf("%d matches", matchCount))
		} else if fileCount == 0 {
			headerParams = append(headerParams, "no results")
		}
	}

	header := toolHeader(sty, opts.Status, "Grep", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() || matchCount == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := renderGrepMatches(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent)
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

// parseGrepContent counts unique files and total match lines from ripgrep output.
// Lines follow "file:line:content" or "file:content" format.
func parseGrepContent(content string) (fileCount, matchCount int) {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if line == "" {
			continue
		}
		matchCount++
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			seen[line[:idx]] = struct{}{}
		}
	}
	return len(seen), matchCount
}

func renderGrepMatches(sty *styles.Styles, content string, width int, expanded bool) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	maxVisible := grepMaxVisible
	if expanded {
		maxVisible = len(lines)
	}
	shown := min(maxVisible, len(lines))

	var out []string
	for i := 0; i < shown; i++ {
		line := lines[i]
		// "file:rest" — file prefix slightly brighter than match content
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			filePart := sty.Tool.ContentText.Render(line[:idx])
			rest := sty.Tool.ResultItemDesc.Render(ansi.Truncate(line[idx:], width-lipgloss.Width(filePart), "…"))
			out = append(out, filePart+rest)
		} else {
			out = append(out, sty.Tool.ContentText.Render(ansi.Truncate(line, width, "…")))
		}
	}

	if remaining := len(lines) - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more matches", remaining)))
	}

	return strings.Join(out, "\n")
}

// -----------------------------------------------------------------------------
// List Directory Tool
// -----------------------------------------------------------------------------

const lsMaxVisible = 10

// lsDirEntry mirrors the JSON shape returned by the list_directory tool.
type lsDirEntry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	SizeBytes   int64  `json:"size_bytes"`
}

// lsDirResult is the top-level JSON returned by list_directory.
type lsDirResult struct {
	Path      string       `json:"path"`
	Entries   []lsDirEntry `json:"entries"`
	Count     int          `json:"count"`
	Truncated bool         `json:"truncated"`
}

// LSToolMessageItem is a message item that represents an ls tool call.
type LSToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*LSToolMessageItem)(nil)

// NewLSToolMessageItem creates a new [LSToolMessageItem].
func NewLSToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &LSToolRenderContext{}, canceled)
}

// LSToolRenderContext renders ls tool messages.
type LSToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (l *LSToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "List Directory", opts.Anim, opts.Compact)
	}

	var params tools.LSParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "List Directory", cappedWidth)
	}

	path := params.Path
	if path == "" {
		path = "."
	}
	path = fsext.PrettyPath(path)

	headerParams := []string{path}

	var dir lsDirResult
	hasParsed := false
	if opts.HasResult() && opts.Result.Content != "" {
		if err := json.Unmarshal([]byte(opts.Result.Content), &dir); err == nil {
			hasParsed = true
			countStr := fmt.Sprintf("%d items", dir.Count)
			if dir.Truncated {
				countStr += "+"
			}
			headerParams = append(headerParams, countStr)
		}
	}

	header := toolHeader(sty, opts.Status, "List Directory", cappedWidth, opts.Compact, headerParams...)
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

	if !hasParsed {
		// Fallback: render raw content if JSON parse failed
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
		return joinToolParts(header, body)
	}

	if len(dir.Entries) == 0 {
		empty := sty.Tool.ResultEmpty.Render("empty directory")
		return joinToolParts(header, sty.Tool.Body.Render(empty))
	}

	body := renderDirEntries(sty, dir.Entries, dir.Count, bodyWidth, opts.ExpandedContent)
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

func renderDirEntries(sty *styles.Styles, entries []lsDirEntry, total int, width int, expanded bool) string {
	maxVisible := lsMaxVisible
	if expanded {
		maxVisible = len(entries)
	}
	shown := min(maxVisible, len(entries))

	// Measure max display-name length across visible entries for size alignment.
	const sizeColWidth = 8 // enough for "999.9 MB"
	nameColWidth := width - sizeColWidth - 1
	if nameColWidth < 12 {
		nameColWidth = width
	}

	// First pass: collect display names and sizes.
	type row struct {
		name    string
		size    string
		nameLen int
	}
	rows := make([]row, shown)
	maxNameLen := 0
	for i := 0; i < shown; i++ {
		e := entries[i]
		name := e.Name
		if e.IsDirectory {
			name += "/"
		}
		size := ""
		if !e.IsDirectory {
			size = formatSize(int(e.SizeBytes))
		}
		nl := len([]rune(name))
		if nl > maxNameLen {
			maxNameLen = nl
		}
		rows[i] = row{name, size, nl}
	}
	if maxNameLen > nameColWidth {
		maxNameLen = nameColWidth
	}

	// Second pass: render aligned rows.
	var out []string
	for _, r := range rows {
		truncName := ansi.Truncate(r.name, nameColWidth, "…")
		nameStr := sty.Tool.ContentText.Render(truncName)
		if r.size == "" {
			out = append(out, nameStr)
		} else {
			pad := strings.Repeat(" ", max(0, maxNameLen-r.nameLen+2))
			sizeStr := sty.Tool.ResultItemDesc.Render(pad + r.size)
			out = append(out, nameStr+sizeStr)
		}
	}

	if remaining := total - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}

	return strings.Join(out, "\n")
}

// -----------------------------------------------------------------------------
// Sourcegraph Tool
// -----------------------------------------------------------------------------

// SourcegraphToolMessageItem is a message item that represents a sourcegraph tool call.
type SourcegraphToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SourcegraphToolMessageItem)(nil)

// NewSourcegraphToolMessageItem creates a new [SourcegraphToolMessageItem].
func NewSourcegraphToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &SourcegraphToolRenderContext{}, canceled)
}

// SourcegraphToolRenderContext renders sourcegraph tool messages.
type SourcegraphToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (s *SourcegraphToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Sourcegraph", opts.Anim, opts.Compact)
	}

	var params tools.SourcegraphParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Sourcegraph", cappedWidth)
	}

	toolParams := []string{params.Query}
	if params.Count != 0 {
		toolParams = append(toolParams, "count", formatNonZero(params.Count))
	}
	if params.ContextWindow != 0 {
		toolParams = append(toolParams, "context", formatNonZero(params.ContextWindow))
	}

	header := toolHeader(sty, opts.Status, "Sourcegraph", cappedWidth, opts.Compact, toolParams...)
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

// -----------------------------------------------------------------------------
// Tool Search Tool
// -----------------------------------------------------------------------------

// ToolSearchToolMessageItem represents a tool_search tool call.
type ToolSearchToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ToolSearchToolMessageItem)(nil)

// NewToolSearchToolMessageItem creates a new [ToolSearchToolMessageItem].
func NewToolSearchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ToolSearchToolRenderContext{}, canceled)
}

// ToolSearchToolRenderContext renders tool_search tool messages.
type ToolSearchToolRenderContext struct{}

type toolSearchParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type toolSearchOutput struct {
	Query   string `json:"query"`
	Matches []struct {
		Name       string `json:"name"`
		SearchHint string `json:"search_hint,omitempty"`
		Category   string `json:"category,omitempty"`
		IsMCP      bool   `json:"is_mcp"`
	} `json:"matches"`
	TotalTools int `json:"total_tools"`
	Scored     int `json:"scored"`
}

const toolSearchMaxVisible = 6

// RenderTool implements the [ToolRenderer] interface.
func (t *ToolSearchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Tool Search", opts.Anim, opts.Compact)
	}

	var params toolSearchParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Tool Search", cappedWidth)
	}

	headerParams := []string{params.Query}

	var out toolSearchOutput
	hasParsed := false
	if opts.HasResult() && opts.Result.Content != "" {
		if err := json.Unmarshal([]byte(opts.Result.Content), &out); err == nil {
			hasParsed = true
			n := len(out.Matches)
			noun := "results"
			if n == 1 {
				noun = "result"
			}
			headerParams = append(headerParams, fmt.Sprintf("%d %s", n, noun))
		}
	}

	header := toolHeader(sty, opts.Status, "Tool Search", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() || !hasParsed || len(out.Matches) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	maxVisible := toolSearchMaxVisible
	if opts.ExpandedContent {
		maxVisible = len(out.Matches)
	}
	shown := min(maxVisible, len(out.Matches))

	var rows []string
	for i := 0; i < shown; i++ {
		m := out.Matches[i]
		name := sty.Tool.ResultItemName.Render(m.Name)
		if m.IsMCP {
			name = sty.Tool.MCPName.Render("mcp") + " " + name
		}
		if m.SearchHint != "" {
			hint := sty.Tool.ResultItemDesc.Render(" · " + m.SearchHint)
			nameW := lipgloss.Width(name)
			hintW := bodyWidth - nameW - 1
			if hintW > 10 {
				hint = sty.Tool.ResultItemDesc.Render(" · " + m.SearchHint[:min(len(m.SearchHint), hintW-3)])
				if len(m.SearchHint) > hintW-3 {
					hint += sty.Tool.ResultItemDesc.Render("…")
				}
			}
			rows = append(rows, name+hint)
		} else {
			rows = append(rows, name)
		}
	}
	if remaining := len(out.Matches) - shown; remaining > 0 {
		rows = append(rows, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}

	body := sty.Tool.Body.Render(strings.Join(rows, "\n"))
	return joinToolParts(header, body)
}
