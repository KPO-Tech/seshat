package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/anim"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

// -----------------------------------------------------------------------------
// Agent Tool
// -----------------------------------------------------------------------------

// NestedToolContainer is an interface for tool items that can contain nested tool calls.
type NestedToolContainer interface {
	NestedTools() []ToolMessageItem
	SetNestedTools(tools []ToolMessageItem)
	AddNestedTool(tool ToolMessageItem)
}

// AgentToolMessageItem is a message item that represents an agent tool call.
type AgentToolMessageItem struct {
	*baseToolMessageItem

	nestedTools []ToolMessageItem
}

var (
	_ ToolMessageItem     = (*AgentToolMessageItem)(nil)
	_ NestedToolContainer = (*AgentToolMessageItem)(nil)
)

// NewAgentToolMessageItem creates a new [AgentToolMessageItem].
func NewAgentToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) *AgentToolMessageItem {
	t := &AgentToolMessageItem{}
	t.baseToolMessageItem = newBaseToolMessageItem(sty, toolCall, result, &AgentToolRenderContext{agent: t}, canceled)
	// For the agent tool we keep spinning until the tool call is finished.
	t.spinningFunc = func(state SpinningState) bool {
		return !state.HasResult() && !state.IsCanceled()
	}
	return t
}

// Animate progresses the message animation if it should be spinning.
//
// Bumps the parent's F6 list-cache version on both the parent-tick and
// nested-tick branches. Nested tools are not list entries of their
// own — their IDs map to this parent's index in idInxMap
// (internal/ui/model/chat.go:240-246) and their renders are embedded
// inline in this parent's output — so the list only checks the
// parent's version. Without the bump, the list cache would serve the
// previously rendered frame indefinitely and the spinner would appear
// frozen.
func (a *AgentToolMessageItem) Animate(msg anim.StepMsg) tea.Cmd {
	if a.result != nil || a.Status() == ToolStatusCanceled {
		return nil
	}
	if msg.ID == a.ID() {
		a.Bump()
		return a.anim.Animate(msg)
	}
	for _, nestedTool := range a.nestedTools {
		if msg.ID != nestedTool.ID() {
			continue
		}
		if s, ok := nestedTool.(Animatable); ok {
			a.Bump()
			return s.Animate(msg)
		}
	}
	return nil
}

// NestedTools returns the nested tools.
func (a *AgentToolMessageItem) NestedTools() []ToolMessageItem {
	return a.nestedTools
}

// SetNestedTools sets the nested tools.
//
// SetNestedTools always bumps the version. The previous design
// deduped when the slice's length and element pointers were
// unchanged, but the live update path in internal/ui/model/ui.go
// mutates existing children in place (SetToolCall / SetResult on the
// same pointers) and then calls SetNestedTools with the same slice.
// Pointer-equality dedupe in that case skips the parent Bump even
// though the parent's rendered output (which embeds the children
// inline) has changed, leaving a stale parent entry in the list
// cache. Always bumping is cheap (one uint64 increment) and called
// at most once per agent event; in the rare case the slice is
// truly unchanged the worst case is one extra parent re-render
// while every child cache hit stays warm.
func (a *AgentToolMessageItem) SetNestedTools(tools []ToolMessageItem) {
	a.nestedTools = tools
	a.clearCache()
	a.Bump()
}

// AddNestedTool adds a nested tool.
func (a *AgentToolMessageItem) AddNestedTool(tool ToolMessageItem) {
	// Mark nested tools as simple (compact) rendering.
	if s, ok := tool.(Compactable); ok {
		s.SetCompact(true)
	}
	a.nestedTools = append(a.nestedTools, tool)
	a.clearCache()
	a.Bump()
}

// AgentToolRenderContext renders agent tool messages.
type AgentToolRenderContext struct {
	agent *AgentToolMessageItem
}

// RenderTool implements the [ToolRenderer] interface.
func (r *AgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if !opts.ToolCall.Finished && !opts.IsCanceled() && len(r.agent.nestedTools) == 0 {
		return pendingTool(sty, "Agent", opts.Anim, opts.Compact)
	}

	var params tools.AgentParams
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	prompt := params.Prompt
	prompt = strings.ReplaceAll(prompt, "\n", " ")

	header := toolHeader(sty, opts.Status, "Agent", cappedWidth, opts.Compact)
	if opts.Compact {
		return header
	}

	// Build the task tag and prompt.
	taskTag := sty.Tool.AgentTaskTag.Render("Task")
	taskTagWidth := lipgloss.Width(taskTag)

	// Calculate remaining width for prompt.
	remainingWidth := min(cappedWidth-taskTagWidth-3, maxTextWidth-taskTagWidth-3) // -3 for spacing

	promptText := sty.Tool.AgentPrompt.Width(remainingWidth).Render(prompt)

	header = lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			taskTag,
			" ",
			promptText,
		),
	)

	// Build tree with nested tool calls.
	childTools := tree.Root(header)

	for _, nestedTool := range r.agent.nestedTools {
		childView := nestedTool.Render(remainingWidth)
		childTools.Child(childView)
	}

	// Build parts.
	var parts []string
	parts = append(parts, childTools.Enumerator(roundedEnumerator(2, taskTagWidth-5)).String())

	// Show animation if still running.
	if !opts.HasResult() && !opts.IsCanceled() {
		parts = append(parts, "", opts.Anim.Render())
	}

	result := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Add body content when completed.
	if opts.HasResult() && opts.Result.Content != "" {
		body := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth-toolBodyLeftPaddingTotal, opts.ExpandedContent)
		return joinToolParts(result, body)
	}

	return result
}

// -----------------------------------------------------------------------------
// Agentic Fetch Tool
// -----------------------------------------------------------------------------

// AgenticFetchToolMessageItem is a message item that represents an agentic fetch tool call.
type AgenticFetchToolMessageItem struct {
	*baseToolMessageItem

	nestedTools []ToolMessageItem
}

var (
	_ ToolMessageItem     = (*AgenticFetchToolMessageItem)(nil)
	_ NestedToolContainer = (*AgenticFetchToolMessageItem)(nil)
)

// NewAgenticFetchToolMessageItem creates a new [AgenticFetchToolMessageItem].
func NewAgenticFetchToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) *AgenticFetchToolMessageItem {
	t := &AgenticFetchToolMessageItem{}
	t.baseToolMessageItem = newBaseToolMessageItem(sty, toolCall, result, &AgenticFetchToolRenderContext{fetch: t}, canceled)
	// For the agentic fetch tool we keep spinning until the tool call is finished.
	t.spinningFunc = func(state SpinningState) bool {
		return !state.HasResult() && !state.IsCanceled()
	}
	return t
}

// Animate progresses the message animation if it should be spinning.
// See [AgentToolMessageItem.Animate] for the parent-bump rationale —
// without an override, the embedded base.Animate would (a) drop
// StepMsgs whose ID matches a nested child instead of the parent
// (anim.Animate's ID check at internal/ui/anim/anim.go:326-329
// silently returns nil), and (b) never invalidate the parent's
// list-cache entry on a parent tick.
func (a *AgenticFetchToolMessageItem) Animate(msg anim.StepMsg) tea.Cmd {
	if a.result != nil || a.Status() == ToolStatusCanceled {
		return nil
	}
	if msg.ID == a.ID() {
		a.Bump()
		return a.anim.Animate(msg)
	}
	for _, nestedTool := range a.nestedTools {
		if msg.ID != nestedTool.ID() {
			continue
		}
		if s, ok := nestedTool.(Animatable); ok {
			a.Bump()
			return s.Animate(msg)
		}
	}
	return nil
}

// NestedTools returns the nested tools.
func (a *AgenticFetchToolMessageItem) NestedTools() []ToolMessageItem {
	return a.nestedTools
}

// SetNestedTools sets the nested tools. Always bumps the version;
// see [AgentToolMessageItem.SetNestedTools] for the rationale.
func (a *AgenticFetchToolMessageItem) SetNestedTools(tools []ToolMessageItem) {
	a.nestedTools = tools
	a.clearCache()
	a.Bump()
}

// AddNestedTool adds a nested tool.
func (a *AgenticFetchToolMessageItem) AddNestedTool(tool ToolMessageItem) {
	// Mark nested tools as simple (compact) rendering.
	if s, ok := tool.(Compactable); ok {
		s.SetCompact(true)
	}
	a.nestedTools = append(a.nestedTools, tool)
	a.clearCache()
	a.Bump()
}

// AgenticFetchToolRenderContext renders agentic fetch tool messages.
type AgenticFetchToolRenderContext struct {
	fetch *AgenticFetchToolMessageItem
}

// agenticFetchParams matches tools.AgenticFetchParams.
type agenticFetchParams struct {
	URL    string `json:"url,omitempty"`
	Prompt string `json:"prompt"`
}

// RenderTool implements the [ToolRenderer] interface.
func (r *AgenticFetchToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if !opts.ToolCall.Finished && !opts.IsCanceled() && len(r.fetch.nestedTools) == 0 {
		return pendingTool(sty, "Agentic Fetch", opts.Anim, opts.Compact)
	}

	var params agenticFetchParams
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	prompt := params.Prompt
	prompt = strings.ReplaceAll(prompt, "\n", " ")

	// Build header with optional URL param.
	var toolParams []string
	if params.URL != "" {
		toolParams = append(toolParams, params.URL)
	}

	header := toolHeader(sty, opts.Status, "Agentic Fetch", cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	// Build the prompt tag.
	promptTag := sty.Tool.AgenticFetchPromptTag.Render("Prompt")
	promptTagWidth := lipgloss.Width(promptTag)

	// Calculate remaining width for prompt text.
	remainingWidth := min(cappedWidth-promptTagWidth-3, maxTextWidth-promptTagWidth-3) // -3 for spacing

	promptText := sty.Tool.AgentPrompt.Width(remainingWidth).Render(prompt)

	header = lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			promptTag,
			" ",
			promptText,
		),
	)

	// Build tree with nested tool calls.
	childTools := tree.Root(header)

	for _, nestedTool := range r.fetch.nestedTools {
		childView := nestedTool.Render(remainingWidth)
		childTools.Child(childView)
	}

	// Build parts.
	var parts []string
	parts = append(parts, childTools.Enumerator(roundedEnumerator(2, promptTagWidth-5)).String())

	// Show animation if still running.
	if !opts.HasResult() && !opts.IsCanceled() {
		parts = append(parts, "", opts.Anim.Render())
	}

	result := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Add body content when completed.
	if opts.HasResult() && opts.Result.Content != "" {
		body := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth-toolBodyLeftPaddingTotal, opts.ExpandedContent)
		return joinToolParts(result, body)
	}

	return result
}

// -----------------------------------------------------------------------------
// List Agents Tool
// -----------------------------------------------------------------------------

// ListAgentsToolMessageItem represents a list_agents tool call.
type ListAgentsToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ListAgentsToolMessageItem)(nil)

// NewListAgentsToolMessageItem creates a new [ListAgentsToolMessageItem].
func NewListAgentsToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ListAgentsToolRenderContext{}, canceled)
}

// ListAgentsToolRenderContext renders list_agents tool messages.
type ListAgentsToolRenderContext struct{}

type listAgentsMeta struct {
	Count int `json:"count"`
}

// RenderTool implements the [ToolRenderer] interface.
func (l *ListAgentsToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "List Agents", opts.Anim, opts.Compact)
	}

	var params struct {
		FilterStatus string `json:"filter_status,omitempty"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	headerParams := []string{}
	if params.FilterStatus != "" {
		headerParams = append(headerParams, params.FilterStatus)
	}

	var meta listAgentsMeta
	if opts.HasResult() && opts.Result.Metadata != "" {
		_ = json.Unmarshal([]byte(opts.Result.Metadata), &meta)
	}
	if opts.HasResult() {
		noun := "agents"
		if meta.Count == 1 {
			noun = "agent"
		}
		headerParams = append(headerParams, fmt.Sprintf("%d %s", meta.Count, noun))
	}

	header := toolHeader(sty, opts.Status, "List Agents", cappedWidth, opts.Compact, headerParams...)
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
// Resume Agent Tool
// -----------------------------------------------------------------------------

// ResumeAgentToolMessageItem represents a resume_agent tool call.
type ResumeAgentToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*ResumeAgentToolMessageItem)(nil)

// NewResumeAgentToolMessageItem creates a new [ResumeAgentToolMessageItem].
func NewResumeAgentToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &ResumeAgentToolRenderContext{}, canceled)
}

// ResumeAgentToolRenderContext renders resume_agent tool messages.
type ResumeAgentToolRenderContext struct{}

type resumeAgentParams struct {
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Task      string `json:"task"`
	Async     bool   `json:"async,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (r *ResumeAgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "Resume Agent", opts.Anim, opts.Compact)
	}

	var params resumeAgentParams
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	ref := params.AgentID
	if ref == "" {
		ref = params.SessionID
	}

	var headerParams []string
	if ref != "" {
		headerParams = append(headerParams, ref)
	}
	if params.Task != "" {
		task := strings.ReplaceAll(params.Task, "\n", " ")
		headerParams = append(headerParams, task)
	}

	header := toolHeader(sty, opts.Status, "Resume Agent", cappedWidth, opts.Compact, headerParams...)
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
	body := sty.Tool.Body.Render(toolOutputMarkdownContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
