package types

import (
	"context"

	internaltypes "github.com/KPO-Tech/seshat/internal/types"
)

type (
	APIProvider           = internaltypes.APIProvider
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

func GetContextWindow(model ModelIdentifier) ContextWindow {
	return internaltypes.GetContextWindow(model)
}
