package chat

import (
	"testing"

	taskTool "github.com/KPO-Tech/seshat-runtime/internal/tools/task"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/message"
)

func taskMessage(parts ...message.ContentPart) *message.Message {
	return &message.Message{ID: "m1", Parts: parts}
}

func TestPlanTaskListFromSingleTaskCalls(t *testing.T) {
	msg := taskMessage(
		message.ToolCall{ID: "c1", Name: taskTool.ToolNameTaskCreate, Input: `{"subject":"Write plan","description":"d"}`},
		message.ToolCall{ID: "c2", Name: taskTool.ToolNameTaskUpdate, Input: `{"taskId":"7","status":"in_progress"}`},
	)
	results := map[string]message.ToolResult{
		"c1": {Data: `{"task":{"id":"7","subject":"Write plan"}}`},
	}
	tasks := collectPlanTaskItems(msg, results)
	if len(tasks) != 1 || tasks[0].ID != "7" || tasks[0].Status != taskTool.TaskStatusInProgress {
		t.Fatalf("unexpected tasks: %#v", tasks)
	}
}

func TestPlanTaskListFromBatchCalls(t *testing.T) {
	msg := taskMessage(
		message.ToolCall{ID: "c1", Name: taskTool.ToolNameTaskCreate, Input: `{"tasks":[{"subject":"Plan","description":"d"},{"subject":"Research","description":"d"}]}`},
		message.ToolCall{ID: "c2", Name: taskTool.ToolNameTaskUpdate, Input: `{"updates":[{"taskId":"1","status":"completed"},{"taskId":"2","status":"in_progress"}]}`},
	)
	results := map[string]message.ToolResult{
		"c1": {Data: `{"tasks":[{"id":"1","subject":"Plan"},{"id":"2","subject":"Research"}]}`},
	}
	tasks := collectPlanTaskItems(msg, results)
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks, got %#v", tasks)
	}
	if tasks[0].Status != taskTool.TaskStatusCompleted || tasks[1].Status != taskTool.TaskStatusInProgress {
		t.Fatalf("batch updates not applied: %#v", tasks)
	}
	if tasks[0].Subject != "Plan" || tasks[1].Subject != "Research" {
		t.Fatalf("batch subjects lost: %#v", tasks)
	}
}
