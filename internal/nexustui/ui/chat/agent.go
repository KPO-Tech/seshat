package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
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

// SubAgentLiveReporter is an interface for tool items that can report live streaming reasoning and content of their sub-agent.
type SubAgentLiveReporter interface {
	SetSubAgentStreaming(reasoning, content string)
	SubAgentStreaming() (reasoning, content string)
}

// AgentToolMessageItem is a message item that represents an agent tool call.
type AgentToolMessageItem struct {
	*baseToolMessageItem

	nestedTools       []ToolMessageItem
	subAgentReasoning string
	subAgentContent   string
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

func (a *AgentToolMessageItem) SetSubAgentStreaming(reasoning, content string) {
	a.subAgentReasoning = reasoning
	a.subAgentContent = content
	a.clearCache()
	a.Bump()
}

func (a *AgentToolMessageItem) SubAgentStreaming() (reasoning, content string) {
	return a.subAgentReasoning, a.subAgentContent
}

func getWrappedTailLines(text string, width, maxLines int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// Wrap first
	wrapped := lipgloss.NewStyle().Width(width).Render(text)
	// Split and tail
	lines := strings.Split(wrapped, "\n")
	if len(lines) <= maxLines {
		return wrapped
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

// renderNestedAgentBlock is the shared renderer for agent and agentic_fetch.
//
// Layout while running:
//
//	● Agent / Agentic Fetch (prompt summary)  [header]
//	    ├─ ✓ Tool A  ...         compact history (max 5 visible, rest collapsed)
//	    ├─ ✓ Tool B  ...
//	    ├─ ● Tool C  ...         currently running (compact)
//
//	    ✓ Tool B  ...           last completed tool — full non-compact render
//	      output line 1
//	      output line 2
//
//	    ⠋ ...                   spinner
//
// When done: compact tree + result body (no live section).
func renderNestedAgentBlock(
	sty *styles.Styles,
	header string,
	nestedTools []ToolMessageItem,
	subAgentReasoning string,
	subAgentContent string,
	opts *ToolRenderOpts,
	cappedWidth int,
) string {
	remainingWidth := max(20, cappedWidth-7)
	isRunning := !opts.HasResult() && !opts.IsCanceled()

	// Find the last completed nested tool for the live preview.
	// Only shown while the agent is still running (once done we collapse everything).
	liveIdx := -1
	if isRunning {
		for i := len(nestedTools) - 1; i >= 0; i-- {
			s := nestedTools[i].Status()
			if s == ToolStatusSuccess || s == ToolStatusError {
				liveIdx = i
				break
			}
		}
	}

	// Compact history tree — limit to last 5 nested tools.
	childTree := tree.Root(header)
	const maxVisibleTools = 5
	if len(nestedTools) > maxVisibleTools {
		collapsedCount := len(nestedTools) - maxVisibleTools
		collapsedText := sty.Tool.ContentTruncation.Render(fmt.Sprintf("… (%d tools collapsed)", collapsedCount))
		childTree.Child(collapsedText)

		for _, tool := range nestedTools[collapsedCount:] {
			childTree.Child(tool.Render(remainingWidth))
		}
	} else {
		for _, tool := range nestedTools {
			childTree.Child(tool.Render(remainingWidth))
		}
	}

	var parts []string
	parts = append(parts, childTree.Enumerator(roundedEnumerator(4, 2)).String())

	// Live preview: last completed tool rendered in full (non-compact), indented by 4 spaces.
	if liveIdx >= 0 {
		preview := indentString(nestedTools[liveIdx].RenderPreview(max(20, cappedWidth-4)), 4)
		parts = append(parts, "", preview)
	}

	if isRunning {
		hasStreamingTail := false
		var tailParts []string

		trimmedReasoning := strings.TrimSpace(subAgentReasoning)
		trimmedContent := strings.TrimSpace(subAgentContent)

		// Calculate available width for the tail
		tailWidth := cappedWidth - toolBodyLeftPaddingTotal - 2 - 4 // subtract left border & padding & indentation

		// Prioritize streaming content over reasoning
		if trimmedContent != "" {
			tail := getWrappedTailLines(trimmedContent, tailWidth, 3)
			if tail != "" {
				prefix := sty.Tool.StateWaiting.Render("✍ Generating:")
				leftBorderColor := sty.Messages.ThinkingBox.GetBorderLeftForeground()
				borderStyle := lipgloss.NewStyle().
					Border(lipgloss.NormalBorder(), false, false, false, true).
					BorderForeground(leftBorderColor).
					PaddingLeft(1)

				indentedTail := borderStyle.Render(tail)
				tailParts = append(tailParts, prefix, indentedTail)
				hasStreamingTail = true
			}
		} else if trimmedReasoning != "" {
			tail := getWrappedTailLines(trimmedReasoning, tailWidth, 3)
			if tail != "" {
				prefix := sty.Tool.StateWaiting.Render("💭 Thinking:")
				leftBorderColor := sty.Messages.ThinkingBox.GetBorderLeftForeground()
				borderStyle := lipgloss.NewStyle().
					Border(lipgloss.NormalBorder(), false, false, false, true).
					BorderForeground(leftBorderColor).
					PaddingLeft(1)

				reasoningStyle := sty.Tool.AgentPrompt
				reasoningTail := reasoningStyle.Italic(true).Render(tail)
				indentedTail := borderStyle.Render(reasoningTail)
				tailParts = append(tailParts, prefix, indentedTail)
				hasStreamingTail = true
			}
		}

		if hasStreamingTail {
			parts = append(parts, "")
			tailBlock := indentString(lipgloss.JoinVertical(lipgloss.Left, tailParts...), 4)
			parts = append(parts, tailBlock)
		}

		parts = append(parts, "", indentString(opts.Anim.Render(), 4))
	}

	result := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if opts.HasResult() && opts.Result.Content != "" {
		body := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth-toolBodyLeftPaddingTotal, opts.ExpandedContent)
		return joinToolParts(result, body)
	}
	return result
}

func getPromptHeaderSummary(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	lines := strings.Split(prompt, "\n")
	var selected []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			selected = append(selected, trimmed)
			if len(selected) == 2 {
				break
			}
		}
	}
	if len(selected) == 0 {
		return ""
	}
	summary := strings.Join(selected, " ")

	// Count total non-empty lines to decide if we append "..."
	nonEmptyCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyCount++
		}
	}
	if nonEmptyCount > len(selected) {
		summary += "..."
	}
	return summary
}

func indentString(text string, spaces int) string {
	if text == "" {
		return ""
	}
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		} else {
			lines[i] = indent
		}
	}
	return strings.Join(lines, "\n")
}

type agentParams struct {
	Type      string `json:"type"`
	AgentType string `json:"agent_type"`
	Task      string `json:"task"`
	Prompt    string `json:"prompt"`
}

func extractPromptFromInput(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var params agentParams
	if err := json.Unmarshal([]byte(input), &params); err == nil {
		if params.Task != "" {
			return params.Task
		}
		if params.Prompt != "" {
			return params.Prompt
		}
	}
	var str string
	if err := json.Unmarshal([]byte(input), &str); err == nil && str != "" {
		return str
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(input), &m); err == nil {
		for k, v := range m {
			kLower := strings.ToLower(k)
			if strings.Contains(kLower, "prompt") || strings.Contains(kLower, "task") {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	if !strings.HasPrefix(input, "{") && !strings.HasPrefix(input, "[") {
		return input
	}
	return ""
}

func getAgentDisplayName(input string) string {
	var params agentParams
	if err := json.Unmarshal([]byte(input), &params); err == nil {
		agentType := params.Type
		if agentType == "" {
			agentType = params.AgentType
		}
		if agentType != "" {
			return humanizedToolName(agentType) + " Agent"
		}
	}
	return "Agent"
}

// AgentToolRenderContext renders agent tool messages.
type AgentToolRenderContext struct {
	agent *AgentToolMessageItem
}

// RenderTool implements the [ToolRenderer] interface.
func (r *AgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	displayName := getAgentDisplayName(opts.ToolCall.Input)

	if !opts.ToolCall.Finished && !opts.IsCanceled() && len(r.agent.nestedTools) == 0 && r.agent.subAgentReasoning == "" && r.agent.subAgentContent == "" {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	prompt := extractPromptFromInput(opts.ToolCall.Input)
	promptSummary := getPromptHeaderSummary(prompt)
	var toolParams []string
	if promptSummary != "" {
		toolParams = append(toolParams, promptSummary)
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	return renderNestedAgentBlock(
		sty, header,
		r.agent.nestedTools, r.agent.subAgentReasoning, r.agent.subAgentContent, opts, cappedWidth,
	)
}

// -----------------------------------------------------------------------------
// Agentic Fetch Tool
// -----------------------------------------------------------------------------

// AgenticFetchToolMessageItem is a message item that represents an agentic fetch tool call.
type AgenticFetchToolMessageItem struct {
	*baseToolMessageItem

	nestedTools       []ToolMessageItem
	subAgentReasoning string
	subAgentContent   string
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

func (a *AgenticFetchToolMessageItem) SetSubAgentStreaming(reasoning, content string) {
	a.subAgentReasoning = reasoning
	a.subAgentContent = content
	a.clearCache()
	a.Bump()
}

func (a *AgenticFetchToolMessageItem) SubAgentStreaming() (reasoning, content string) {
	return a.subAgentReasoning, a.subAgentContent
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
	cappedWidth := width
	if !opts.ToolCall.Finished && !opts.IsCanceled() && len(r.fetch.nestedTools) == 0 && r.fetch.subAgentReasoning == "" && r.fetch.subAgentContent == "" {
		return pendingTool(sty, "Agentic Fetch", opts.Anim, opts.Compact)
	}

	var params agenticFetchParams
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	prompt := params.Prompt
	if prompt == "" {
		prompt = extractPromptFromInput(opts.ToolCall.Input)
	}

	var toolParams []string
	if params.URL != "" {
		toolParams = append(toolParams, params.URL)
	}
	if summary := getPromptHeaderSummary(prompt); summary != "" {
		if len(toolParams) == 0 {
			toolParams = append(toolParams, summary)
		} else {
			toolParams = append(toolParams, "prompt", summary)
		}
	}
	header := toolHeader(sty, opts.Status, "Agentic Fetch", cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	return renderNestedAgentBlock(
		sty, header,
		r.fetch.nestedTools, r.fetch.subAgentReasoning, r.fetch.subAgentContent, opts, cappedWidth,
	)
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
	cappedWidth := width
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
// Spawn Agent Tool
// -----------------------------------------------------------------------------

// SpawnAgentToolMessageItem represents a spawn_agent tool call.
type SpawnAgentToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SpawnAgentToolMessageItem)(nil)

// NewSpawnAgentToolMessageItem creates a new [SpawnAgentToolMessageItem].
func NewSpawnAgentToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &SpawnAgentToolRenderContext{}, canceled)
}

// SpawnAgentToolRenderContext renders spawn_agent tool messages.
type SpawnAgentToolRenderContext struct{}

type spawnAgentInput struct {
	Prompt    string `json:"prompt"`
	AgentType string `json:"agent_type,omitempty"`
	Role      string `json:"role,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	MaxTurns  int    `json:"max_turns,omitempty"`
}

type spawnAgentOutput struct {
	AgentID  string `json:"agent_id"`
	Status   string `json:"status"`
	Nickname string `json:"nickname,omitempty"`
	Role     string `json:"role,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (s *SpawnAgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Spawn Agent", opts.Anim, opts.Compact)
	}

	var params spawnAgentInput
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	agentType := params.AgentType
	if agentType == "" {
		agentType = "general-purpose"
	}

	var headerParams []string
	if params.Nickname != "" {
		headerParams = append(headerParams, params.Nickname+" · "+agentType)
	} else {
		headerParams = append(headerParams, agentType)
	}

	header := toolHeader(sty, opts.Status, "Spawn Agent", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	prompt := strings.ReplaceAll(params.Prompt, "\n", " ")
	taskTag := sty.Tool.AgentTaskTag.Render("Task")
	taskTagWidth := lipgloss.Width(taskTag)
	promptWidth := min(bodyWidth-taskTagWidth-1, maxTextWidth-taskTagWidth-1)
	if promptWidth < 1 {
		promptWidth = 1
	}
	promptText := sty.Tool.AgentPrompt.Width(promptWidth).Render(prompt)
	taskLine := lipgloss.JoinHorizontal(lipgloss.Left, taskTag, " ", promptText)

	var bodyParts []string
	bodyParts = append(bodyParts, taskLine)

	// When done, show agent_id + status.
	if opts.HasResult() {
		var out spawnAgentOutput
		_ = json.Unmarshal([]byte(opts.Result.Content), &out)
		if out.AgentID == "" {
			// fallback: parse from plain content "Agent spawned: ID (status: S)"
			content := opts.Result.Content
			if idx := strings.Index(content, "Agent spawned: "); idx >= 0 {
				rest := content[idx+len("Agent spawned: "):]
				if spaceIdx := strings.Index(rest, " "); spaceIdx > 0 {
					out.AgentID = rest[:spaceIdx]
				}
			}
		}
		if out.AgentID != "" {
			idLine := sty.Tool.AgentPrompt.Render("→ " + out.AgentID)
			if out.Status != "" {
				idLine += "  " + sty.Tool.StateWaiting.Render("("+out.Status+")")
			}
			bodyParts = append(bodyParts, "", idLine)
		}
	}

	body := sty.Tool.Body.Render(strings.Join(bodyParts, "\n"))
	return joinToolParts(header, body)
}

// -----------------------------------------------------------------------------
// Send Agent Message Tool
// -----------------------------------------------------------------------------

// SendAgentMessageToolMessageItem represents a send_agent_message tool call.
type SendAgentMessageToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*SendAgentMessageToolMessageItem)(nil)

// NewSendAgentMessageToolMessageItem creates a new [SendAgentMessageToolMessageItem].
func NewSendAgentMessageToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &SendAgentMessageToolRenderContext{}, canceled)
}

// SendAgentMessageToolRenderContext renders send_agent_message tool messages.
type SendAgentMessageToolRenderContext struct{}

type sendAgentMessageInput struct {
	AgentID string `json:"agent_id"`
	Message string `json:"message"`
}

// RenderTool implements the [ToolRenderer] interface.
func (s *SendAgentMessageToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Send Message", opts.Anim, opts.Compact)
	}

	var params sendAgentMessageInput
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.AgentID != "" {
		headerParams = append(headerParams, "→ "+params.AgentID)
	}

	header := toolHeader(sty, opts.Status, "Send Message", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if params.Message == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	msg := strings.ReplaceAll(params.Message, "\n", " ")
	msgLine := sty.Tool.AgentPrompt.Render("↳ " + msg)
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, msgLine, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// -----------------------------------------------------------------------------
// Wait Agent Tool
// -----------------------------------------------------------------------------

// WaitAgentToolMessageItem represents a wait_agent tool call.
type WaitAgentToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*WaitAgentToolMessageItem)(nil)

// NewWaitAgentToolMessageItem creates a new [WaitAgentToolMessageItem].
func NewWaitAgentToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	t := &WaitAgentToolMessageItem{}
	t.baseToolMessageItem = newBaseToolMessageItem(sty, toolCall, result, &WaitAgentToolRenderContext{}, canceled)
	// Keep spinning until a result arrives (blocking call that can take minutes).
	t.spinningFunc = func(state SpinningState) bool {
		return !state.HasResult() && !state.IsCanceled()
	}
	return t
}

// WaitAgentToolRenderContext renders wait_agent tool messages.
type WaitAgentToolRenderContext struct{}

type waitAgentInput struct {
	AgentID        string `json:"agent_id"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (w *WaitAgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Wait Agent", opts.Anim, opts.Compact)
	}

	var params waitAgentInput
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.AgentID != "" {
		headerParams = append(headerParams, params.AgentID)
	}

	header := toolHeader(sty, opts.Status, "Wait Agent", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if opts.HasEmptyResult() {
		return header
	}

	body := toolOutputMarkdownContent(sty, opts.Result.Content, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// -----------------------------------------------------------------------------
// Close Agent Tool
// -----------------------------------------------------------------------------

// CloseAgentToolMessageItem represents a close_agent tool call.
type CloseAgentToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*CloseAgentToolMessageItem)(nil)

// NewCloseAgentToolMessageItem creates a new [CloseAgentToolMessageItem].
func NewCloseAgentToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &CloseAgentToolRenderContext{}, canceled)
}

// CloseAgentToolRenderContext renders close_agent tool messages.
type CloseAgentToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (c *CloseAgentToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Close Agent", opts.Anim, opts.Compact)
	}

	var params struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &params)

	var headerParams []string
	if params.AgentID != "" {
		headerParams = append(headerParams, params.AgentID)
	}

	return toolHeader(sty, opts.Status, "Close Agent", cappedWidth, opts.Compact, headerParams...)
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
	cappedWidth := width
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
