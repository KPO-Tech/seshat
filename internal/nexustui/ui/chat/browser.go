package chat

import (
	"encoding/json"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// BrowserToolMessageItem represents any browser_* tool call.
type BrowserToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*BrowserToolMessageItem)(nil)

// NewBrowserToolMessageItem creates a new [BrowserToolMessageItem].
func NewBrowserToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &BrowserToolRenderContext{}, canceled)
}

// BrowserToolRenderContext renders browser_* tool messages with a generic
// handler that extracts the most meaningful display param per tool name.
type BrowserToolRenderContext struct{}

// browserDisplayName returns the human-readable label for a browser tool.
func browserDisplayName(toolName string) string {
	switch toolName {
	case "browser_open":
		return "Browser Open"
	case "browser_navigate":
		return "Browser Navigate"
	case "browser_screenshot":
		return "Screenshot"
	case "browser_snapshot":
		return "Page Snapshot"
	case "browser_extract":
		return "Extract"
	case "browser_list_pages":
		return "Browser Pages"
	case "browser_network_list":
		return "Network Traffic"
	case "browser_list_downloads":
		return "Downloads"
	case "browser_search_content":
		return "Search Page"
	case "browser_get_network_policy":
		return "Network Policy"
	case "browser_set_network_policy":
		return "Set Network Policy"
	case "browser_select_page":
		return "Select Page"
	case "browser_close_page":
		return "Close Page"
	case "browser_click":
		return "Click"
	case "browser_type":
		return "Type"
	case "browser_press":
		return "Press Key"
	case "browser_scroll":
		return "Scroll"
	case "browser_wait":
		return "Browser Wait"
	default:
		// Fallback: humanize "browser_foo_bar" → "Foo Bar"
		name := strings.TrimPrefix(toolName, "browser_")
		return humanizedToolName(name)
	}
}

// browserExtractParam parses the tool input JSON and returns the most
// meaningful display parameter (URL, selector, key, text, …).
func browserExtractParam(toolName string, inputJSON string, cappedWidth int) []string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &raw); err != nil {
		return nil
	}

	maxParam := cappedWidth / 2

	getString := func(key string) string {
		if v, ok := raw[key].(string); ok && v != "" {
			return ansi.Truncate(v, maxParam, "…")
		}
		return ""
	}

	switch toolName {
	case "browser_open", "browser_navigate":
		if url := getString("url"); url != "" {
			return []string{url}
		}
	case "browser_click":
		if el := getString("element_id"); el != "" {
			return []string{el}
		}
	case "browser_type":
		var params []string
		if el := getString("element_id"); el != "" {
			params = append(params, el)
		}
		if text := getString("text"); text != "" {
			params = append(params, ansi.Truncate(text, maxParam, "…"))
		}
		return params
	case "browser_press":
		if key := getString("key"); key != "" {
			return []string{key}
		}
	case "browser_scroll":
		if dir := getString("direction"); dir != "" {
			if amt, ok := raw["amount"].(float64); ok && amt != 0 {
				return []string{dir, formatBrowserAmount(int(amt))}
			}
			return []string{dir}
		}
	case "browser_search_content":
		if q := getString("query"); q != "" {
			return []string{q}
		}
	case "browser_select_page", "browser_close_page":
		if pid := getString("page_id"); pid != "" {
			return []string{pid}
		}
	case "browser_extract":
		if sel := getString("selector"); sel != "" {
			return []string{sel}
		}
	case "browser_screenshot":
		if pid := getString("page_id"); pid != "" {
			return []string{pid}
		}
	case "browser_snapshot":
		if pid := getString("page_id"); pid != "" {
			return []string{pid}
		}
	case "browser_wait":
		if dur := getString("duration"); dur != "" {
			return []string{dur}
		}
	}
	return nil
}

func formatBrowserAmount(n int) string {
	if n >= 1000 {
		return "large"
	}
	if n <= 0 {
		return ""
	}
	return strings.Repeat("↓", min(n/100, 5))
}

// RenderTool implements the [ToolRenderer] interface.
func (b *BrowserToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	displayName := browserDisplayName(opts.ToolCall.Name)

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	headerParams := browserExtractParam(opts.ToolCall.Name, opts.ToolCall.Input, cappedWidth)
	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
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
