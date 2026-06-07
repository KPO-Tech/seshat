package state

import "github.com/EngineerProjects/nexus-engine/internal/types"

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
	// Preview is the first user message text, truncated to ~120 runes.
	// Empty for sessions saved before this field was introduced.
	Preview string `json:"preview,omitempty"`
}
