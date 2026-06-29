package automation

import (
	"context"
	"fmt"
	"time"
)

// ─── Trigger ──────────────────────────────────────────────────────────────────

type TriggerType string

const (
	TriggerTypeCron     TriggerType = "cron"
	TriggerTypeInterval TriggerType = "interval"
	TriggerTypeOnce     TriggerType = "once"
)

// Trigger defines when an automation job fires.
type Trigger struct {
	Type     TriggerType
	Cron     string        // valid when Type == TriggerTypeCron
	Interval time.Duration // valid when Type == TriggerTypeInterval
	RunAt    *time.Time    // valid when Type == TriggerTypeOnce
}

// ToSchedule converts a Trigger to the Schedule interface used by the engine.
func (t Trigger) ToSchedule() (Schedule, error) {
	switch t.Type {
	case TriggerTypeCron:
		return Cron(t.Cron)
	case TriggerTypeInterval:
		if t.Interval <= 0 {
			return nil, fmt.Errorf("trigger interval must be positive")
		}
		return Every(t.Interval), nil
	case TriggerTypeOnce:
		if t.RunAt == nil {
			return nil, fmt.Errorf("trigger type 'once' requires RunAt")
		}
		return Once(*t.RunAt), nil
	default:
		return nil, fmt.Errorf("unknown trigger type %q", t.Type)
	}
}

func (t Trigger) String() string {
	switch t.Type {
	case TriggerTypeCron:
		return fmt.Sprintf("cron(%s)", t.Cron)
	case TriggerTypeInterval:
		return fmt.Sprintf("every %s", t.Interval)
	case TriggerTypeOnce:
		if t.RunAt != nil {
			return fmt.Sprintf("once at %s", t.RunAt.Format(time.RFC3339))
		}
		return "once (unset)"
	default:
		return string(t.Type)
	}
}

// ─── AgentConfig ──────────────────────────────────────────────────────────────

// AgentConfig defines the agent that executes a job.
// When Slug is set the daemon resolves the full agent definition from seshat-ai
// and uses it as the base configuration; all other fields act as overrides.
type AgentConfig struct {
	// Slug references a named agent definition in seshat-ai (e.g. "accounting-agent").
	// When set, the daemon fetches the agent's model, tools, system_prompt, and
	// max_turns from seshat-ai and merges them with any inline overrides below.
	Slug string
	// BaseType is the built-in agent type to use as a starting point.
	// Empty defaults to "general-purpose".
	BaseType string
	// Tools is the list of tool name patterns the agent is allowed to use
	// (e.g. "read", "web_search", "bash"). Empty = all tools (or inherits from agent).
	Tools []string
	// Skills is the list of skill names to load for this agent.
	Skills []string
	// Model overrides the agent/server default (format "provider:model").
	Model string
	// MaxTurns caps autonomous execution. 0 = inherit from agent or single turn.
	MaxTurns int
	// SystemPrompt overrides the default system prompt when non-empty.
	SystemPrompt string
}

// ─── Job ──────────────────────────────────────────────────────────────────────

type JobStatus string

const (
	JobStatusActive   JobStatus = "active"
	JobStatusPaused   JobStatus = "paused"
	JobStatusInactive JobStatus = "inactive" // once-trigger that already fired
)

// Job is a persisted automation task: trigger + agent config + task description.
type Job struct {
	ID            string
	OwnerID       string // user ID from the calling app — empty means unowned (system job)
	Name          string
	Description   string
	Trigger       Trigger
	Agent         AgentConfig
	Task          string
	Status        JobStatus
	LastRunAt     *time.Time
	NextRunAt     *time.Time
	LastRunStatus string // "success" | "error" | ""
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ─── JobRun ───────────────────────────────────────────────────────────────────

type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusError   RunStatus = "error"
)

// JobRun records a single execution of a Job.
type JobRun struct {
	ID        string
	JobID     string
	StartedAt time.Time
	EndedAt   *time.Time
	Status    RunStatus
	Output    string
	Error     string
}

// ─── JobStore ─────────────────────────────────────────────────────────────────

// JobStore is the persistence interface for automation jobs and their runs.
type JobStore interface {
	CreateJob(ctx context.Context, job *Job) error
	// GetJob returns a job by ID regardless of owner (for internal scheduler use).
	GetJob(ctx context.Context, id string) (*Job, error)
	// ListJobs returns all jobs for ownerID; pass "" to list all (scheduler/admin).
	ListJobs(ctx context.Context, ownerID string) ([]*Job, error)
	UpdateJob(ctx context.Context, job *Job) error
	// DeleteJob deletes a job by ID; ownerID="" bypasses ownership check (admin).
	DeleteJob(ctx context.Context, id, ownerID string) error

	CreateRun(ctx context.Context, run *JobRun) error
	UpdateRun(ctx context.Context, run *JobRun) error
	ListRuns(ctx context.Context, jobID string, limit int) ([]*JobRun, error)
	GetRun(ctx context.Context, id string) (*JobRun, error)
}
