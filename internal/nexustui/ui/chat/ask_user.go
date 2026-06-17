package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// AnswerAskUserMsg is sent when the user completes an ask_user_question bubble.
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

// AskUserToolMessageItem renders an ask_user_question tool call as a paged survey.
type AskUserToolMessageItem struct {
	*baseToolMessageItem

	activeReq        *tools.AskUserRequest
	waitingForUser   bool
	currentQuestion  int
	optionCursor     []int
	singleSelections []int
	multiSelections  [][]bool
	customAnswers    []string
	customMode       bool
	customInput      textinput.Model
	submittedAnswers map[string]string
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
	item.spinningFunc = func(s SpinningState) bool {
		if item.waitingForUser || s.Result != nil {
			return false
		}
		return !s.ToolCall.Finished
	}
	return item
}

// ActivateQuestion switches the item into interactive mode for the given question set.
func (a *AskUserToolMessageItem) ActivateQuestion(req tools.AskUserRequest) {
	questions := req.Questions
	if len(questions) == 0 {
		questions = []tools.AskUserQuestion{{
			Question:    req.Question,
			Header:      req.Header,
			Options:     append([]tools.AskUserOption(nil), req.Options...),
			MultiSelect: req.MultiSelect,
		}}
		req.Questions = questions
	}

	a.activeReq = &req
	a.waitingForUser = true
	a.currentQuestion = 0
	a.customMode = false
	a.submittedAnswers = nil
	a.optionCursor = make([]int, len(questions))
	a.singleSelections = make([]int, len(questions))
	a.multiSelections = make([][]bool, len(questions))
	a.customAnswers = make([]string, len(questions))
	for i, question := range questions {
		a.singleSelections[i] = -1
		a.multiSelections[i] = make([]bool, len(question.Options))
	}
	a.customInput = textinput.New()
	a.customInput.Placeholder = "Type custom answer..."
	a.customInput.Prompt = ""
	a.customInput.SetVirtualCursor(true)
	a.clearCache()
	a.Bump()
}

// HandleKeyEvent overrides the base handler to provide survey navigation.
func (a *AskUserToolMessageItem) HandleKeyEvent(key tea.KeyMsg) (bool, tea.Cmd) {
	if !a.waitingForUser || a.activeReq == nil {
		return a.baseToolMessageItem.HandleKeyEvent(key)
	}
	if a.customMode {
		return a.handleCustomInputKey(key)
	}

	switch key.String() {
	case "up", "k":
		a.moveOptionCursor(-1)
		return true, nil
	case "down", "j":
		a.moveOptionCursor(1)
		return true, nil
	case "space":
		if a.currentSurveyQuestion().MultiSelect {
			a.toggleCurrentOption()
			return true, nil
		}
	case "enter":
		a.selectCurrentOption()
		return true, nil
	case "tab", "right", "l":
		return true, a.advance()
	case "left", "h":
		a.goToPreviousQuestion()
		return true, nil
	case "ctrl+y":
		if a.canSubmit() {
			return true, a.submitAnswers()
		}
		return true, nil
	}
	return false, nil
}

func (a *AskUserToolMessageItem) handleCustomInputKey(key tea.KeyMsg) (bool, tea.Cmd) {
	switch key.String() {
	case "enter":
		value := strings.TrimSpace(a.customInput.Value())
		if value == "" {
			return true, nil
		}
		a.customAnswers[a.currentQuestion] = value
		a.customMode = false
		a.clearCache()
		a.Bump()
		return true, nil
	case "esc":
		a.customMode = false
		a.clearCache()
		a.Bump()
		return true, nil
	default:
		var cmd tea.Cmd
		a.customInput, cmd = a.customInput.Update(key)
		a.clearCache()
		a.Bump()
		return true, cmd
	}
}

func (a *AskUserToolMessageItem) moveOptionCursor(delta int) {
	question := a.currentSurveyQuestion()
	if len(question.Options) == 0 {
		return
	}
	cursor := a.optionCursor[a.currentQuestion] + delta
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(question.Options) {
		cursor = len(question.Options) - 1
	}
	a.optionCursor[a.currentQuestion] = cursor
	a.clearCache()
	a.Bump()
}

func (a *AskUserToolMessageItem) selectCurrentOption() {
	question := a.currentSurveyQuestion()
	if len(question.Options) == 0 {
		return
	}
	if a.currentOptionIsOther() {
		a.customMode = true
		a.customInput.SetValue(a.customAnswers[a.currentQuestion])
		a.customInput.Focus()
		a.clearCache()
		a.Bump()
		return
	}
	if question.MultiSelect {
		a.toggleCurrentOption()
		return
	}
	a.singleSelections[a.currentQuestion] = a.optionCursor[a.currentQuestion]
	a.customAnswers[a.currentQuestion] = ""
	a.clearCache()
	a.Bump()
}

func (a *AskUserToolMessageItem) toggleCurrentOption() {
	question := a.currentSurveyQuestion()
	cursor := a.optionCursor[a.currentQuestion]
	if cursor < 0 || cursor >= len(question.Options) {
		return
	}
	if a.currentOptionIsOther() {
		a.customMode = true
		a.customInput.SetValue(a.customAnswers[a.currentQuestion])
		a.customInput.Focus()
		for i := range a.multiSelections[a.currentQuestion] {
			a.multiSelections[a.currentQuestion][i] = false
		}
		a.clearCache()
		a.Bump()
		return
	}
	a.customAnswers[a.currentQuestion] = ""
	a.multiSelections[a.currentQuestion][cursor] = !a.multiSelections[a.currentQuestion][cursor]
	a.clearCache()
	a.Bump()
}

func (a *AskUserToolMessageItem) goToPreviousQuestion() {
	if a.currentQuestion == 0 {
		return
	}
	a.currentQuestion--
	a.customMode = false
	a.clearCache()
	a.Bump()
}

func (a *AskUserToolMessageItem) advance() tea.Cmd {
	if !a.currentQuestionAnswered() {
		return nil
	}
	if a.currentQuestion < len(a.activeReq.Questions)-1 {
		a.currentQuestion++
		a.customMode = false
		a.clearCache()
		a.Bump()
		return nil
	}
	if a.canSubmit() {
		return a.submitAnswers()
	}
	return nil
}

func (a *AskUserToolMessageItem) canSubmit() bool {
	if a.activeReq == nil {
		return false
	}
	for i := range a.activeReq.Questions {
		if strings.TrimSpace(a.answerForQuestion(i)) == "" {
			return false
		}
	}
	return true
}

func (a *AskUserToolMessageItem) currentQuestionAnswered() bool {
	return strings.TrimSpace(a.answerForQuestion(a.currentQuestion)) != ""
}

func (a *AskUserToolMessageItem) answerForQuestion(index int) string {
	if index < 0 || a.activeReq == nil || index >= len(a.activeReq.Questions) {
		return ""
	}
	if custom := strings.TrimSpace(a.customAnswers[index]); custom != "" {
		return custom
	}
	question := a.activeReq.Questions[index]
	if question.MultiSelect {
		var selected []string
		for optIndex, selectedOpt := range a.multiSelections[index] {
			if !selectedOpt || optIndex >= len(question.Options) {
				continue
			}
			selected = append(selected, question.Options[optIndex].Value)
		}
		return strings.Join(selected, ", ")
	}
	selectedIndex := a.singleSelections[index]
	if selectedIndex < 0 || selectedIndex >= len(question.Options) {
		return ""
	}
	return question.Options[selectedIndex].Value
}

func (a *AskUserToolMessageItem) submitAnswers() tea.Cmd {
	if a.activeReq == nil {
		return nil
	}
	questions := append([]tools.AskUserQuestion(nil), a.activeReq.Questions...)
	answers := make(map[string]string, len(questions))
	for i, question := range questions {
		answers[question.Question] = a.answerForQuestion(i)
	}
	payload := ""
	if len(questions) == 1 {
		payload = answers[questions[0].Question]
	} else {
		raw, err := json.Marshal(answers)
		if err != nil {
			return nil
		}
		payload = string(raw)
	}
	id := a.activeReq.ID
	a.waitingForUser = false
	a.submittedAnswers = answers
	a.activeReq = nil
	a.customMode = false
	a.clearCache()
	a.Bump()
	return func() tea.Msg {
		return AnswerAskUserMsg{ID: id, Value: payload}
	}
}

func (a *AskUserToolMessageItem) currentSurveyQuestion() tools.AskUserQuestion {
	if a.activeReq == nil || len(a.activeReq.Questions) == 0 {
		return tools.AskUserQuestion{}
	}
	return a.activeReq.Questions[a.currentQuestion]
}

func (a *AskUserToolMessageItem) currentOptionIsOther() bool {
	question := a.currentSurveyQuestion()
	cursor := a.optionCursor[a.currentQuestion]
	if cursor < 0 || cursor >= len(question.Options) {
		return false
	}
	option := question.Options[cursor]
	return option.Value == "__other__" || strings.EqualFold(option.Label, "Other")
}

// askUserRenderContext bridges the baseToolMessageItem ToolRenderer interface.
type askUserRenderContext struct {
	item *AskUserToolMessageItem
}

// RenderTool implements [ToolRenderer].
func (r *askUserRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	a := r.item
	cappedWidth := width

	if !a.waitingForUser && a.activeReq == nil && !opts.HasResult() && len(a.submittedAnswers) == 0 {
		return pendingTool(sty, "Ask User", opts.Anim, opts.Compact)
	}

	header := toolHeader(sty, opts.Status, "Ask User", cappedWidth, opts.Compact)
	if opts.Compact {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	var parts []string
	parts = append(parts, header)

	if opts.HasResult() {
		if opts.Result != nil {
			parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render(opts.Result.Content)))
		}
		return strings.Join(parts, "\n")
	}

	if !a.waitingForUser && len(a.submittedAnswers) > 0 {
		parts = append(parts, "")
		parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render("Answers submitted...")))
		return strings.Join(parts, "\n")
	}

	if a.waitingForUser && a.activeReq != nil {
		question := a.currentSurveyQuestion()
		parts = append(parts, "")
		if len(a.activeReq.Questions) > 1 {
			progress := fmt.Sprintf("Question %d/%d", a.currentQuestion+1, len(a.activeReq.Questions))
			parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render(progress)))
			parts = append(parts, "")
		}
		if question.Header != "" {
			parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render("["+question.Header+"]")))
			parts = append(parts, "")
		}
		parts = append(parts, sty.Tool.Body.Render(ansi.Wordwrap(question.Question, bodyWidth, "")))
		parts = append(parts, "")

		var optLines []string
		for i, opt := range question.Options {
			cursor := "  "
			if i == a.optionCursor[a.currentQuestion] {
				cursor = sty.Tool.AskUserCursor.Render("▶ ")
			}

			var label string
			if a.customMode && i == a.optionCursor[a.currentQuestion] && a.currentOptionIsOther() {
				label = a.customInput.View()
			} else {
				label = opt.Label
				if i == a.optionCursor[a.currentQuestion] {
					label = sty.Tool.AskUserOptionFocused.Render(label)
				} else {
					label = sty.Tool.ResultItemName.Render(label)
				}
			}

			prefix := cursor
			if question.MultiSelect {
				if a.multiSelections[a.currentQuestion][i] {
					prefix += sty.Tool.ResultAdded.Render("[✓]") + " "
				} else {
					prefix += sty.Tool.StateCancelled.Render("[ ]") + " "
				}
			} else if a.singleSelections[a.currentQuestion] == i || (a.customAnswers[a.currentQuestion] != "" && (opt.Value == "__other__" || strings.EqualFold(opt.Label, "Other"))) {
				prefix += sty.Tool.ResultAdded.Render("[✓]") + " "
			}

			line := prefix + label
			if opt.Description != "" && !(a.customMode && i == a.optionCursor[a.currentQuestion] && a.currentOptionIsOther()) {
				desc := sty.Tool.ResultItemDesc.Render(ansi.Truncate(opt.Description, bodyWidth-8, "…"))
				line += "\n    " + desc
			}
			optLines = append(optLines, line)
		}
		if len(optLines) > 0 {
			parts = append(parts, sty.Tool.Body.Render(strings.Join(optLines, "\n")))
		}

		hint := "↑↓ navigate · Enter select · → next"
		if question.MultiSelect {
			hint = "↑↓ navigate · Space/Enter toggle · → next"
		}
		if a.customMode {
			hint = "Type your answer · Enter confirm · Esc cancel"
		} else if a.currentQuestion > 0 {
			hint += " · ← previous"
		}
		if a.currentQuestion == len(a.activeReq.Questions)-1 {
			hint += " · Ctrl+Y/→ submit"
		}
		parts = append(parts, "")
		parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserFooter.Render(hint)))
	}

	return strings.Join(parts, "\n")
}
