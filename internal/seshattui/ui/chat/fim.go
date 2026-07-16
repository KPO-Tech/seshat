package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// FIMToolMessageItem represents a code_complete (FIM) tool call.
type FIMToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*FIMToolMessageItem)(nil)

func NewFIMToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &FIMRenderContext{}, canceled)
}

type FIMRenderContext struct{}

type fimOutput struct {
	Completion   string `json:"completion"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	FinishReason string `json:"finish_reason"`
	TokensUsed   int    `json:"tokens_used"`
}

func (f *FIMRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Code Complete"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Prompt string `json:"prompt"`
		Suffix string `json:"suffix"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	// Use the last non-empty line of the prompt as the header context hint.
	var headerParams []string
	if params.Prompt != "" {
		lines := strings.Split(strings.TrimRight(params.Prompt, "\n "), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(lines[i]); l != "" {
				headerParams = append(headerParams, ansi.Truncate(l, 50, "…"))
				break
			}
		}
	}

	var out fimOutput
	hasParsed := false
	if opts.HasResult() && opts.Result.Content != "" {
		if err := json.Unmarshal([]byte(opts.Result.Content), &out); err == nil {
			hasParsed = true
			if out.Provider != "" && out.Model != "" {
				headerParams = append(headerParams, out.Provider+"/"+out.Model)
			} else if out.Provider != "" {
				headerParams = append(headerParams, out.Provider)
			}
			if out.TokensUsed > 0 {
				headerParams = append(headerParams, fmt.Sprintf("%d tokens", out.TokensUsed))
			}
		}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	if !hasParsed || out.Completion == "" {
		if opts.HasResult() && opts.Result.Content != "" {
			bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
			body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
			return joinToolParts(header, body)
		}
		return header
	}

	// Render the completion as a syntax-highlighted code block.
	// Pass empty path so the highlighter picks a generic lang.
	body := toolOutputCodeContent(sty, "", out.Completion, 0, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, body)
}
