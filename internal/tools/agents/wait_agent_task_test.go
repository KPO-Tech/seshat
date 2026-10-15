package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/runtime/tasks"
)

func TestWaitAgentTool_backgroundTaskID(t *testing.T) {
	previous := tasks.GlobalManager()
	t.Cleanup(func() { tasks.SetGlobalManager(previous) })

	manager := tasks.NewManager(nil, &tasks.ManagerConfig{
		MaxConcurrentTasks: 2,
		TaskTimeout:        30 * time.Second,
		OutputDir:          t.TempDir(),
	})
	tasks.SetGlobalManager(manager)

	task, err := manager.CreateBashTask(context.Background(), "echo background-answer")
	if err != nil {
		t.Fatalf("CreateBashTask: %v", err)
	}

	res, err := NewWaitAgentTool().Call(context.Background(), makeCallInput(map[string]any{
		"agent_id":        string(task.ID),
		"timeout_seconds": float64(20),
	}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("a background task id must not be reported as an unknown agent: %s", res.Content)
	}
	if !strings.Contains(res.Content, "completed") || !strings.Contains(res.Content, "background-answer") {
		t.Fatalf("result should carry the task answer, got %q", res.Content)
	}
}
