package chat

import (
	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	configTool "github.com/KPO-Tech/seshat/internal/tools/special/config"
	requestPermTool "github.com/KPO-Tech/seshat/internal/tools/special/request_permissions"
	worktreeTool "github.com/KPO-Tech/seshat/internal/tools/special/worktree"
	planTool "github.com/KPO-Tech/seshat/internal/tools/system/plan"
	taskTool "github.com/KPO-Tech/seshat/internal/tools/task"
)

// ShouldRenderToolCall reports whether a tool call should appear as a normal
// transcript item. System-state tools can be hidden here and represented by
// dedicated UI surfaces instead.
func ShouldRenderToolCall(tc message.ToolCall) bool {
	return ShouldRenderToolName(tc.Name)
}

// ShouldRenderToolName reports whether a tool with the given name should
// appear as a normal transcript item.
func ShouldRenderToolName(name string) bool {
	switch name {
	case planTool.ToolNameEnterPlanMode, planTool.ToolNameExitPlanMode, planTool.ToolNameSubmitPlan,
		taskTool.ToolNameTaskCreate, taskTool.ToolNameTaskUpdate,
		worktreeTool.ToolNameEnterWorktree, worktreeTool.ToolNameExitWorktree,
		requestPermTool.ToolName,
		configTool.ToolNameConfig:
		return false
	default:
		return true
	}
}
