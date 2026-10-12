package agents

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	coreagent "github.com/KPO-Tech/seshat-runtime/internal/agent"
	"github.com/KPO-Tech/seshat-runtime/internal/runtime/tasks"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// waitForEvent blocks until emit has been called once (or the test times out),
// returning the captured event. Using a real bash task (via CreateBashTask,
// which needs no query engine) as a stand-in for an agent task exercises
// notifyAgentTaskCompletion's actual wait-then-emit path end to end without
// requiring a full engine/session setup.
func waitForEvent(t *testing.T, register func(func(types.RuntimeEvent))) types.RuntimeEvent {
	t.Helper()
	var mu sync.Mutex
	var got types.RuntimeEvent
	done := make(chan struct{})
	register(func(event types.RuntimeEvent) {
		mu.Lock()
		got = event
		mu.Unlock()
		close(done)
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for completion notification")
	}
	mu.Lock()
	defer mu.Unlock()
	return got
}

func TestNotifyAgentTaskCompletion_Success(t *testing.T) {
	requireWorkingBash(t)

	manager := tasks.NewManager(nil, tasks.DefaultManagerConfig())
	task, err := manager.CreateBashTask(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("CreateBashTask: %v", err)
	}

	at := &AgentTool{}
	event := waitForEvent(t, func(emit func(types.RuntimeEvent)) {
		go at.notifyAgentTaskCompletion(manager, task.ID, "call-1", emit)
	})

	if event.ToolProgress == nil {
		t.Fatal("expected a ToolProgress event")
	}
	if event.ToolProgress.ToolUseID != "call-1" {
		t.Errorf("expected tool_use_id 'call-1', got %q", event.ToolProgress.ToolUseID)
	}
	if event.ToolProgress.ToolName != coreagent.ToolNameAgent {
		t.Errorf("expected tool_name %q, got %q", coreagent.ToolNameAgent, event.ToolProgress.ToolName)
	}
	if string(event.ToolProgress.Stage) != "completed" {
		t.Errorf("expected stage 'completed', got %q", event.ToolProgress.Stage)
	}
	finished, _ := event.ToolProgress.Metadata["subagent_finished"].(bool)
	if !finished {
		t.Error("expected metadata.subagent_finished=true, matching spawn_agent's own marker")
	}
	content, _ := event.ToolProgress.Metadata["content"].(string)
	if content != "hello" {
		t.Errorf("expected metadata.content to carry the task's real output, got %q", content)
	}
}

func TestNotifyAgentTaskCompletion_Failure(t *testing.T) {
	requireWorkingBash(t)

	manager := tasks.NewManager(nil, tasks.DefaultManagerConfig())
	task, err := manager.CreateBashTask(context.Background(), "exit 1")
	if err != nil {
		t.Fatalf("CreateBashTask: %v", err)
	}

	at := &AgentTool{}
	event := waitForEvent(t, func(emit func(types.RuntimeEvent)) {
		go at.notifyAgentTaskCompletion(manager, task.ID, "call-2", emit)
	})

	if event.ToolProgress == nil {
		t.Fatal("expected a ToolProgress event")
	}
	if string(event.ToolProgress.Stage) != "failed" {
		t.Errorf("expected stage 'failed' for a non-zero exit code, got %q", event.ToolProgress.Stage)
	}
	finished, _ := event.ToolProgress.Metadata["subagent_finished"].(bool)
	if !finished {
		t.Error("expected metadata.subagent_finished=true even on failure")
	}
}

func TestNotifyAgentTaskCompletion_UnknownTask(t *testing.T) {
	manager := tasks.NewManager(nil, tasks.DefaultManagerConfig())

	at := &AgentTool{}
	event := waitForEvent(t, func(emit func(types.RuntimeEvent)) {
		go at.notifyAgentTaskCompletion(manager, tasks.TaskID("does-not-exist"), "call-3", emit)
	})

	if event.ToolProgress == nil {
		t.Fatal("expected a ToolProgress event even when the task can't be found")
	}
	if string(event.ToolProgress.Stage) != "failed" {
		t.Errorf("expected stage 'failed' for an unknown task, got %q", event.ToolProgress.Stage)
	}
	if _, hasError := event.ToolProgress.Metadata["error"]; !hasError {
		t.Error("expected metadata.error to explain why it failed")
	}
}

func requireWorkingBash(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "bash", "-lc", "true").Run(); err != nil {
		t.Skipf("bash is not available for background-task integration test: %v", err)
	}
}
