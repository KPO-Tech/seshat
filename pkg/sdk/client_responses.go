package sdk

import "github.com/EngineerProjects/nexus-engine/internal/types"

// AskResponse represents the response from a single-turn query.
type AskResponse struct {
	Content     string           `json:"content"`
	Thinking    string           `json:"thinking,omitempty"`
	ToolUses    []ToolUseContent `json:"tool_uses"`
	ToolResults []CallResult     `json:"tool_results"`
	Usage       *TokenUsage      `json:"usage"`
	IsComplete  bool             `json:"is_complete"`
}

// SessionResponse represents the response from a multi-turn session.
type SessionResponse struct {
	Messages    []Message        `json:"messages"`
	StopReason  string           `json:"stop_reason"`
	ToolUses    []ToolUseContent `json:"tool_uses"`
	ToolResults []CallResult     `json:"tool_results"`
	Usage       *TokenUsage      `json:"usage"`
	TotalTokens int              `json:"total_tokens"`
	TurnNumber  int              `json:"turn_number"`
	IsComplete  bool             `json:"is_complete"`
	Compacted   bool             `json:"compacted,omitempty"`
}

// --- message helpers (used by Ask and sdk_session) ---

func extractTextContent(msg types.Message) string {
	var text string
	for _, block := range msg.Content {
		if textBlock, ok := block.(types.TextContent); ok {
			text += textBlock.Text
		}
	}
	return text
}

func extractThinkingContent(msg types.Message) string {
	var thinking string
	for _, block := range msg.Content {
		if tb, ok := block.(types.ThinkingContent); ok {
			thinking += tb.Thinking
		}
	}
	return thinking
}

func lastAssistantMessage(messages []types.Message) (types.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleAssistant {
			return messages[i], true
		}
	}
	return types.Message{}, false
}

func assistantMessagesForLatestTurn(messages []types.Message) []types.Message {
	lastMessage, ok := lastAssistantMessage(messages)
	if !ok {
		return nil
	}
	if lastMessage.Metadata == nil || lastMessage.Metadata.TurnID == "" {
		return []types.Message{lastMessage}
	}
	turnID := lastMessage.Metadata.TurnID
	var result []types.Message
	for _, m := range messages {
		if m.Role == types.RoleAssistant && m.Metadata != nil && m.Metadata.TurnID == turnID {
			result = append(result, m)
		}
	}
	if len(result) == 0 {
		return []types.Message{lastMessage}
	}
	return result
}

func extractThinkingFromMessages(messages []types.Message) string {
	var thinking string
	for _, m := range messages {
		thinking += extractThinkingContent(m)
	}
	return thinking
}
