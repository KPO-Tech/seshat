package chat

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/agent/tools"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/message"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/styles"
)

func newSurvey(t *testing.T, questions ...tools.AskUserQuestion) *AskUserToolMessageItem {
	t.Helper()
	sty := styles.SeshatDark()
	item := NewAskUserToolMessageItem(&sty, message.ToolCall{ID: "ask-1", Name: tools.AskUserToolName}, nil, false)
	item.ActivateQuestion(tools.AskUserRequest{ID: "req-1", Questions: questions})
	return item
}

func pickOne(question string, labels ...string) tools.AskUserQuestion {
	q := tools.AskUserQuestion{Question: question}
	for _, l := range labels {
		q.Options = append(q.Options, tools.AskUserOption{Label: l, Value: l})
	}
	// The UI appends the free-text option itself.
	q.Options = append(q.Options, tools.AskUserOption{Label: "Other", Value: "__other__"})
	return q
}

func enter() tea.KeyMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func TestPickOneAnswerMovesToTheNextQuestionByItself(t *testing.T) {
	item := newSurvey(t, pickOne("one?", "a", "b"), pickOne("two?", "c", "d"))
	if _, cmd := item.HandleKeyEvent(enter()); cmd != nil {
		t.Fatal("answering the first of two questions must not submit")
	}
	if item.currentQuestion != 1 {
		t.Fatalf("expected to be on question 2, on question %d", item.currentQuestion+1)
	}
}

func TestPickOneAnswerOnTheLastQuestionSubmits(t *testing.T) {
	item := newSurvey(t, pickOne("one?", "a", "b"), pickOne("two?", "c", "d"))
	item.HandleKeyEvent(enter())
	_, cmd := item.HandleKeyEvent(enter())
	if cmd == nil {
		t.Fatal("answering the last question must submit")
	}
	msg, ok := cmd().(AnswerAskUserMsg)
	if !ok || msg.ID != "req-1" {
		t.Fatalf("expected the answer message for req-1, got %#v", cmd())
	}
	if item.waitingForUser {
		t.Fatal("the survey must stop waiting once submitted")
	}
}

func TestSingleQuestionSubmitsOnEnter(t *testing.T) {
	item := newSurvey(t, pickOne("only?", "a", "b"))
	_, cmd := item.HandleKeyEvent(enter())
	if cmd == nil {
		t.Fatal("a lone pick-one question must submit on Enter")
	}
}

func TestMultiSelectDoesNotAdvanceOnToggle(t *testing.T) {
	q := pickOne("many?", "a", "b")
	q.MultiSelect = true
	item := newSurvey(t, q, pickOne("two?", "c", "d"))
	if _, cmd := item.HandleKeyEvent(enter()); cmd != nil {
		t.Fatal("toggling a multi-select option must not submit")
	}
	if item.currentQuestion != 0 {
		t.Fatalf("a multi-select question waits for Tab; moved to question %d", item.currentQuestion+1)
	}
}

func TestChoosingOtherOpensTheTextFieldInsteadOfAdvancing(t *testing.T) {
	item := newSurvey(t, pickOne("one?", "a"), pickOne("two?", "c"))
	item.optionCursor[0] = 1 // "Other"
	item.HandleKeyEvent(enter())
	if !item.customMode || item.currentQuestion != 0 {
		t.Fatalf("Other must open the text field on the same question (custom=%v, question=%d)", item.customMode, item.currentQuestion+1)
	}
}
