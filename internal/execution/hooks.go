package execution

import runtimehooks "github.com/KPO-Tech/seshat/internal/runtime/hooks"

type ToolHookStage = runtimehooks.ToolHookStage

const (
	ToolHookStagePre  = runtimehooks.ToolHookStagePre
	ToolHookStagePost = runtimehooks.ToolHookStagePost
)

type ToolHookResult = runtimehooks.ToolHookResult
type ToolHookStop = runtimehooks.ToolHookStop
type ToolHook = runtimehooks.ToolHook
type ToolHookInput = runtimehooks.ToolHookInput
