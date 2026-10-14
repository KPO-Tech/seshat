package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// interruptedToolResult is what a tool call gets as its result when the turn
// ended before the tool returned one. The call may have started, so the text
// says so instead of pretending it never ran.
const interruptedToolResult = "This tool call was interrupted before it returned a result. It may or may not have run: check the current state before retrying it."

// maxInterruptionReason bounds the error text copied into the note, so a long
// provider error body does not fill the next request.
const maxInterruptionReason = 300

// keepInterruptedTurn keeps what a turn did when it ended in an error or was
// cancelled, instead of dropping it. Without this, a long turn that failed near
// its end left only the user's first message in the conversation, and a
// "continue" restarted the work from scratch.
//
// turnMessages is the transcript as the loop left it; acceptedMessages is the
// transcript persisted when the user's message was accepted. When the turn
// produced nothing beyond that message there is nothing to keep.
//
// The turn is not counted as completed (the caller does not advance the turn
// number): an errored loop must not exhaust the turn limit.
func (s *Session) keepInterruptedTurn(turnMessages, acceptedMessages []types.Message, cause error) {
	if len(turnMessages) == 0 || sameTranscriptEnd(turnMessages, acceptedMessages) {
		return
	}
	kept := repairUnansweredToolCalls(turnMessages)
	kept = append(kept, interruptedTurnNote(cause, s.state.TurnID))
	s.state.ReplaceMessages(kept)
	if s.state.Metadata != nil {
		s.state.Metadata.UpdatedAt = currentTime()
	}
	if err := s.persistSessionState(acceptedMessages); err != nil {
		// The original error is the one the caller needs; a failure to save the
		// partial turn must not replace it.
		slog.Warn("could not persist the interrupted turn", "error", err)
	}
}

// interruptedTurnNote tells the model why the previous turn stopped, so that a
// "continue" resumes the work instead of starting it again.
func interruptedTurnNote(cause error, turnID types.TurnID) types.Message {
	var why string
	if errors.Is(cause, context.Canceled) {
		why = "was interrupted by the user"
	} else {
		why = "stopped because of an error"
		if reason := firstLine(cause); reason != "" {
			why += ": " + reason
		}
	}
	text := fmt.Sprintf("[The previous turn %s before it finished. Everything done in it is preserved above. If asked to continue, resume from where it stopped; do not start over.]", why)
	note := types.UserMessage(fmt.Sprintf("msg-interrupted-%s", turnID), text)
	note.Metadata = &types.MessageMetadata{TurnID: turnID.String()}
	return note
}

func firstLine(err error) string {
	if err == nil {
		return ""
	}
	line := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(line) > maxInterruptionReason {
		line = line[:maxInterruptionReason] + "..."
	}
	return line
}

// repairUnansweredToolCalls makes a transcript that was cut mid-turn valid
// again: every tool call of an assistant message gets a result. The results a
// call already has are the messages right after the assistant message that
// carry a tool result; a call without one gets an error result placed after
// them. Providers reject a request in which a tool call has no result.
func repairUnansweredToolCalls(messages []types.Message) []types.Message {
	out := make([]types.Message, 0, len(messages))
	for i := 0; i < len(messages); {
		message := messages[i]
		out = append(out, message)
		i++
		if message.Role != types.RoleAssistant {
			continue
		}
		calls := toolCallsOf(message)
		if len(calls) == 0 {
			continue
		}
		answered := make(map[string]bool, len(calls))
		for i < len(messages) && isToolResultMessage(messages[i]) {
			for _, block := range messages[i].Content {
				if result, ok := block.(types.ToolResultContent); ok {
					answered[result.ToolUseID] = true
				}
			}
			out = append(out, messages[i])
			i++
		}
		for _, call := range calls {
			if answered[call.ID] {
				continue
			}
			out = append(out, interruptedResultMessage(call, message))
		}
	}
	return out
}

func toolCallsOf(message types.Message) []types.ToolUseContent {
	var calls []types.ToolUseContent
	for _, block := range message.Content {
		if call, ok := block.(types.ToolUseContent); ok {
			calls = append(calls, call)
		}
	}
	return calls
}

func interruptedResultMessage(call types.ToolUseContent, assistant types.Message) types.Message {
	result := types.UserMessage(fmt.Sprintf("msg-%s-interrupted", call.ID), "")
	result.Content = []types.ContentBlock{types.ToolResultContent{
		ToolUseID: call.ID,
		Content:   interruptedToolResult,
		IsError:   true,
	}}
	if assistant.Metadata != nil {
		result.Metadata = &types.MessageMetadata{TurnID: assistant.Metadata.TurnID}
	}
	return result
}

// sameTranscriptEnd reports whether the turn left the transcript as it was when
// the user's message was accepted. A turn that compacted the transcript can be
// shorter than the accepted one while still having done work, so length alone
// is not the test.
func sameTranscriptEnd(turnMessages, acceptedMessages []types.Message) bool {
	if len(turnMessages) != len(acceptedMessages) || len(turnMessages) == 0 {
		return false
	}
	return turnMessages[len(turnMessages)-1].ID == acceptedMessages[len(acceptedMessages)-1].ID
}
