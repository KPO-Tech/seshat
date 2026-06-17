package chat

import (
	"encoding/json"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// DocxToolMessageItem represents a docx tool call.
type DocxToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*DocxToolMessageItem)(nil)

func NewDocxToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &DocxRenderContext{}, canceled)
}

type DocxRenderContext struct{}

func (d *DocxRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Docx"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		DocumentPath string `json:"document_path"`
		Action       string `json:"action"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Action != "" {
		headerParams = append(headerParams, params.Action)
	}
	if params.DocumentPath != "" {
		headerParams = append(headerParams, ansi.Truncate(fsext.PrettyPath(params.DocumentPath), 50, "…"))
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// Body only on error — on success the header is self-explanatory.
	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
