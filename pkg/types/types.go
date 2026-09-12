package types

import (
	"context"

	internaltypes "github.com/KPO-Tech/seshat/internal/types"
)

type (
	APIProvider           = internaltypes.APIProvider
	APIRequest            = internaltypes.APIRequest
	APIResponse           = internaltypes.APIResponse
	APIChunkType          = internaltypes.APIChunkType
	APIResponseChunk      = internaltypes.APIResponseChunk
	ContentBlock          = internaltypes.ContentBlock
	ExecutionOrigin       = internaltypes.ExecutionOrigin
	ImageContent          = internaltypes.ImageContent
	Message               = internaltypes.Message
	ModelIdentifier       = internaltypes.ModelIdentifier
	PermissionMode        = internaltypes.PermissionMode
	PromptFn              = internaltypes.PromptFn
	PromptOption          = internaltypes.PromptOption
	PromptRequest         = internaltypes.PromptRequest
	PromptResponse        = internaltypes.PromptResponse
	PromptType            = internaltypes.PromptType
	Role                  = internaltypes.Role
	RuntimeEvent          = internaltypes.RuntimeEvent
	RuntimeEventType      = internaltypes.RuntimeEventType
	SessionID             = internaltypes.SessionID
	TextContent           = internaltypes.TextContent
	TokenUsage            = internaltypes.TokenUsage
	ContextWindow         = internaltypes.ContextWindow
	ToolPermissionRequest = internaltypes.ToolPermissionRequest
	ToolResultContent     = internaltypes.ToolResultContent
	ToolUseContent        = internaltypes.ToolUseContent
	TurnID                = internaltypes.TurnID
)

const (
	APIChunkTypeContentBlockStart = internaltypes.APIChunkTypeContentBlockStart
	APIChunkTypeContentBlockDelta = internaltypes.APIChunkTypeContentBlockDelta
	APIChunkTypeContentBlockStop  = internaltypes.APIChunkTypeContentBlockStop
	APIChunkTypeMessageDelta      = internaltypes.APIChunkTypeMessageDelta
	APIChunkTypeMessageStop       = internaltypes.APIChunkTypeMessageStop
	APIChunkTypeError             = internaltypes.APIChunkTypeError

	APIProviderAnthropic  = internaltypes.APIProviderAnthropic
	APIProviderBedrock    = internaltypes.APIProviderBedrock
	APIProviderCodex      = internaltypes.APIProviderCodex
	APIProviderDeepSeek   = internaltypes.APIProviderDeepSeek
	APIProviderFoundry    = internaltypes.APIProviderFoundry
	APIProviderGemini     = internaltypes.APIProviderGemini
	APIProviderKimi       = internaltypes.APIProviderKimi
	APIProviderMiniMax    = internaltypes.APIProviderMiniMax
	APIProviderMistral    = internaltypes.APIProviderMistral
	APIProviderOllama     = internaltypes.APIProviderOllama
	APIProviderOpenAI     = internaltypes.APIProviderOpenAI
	APIProviderOpenCode   = internaltypes.APIProviderOpenCode
	APIProviderOpenRouter = internaltypes.APIProviderOpenRouter
	APIProviderVertex     = internaltypes.APIProviderVertex
	APIProviderWorkersAI  = internaltypes.APIProviderWorkersAI
	APIProviderZAi        = internaltypes.APIProviderZAi

	ExecutionOriginInteractive = internaltypes.ExecutionOriginInteractive
	ExecutionOriginAutomation  = internaltypes.ExecutionOriginAutomation
	ExecutionOriginSkillAgent  = internaltypes.ExecutionOriginSkillAgent

	PermissionModeOnRequest   = internaltypes.PermissionModeOnRequest
	PermissionModeAuto        = internaltypes.PermissionModeAuto
	PermissionModeAcceptEdits = internaltypes.PermissionModeAcceptEdits
	PermissionModeBypass      = internaltypes.PermissionModeBypass
	PermissionModeNever       = internaltypes.PermissionModeNever
	PermissionModeGranular    = internaltypes.PermissionModeGranular

	PromptTypeConfirm = internaltypes.PromptTypeConfirm

	RoleAssistant = internaltypes.RoleAssistant
	RoleUser      = internaltypes.RoleUser
	RoleSystem    = internaltypes.RoleSystem

	RuntimeEventTypePromptRequired         = internaltypes.RuntimeEventTypePromptRequired
	RuntimeEventTypeToolPermissionRequired = internaltypes.RuntimeEventTypeToolPermissionRequired

	StopReasonEndTurn = internaltypes.StopReasonEndTurn
)

var RuntimeEventEmitterKey = internaltypes.RuntimeEventEmitterKey

func NormalizePermissionMode(raw string) (PermissionMode, bool) {
	return internaltypes.NormalizePermissionMode(raw)
}

func NormalizePermissionModeOrDefault(mode PermissionMode, fallback PermissionMode) PermissionMode {
	return internaltypes.NormalizePermissionModeOrDefault(mode, fallback)
}

func NormalizeExecutionOrigin(raw string) ExecutionOrigin {
	return internaltypes.NormalizeExecutionOrigin(raw)
}

func WithAgentUserID(ctx context.Context, userID string) context.Context {
	return internaltypes.WithAgentUserID(ctx, userID)
}

func AgentUserIDFromContext(ctx context.Context) string {
	return internaltypes.AgentUserIDFromContext(ctx)
}

func WithSubAgentMaxDepth(ctx context.Context, depth int) context.Context {
	return internaltypes.WithSubAgentMaxDepth(ctx, depth)
}

func SubAgentMaxDepthFromContext(ctx context.Context) int {
	return internaltypes.SubAgentMaxDepthFromContext(ctx)
}

// WithPromptFn returns a context carrying this turn's interactive-prompt
// bridge (consumed by ask_user_question and the permission integrator).
// Prefer this over Client.SetPromptFn / Session.SetPromptFn when the same
// *sdk.Client may be reused across concurrent turns (e.g. a per-user/provider
// client cache) - those setters mutate shared state on the client/tools and
// would otherwise race between turns sharing that client.
func WithPromptFn(ctx context.Context, fn PromptFn) context.Context {
	return internaltypes.WithPromptFn(ctx, fn)
}

// PromptFnFromContext returns the current turn's prompt bridge, or nil if none was set.
func PromptFnFromContext(ctx context.Context) PromptFn {
	return internaltypes.PromptFnFromContext(ctx)
}

// WithWebSearchRunner returns a context carrying this turn's web_search
// execution runner (fn should be an sdk.WebSearchRunnerFn). Prefer this over
// Client.SetWebSearchRunner for the same reason as WithPromptFn above.
func WithWebSearchRunner(ctx context.Context, fn any) context.Context {
	return internaltypes.WithWebSearchRunner(ctx, fn)
}

// WebSearchRunnerFromContext returns the current turn's web_search runner
// (as `any` - assert to sdk.WebSearchRunnerFn), or nil if none was set.
func WebSearchRunnerFromContext(ctx context.Context) any {
	return internaltypes.WebSearchRunnerFromContext(ctx)
}

func GetContextWindow(model ModelIdentifier) ContextWindow {
	return internaltypes.GetContextWindow(model)
}

// UserMessage builds a single-turn user Message - the minimal constructor
// needed by callers doing a one-shot providers.Client.CreateMessage call
// (title generation, memory extraction, and similar) rather than a full
// agentic sdk.Session.
func UserMessage(id string, content string) Message {
	return internaltypes.UserMessage(id, content)
}
