package chat

import (
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	planTool "github.com/EngineerProjects/nexus-engine/internal/tools/system/plan"
	taskTool "github.com/EngineerProjects/nexus-engine/internal/tools/task"
)

func TestExtractMessageItemsSkipsHiddenPlanModeTools(t *testing.T) {
	msg := &message.Message{
		ID:   "assistant-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tool-enter", Name: planTool.ToolNameEnterPlanMode, Finished: true},
			message.TextContent{Text: "entered plan mode"},
		},
	}

	items := ExtractMessageItems(&styles.Styles{}, msg, nil)
	if len(items) != 1 {
		t.Fatalf("expected only assistant text item, got %d items", len(items))
	}
	if _, ok := items[0].(*AssistantMessageItem); !ok {
		t.Fatalf("expected assistant item, got %T", items[0])
	}
}

func TestExtractMessageItemsKeepsVisibleToolsWhileSkippingHiddenPlanModeTools(t *testing.T) {
	msg := &message.Message{
		ID:   "assistant-2",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tool-enter", Name: planTool.ToolNameEnterPlanMode, Finished: true},
			message.ToolCall{ID: "tool-bash", Name: "bash", Finished: true},
			message.TextContent{Text: "done"},
		},
	}

	items := ExtractMessageItems(&styles.Styles{}, msg, nil)
	if len(items) != 2 {
		t.Fatalf("expected visible bash tool plus assistant item, got %d items", len(items))
	}
	toolItem, ok := items[0].(ToolMessageItem)
	if !ok {
		t.Fatalf("expected first item to be a visible tool item, got %T", items[0])
	}
	if got := toolItem.ToolCall().Name; got != "bash" {
		t.Fatalf("expected visible tool to be bash, got %q", got)
	}
	if _, ok := items[1].(*AssistantMessageItem); !ok {
		t.Fatalf("expected second item to be assistant item, got %T", items[1])
	}
}

func TestShouldRenderToolNameHidesPlanReviewSystemTools(t *testing.T) {
	if ShouldRenderToolName(planTool.ToolNameEnterPlanMode) {
		t.Fatalf("expected enter_plan_mode to be hidden from chat")
	}
	if ShouldRenderToolName(planTool.ToolNameExitPlanMode) {
		t.Fatalf("expected exit_plan_mode to be hidden from chat")
	}
	if ShouldRenderToolName(planTool.ToolNameSubmitPlan) {
		t.Fatalf("expected submit_plan to be hidden from chat")
	}
	if ShouldRenderToolName(taskTool.ToolNameTaskCreate) {
		t.Fatalf("expected task_create to be hidden from chat")
	}
	if ShouldRenderToolName(taskTool.ToolNameTaskUpdate) {
		t.Fatalf("expected task_update to be hidden from chat")
	}
	if !ShouldRenderToolName("bash") {
		t.Fatalf("expected bash to remain visible in chat")
	}
}

func TestShouldRenderToolNameKeepsTaskInspectionToolsVisible(t *testing.T) {
	if !ShouldRenderToolName(taskTool.ToolNameTaskList) {
		t.Fatalf("expected task_list to remain visible in chat")
	}
	if !ShouldRenderToolName(taskTool.ToolNameTaskGet) {
		t.Fatalf("expected task_get to remain visible in chat")
	}
	if !ShouldRenderToolName(taskTool.ToolNameTaskStop) {
		t.Fatalf("expected task_stop to remain visible in chat")
	}
}

func TestNewToolMessageItemUsesTaskRenderers(t *testing.T) {
	sty := &styles.Styles{}
	if _, ok := NewToolMessageItem(sty, "assistant-1", message.ToolCall{ID: "task-list", Name: taskTool.ToolNameTaskList}, nil, false).(*TaskListToolMessageItem); !ok {
		t.Fatalf("expected task_list renderer")
	}
	if _, ok := NewToolMessageItem(sty, "assistant-1", message.ToolCall{ID: "task-get", Name: taskTool.ToolNameTaskGet}, nil, false).(*TaskGetToolMessageItem); !ok {
		t.Fatalf("expected task_get renderer")
	}
	if _, ok := NewToolMessageItem(sty, "assistant-1", message.ToolCall{ID: "task-stop", Name: taskTool.ToolNameTaskStop}, nil, false).(*TaskStopToolMessageItem); !ok {
		t.Fatalf("expected task_stop renderer")
	}
}
