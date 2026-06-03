package types

import "time"

// RuntimeEventType identifies a structured runtime event emitted during a turn.
type RuntimeEventType string

const (
	RuntimeEventTypeTurnStarted            RuntimeEventType = "turn.started"
	RuntimeEventTypeTurnCompleted          RuntimeEventType = "turn.completed"
	RuntimeEventTypeTurnFailed             RuntimeEventType = "turn.failed"
	RuntimeEventTypeResponseChunk          RuntimeEventType = "response.chunk"
	RuntimeEventTypeToolProgress           RuntimeEventType = "tool.progress"
	RuntimeEventTypeBrowserSession         RuntimeEventType = "browser.session"
	RuntimeEventTypeBrowserAction          RuntimeEventType = "browser.action"
	RuntimeEventTypeBrowserPage            RuntimeEventType = "browser.page"
	RuntimeEventTypeBrowserSnapshot        RuntimeEventType = "browser.snapshot"
	RuntimeEventTypeBrowserScreenshot      RuntimeEventType = "browser.screenshot"
	RuntimeEventTypeToolPermissionRequired RuntimeEventType = "tool.permission_required"
	RuntimeEventTypePromptRequired         RuntimeEventType = "prompt.request"
	RuntimeEventTypePlanSubmitted          RuntimeEventType = "plan.submitted"
	RuntimeEventTypePlanStatusChanged      RuntimeEventType = "plan.status_changed"

	// Goal events — mirrors Codex ThreadGoalUpdatedNotification / ThreadGoalUpdatedEvent.
	RuntimeEventTypeGoalUpdated RuntimeEventType = "goal.updated"

	// Multi-agent collab events — mirrors Codex CollabAgent* protocol events.
	RuntimeEventTypeAgentSpawnBegin       RuntimeEventType = "agent.spawn.begin"
	RuntimeEventTypeAgentSpawnEnd         RuntimeEventType = "agent.spawn.end"
	RuntimeEventTypeAgentWaitBegin        RuntimeEventType = "agent.wait.begin"
	RuntimeEventTypeAgentWaitEnd          RuntimeEventType = "agent.wait.end"
	RuntimeEventTypeAgentInteractionBegin RuntimeEventType = "agent.interaction.begin"
	RuntimeEventTypeAgentInteractionEnd   RuntimeEventType = "agent.interaction.end"
	RuntimeEventTypeAgentCloseBegin       RuntimeEventType = "agent.close.begin"
	RuntimeEventTypeAgentCloseEnd         RuntimeEventType = "agent.close.end"
)

// RuntimeEvent is the structured event envelope emitted by the runtime.
type RuntimeEvent struct {
	Type RuntimeEventType `json:"type"`

	SessionID SessionID `json:"session_id,omitempty"`
	TurnID    TurnID    `json:"turn_id,omitempty"`

	// TurnNumber is 1-indexed for the active turn being processed.
	TurnNumber int `json:"turn_number,omitempty"`

	Timestamp time.Time `json:"timestamp,omitempty"`

	StopReason    string      `json:"stop_reason,omitempty"`
	ExecutionMode string      `json:"execution_mode,omitempty"`
	Usage         *TokenUsage `json:"usage,omitempty"`

	Chunk             *APIResponseChunk      `json:"chunk,omitempty"`
	ToolProgress      *ToolProgress          `json:"tool_progress,omitempty"`
	Browser           *BrowserRuntimeEvent   `json:"browser,omitempty"`
	PermissionRequest *ToolPermissionRequest `json:"permission_request,omitempty"`
	PromptRequest     *PromptRequest         `json:"prompt_request,omitempty"`
	PlanEvent         *PlanRuntimeEvent      `json:"plan_event,omitempty"`

	// AgentToolUseID is set when this event originates from a sub-agent.
	// It identifies the parent agent tool_use block that spawned the sub-agent.
	AgentToolUseID string `json:"agent_tool_use_id,omitempty"`

	// GoalEvent carries structured payload for goal.updated events.
	GoalEvent *GoalRuntimeEvent `json:"goal_event,omitempty"`

	// AgentEvent carries structured payload for multi-agent collab events.
	AgentEvent *AgentRuntimeEvent `json:"agent_event,omitempty"`

	Error string `json:"error,omitempty"`
}

// GoalRuntimeEvent is the structured payload for goal.updated events.
// Mirrors Codex's ThreadGoalUpdatedEvent / ThreadGoalUpdatedNotification.
type GoalRuntimeEvent struct {
	// SessionID identifies the session this goal belongs to.
	SessionID string `json:"session_id"`
	// Objective is the current goal objective text.
	Objective string `json:"objective"`
	// Status is the current ThreadGoalStatus string.
	Status string `json:"status"`
	// TokenBudget is the optional max token budget.
	TokenBudget *int64 `json:"token_budget,omitempty"`
	// TokensUsed is the cumulative tokens consumed so far.
	TokensUsed int64 `json:"tokens_used"`
	// TimeUsedSeconds is the elapsed wall-clock time since goal creation.
	TimeUsedSeconds int64 `json:"time_used_seconds"`
	// CreatedAt is the Unix millisecond timestamp when the goal was created.
	CreatedAt int64 `json:"created_at"`
	// UpdatedAt is the Unix millisecond timestamp of the last update.
	UpdatedAt int64 `json:"updated_at"`
}

// AgentRuntimeEvent is the structured payload for agent.spawn/wait/interaction/close events.
// Mirrors Codex's CollabAgent*Event family from protocol.rs.
type AgentRuntimeEvent struct {
	// CallID is the tool_use ID of the spawning / waiting / interaction call.
	CallID string `json:"call_id"`
	// AgentID is the stable agent identifier (returned by spawn_agent).
	AgentID string `json:"agent_id"`
	// AgentNickname is the optional random human-friendly name (e.g. "Orion").
	AgentNickname string `json:"agent_nickname,omitempty"`
	// AgentRole is the optional role the agent was spawned with (e.g. "reviewer").
	AgentRole string `json:"agent_role,omitempty"`
	// Prompt is the initial or follow-up prompt (may be empty to prevent leaking CoT).
	Prompt string `json:"prompt,omitempty"`
	// Status is the last known CollabAgentStatus of the agent.
	Status string `json:"status,omitempty"`
	// Message is the inter-agent message text for interaction events.
	Message string `json:"message,omitempty"`
	// StartedAtMs is the Unix millisecond timestamp for begin events.
	StartedAtMs int64 `json:"started_at_ms,omitempty"`
	// CompletedAtMs is the Unix millisecond timestamp for end events.
	CompletedAtMs int64 `json:"completed_at_ms,omitempty"`
}

// PlanRuntimeEvent is emitted when a plan document is submitted or its status changes.
type PlanRuntimeEvent struct {
	PlanID   string `json:"plan_id"`
	Slug     string `json:"slug"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Version  int    `json:"version"`
}

// BrowserRuntimeEvent is the structured payload emitted for browser lifecycle and interaction activity.
type BrowserRuntimeEvent struct {
	Action string `json:"action,omitempty"`

	PageID       string `json:"page_id,omitempty"`
	URL          string `json:"url,omitempty"`
	Title        string `json:"title,omitempty"`
	ActivePageID string `json:"active_page_id,omitempty"`

	PageCount    int `json:"page_count,omitempty"`
	ActionCount  int `json:"action_count,omitempty"`
	ElementCount int `json:"element_count,omitempty"`
	HeadingCount int `json:"heading_count,omitempty"`
	TextLength   int `json:"text_length,omitempty"`

	Bytes         int    `json:"bytes,omitempty"`
	PersistedPath string `json:"persisted_path,omitempty"`
	PersistedSize int    `json:"persisted_size,omitempty"`
	FullPage      bool   `json:"full_page,omitempty"`
}
