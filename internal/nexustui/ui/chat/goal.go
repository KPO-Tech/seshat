package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// parseGoalStatus extracts the status from the formatted Content string produced
// by formatGoalSummary, e.g. "Goal created (status: active)\nObjective: …".
func parseGoalStatus(content string) string {
	first, _, _ := strings.Cut(content, "\n")
	lo := strings.ToLower(first)
	if i := strings.Index(lo, "(status: "); i >= 0 {
		rest := first[i+9:]
		if j := strings.Index(rest, ")"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return ""
}

// renderGoalBody renders the Content text that formatGoalSummary produced,
// skipping the first line (already in header) when full is false.
func renderGoalBody(sty *styles.Styles, content string, width int, expanded bool) string {
	// Drop the first line (header summary) — it's already in the header.
	lines := strings.SplitN(content, "\n", 2)
	body := content
	if len(lines) == 2 {
		body = strings.TrimSpace(lines[1])
	}
	if body == "" {
		return ""
	}
	return toolOutputPlainContent(sty, body, width, expanded)
}

// ─── create_goal ─────────────────────────────────────────────────────────────

type CreateGoalToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*CreateGoalToolMessageItem)(nil)

func NewCreateGoalToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &CreateGoalRenderContext{}, canceled)
}

type CreateGoalRenderContext struct{}

func (c *CreateGoalRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Create Goal"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Objective   string `json:"objective"`
		TokenBudget *int64 `json:"token_budget,omitempty"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	objective := strings.TrimSpace(params.Objective)

	var headerParams []string
	if objective != "" {
		truncated := ansi.Truncate(objective, 50, "…")
		headerParams = append(headerParams, truncated)
	}
	if opts.HasResult() && opts.Result.Content != "" {
		if status := parseGoalStatus(opts.Result.Content); status != "" {
			headerParams = append(headerParams, status)
		}
	}
	if params.TokenBudget != nil {
		headerParams = append(headerParams, fmt.Sprintf("%d token budget", *params.TokenBudget))
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

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderGoalBody(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// ─── get_goal ────────────────────────────────────────────────────────────────

type GetGoalToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*GetGoalToolMessageItem)(nil)

func NewGetGoalToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &GetGoalRenderContext{}, canceled)
}

type GetGoalRenderContext struct{}

func (g *GetGoalRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Goal"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var headerParams []string
	noGoal := false

	if opts.HasResult() && opts.Result.Content != "" {
		content := opts.Result.Content
		if strings.HasPrefix(content, "No active goal") {
			headerParams = append(headerParams, "not set")
			noGoal = true
		} else {
			if status := parseGoalStatus(content); status != "" {
				headerParams = append(headerParams, status)
			}
			// Extract "Objective: ..." line for a brief header hint.
			for _, line := range strings.Split(content, "\n") {
				if after, ok := strings.CutPrefix(line, "Objective: "); ok {
					headerParams = append(headerParams, ansi.Truncate(strings.TrimSpace(after), 50, "…"))
					break
				}
			}
		}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact || noGoal {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	if !opts.HasResult() || opts.Result.Content == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderGoalBody(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// ─── update_goal ─────────────────────────────────────────────────────────────

type UpdateGoalToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*UpdateGoalToolMessageItem)(nil)

func NewUpdateGoalToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &UpdateGoalRenderContext{}, canceled)
}

type UpdateGoalRenderContext struct{}

func (u *UpdateGoalRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Update Goal"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Status    string `json:"status"`
		Objective string `json:"objective"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Status != "" {
		headerParams = append(headerParams, params.Status)
	} else if strings.TrimSpace(params.Objective) != "" {
		headerParams = append(headerParams, "objective")
	}

	// Enrich with actual status from result if available.
	if opts.HasResult() && opts.Result.Content != "" {
		if resultStatus := parseGoalStatus(opts.Result.Content); resultStatus != "" && resultStatus != params.Status {
			headerParams = append(headerParams, resultStatus)
		}
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

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderGoalBody(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
