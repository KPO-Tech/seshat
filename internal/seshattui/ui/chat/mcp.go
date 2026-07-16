package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// ─── MCP tool classification ──────────────────────────────────────────────────

type mcpToolCategory int

const (
	mcpCatRead    mcpToolCategory = iota // get, fetch, read, open, show → header-only on success
	mcpCatSearch                         // search, find, list, query, scan → always show body
	mcpCatCreate                         // create, write, add, insert, send → header-only on success
	mcpCatDestroy                        // delete, remove, destroy, drop, purge → destructive style + header-only on success
	mcpCatUpdate                         // update, edit, patch, modify → header-only on success
	mcpCatGeneric                        // fallback: show body if content exists
)

// mcpClassifyTool infers the semantic category from the tool name.
// Destroy is checked first so "delete_and_create" is treated as destructive.
func mcpClassifyTool(toolName string) mcpToolCategory {
	lower := strings.ToLower(toolName)
	switch {
	case mcpNameContains(lower, "delete", "remove", "destroy", "drop", "purge", "clear", "unlink", "revoke", "dismiss"):
		return mcpCatDestroy
	case mcpNameContains(lower, "create", "write", "add", "insert", "send", "post", "push", "publish", "upload", "append"):
		return mcpCatCreate
	case mcpNameContains(lower, "update", "edit", "patch", "modify", "rename", "move", "set", "replace", "put"):
		return mcpCatUpdate
	case mcpNameContains(lower, "search", "find", "list", "query", "scan", "browse", "lookup", "suggest", "enumerate"):
		return mcpCatSearch
	case mcpNameContains(lower, "read", "get", "fetch", "open", "show", "view", "download", "load", "describe", "info", "check", "inspect"):
		return mcpCatRead
	default:
		return mcpCatGeneric
	}
}

func mcpNameContains(lower string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// mcpPrimaryParam extracts the most informative single parameter from MCP input.
// Checks well-known semantic keys in priority order before falling back to any string value.
var mcpKnownParamKeys = []string{
	"path", "query", "url", "name", "message", "key", "id", "uri",
	"file", "command", "text", "content", "repo", "topic", "issue",
}

func mcpPrimaryParam(params map[string]any) string {
	for _, key := range mcpKnownParamKeys {
		if v, ok := params[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	// Fallback: first non-empty string value found
	for _, v := range params {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// ─── MCPTool renderer ─────────────────────────────────────────────────────────

// MCPToolMessageItem is a message item that represents an MCP tool call.
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
	return &MCPToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &MCPToolRenderContext{}, canceled)}
}

// MCPToolRenderContext renders generic MCP tool messages.
type MCPToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (b *MCPToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width

	// Tool name is "mcp_{server}_{tool}" — split into server + action.
	toolNameParts := strings.SplitN(opts.ToolCall.Name, "_", 3)
	if len(toolNameParts) != 3 {
		return toolErrorContent(sty, &message.ToolResult{Content: "Invalid tool name"}, cappedWidth)
	}
	mcpServer := toolNameParts[1]
	mcpTool := toolNameParts[2]
	cat := mcpClassifyTool(mcpTool)

	// Build styled "ServerName → ActionName" display.
	serverStyled := sty.Tool.MCPName.Render(humanizedToolName(mcpServer))
	arrow := sty.Tool.MCPArrow.String()

	var actionStyled string
	switch cat {
	case mcpCatDestroy:
		actionStyled = sty.Tool.ActionDestroy.Render(humanizedToolName(mcpTool))
	case mcpCatCreate:
		actionStyled = sty.Tool.ActionCreate.Render(humanizedToolName(mcpTool))
	default:
		actionStyled = sty.Tool.MCPToolName.Render(humanizedToolName(mcpTool))
	}
	name := fmt.Sprintf("%s %s %s", serverStyled, arrow, actionStyled)

	if opts.IsPending() {
		return pendingTool(sty, name, opts.Anim, opts.Compact)
	}

	// Extract the most informative single param for the header.
	var params map[string]any
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var toolParams []string
	if primary := mcpPrimaryParam(params); primary != "" {
		toolParams = append(toolParams, primary)
	} else if len(params) > 0 {
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

	// Show body when: error (all categories) OR successful search/generic with content.
	needsBody := opts.HasResult() &&
		opts.Result.Content != "" &&
		(opts.Result.IsError || cat == mcpCatSearch || cat == mcpCatGeneric)

	if !needsBody {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := renderToolResultTextContent(sty, opts.Result.Content, toolResultContentWidths{Body: bodyWidth, Diff: cappedWidth}, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// ─── MCP List Resources ───────────────────────────────────────────────────────

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
	return &MCPListResourcesToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &MCPListResourcesToolRenderContext{}, canceled)}
}

// MCPListResourcesToolRenderContext renders mcp_list_resources tool messages.
type MCPListResourcesToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (m *MCPListResourcesToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
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

// ─── MCP Read Resource ────────────────────────────────────────────────────────

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
	return &MCPReadResourceToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &MCPReadResourceToolRenderContext{}, canceled)}
}

// MCPReadResourceToolRenderContext renders mcp_read_resource tool messages.
type MCPReadResourceToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (m *MCPReadResourceToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
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

	// Read resource: header-only on success (URI already shown in header).
	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}

	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, content)
}
