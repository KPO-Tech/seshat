package chat

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// AnswerAskUserMsg is sent when the user selects an answer in an ask_user_question bubble.
// ui.go handles this by calling Workspace.AnswerAskUser.
type AnswerAskUserMsg struct {
	ID    string
	Value string
}

// AskUserActivatable is implemented by items that can receive ask_user_question prompts.
type AskUserActivatable interface {
	MessageItem
	ActivateQuestion(req tools.AskUserRequest)
}

// askUserAnswered holds a single resolved Q→A pair shown in the history section.
type askUserAnswered struct {
	question string
	answer   string
}

// AskUserToolMessageItem renders an ask_user_question tool call with an
// interactive option picker while the agent is waiting for the user's answer.
type AskUserToolMessageItem struct {
	*baseToolMessageItem

	// Interactive state for the current pending question.
	activeReq      *tools.AskUserRequest
	cursor         int
	selections     []bool // multi-select toggle state, indexed by option
	waitingForUser bool

	// History of resolved Q→A pairs from earlier questions in this call.
	history []askUserAnswered
}

var _ AskUserActivatable = (*AskUserToolMessageItem)(nil)

// NewAskUserToolMessageItem creates a new [AskUserToolMessageItem].
func NewAskUserToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) *AskUserToolMessageItem {
	item := &AskUserToolMessageItem{}
	renderCtx := &askUserRenderContext{item: item}
	item.baseToolMessageItem = newBaseToolMessageItem(sty, toolCall, result, renderCtx, canceled)
	// Suppress the spinner when we're in interactive mode or the result is in.
	item.spinningFunc = func(s SpinningState) bool {
		if item.waitingForUser || s.Result != nil {
			return false
		}
		return !s.ToolCall.Finished
	}
	return item
}

// ActivateQuestion switches the item into interactive mode for the given question.
func (a *AskUserToolMessageItem) ActivateQuestion(req tools.AskUserRequest) {
	a.activeReq = &req
	a.cursor = 0
	a.selections = make([]bool, len(req.Options))
	a.waitingForUser = true
	a.clearCache()
	a.Bump()
}

// HandleKeyEvent overrides the base handler to provide option navigation and selection.
func (a *AskUserToolMessageItem) HandleKeyEvent(key tea.KeyMsg) (bool, tea.Cmd) {
	if !a.waitingForUser || a.activeReq == nil || a.activeReq.IsCustomText {
		return a.baseToolMessageItem.HandleKeyEvent(key)
	}
	req := a.activeReq
	switch key.String() {
	case "up", "k":
		if a.cursor > 0 {
			a.cursor--
			a.clearCache()
			a.Bump()
		}
		return true, nil
	case "down", "j":
		if a.cursor < len(req.Options)-1 {
			a.cursor++
			a.clearCache()
			a.Bump()
		}
		return true, nil
	case "space":
		if req.MultiSelect && len(a.selections) > a.cursor {
			a.selections[a.cursor] = !a.selections[a.cursor]
			a.clearCache()
			a.Bump()
		}
		return true, nil
	case "enter":
		return true, a.confirmSelection()
	}
	return false, nil
}

// confirmSelection resolves the current question and returns the answer command.
func (a *AskUserToolMessageItem) confirmSelection() tea.Cmd {
	req := a.activeReq
	a.waitingForUser = false

	var answer string
	if req.MultiSelect {
		var selected []string
		for i, sel := range a.selections {
			if sel && i < len(req.Options) {
				selected = append(selected, req.Options[i].Label)
			}
		}
		if len(selected) == 0 {
			// Nothing selected yet — stay interactive.
			a.waitingForUser = true
			return nil
		}
		sort.Strings(selected)
		answer = strings.Join(selected, ", ")
	} else {
		if a.cursor < len(req.Options) {
			answer = req.Options[a.cursor].Value
		}
	}

	// Append to local history so the item can display past Q→As.
	a.history = append(a.history, askUserAnswered{
		question: req.Question,
		answer:   answer,
	})
	a.activeReq = nil
	a.clearCache()
	a.Bump()

	id := req.ID
	return func() tea.Msg {
		return AnswerAskUserMsg{ID: id, Value: answer}
	}
}

// askUserRenderContext bridges the baseToolMessageItem ToolRenderer interface
// to the mutable state on AskUserToolMessageItem.
type askUserRenderContext struct {
	item *AskUserToolMessageItem
}

// RenderTool implements [ToolRenderer].
func (r *askUserRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	a := r.item
	cappedWidth := cappedToolWidth(width)

	// Still waiting for the agent to begin — show the pending spinner.
	if !a.waitingForUser && a.activeReq == nil && !opts.HasResult() && len(a.history) == 0 {
		return pendingTool(sty, "Ask User", opts.Anim, opts.Compact)
	}

	header := toolHeader(sty, opts.Status, "Ask User", cappedWidth, opts.Compact)
	if opts.Compact {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	var parts []string
	parts = append(parts, header)

	// Render history of already-answered questions.
	for _, ans := range a.history {
		q := sty.Tool.AskUserHistory.Render("Q: " + ansi.Truncate(ans.question, bodyWidth-3, "…"))
		av := sty.Tool.AskUserHistory.Render("A: " + ansi.Truncate(ans.answer, bodyWidth-3, "…"))
		parts = append(parts, sty.Tool.Body.Render(q+"\n"+av))
	}

	// If the result is in (all questions answered), we're done — show only history.
	if opts.HasResult() {
		if len(a.history) == 0 && opts.Result != nil {
			parts = append(parts, sty.Tool.Body.Render(
				sty.Tool.AskUserHistory.Render(opts.Result.Content),
			))
		}
		return strings.Join(parts, "\n")
	}

	// Render the active question UI.
	if a.waitingForUser && a.activeReq != nil {
		req := a.activeReq
		parts = append(parts, "")
		qLine := sty.Tool.Body.Render(ansi.Truncate(req.Question, bodyWidth, "…"))
		parts = append(parts, qLine)
		parts = append(parts, "")

		var optLines []string
		for i, opt := range req.Options {
			cursor := "  "
			if i == a.cursor {
				cursor = sty.Tool.AskUserCursor.Render("▶ ")
			}

			var selMark string
			if req.MultiSelect {
				if i < len(a.selections) && a.selections[i] {
					selMark = sty.Tool.ResultAdded.Render("[✓]") + " "
				} else {
					selMark = sty.Tool.StateCancelled.Render("[ ]") + " "
				}
			}

			var label string
			if i == a.cursor {
				label = sty.Tool.AskUserOptionFocused.Render(opt.Label)
			} else {
				label = sty.Tool.ResultItemName.Render(opt.Label)
			}

			line := cursor + selMark + label
			if opt.Description != "" {
				desc := sty.Tool.ResultItemDesc.Render(
					ansi.Truncate(opt.Description, bodyWidth-6, "…"),
				)
				line += "\n    " + desc
			}
			optLines = append(optLines, line)
		}

		if len(optLines) > 0 {
			parts = append(parts, sty.Tool.Body.Render(strings.Join(optLines, "\n")))
		}

		hint := "↑↓ navigate · Enter select"
		if req.MultiSelect {
			hint = "↑↓ navigate · Space toggle · Enter confirm"
		}
		if req.IsCustomText {
			hint = "type your answer in the editor below and press Enter"
		}
		parts = append(parts, "")
		parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserFooter.Render(hint)))
	}

	return strings.Join(parts, "\n")
}
