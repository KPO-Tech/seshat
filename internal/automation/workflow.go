package automation

import (
	"context"
	"time"

	"github.com/KPO-Tech/seshat/pkg/sdk"
)

// Workflow is the interface every automation workflow must implement.
type Workflow interface {
	Name() string
	Description() string
	Run(ctx context.Context, session *sdk.Session) error
}

// SystemPrompter is an optional interface a Workflow can implement to provide
// a fully custom system prompt that replaces the Seshat default.
// When satisfied, the Executor builds a dedicated SDK client for that execution.
type SystemPrompter interface {
	SystemPrompt() string
}

// Result holds the complete outcome of a single workflow execution.
type Result struct {
	WorkflowName string
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	Output       string // accumulated text streamed by the agent
	Error        error
	Metadata     map[string]any // arbitrary key-value pairs set by middleware or workflow
}

// Success reports whether the workflow completed without error.
func (r Result) Success() bool { return r.Error == nil }
