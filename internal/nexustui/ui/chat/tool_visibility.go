package chat

import (
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	requestPermTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/request_permissions"
	worktreeTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/worktree"
	planTool "github.com/EngineerProjects/nexus-engine/internal/tools/system/plan"
	taskTool "github.com/EngineerProjects/nexus-engine/internal/tools/task"
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
		requestPermTool.ToolName:
		return false
	default:
		return true
	}
}
