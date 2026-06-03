package engine

import (
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// SessionResponse represents the response from a session turn.
type SessionResponse struct {
	Messages    []types.Message        `json:"messages"`
	StopReason  string                 `json:"stop_reason"`
	ToolUses    []types.ToolUseContent `json:"tool_uses"`
	ToolResults []tool.CallResult      `json:"tool_results"`
	Usage       *types.TokenUsage      `json:"usage"`
	TotalTokens int                    `json:"total_tokens"`
	TurnNumber  int                    `json:"turn_number"`
	Compacted   bool                   `json:"compacted"`
}

// GetLastAssistantMessage returns the last assistant message.
func (r *SessionResponse) GetLastAssistantMessage() (types.Message, bool) {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == types.RoleAssistant {
			return r.Messages[i], true
		}
	}
	return types.Message{}, false
}

// GetLastToolResults returns the last tool results.
func (r *SessionResponse) GetLastToolResults() []tool.CallResult {
	return r.ToolResults
}

// IsComplete returns true if the session is complete.
func (r *SessionResponse) IsComplete() bool {
	return r.StopReason == types.StopReasonEndTurn ||
		r.StopReason == types.StopReasonMaxTokens ||
		r.StopReason == types.StopReasonStopSequence
}
