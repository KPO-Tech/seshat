package state

import "github.com/KPO-Tech/seshat-runtime/internal/types"

// Checkpoint represents a session checkpoint for recovery.
type Checkpoint struct {
	SessionID    types.SessionID `json:"session_id"`
	TurnNumber   int             `json:"turn_number"`
	MessagesHash string          `json:"messages_hash"`
	Timestamp    int64           `json:"timestamp"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

// SessionInfo contains basic information about a session.
type SessionInfo struct {
	ID          types.SessionID     `json:"id"`
	Status      types.SessionStatus `json:"status"`
	CreatedAt   int64               `json:"created_at"`
	UpdatedAt   int64               `json:"updated_at"`
	TotalTurns  int                 `json:"total_turns"`
	TotalTokens int                 `json:"total_tokens"`
	// Title is the human-readable name assigned at creation (e.g. "untitled_session_1").
	// Empty for sessions created before this field was introduced.
	Title string `json:"title,omitempty"`
	// Preview is the first user message text, truncated to ~120 runes.
	// Empty for sessions saved before this field was introduced.
	Preview string `json:"preview,omitempty"`
	// Subagent is true for a session that a sub-agent or a background agent task
	// ran in, as opposed to a conversation the user opened. Such a session is not
	// something to resume from a list of conversations.
	Subagent bool `json:"subagent,omitempty"`
	// ParentID is the session that launched this one, when it was recorded. Empty
	// for a conversation, and for a sub-agent session saved before it was recorded.
	ParentID types.SessionID `json:"parent_id,omitempty"`
}
