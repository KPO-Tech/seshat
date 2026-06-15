package chat

import (
	"encoding/json"
	"fmt"
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

// BrowserToolRenderContext renders browser_* tool messages.
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
		name := strings.TrimPrefix(toolName, "browser_")
		return humanizedToolName(name)
	}
}

// browserInputParam extracts the primary display parameter from the tool input.
func browserInputParam(toolName string, inputJSON string, maxLen int) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &raw); err != nil {
		return ""
	}

	str := func(key string) string {
		if v, ok := raw[key].(string); ok && v != "" {
			return ansi.Truncate(v, maxLen, "…")
		}
		return ""
	}

	switch toolName {
	case "browser_open", "browser_navigate":
		return str("url")
	case "browser_click":
		return str("element_id")
	case "browser_type":
		if el := str("element_id"); el != "" {
			return el
		}
		return str("text")
	case "browser_press":
		return str("key")
	case "browser_scroll":
		dir := str("direction")
		if amt, ok := raw["amount"].(float64); ok && amt > 0 && dir != "" {
			return fmt.Sprintf("%s %s", dir, formatBrowserAmount(int(amt)))
		}
		return dir
	case "browser_search_content":
		return str("query")
	case "browser_select_page", "browser_close_page":
		return str("page_id")
	case "browser_extract":
		return str("selector")
	case "browser_screenshot", "browser_snapshot":
		return str("page_id")
	case "browser_wait":
		if dur := str("duration"); dur != "" {
			return dur
		}
		return str("condition")
	}
	return ""
}

// browserResultParam extracts a more specific display parameter from the result
// content, replacing or augmenting the input param for tools where the result
// provides better context (URL after navigation, counts for list tools).
func browserResultParam(toolName, content string, maxLen int) string {
	if content == "" {
		return ""
	}

	// For content/capture tools, extract the URL from the formatted result.
	switch toolName {
	case "browser_snapshot", "browser_extract", "browser_screenshot":
		for _, line := range strings.SplitN(content, "\n", 8) {
			line = strings.TrimSpace(line)
			if url, ok := strings.CutPrefix(line, "URL: "); ok && url != "" {
				return ansi.Truncate(url, maxLen, "…")
			}
		}

	// For list tools, count the entries.
	case "browser_list_pages":
		n := strings.Count(content, "\n- ")
		if strings.HasPrefix(content, "- ") {
			n++
		}
		if n > 0 {
			return fmt.Sprintf("%d pages", n)
		}
	case "browser_network_list":
		n := strings.Count(content, "\n- ")
		if strings.HasPrefix(content, "- ") {
			n++
		}
		if n > 0 {
			return fmt.Sprintf("%d requests", n)
		}
	case "browser_list_downloads":
		n := strings.Count(content, "\n- ")
		if strings.HasPrefix(content, "- ") {
			n++
		}
		if n > 0 {
			return fmt.Sprintf("%d downloads", n)
		}
	}
	return ""
}

// browserSuccessNoBody returns true for tools where a successful result needs
// no body — the header already communicates the outcome.
func browserSuccessNoBody(toolName string, result *message.ToolResult) bool {
	if result == nil || result.IsError {
		return false
	}
	switch toolName {
	case "browser_open", "browser_navigate",
		"browser_click", "browser_type", "browser_press", "browser_scroll", "browser_wait",
		"browser_select_page", "browser_close_page":
		return true
	}
	return false
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

	maxParam := cappedWidth / 2
	param := browserInputParam(opts.ToolCall.Name, opts.ToolCall.Input, maxParam)

	// For certain tools, the result provides a better/more specific header param.
	if opts.HasResult() {
		if rp := browserResultParam(opts.ToolCall.Name, opts.Result.Content, maxParam); rp != "" {
			param = rp
		}
	}

	var headerParams []string
	if param != "" {
		headerParams = []string{param}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() || browserSuccessNoBody(opts.ToolCall.Name, opts.Result) {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
