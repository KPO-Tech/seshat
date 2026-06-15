package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

// LSPToolMessageItem represents an lsp tool call.
type LSPToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*LSPToolMessageItem)(nil)

func NewLSPToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &LSPRenderContext{}, canceled)
}

type LSPRenderContext struct{}

func (l *LSPRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "LSP"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Operation string  `json:"operation"`
		FilePath  string  `json:"file_path"`
		Line      float64 `json:"line"`
		Column    float64 `json:"column"`
		Query     string  `json:"query"`
		Server    string  `json:"server"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Operation != "" {
		headerParams = append(headerParams, params.Operation)
	}
	if params.FilePath != "" {
		loc := fsext.PrettyPath(params.FilePath)
		if params.Line > 0 {
			if params.Column > 0 {
				loc = fmt.Sprintf("%s:%d:%d", loc, int(params.Line), int(params.Column))
			} else {
				loc = fmt.Sprintf("%s:%d", loc, int(params.Line))
			}
		}
		headerParams = append(headerParams, loc)
	}
	if params.Query != "" {
		headerParams = append(headerParams, params.Query)
	}

	// Enrich with summary from result content (comes from FormatResult which
	// serializes map[operation:... result:... summary:...] via %v).
	var summary string
	if opts.HasResult() && opts.Result.Content != "" {
		summary = extractLSPSummary(opts.Result.Content)
		if summary != "" {
			headerParams = append(headerParams, summary)
		}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact || summary != "" {
		// When we have a summary in the header, the body adds little value.
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	if !opts.HasResult() || opts.Result.Content == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// extractLSPSummary pulls the human-readable summary from the %v-formatted
// map string produced by lsp.Tool.FormatResult.
// Input looks like: "map[operation:hover result:... summary:Found 2 location(s)]"
func extractLSPSummary(content string) string {
	const key = "summary:"
	idx := strings.Index(content, key)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(content[idx+len(key):])
	// The summary value ends at the closing ']' of the outer map, or at a
	// space-followed-by-a-key pattern like " operation:".  We take the first
	// ']' as the simplest safe terminator.
	if end := strings.IndexByte(rest, ']'); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return rest
}
