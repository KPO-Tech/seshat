package engine

import (
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// DetectTurnStage examines the messages generated after the initial user message
// for this turn and returns the prompt.ExecutionStage that best matches the
// current loop context.
//
// Rules applied in order (highest priority first):
//
//  1. Last user message (after turn start) contains ToolResultContent → StageToolResult
//  2. Last user message (after turn start) is text-only and follows an assistant
//     message within the turn → StageContinuation (loop-injected nudge)
//  3. Otherwise → StageDefault (fresh turn or no in-turn history yet)
//
// The turnID is used to locate the first user message of the current turn so that
// messages from previous turns are excluded from stage detection.
func DetectTurnStage(messages []types.Message, turnID types.TurnID) prompt.ExecutionStage {
	// Find the first user message belonging to the current turn.
	turnStart := -1
	for i, msg := range messages {
		if msg.Role == types.RoleUser &&
			msg.Metadata != nil &&
			msg.Metadata.TurnID == turnID.String() {
			turnStart = i
			break
		}
	}

	// No current-turn anchor found, or no messages follow the turn-start message.
	if turnStart < 0 || turnStart >= len(messages)-1 {
		return prompt.StageDefault
	}

	// Scan backwards through messages that were generated DURING the current turn
	// (i.e., after the initial user message).
	tail := messages[turnStart+1:]
	for i := len(tail) - 1; i >= 0; i-- {
		msg := tail[i]
		if msg.Role != types.RoleUser {
			continue
		}
		// Tool results: any tool_result content block signals the model just got tool output.
		for _, block := range msg.Content {
			if _, ok := block.(types.ToolResultContent); ok {
				return prompt.StageToolResult
			}
		}
		// Text-only user message that follows an assistant message within the turn
		// is a loop-injected continuation nudge.
		if i > 0 && tail[i-1].Role == types.RoleAssistant {
			return prompt.StageContinuation
		}
		break
	}
	return prompt.StageDefault
}
