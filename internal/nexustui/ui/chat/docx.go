package chat

import (
	"encoding/json"
	"strings"

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
	if !opts.HasResult() || opts.Result.Content == "" {
		return header
	}

	// Content format: "Document: ...\nSuccess: ...\nMessage: ...\n[Content:\n...]"
	// Show only Message + Content lines — skip the Document/Success prefix noise.
	body := extractDocxBody(sty, opts.Result.Content, cappedWidth-toolBodyLeftPaddingTotal, opts.ExpandedContent)
	if body == "" {
		return header
	}
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

// extractDocxBody drops the first two lines ("Document:" / "Success:") and
// returns the remainder (Message + optional Content block).
func extractDocxBody(sty *styles.Styles, content string, width int, expanded bool) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var kept []string
	for _, l := range lines {
		if strings.HasPrefix(l, "Document:") || strings.HasPrefix(l, "Success:") {
			continue
		}
		kept = append(kept, l)
	}
	if len(kept) == 0 {
		return ""
	}
	return toolOutputPlainContent(sty, strings.Join(kept, "\n"), width, expanded)
}
