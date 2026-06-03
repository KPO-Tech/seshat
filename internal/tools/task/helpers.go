package task

import (
	"fmt"

	bashTool "github.com/EngineerProjects/nexus-engine/internal/tools/bash"
)

// statusToString converts TaskStatus to string
func StatusToString(status bashTool.TaskStatus) string {
	switch status {
	case bashTool.TaskStatusRunning:
		return "running"
	case bashTool.TaskStatusBackgrounded:
		return "backgrounded"
	case bashTool.TaskStatusCompleted:
		return "completed"
	case bashTool.TaskStatusKilled:
		return "killed"
	case bashTool.TaskStatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// JoinLines joins lines with newlines
func JoinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// formatTaskOutput formats task output for display
func FormatTaskOutput(taskID, status, output string) string {
	lines := []string{
		fmt.Sprintf("Task ID: %s", taskID),
		fmt.Sprintf("Status: %s", status),
	}

	if output != "" {
		preview := output
		if len(preview) > 500 {
			preview = preview[:497] + "..."
		}
		lines = append(lines, fmt.Sprintf("\nOutput:\n%s", preview))
	}

	return JoinLines(lines)
}
