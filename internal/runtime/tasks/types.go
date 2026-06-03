package tasks

import (
	"context"
	"os/exec"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskCompletionCallback is called when a task completes.
type TaskCompletionCallback func(task *Task)

// ManagerConfig represents the manager configuration.
type ManagerConfig struct {
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
	TaskTimeout        time.Duration `json:"task_timeout"`
	OutputDir          string        `json:"output_dir"`
}

// TaskID uniquely identifies a task.
type TaskID string

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusKilled    TaskStatus = "killed"
)

// TaskType represents the type of task.
type TaskType string

const (
	TaskTypeBash  TaskType = "bash"
	TaskTypeAgent TaskType = "agent"
)

// Task represents a background task.
type Task struct {
	ID           TaskID          `json:"id"`
	Type         TaskType        `json:"type"`
	Status       TaskStatus      `json:"status"`
	Command      string          `json:"command,omitempty"`
	Prompt       string          `json:"prompt,omitempty"`
	SessionID    types.SessionID `json:"session_id,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	Output       string          `json:"output,omitempty"`
	OutputFile   string          `json:"output_file,omitempty"`
	ExitCodeFile string          `json:"exit_code_file,omitempty"`
	Error        string          `json:"error,omitempty"`
	Progress     int             `json:"progress"`
	Description  string          `json:"description,omitempty"`
	ExitCode     *int            `json:"exit_code,omitempty"`
	PID          int             `json:"pid,omitempty"`

	restored bool
	cancel   context.CancelFunc
	cmd      *exec.Cmd
}
