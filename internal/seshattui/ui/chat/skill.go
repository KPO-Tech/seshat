package chat

import (
	"encoding/json"

	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// ─── skill ─────────────────────────────────────────────────────────────────

// SkillToolMessageItem represents a skill tool call.
type SkillToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SkillToolMessageItem)(nil)

// NewSkillToolMessageItem creates a new [SkillToolMessageItem].
func NewSkillToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &SkillToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &SkillToolRenderContext{}, canceled)}
}

// SkillToolRenderContext renders skill tool messages.
type SkillToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (s *SkillToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width

	var params struct {
		Skill string `json:"skill"`
		Args  string `json:"args"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	name := "Skill"
	if params.Skill != "" {
		name = "Skill: " + params.Skill
	}

	if opts.IsPending() {
		return pendingTool(sty, name, opts.Anim, opts.Compact)
	}

	var headerParams []string
	if params.Args != "" {
		headerParams = append(headerParams, ansi.Truncate(params.Args, cappedWidth/2, "…"))
	}

	header := toolHeader(sty, opts.Status, name, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// Header-only on success: a successful skill call injects its prompt
	// into the agent's own context, which then surfaces through the tool
	// calls that follow — not as a sub-result to redisplay here.
	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}

	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, content)
}

// ─── seshat_list_skills ────────────────────────────────────────────────────

// SeshatListSkillsToolMessageItem represents a seshat_list_skills tool call.
type SeshatListSkillsToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SeshatListSkillsToolMessageItem)(nil)

// NewSeshatListSkillsToolMessageItem creates a new [SeshatListSkillsToolMessageItem].
func NewSeshatListSkillsToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &SeshatListSkillsToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &SeshatListSkillsToolRenderContext{}, canceled)}
}

// SeshatListSkillsToolRenderContext renders seshat_list_skills tool messages.
type SeshatListSkillsToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *SeshatListSkillsToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "List Skills", opts.Anim, opts.Compact)
	}

	var params struct {
		Collection string `json:"collection"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Collection != "" && params.Collection != "all" {
		headerParams = append(headerParams, params.Collection)
	}

	header := toolHeader(sty, opts.Status, "List Skills", cappedWidth, opts.Compact, headerParams...)
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
	body := toolOutputMarkdownContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// ─── seshat_read_skill ─────────────────────────────────────────────────────

// SeshatReadSkillToolMessageItem represents a seshat_read_skill tool call.
type SeshatReadSkillToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SeshatReadSkillToolMessageItem)(nil)

// NewSeshatReadSkillToolMessageItem creates a new [SeshatReadSkillToolMessageItem].
func NewSeshatReadSkillToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &SeshatReadSkillToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &SeshatReadSkillToolRenderContext{}, canceled)}
}

// SeshatReadSkillToolRenderContext renders seshat_read_skill tool messages.
type SeshatReadSkillToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *SeshatReadSkillToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Read Skill", opts.Anim, opts.Compact)
	}

	var params struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Name != "" {
		headerParams = append(headerParams, ansi.Truncate(params.Name, cappedWidth/2, "…"))
	}

	header := toolHeader(sty, opts.Status, "Read Skill", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// Header-only on success: the skill name is already in the header, and
	// the full skill.md body would just duplicate context the agent already has.
	if !opts.HasResult() || !opts.Result.IsError {
		return header
	}

	content := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, content)
}

// ─── seshat_validate_skill ─────────────────────────────────────────────────

// SeshatValidateSkillToolMessageItem represents a seshat_validate_skill tool call.
type SeshatValidateSkillToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SeshatValidateSkillToolMessageItem)(nil)

// NewSeshatValidateSkillToolMessageItem creates a new [SeshatValidateSkillToolMessageItem].
func NewSeshatValidateSkillToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &SeshatValidateSkillToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &SeshatValidateSkillToolRenderContext{}, canceled)}
}

// SeshatValidateSkillToolRenderContext renders seshat_validate_skill tool messages.
type SeshatValidateSkillToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *SeshatValidateSkillToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Validate Skill", opts.Anim, opts.Compact)
	}

	var params struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.Path != "" {
		headerParams = append(headerParams, ansi.Truncate(params.Path, cappedWidth/2, "…"))
	}

	header := toolHeader(sty, opts.Status, "Validate Skill", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() {
		return header
	}

	// Always show the body: a successful call doesn't mean "no warnings".
	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
