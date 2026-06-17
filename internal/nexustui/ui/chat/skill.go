package chat

import (
	"encoding/json"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
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

// ─── nexus_list_skills ────────────────────────────────────────────────────

// NexusListSkillsToolMessageItem represents a nexus_list_skills tool call.
type NexusListSkillsToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*NexusListSkillsToolMessageItem)(nil)

// NewNexusListSkillsToolMessageItem creates a new [NexusListSkillsToolMessageItem].
func NewNexusListSkillsToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &NexusListSkillsToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &NexusListSkillsToolRenderContext{}, canceled)}
}

// NexusListSkillsToolRenderContext renders nexus_list_skills tool messages.
type NexusListSkillsToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *NexusListSkillsToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
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

// ─── nexus_read_skill ─────────────────────────────────────────────────────

// NexusReadSkillToolMessageItem represents a nexus_read_skill tool call.
type NexusReadSkillToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*NexusReadSkillToolMessageItem)(nil)

// NewNexusReadSkillToolMessageItem creates a new [NexusReadSkillToolMessageItem].
func NewNexusReadSkillToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &NexusReadSkillToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &NexusReadSkillToolRenderContext{}, canceled)}
}

// NexusReadSkillToolRenderContext renders nexus_read_skill tool messages.
type NexusReadSkillToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *NexusReadSkillToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
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

// ─── nexus_validate_skill ─────────────────────────────────────────────────

// NexusValidateSkillToolMessageItem represents a nexus_validate_skill tool call.
type NexusValidateSkillToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*NexusValidateSkillToolMessageItem)(nil)

// NewNexusValidateSkillToolMessageItem creates a new [NexusValidateSkillToolMessageItem].
func NewNexusValidateSkillToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return &NexusValidateSkillToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &NexusValidateSkillToolRenderContext{}, canceled)}
}

// NexusValidateSkillToolRenderContext renders nexus_validate_skill tool messages.
type NexusValidateSkillToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (n *NexusValidateSkillToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
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
