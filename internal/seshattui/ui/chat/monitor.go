package chat

import (
	"encoding/json"
	"strings"

	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// MonitorToolMessageItem represents a monitor tool call.
type MonitorToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*MonitorToolMessageItem)(nil)

func NewMonitorToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MonitorRenderContext{}, canceled)
}

type MonitorRenderContext struct{}

func (m *MonitorRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Monitor"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	cmd := strings.TrimSpace(params.Command)
	if cmd != "" {
		firstLine, _, _ := strings.Cut(cmd, "\n")
		headerParams = append(headerParams, ansi.Truncate(firstLine, 50, "…"))
	}
	if params.Description != "" {
		headerParams = append(headerParams, ansi.Truncate(params.Description, 40, "…"))
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// Background job — no output to show inline. Body only on error.
	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
