package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
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
	// optionYOffsets stores the terminal row (relative to item top) where each
	// option in the current question starts. Populated by RenderTool and used
	// by HandleMouseClick to map a click Y coordinate to an option index.
	optionYOffsets []int
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
	a.optionYOffsets = nil
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

// HandleMouseClick overrides the base handler for interactive survey mode.
// It maps the click Y position to an option and toggles/selects it.
func (a *AskUserToolMessageItem) HandleMouseClick(btn ansi.MouseButton, x, y int) bool {
	if btn != ansi.MouseLeft {
		return false
	}
	if !a.waitingForUser || a.activeReq == nil || a.customMode {
		return a.baseToolMessageItem.HandleMouseClick(btn, x, y)
	}
	optIdx := a.optionIndexAtY(y)
	if optIdx < 0 {
		// Click not on any option — still mark as handled to suppress expansion toggle.
		return true
	}
	a.optionCursor[a.currentQuestion] = optIdx
	a.selectCurrentOption()
	return true
}

// ToggleExpanded is a no-op while the survey is waiting for user input so that
// a click on the bubble does not collapse the question.
func (a *AskUserToolMessageItem) ToggleExpanded() bool {
	if a.waitingForUser {
		return true
	}
	return a.baseToolMessageItem.ToggleExpanded()
}

// optionIndexAtY returns the option index for the given item-relative Y coordinate,
// or -1 if no option occupies that row.
func (a *AskUserToolMessageItem) optionIndexAtY(y int) int {
	if len(a.optionYOffsets) == 0 {
		return -1
	}
	question := a.currentSurveyQuestion()
	for i, startY := range a.optionYOffsets {
		if y < startY {
			break
		}
		endY := startY + 1
		if i < len(question.Options) {
			opt := question.Options[i]
			isOther := opt.Value == "__other__" || strings.EqualFold(opt.Label, "Other")
			showDesc := opt.Description != "" && !(a.customMode && i == a.optionCursor[a.currentQuestion] && isOther)
			if showDesc {
				endY = startY + 2
			}
		}
		if y < endY {
			return i
		}
	}
	return -1
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

// multiSelectCount returns the number of currently checked options for question index.
func (a *AskUserToolMessageItem) multiSelectCount(qIdx int) int {
	if a.activeReq == nil || qIdx >= len(a.multiSelections) {
		return 0
	}
	count := 0
	for _, v := range a.multiSelections[qIdx] {
		if v {
			count++
		}
	}
	return count
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

		// preambleLines tracks how many terminal rows precede the options block,
		// used by optionIndexAtY for mouse click detection.
		// header(1) + blank separator(1) = 2 so far.
		preambleLines := 2

		if len(a.activeReq.Questions) > 1 {
			progress := fmt.Sprintf("Question %d/%d", a.currentQuestion+1, len(a.activeReq.Questions))
			parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render(progress)))
			parts = append(parts, "")
			preambleLines += 2
		}
		if question.Header != "" {
			parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserHistory.Render("["+question.Header+"]")))
			parts = append(parts, "")
			preambleLines += 2
		}

		questionWrapped := ansi.Wordwrap(question.Question, bodyWidth, "")
		preambleLines += strings.Count(questionWrapped, "\n") + 1
		preambleLines += 1 // blank after question

		parts = append(parts, sty.Tool.Body.Render(questionWrapped))
		parts = append(parts, "")

		// Compute per-option Y offsets for mouse click hit detection.
		a.optionYOffsets = a.optionYOffsets[:0]
		lineY := preambleLines
		for i, opt := range question.Options {
			a.optionYOffsets = append(a.optionYOffsets, lineY)
			isOtherOffset := opt.Value == "__other__" || strings.EqualFold(opt.Label, "Other")
			hasCustomOffset := isOtherOffset && strings.TrimSpace(a.customAnswers[a.currentQuestion]) != ""
			// Description hidden when: actively typing in this option, OR custom text replaces it.
			showDesc := opt.Description != "" && !(a.customMode && i == a.optionCursor[a.currentQuestion] && isOtherOffset) && !hasCustomOffset
			if showDesc {
				lineY += 2
			} else {
				lineY += 1
			}
		}

		// Render options.
		var optLines []string
		for i, opt := range question.Options {
			isFocused := i == a.optionCursor[a.currentQuestion]
			isSelected := question.MultiSelect && a.multiSelections[a.currentQuestion][i]
			isSingleSelected := !question.MultiSelect && a.singleSelections[a.currentQuestion] == i

			cursor := "  "
			if isFocused {
				cursor = sty.Tool.AskUserCursor.Render("▶ ")
			}

			isOtherOpt := opt.Value == "__other__" || strings.EqualFold(opt.Label, "Other")
			customText := a.customAnswers[a.currentQuestion]
			hasCustom := isOtherOpt && strings.TrimSpace(customText) != ""

			var label string
			if a.customMode && isFocused && isOtherOpt {
				// Actively typing: show the live input.
				label = a.customInput.View()
			} else if hasCustom && !isFocused {
				// Confirmed custom text, not focused: show what was typed, not "Other".
				label = sty.Tool.AskUserOptionSelected.Render(customText)
			} else if hasCustom && isFocused {
				// Confirmed custom text, re-focused: show typed text in focused style.
				label = sty.Tool.AskUserOptionFocused.Render(customText)
			} else {
				label = opt.Label
				switch {
				case isFocused:
					label = sty.Tool.AskUserOptionFocused.Render(label)
				case isSelected || isSingleSelected:
					label = sty.Tool.AskUserOptionSelected.Render(label)
				default:
					label = sty.Tool.ResultItemName.Render(label)
				}
			}

			prefix := cursor
			if question.MultiSelect {
				if isSelected {
					prefix += sty.Tool.AskUserCheckOn.Render("[✓]") + " "
				} else {
					prefix += sty.Tool.AskUserCheckOff.Render("[ ]") + " "
				}
			} else if isSingleSelected ||
				(a.customAnswers[a.currentQuestion] != "" && (opt.Value == "__other__" || strings.EqualFold(opt.Label, "Other"))) {
				prefix += sty.Tool.AskUserCheckOn.Render("[✓]") + " "
			}

			line := prefix + label
			if opt.Description != "" && !(a.customMode && isFocused && isOtherOpt) && !hasCustom {
				desc := sty.Tool.ResultItemDesc.Render(ansi.Truncate(opt.Description, bodyWidth-8, "…"))
				line += "\n    " + desc
			}
			optLines = append(optLines, line)
		}
		if len(optLines) > 0 {
			parts = append(parts, sty.Tool.Body.Render(strings.Join(optLines, "\n")))
		}

		// Build hint line.
		var hintParts []string
		if a.customMode {
			hintParts = append(hintParts, "Type · Enter confirm · Esc cancel")
		} else {
			if question.MultiSelect {
				hintParts = append(hintParts, "↑↓ navigate")
				hintParts = append(hintParts, "Space/Enter toggle")
				if n := a.multiSelectCount(a.currentQuestion); n > 0 {
					hintParts = append(hintParts, sty.Tool.AskUserCount.Render(fmt.Sprintf("%d selected", n)))
				}
			} else {
				hintParts = append(hintParts, "↑↓ navigate")
				hintParts = append(hintParts, "Enter select")
			}
			if a.currentQuestion > 0 {
				hintParts = append(hintParts, "← back")
			}
			if a.currentQuestion == len(a.activeReq.Questions)-1 {
				hintParts = append(hintParts, "Ctrl+Y submit")
			} else {
				hintParts = append(hintParts, "Tab/→ next")
			}
		}
		parts = append(parts, "")
		parts = append(parts, sty.Tool.Body.Render(sty.Tool.AskUserFooter.Render(strings.Join(hintParts, " · "))))
	}

	return strings.Join(parts, "\n")
}
