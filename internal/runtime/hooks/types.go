package hooks

import (
	"context"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ToolHookStage identifies when a hook fires.
type ToolHookStage string

const (
	ToolHookStagePre  ToolHookStage = "pre"
	ToolHookStagePost ToolHookStage = "post"
)

// ToolHookResult is the outcome of a single hook invocation.
type ToolHookResult struct {
	Stop *ToolHookStop

	UpdatedInput map[string]any

	ExtraMessages []types.Message
}

// ToolHookStop carries the information emitted when a hook stops execution.
type ToolHookStop struct {
	Content string
	IsError bool
}

// ToolHook is a single pre- or post-tool-use hook.
type ToolHook struct {
	Stage    ToolHookStage
	Priority int
	ID       string
	Execute  func(ctx context.Context, input ToolHookInput) ToolHookResult
}

// ToolHookInput is the data passed to a hook.
type ToolHookInput struct {
	ToolName  string
	ToolUseID string
	Input     map[string]any
	ToolCtx   tool.ToolUseContext
}
