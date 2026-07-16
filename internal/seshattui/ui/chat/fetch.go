package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat/internal/seshattui/agent/tools"
	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// -----------------------------------------------------------------------------
// Fetch Tool
// -----------------------------------------------------------------------------

// FetchToolMessageItem is a message item that represents a fetch tool call.
type FetchToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*FetchToolMessageItem)(nil)

// NewFetchToolMessageItem creates a new [FetchToolMessageItem].
func NewFetchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &FetchToolRenderContext{}, canceled)
}

// FetchToolRenderContext renders fetch tool messages.
type FetchToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (f *FetchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Fetch", opts.Anim, opts.Compact)
	}

	var params tools.FetchParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Fetch", cappedWidth)
	}

	toolParams := []string{params.URL}
	if params.Format != "" {
		toolParams = append(toolParams, "format", params.Format)
	}
	if params.Timeout != 0 {
		toolParams = append(toolParams, "timeout", formatTimeout(params.Timeout))
	}

	header := toolHeader(sty, opts.Status, "Fetch", cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() {
		return header
	}

	// Determine file extension for syntax highlighting based on format.
	file := getFileExtensionForFormat(params.Format)
	body := toolOutputCodeContent(sty, file, opts.Result.Content, 0, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// getFileExtensionForFormat returns a filename with appropriate extension for syntax highlighting.
func getFileExtensionForFormat(format string) string {
	switch format {
	case "text":
		return "fetch.txt"
	case "html":
		return "fetch.html"
	default:
		return "fetch.md"
	}
}

// -----------------------------------------------------------------------------
// WebFetch Tool
// -----------------------------------------------------------------------------

// WebFetchToolMessageItem is a message item that represents a web_fetch tool call.
type WebFetchToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*WebFetchToolMessageItem)(nil)

// NewWebFetchToolMessageItem creates a new [WebFetchToolMessageItem].
func NewWebFetchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &WebFetchToolRenderContext{}, canceled)
}

// WebFetchToolRenderContext renders web_fetch tool messages.
type WebFetchToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (w *WebFetchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Web Fetch", opts.Anim, opts.Compact)
	}

	var params tools.WebFetchParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Web Fetch", cappedWidth)
	}

	header := toolHeader(sty, opts.Status, "Web Fetch", cappedWidth, opts.Compact, params.URL)
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

	var bodyParts []string

	// Show the prompt as a subtle context line before the fetched content.
	if params.Prompt != "" {
		prompt := sty.Tool.WebFetchPrompt.Render("↳ " + ansi.Truncate(params.Prompt, bodyWidth-2, "…"))
		bodyParts = append(bodyParts, sty.Tool.Body.Render(prompt))
	}

	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	bodyParts = append(bodyParts, content)

	return joinToolParts(header, strings.Join(bodyParts, "\n"))
}

// -----------------------------------------------------------------------------
// WebSearch Tool
// -----------------------------------------------------------------------------

// WebSearchToolMessageItem is a message item that represents a web_search tool call.
type WebSearchToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*WebSearchToolMessageItem)(nil)

// NewWebSearchToolMessageItem creates a new [WebSearchToolMessageItem].
func NewWebSearchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &WebSearchToolRenderContext{}, canceled)
}

// WebSearchToolRenderContext renders web_search tool messages.
type WebSearchToolRenderContext struct{}

// webSearchMeta is the relevant subset of the tool result metadata for web_search.
type webSearchMeta struct {
	Additional struct {
		ResultCount int    `json:"result_count"`
		Provider    string `json:"provider"`
	} `json:"additional"`
}

// webSearchHit holds one parsed search result.
type webSearchHit struct {
	title string
	url   string
	desc  string
}

const webSearchMaxVisible = 5

// RenderTool implements the [ToolRenderer] interface.
func (w *WebSearchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Web Search", opts.Anim, opts.Compact)
	}

	var params tools.WebSearchParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Web Search", cappedWidth)
	}

	// Read result count from metadata.
	var meta webSearchMeta
	if opts.HasResult() && opts.Result.Metadata != "" {
		json.Unmarshal([]byte(opts.Result.Metadata), &meta)
	}
	resultCount := meta.Additional.ResultCount

	// Build header: query + count.
	headerParams := []string{params.Query}
	if opts.HasResult() {
		if resultCount > 0 {
			noun := "results"
			if resultCount == 1 {
				noun = "result"
			}
			headerParams = append(headerParams, fmt.Sprintf("%d %s", resultCount, noun))
		} else {
			headerParams = append(headerParams, "no results")
		}
	}

	header := toolHeader(sty, opts.Status, "Web Search", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() || resultCount == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	hits := parseWebSearchContent(opts.Result.Content)
	body := renderWebSearchHits(sty, hits, bodyWidth, opts.ExpandedContent)
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

// parseWebSearchContent extracts structured hits from the text output of formatOutput.
// Expected format per hit:
//
//	N. Title
//	   URL
//	   Description (optional)
//	   Source: ... (ignored)
func parseWebSearchContent(content string) []webSearchHit {
	var hits []webSearchHit
	var current *webSearchHit
	for _, line := range strings.Split(content, "\n") {
		// Match "N. " prefix (1–3 digit result numbers).
		if dotIdx := strings.Index(line, ". "); dotIdx > 0 && dotIdx <= 3 {
			allDigits := true
			for _, c := range line[:dotIdx] {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				if current != nil {
					hits = append(hits, *current)
				}
				current = &webSearchHit{title: strings.TrimSpace(line[dotIdx+2:])}
				continue
			}
		}
		if current == nil {
			continue
		}
		if !strings.HasPrefix(line, "   ") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Source:") {
			continue
		}
		if current.url == "" && (strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")) {
			current.url = trimmed
		} else if current.desc == "" && current.url != "" {
			current.desc = trimmed
		}
	}
	if current != nil {
		hits = append(hits, *current)
	}
	return hits
}

// -----------------------------------------------------------------------------
// Read Document URL Tool
// -----------------------------------------------------------------------------

// ReadDocumentURLToolMessageItem represents a read_document_url tool call.
type ReadDocumentURLToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ReadDocumentURLToolMessageItem)(nil)

// NewReadDocumentURLToolMessageItem creates a new [ReadDocumentURLToolMessageItem].
func NewReadDocumentURLToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ReadDocumentURLToolRenderContext{}, canceled)
}

// ReadDocumentURLToolRenderContext renders read_document_url tool messages.
type ReadDocumentURLToolRenderContext struct{}

type readDocumentURLParams struct {
	URL      string `json:"url"`
	SavePath string `json:"save_path,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (r *ReadDocumentURLToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Read Document", opts.Anim, opts.Compact)
	}

	var params readDocumentURLParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Read Document", cappedWidth)
	}

	headerParams := []string{params.URL}
	if params.SavePath != "" {
		headerParams = append(headerParams, "→ "+params.SavePath)
	}

	header := toolHeader(sty, opts.Status, "Read Document", cappedWidth, opts.Compact, headerParams...)
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
	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	if params.Prompt != "" {
		prompt := sty.Tool.WebFetchPrompt.Render("↳ " + ansi.Truncate(params.Prompt, bodyWidth-2, "…"))
		promptBody := sty.Tool.Body.Render(prompt)
		return joinToolParts(header, strings.Join([]string{promptBody, content}, "\n"))
	}
	return joinToolParts(header, content)
}

// renderWebSearchHits renders a compact list of search hits with title, URL, and description.
func renderWebSearchHits(sty *styles.Styles, hits []webSearchHit, width int, expanded bool) string {
	maxVisible := webSearchMaxVisible
	if expanded {
		maxVisible = len(hits)
	}
	shown := min(maxVisible, len(hits))

	var out []string
	for i := 0; i < shown; i++ {
		h := hits[i]
		num := sty.Tool.ResultItemDesc.Render(fmt.Sprintf("%d.", i+1))
		numW := lipgloss.Width(num)
		title := sty.Tool.ResultItemName.Render(ansi.Truncate(h.title, width-numW-1, "…"))
		out = append(out, num+" "+title)
		if h.url != "" {
			out = append(out, "  "+sty.Tool.WebSearchURL.Render(ansi.Truncate(h.url, width-2, "…")))
		}
		if h.desc != "" {
			out = append(out, "  "+sty.Tool.ResultItemDesc.Render(ansi.Truncate(h.desc, width-2, "…")))
		}
		if i < shown-1 {
			out = append(out, "")
		}
	}

	if remaining := len(hits) - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}

	return strings.Join(out, "\n")
}
