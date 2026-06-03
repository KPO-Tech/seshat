package task

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type RuntimeTaskStatus string

type RuntimeTaskType string

const (
	RuntimeTaskStatusPending   RuntimeTaskStatus = "pending"
	RuntimeTaskStatusRunning   RuntimeTaskStatus = "running"
	RuntimeTaskStatusCompleted RuntimeTaskStatus = "completed"
	RuntimeTaskStatusFailed    RuntimeTaskStatus = "failed"
	RuntimeTaskStatusKilled    RuntimeTaskStatus = "killed"
)

type RuntimeTask struct {
	ID          string
	Type        RuntimeTaskType
	Status      RuntimeTaskStatus
	Command     string
	Description string
	Output      string
	OutputFile  string
	ExitCode    *int
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Runtime is the small contract that task-facing tools depend on. Keeping it
// here lets the task tools ask for task state/output without importing the full
// tasks manager package or its process-control details.
type Runtime interface {
	GetTask(ctx context.Context, taskID string) (*RuntimeTask, error)
	ListTasks(ctx context.Context) ([]*RuntimeTask, error)
	ReadTaskOutput(ctx context.Context, taskID string) (string, error)
	WaitForTask(ctx context.Context, taskID string, timeout time.Duration) (*RuntimeTask, error)
	KillTask(ctx context.Context, taskID string) error
}

var (
	runtimeMu   sync.RWMutex
	runtimeImpl Runtime
)

func SetRuntime(runtime Runtime) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeImpl = runtime
}

func RuntimeImpl() Runtime {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return runtimeImpl
}

// requireRuntime turns the optional global runtime registration into an
// explicit error at the tool boundary. Tool handlers call this instead of
// reaching into global state directly so the failure mode stays predictable.
func requireRuntime() (Runtime, error) {
	runtime := RuntimeImpl()
	if runtime == nil {
		return nil, fmt.Errorf("task runtime not available")
	}
	return runtime, nil
}

func statusToString(status RuntimeTaskStatus) string {
	return string(status)
}

func cloneRuntimeTask(task *RuntimeTask) *RuntimeTask {
	if task == nil {
		return nil
	}
	cloned := *task
	return &cloned
}

func sortRuntimeTasks(tasks []*RuntimeTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}
