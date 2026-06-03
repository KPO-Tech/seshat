package tasks

import (
	"context"
	"time"

	taskTool "github.com/EngineerProjects/nexus-engine/internal/tools/task"
)

// runtimeAdapter is the narrow bridge between the task manager and the
// taskTool runtime contract. It keeps the task tool package decoupled from the
// concrete Manager implementation while still exposing live task state.
type runtimeAdapter struct {
	manager *Manager
}

func (a *runtimeAdapter) GetTask(ctx context.Context, taskID string) (*taskTool.RuntimeTask, error) {
	_ = ctx
	task, err := a.manager.GetTask(TaskID(taskID))
	if err != nil {
		return nil, err
	}
	return toRuntimeTask(task), nil
}

func (a *runtimeAdapter) ListTasks(ctx context.Context) ([]*taskTool.RuntimeTask, error) {
	_ = ctx
	tasks := a.manager.ListTasks()
	result := make([]*taskTool.RuntimeTask, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, toRuntimeTask(task))
	}
	return result, nil
}

func (a *runtimeAdapter) ReadTaskOutput(ctx context.Context, taskID string) (string, error) {
	_ = ctx
	return a.manager.ReadTaskOutput(TaskID(taskID))
}

func (a *runtimeAdapter) WaitForTask(ctx context.Context, taskID string, timeout time.Duration) (*taskTool.RuntimeTask, error) {
	task, err := a.manager.WaitForTask(ctx, TaskID(taskID), timeout)
	if err != nil {
		return toRuntimeTask(task), err
	}
	return toRuntimeTask(task), nil
}

func (a *runtimeAdapter) KillTask(ctx context.Context, taskID string) error {
	_ = ctx
	return a.manager.KillTask(TaskID(taskID))
}

// toRuntimeTask converts the internal task record into the stable runtime view
// consumed by task tools. The conversion intentionally drops manager-only fields
// such as live process handles and cancel functions.
func toRuntimeTask(task *Task) *taskTool.RuntimeTask {
	if task == nil {
		return nil
	}
	return &taskTool.RuntimeTask{
		ID:          string(task.ID),
		Type:        taskTool.RuntimeTaskType(task.Type),
		Status:      taskTool.RuntimeTaskStatus(task.Status),
		Command:     task.Command,
		Description: task.Description,
		Output:      task.Output,
		OutputFile:  task.OutputFile,
		ExitCode:    task.ExitCode,
		CreatedAt:   task.CreatedAt,
		StartedAt:   task.StartedAt,
		CompletedAt: task.CompletedAt,
	}
}
