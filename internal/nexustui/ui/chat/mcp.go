package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// MCPToolMessageItem is a message item that represents a bash tool call.
type MCPToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MCPToolMessageItem)(nil)

// NewMCPToolMessageItem creates a new [MCPToolMessageItem].
func NewMCPToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MCPToolRenderContext{}, canceled)
}

// MCPToolRenderContext renders bash tool messages.
type MCPToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (b *MCPToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	toolNameParts := strings.SplitN(opts.ToolCall.Name, "_", 3)
	if len(toolNameParts) != 3 {
		return toolErrorContent(sty, &message.ToolResult{Content: "Invalid tool name"}, cappedWidth)
	}
	mcpName := humanizedToolName(toolNameParts[1])
	toolName := humanizedToolName(toolNameParts[2])

	mcpName = sty.Tool.MCPName.Render(mcpName)
	toolName = sty.Tool.MCPToolName.Render(toolName)

	name := fmt.Sprintf("%s %s %s", mcpName, sty.Tool.MCPArrow.String(), toolName)

	if opts.IsPending() {
		return pendingTool(sty, name, opts.Anim, opts.Compact)
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, name, cappedWidth)
	}

	var toolParams []string
	if len(params) > 0 {
		parsed, _ := json.Marshal(params)
		toolParams = append(toolParams, string(parsed))
	}

	header := toolHeader(sty, opts.Status, name, cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() || opts.Result.Content == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := renderToolResultTextContent(sty, opts.Result.Content, toolResultContentWidths{Body: bodyWidth, Diff: cappedWidth}, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// -----------------------------------------------------------------------------
// MCP List Resources Tool
// -----------------------------------------------------------------------------

// MCPListResourcesToolMessageItem represents a mcp_list_resources tool call.
type MCPListResourcesToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MCPListResourcesToolMessageItem)(nil)

// NewMCPListResourcesToolMessageItem creates a new [MCPListResourcesToolMessageItem].
func NewMCPListResourcesToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MCPListResourcesToolRenderContext{}, canceled)
}

// MCPListResourcesToolRenderContext renders mcp_list_resources tool messages.
type MCPListResourcesToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (m *MCPListResourcesToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "MCP Resources", opts.Anim, opts.Compact)
	}

	header := toolHeader(sty, opts.Status, "MCP Resources", cappedWidth, opts.Compact)
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
// MCP Read Resource Tool
// -----------------------------------------------------------------------------

// MCPReadResourceToolMessageItem represents a mcp_read_resource tool call.
type MCPReadResourceToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MCPReadResourceToolMessageItem)(nil)

// NewMCPReadResourceToolMessageItem creates a new [MCPReadResourceToolMessageItem].
func NewMCPReadResourceToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MCPReadResourceToolRenderContext{}, canceled)
}

// MCPReadResourceToolRenderContext renders mcp_read_resource tool messages.
type MCPReadResourceToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (m *MCPReadResourceToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "MCP Read Resource", opts.Anim, opts.Compact)
	}

	var params struct {
		URI string `json:"uri"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.URI != "" {
		headerParams = append(headerParams, ansi.Truncate(params.URI, cappedWidth/2, "…"))
	}

	header := toolHeader(sty, opts.Status, "MCP Read Resource", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() {
		return header
	}

	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, content)
}
