package registry

import (
	"context"

	contract "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type Tool = contract.Tool
type Toolset = contract.Toolset
type PermissionMatcherTool = contract.PermissionMatcherTool
type RequiresUserInteractionTool = contract.RequiresUserInteractionTool
type Definition = contract.Definition
type CallInput = contract.CallInput
type CallResult = contract.CallResult
type ContentType = contract.ContentType
type ResultMetadata = contract.ResultMetadata
type ContextModifier = contract.ContextModifier
type ToolUseContext = contract.ToolUseContext

const (
	PermissionRuleMatcherKey = contract.PermissionRuleMatcherKey
	ResolvedToolMetadataKey  = contract.ResolvedToolMetadataKey

	ContentTypeText   = contract.ContentTypeText
	ContentTypeJSON   = contract.ContentTypeJSON
	ContentTypeBinary = contract.ContentTypeBinary
	ContentTypeStream = contract.ContentTypeStream
	ContentTypeMixed  = contract.ContentTypeMixed
)

func RequiresUserInteraction(t Tool) bool {
	return contract.RequiresUserInteraction(t)
}

func ToolFromMetadata(metadata map[string]any) Tool {
	return contract.ToolFromMetadata(metadata)
}

func PermissionMatcherFromMetadata(metadata map[string]any) func(string) bool {
	return contract.PermissionMatcherFromMetadata(metadata)
}

func AttachPermissionMatcherMetadata(metadata map[string]any, resolvedTool Tool, matcher func(string) bool) map[string]any {
	return contract.AttachPermissionMatcherMetadata(metadata, resolvedTool, matcher)
}

func BuildPermissionMatcher(ctx context.Context, resolvedTool Tool, input map[string]any) (func(string) bool, error) {
	return contract.BuildPermissionMatcher(ctx, resolvedTool, input)
}

func NewToolUseContext(sessionID types.SessionID, turnID types.TurnID, toolUseID string, permissionMode types.PermissionMode) ToolUseContext {
	return contract.NewToolUseContext(sessionID, turnID, toolUseID, permissionMode)
}

func NewTextResult(content string) CallResult {
	return contract.NewTextResult(content)
}

func NewJSONResult(data any) CallResult {
	return contract.NewJSONResult(data)
}

func NewErrorResult(err error) CallResult {
	return contract.NewErrorResult(err)
}

func IsValidToolName(name string) bool {
	return contract.IsValidToolName(name)
}
