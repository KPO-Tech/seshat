package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	slackgo "github.com/slack-go/slack"
)

const promptTimeout = 5 * time.Minute

// channelCtxKey is the context key for the current Slack channel/thread info.
type channelCtxKey struct{}

// channelCtxVal carries the channel and thread TS through the tool-call context.
type channelCtxVal struct {
	Channel  string
	ThreadTS string
}

// makeSlackPromptFn returns a PromptFn that routes ask_user_question calls to
// Slack Block Kit instead of stdin. The model posts an interactive message and
// the tool call blocks until the user clicks or the timeout fires.
func (b *bot) makeSlackPromptFn() sdk.PromptFn {
	return func(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
		ch, ok := ctx.Value(channelCtxKey{}).(channelCtxVal)
		if !ok || ch.Channel == "" {
			return sdk.PromptResponse{
				Value: "No Slack context — proceeding with best judgment.",
			}, nil
		}

		switch string(req.Type) {
		case "choice":
			return b.promptChoice(ctx, ch, req)
		case "confirm":
			return b.promptConfirm(ctx, ch, req)
		default: // "text" and unknown
			return b.promptText(ctx, ch, req)
		}
	}
}

// promptChoice posts a Block Kit actions message and waits for a button click.
func (b *bot) promptChoice(ctx context.Context, ch channelCtxVal, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	blockID := fmt.Sprintf("nexus_q_%d", time.Now().UnixNano())

	header := slackgo.NewSectionBlock(
		slackgo.NewTextBlockObject("mrkdwn", fmt.Sprintf("❓ *%s*", req.Message), false, false),
		nil, nil,
	)

	// Build rows of up to 5 buttons each (Slack limit per actions block).
	var rows []slackgo.Block
	var row []slackgo.BlockElement
	for i, opt := range req.Options {
		label := truncate(fmt.Sprintf("%v", opt.Label), 75)
		value := fmt.Sprintf("%v", opt.Value)
		actionID := fmt.Sprintf("%s_%d", blockID, i)
		btn := slackgo.NewButtonBlockElement(actionID, value,
			slackgo.NewTextBlockObject("plain_text", label, false, false))
		row = append(row, btn)
		if len(row) == 5 {
			rows = append(rows, slackgo.NewActionBlock(blockID, row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, slackgo.NewActionBlock(blockID, row...))
	}

	blocks := append([]slackgo.Block{header}, rows...)

	resultCh := make(chan string, 1)
	b.pendingMu.Lock()
	b.pending[blockID] = resultCh
	b.pendingMu.Unlock()
	defer func() {
		b.pendingMu.Lock()
		delete(b.pending, blockID)
		b.pendingMu.Unlock()
	}()

	if _, _, err := b.api.PostMessageContext(ctx, ch.Channel,
		slackgo.MsgOptionBlocks(blocks...),
		slackgo.MsgOptionTS(ch.ThreadTS),
		slackgo.MsgOptionDisableLinkUnfurl(),
	); err != nil {
		return sdk.PromptResponse{}, fmt.Errorf("post question: %w", err)
	}

	select {
	case answer := <-resultCh:
		return sdk.PromptResponse{Value: answer}, nil
	case <-time.After(promptTimeout):
		return sdk.PromptResponse{Value: "timeout — proceeding with best judgment"}, nil
	case <-ctx.Done():
		return sdk.PromptResponse{}, ctx.Err()
	}
}

// promptText posts a question as mrkdwn and waits for the next thread reply.
func (b *bot) promptText(ctx context.Context, ch channelCtxVal, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	threadKey := ch.Channel + ":" + ch.ThreadTS

	resultCh := make(chan string, 1)
	b.pendingMu.Lock()
	b.textPending[threadKey] = resultCh
	b.pendingMu.Unlock()
	defer func() {
		b.pendingMu.Lock()
		delete(b.textPending, threadKey)
		b.pendingMu.Unlock()
	}()

	msg := fmt.Sprintf("❓ *%s*\n_Reply in this thread to continue._", req.Message)
	if _, _, err := b.api.PostMessageContext(ctx, ch.Channel,
		slackgo.MsgOptionText(msg, false),
		slackgo.MsgOptionTS(ch.ThreadTS),
		slackgo.MsgOptionDisableLinkUnfurl(),
	); err != nil {
		return sdk.PromptResponse{}, fmt.Errorf("post text question: %w", err)
	}

	select {
	case answer := <-resultCh:
		return sdk.PromptResponse{Value: answer}, nil
	case <-time.After(promptTimeout):
		return sdk.PromptResponse{Value: "timeout — proceeding with best judgment"}, nil
	case <-ctx.Done():
		return sdk.PromptResponse{}, ctx.Err()
	}
}

// promptConfirm renders a Yes / No choice.
func (b *bot) promptConfirm(ctx context.Context, ch channelCtxVal, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	confirmReq := req
	confirmReq.Options = []sdk.PromptOption{
		{Label: "Yes", Value: "yes"},
		{Label: "No", Value: "no"},
	}
	resp, err := b.promptChoice(ctx, ch, confirmReq)
	if err != nil {
		return resp, err
	}
	resp.Value = strings.EqualFold(fmt.Sprintf("%v", resp.Value), "yes")
	return resp, nil
}

// handleInteraction routes a Slack button click to the waiting promptChoice call.
func (b *bot) handleInteraction(callback slackgo.InteractionCallback) {
	for _, action := range callback.ActionCallback.BlockActions {
		b.pendingMu.Lock()
		ch, ok := b.pending[action.BlockID]
		b.pendingMu.Unlock()
		if !ok {
			continue
		}
		select {
		case ch <- action.Value:
		default:
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
