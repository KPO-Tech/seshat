package workspace

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agenttools "github.com/KPO-Tech/seshat/internal/seshattui/agent/tools"
	"github.com/KPO-Tech/seshat/internal/seshattui/pubsub"
	"github.com/KPO-Tech/seshat/internal/types"
	"github.com/KPO-Tech/seshat/pkg/sdk"
)

func TestPromptAskUserBuffersSurveyAnswers(t *testing.T) {
	w := NewSeshatWorkspace(nil, t.TempDir(), "openai:gpt-4o")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	questions := []agenttools.AskUserQuestion{
		{
			Header:   "First",
			Question: "Choose the first option",
			Options: []agenttools.AskUserOption{
				{Label: "Alpha", Value: "Alpha"},
				{Label: "Beta", Value: "Beta"},
				{Label: "Other", Value: "__other__"},
			},
		},
		{
			Header:   "Second",
			Question: "Choose the second option",
			Options: []agenttools.AskUserOption{
				{Label: "Blue", Value: "Blue"},
				{Label: "Green", Value: "Green"},
				{Label: "Other", Value: "__other__"},
			},
		},
	}
	rawQuestions, err := json.Marshal(questions)
	if err != nil {
		t.Fatalf("marshal questions: %v", err)
	}

	events := w.askUserBroker.Subscribe(ctx)
	firstReq := sdk.PromptRequest{
		Type:    types.PromptTypeChoice,
		Message: questions[0].Question,
		Metadata: map[string]any{
			"tool_name":             agenttools.AskUserToolName,
			"tool_use_id":           "tool-1",
			"survey_questions_json": string(rawQuestions),
			"survey_question_index": 0,
		},
	}

	respCh := make(chan sdk.PromptResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := w.promptAskUser(ctx, firstReq, "tool-1")
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	var event pubsub.Event[agenttools.AskUserRequest]
	select {
	case event = <-events:
	case err := <-errCh:
		t.Fatalf("first prompt failed early: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for ask_user event")
	}

	answersJSON, err := json.Marshal(map[string]string{
		questions[0].Question: "Alpha",
		questions[1].Question: "Green",
	})
	if err != nil {
		t.Fatalf("marshal survey answers: %v", err)
	}
	if ok := w.AnswerAskUser(event.Payload.ID, string(answersJSON)); !ok {
		t.Fatal("expected AnswerAskUser to resolve pending survey")
	}

	select {
	case resp := <-respCh:
		if got := resp.Value; got != "Alpha" {
			t.Fatalf("expected first buffered answer Alpha, got %#v", got)
		}
	case err := <-errCh:
		t.Fatalf("first prompt returned error: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for first prompt response")
	}

	secondReq := sdk.PromptRequest{
		Type:    types.PromptTypeChoice,
		Message: questions[1].Question,
		Metadata: map[string]any{
			"tool_name":             agenttools.AskUserToolName,
			"tool_use_id":           "tool-1",
			"survey_questions_json": string(rawQuestions),
			"survey_question_index": 1,
		},
	}
	secondResp, err := w.promptAskUser(ctx, secondReq, "tool-1")
	if err != nil {
		t.Fatalf("second prompt: %v", err)
	}
	if got := secondResp.Value; got != "Green" {
		t.Fatalf("expected second buffered answer Green, got %#v", got)
	}
	if _, ok := w.askUserSurveyAnswers.Load("tool-1"); ok {
		t.Fatal("expected buffered survey answers to be cleared after final question")
	}
}
