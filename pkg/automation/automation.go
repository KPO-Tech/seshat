// Package automation exposes the automation workflow primitives for use by
// external embedders, seshat-ai, and cmd/automation workflows.
package automation

import (
	"log"
	"time"

	"github.com/KPO-Tech/seshat/internal/automation"
	"github.com/KPO-Tech/seshat/internal/providers"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
	"github.com/KPO-Tech/seshat/pkg/sdk"
)

// ─── Core types ───────────────────────────────────────────────────────────────

type (
	// Workflow is the interface every automation workflow must implement.
	Workflow = automation.Workflow
	// Result holds the complete outcome of a single workflow execution.
	Result = automation.Result
	// Options configure a single workflow execution.
	// Workflow-specific parameters (e.g. topics, target path) belong in the
	// workflow struct itself; Options covers cross-cutting execution concerns.
	Options = automation.Options
	// ExecuteFunc is the core execution signature threaded through the middleware chain.
	ExecuteFunc = automation.ExecuteFunc
	// Middleware wraps an ExecuteFunc to add cross-cutting behaviour.
	// Middlewares are applied innermost-first: Chain(A, B, C) produces A(B(C(core))).
	Middleware = automation.Middleware
	// Sink receives a completed Result after a workflow finishes.
	// Sinks are called sequentially by the Executor; use MultiSink for fan-out.
	Sink = automation.Sink
	// StateStore persists and retrieves execution state across process restarts.
	StateStore = automation.StateStore
	// ExecutionState tracks the persistent history of a workflow across runs.
	ExecutionState = automation.ExecutionState
	// ScheduledRun is a read-only view of a single job's next execution.
	ScheduledRun = automation.ScheduledRun
	// Schedule computes the next trigger time after a given reference point.
	Schedule = automation.Schedule
)

// ─── Runner ───────────────────────────────────────────────────────────────────

type (
	// Runner creates a fresh SDK client for each workflow execution.
	// It holds no mutable state, making it safe for concurrent use.
	Runner = automation.Runner
	// RunnerConfig is the base template used to build an SDK client for each
	// workflow execution.
	RunnerConfig = automation.RunnerConfig
	// ExecuteConfig holds per-execution overrides applied on top of RunnerConfig.
	ExecuteConfig = automation.ExecuteConfig
	// SystemPrompter is an optional interface a Workflow can implement to provide
	// a fully custom system prompt that replaces the Seshat default.
	// When satisfied, the Executor builds a dedicated SDK client for that execution.
	SystemPrompter = automation.SystemPrompter
)

// NewRunner builds a Runner from cfg.
func NewRunner(cfg RunnerConfig) (*Runner, error) { return automation.NewRunner(cfg) }

// ─── Executor ─────────────────────────────────────────────────────────────────

type (
	// Executor orchestrates workflow execution: it combines the Runner with
	// middleware, output sinks, and state management.
	Executor = automation.Executor
	// ExecutorConfig configures the Executor.
	ExecutorConfig = automation.ExecutorConfig
)

// NewExecutor builds an Executor from cfg.
// WithRecovery() is always prepended so panics never escape.
func NewExecutor(cfg ExecutorConfig) (*Executor, error) { return automation.NewExecutor(cfg) }

// ─── Registry ─────────────────────────────────────────────────────────────────

// Registry is a thread-safe catalog of named workflows.
// Workflows are registered once at startup and looked up by name at runtime.
type Registry = automation.Registry

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return automation.NewRegistry() }

// ─── Scheduler ────────────────────────────────────────────────────────────────

// Scheduler runs registered workflows according to their schedules.
// It uses a single goroutine and a timer-based loop; it does not spawn a
// goroutine per job. Concurrent job execution is not supported by design —
// use multiple Schedulers if you need parallel pipelines.
type Scheduler = automation.Scheduler

// NewScheduler creates a Scheduler backed by executor.
func NewScheduler(e *Executor) *Scheduler { return automation.NewScheduler(e) }

// ─── Schedules ────────────────────────────────────────────────────────────────

type (
	// IntervalSchedule fires every fixed Interval starting from first use.
	IntervalSchedule = automation.IntervalSchedule
	// OnceSchedule fires exactly once at At.
	OnceSchedule = automation.OnceSchedule
	// CronSchedule fires according to a standard 5-field cron expression:
	//
	// 	┌───────────── minute  (0–59)
	// 	│ ┌─────────── hour    (0–23)
	// 	│ │ ┌───────── dom     (1–31)
	// 	│ │ │ ┌─────── month   (1–12)
	// 	│ │ │ │ ┌───── weekday (0–6, 0=Sun)
	// 	│ │ │ │ │
	// 	* * * * *
	//
	// Supports: *, N, N-M, */N, N,M,…, and combinations thereof. When both dom
	// and dow are restricted (neither is *), POSIX cron semantics fire on
	// EITHER matching (not both) - e.g. "0 9 15 * 1" fires at 9am on the 15th
	// of every month AND every Monday, not only when the 15th is a Monday.
	// Parsing/computation is delegated to robfig/cron (MIT), which already
	// implements this correctly - a hand-rolled bitmask parser used to live
	// here and got the dom/dow OR-semantics wrong (AND instead of OR). Idea to
	// use a battle-tested cron library instead of a hand-rolled parser came
	// from studying neul-labs/m9m (MIT), which does the same.
	CronSchedule = automation.CronSchedule
)

// Every returns a schedule that fires at a fixed interval d after the previous run.
func Every(d time.Duration) *IntervalSchedule { return automation.Every(d) }

// Once returns a schedule that fires a single time, at the given instant. A time that has already passed never fires.
func Once(at time.Time) *OnceSchedule { return automation.Once(at) }

// Cron parses a 5-field cron expression. Returns an error on invalid syntax.
func Cron(expr string) (*CronSchedule, error) { return automation.Cron(expr) }

// MustCron parses expr and panics on error.
func MustCron(expr string) *CronSchedule { return automation.MustCron(expr) }

// ─── Sinks ────────────────────────────────────────────────────────────────────

type (
	// StdoutSink prints a human-readable summary of the result to stdout.
	// The workflow's streaming output is already printed in real time via StreamFn;
	// StdoutSink adds a completion footer (duration, status).
	StdoutSink = automation.StdoutSink
	// FileSink writes the result to a file.
	// The filename is derived from the workflow name and the run timestamp.
	FileSink = automation.FileSink
	// WebhookSink sends the result as a JSON POST to a URL.
	WebhookSink = automation.WebhookSink
	// MultiSink fans a Result out to multiple sinks.
	// All sinks are called even if one errors; errors are joined.
	MultiSink = automation.MultiSink
	// DiscardSink silently drops every result. Useful in tests.
	DiscardSink = automation.DiscardSink
)

// NewWebhookSink creates a sink that posts each result as JSON (workflow name, start and end times, duration, success, output, metadata and error) to url, with a timeout of 15 seconds.
func NewWebhookSink(url string) *WebhookSink { return automation.NewWebhookSink(url) }

// NewMultiSink creates a sink that writes each result to every one of sinks. A failure of one does not stop the others; the errors are returned together.
func NewMultiSink(sinks ...Sink) *MultiSink { return automation.NewMultiSink(sinks...) }

// ─── State stores ─────────────────────────────────────────────────────────────

type (
	// MemoryStateStore is an in-process store with no persistence.
	// Suitable for testing or ephemeral single-run scenarios.
	MemoryStateStore = automation.MemoryStateStore
	// FileStateStore persists one JSON file per workflow in a directory.
	// It is safe for use by a single process (no cross-process locking).
	FileStateStore = automation.FileStateStore
)

// NewMemoryStateStore creates a state store held in memory: the states are lost when the process ends.
func NewMemoryStateStore() *MemoryStateStore { return automation.NewMemoryStateStore() }

// NewFileStateStore creates a state store that keeps one JSON file per workflow in dir, creating the directory if needed, so the states survive a restart.
func NewFileStateStore(dir string) (*FileStateStore, error) { return automation.NewFileStateStore(dir) }

// ─── Middleware constructors ──────────────────────────────────────────────────

// Chain composes a slice of middlewares into a single middleware.
// The first middleware is the outermost (runs first on entry, last on exit).
func Chain(mw ...Middleware) Middleware { return automation.Chain(mw...) }

// WithRetry retries a failed workflow up to maxAttempts additional times,
// waiting backoff (doubled each attempt) between attempts.
// Overrides opts.MaxRetries and opts.RetryBackoff when both are non-zero.
func WithRetry(n int, b time.Duration) Middleware { return automation.WithRetry(n, b) }

// WithTimeout cancels the workflow if it exceeds d.
// Opts.Timeout takes precedence when non-zero.
func WithTimeout(d time.Duration) Middleware { return automation.WithTimeout(d) }

// WithLogging logs workflow start, completion, and errors to logger.
// Pass nil to use the default log package.
func WithLogging(l *log.Logger) Middleware { return automation.WithLogging(l) }

// WithRecovery catches panics from the workflow, converts them to errors,
// and ensures the Result is always populated.
func WithRecovery() Middleware { return automation.WithRecovery() }

// WithMetrics calls onResult after every execution (success or failure).
// Use this to feed Prometheus counters, Datadog metrics, or custom logging.
func WithMetrics(fn func(Result)) Middleware { return automation.WithMetrics(fn) }

// ─── Options ─────────────────────────────────────────────────────────────────

// DefaultOptions returns sensible defaults for interactive or scheduled runs.
func DefaultOptions() Options { return automation.DefaultOptions() }

// ─── Jobs & persistent scheduler ─────────────────────────────────────────────

type (
	// TriggerType is how a job is triggered: cron, interval, once, or event (on demand, dispatched by the application, with no schedule).
	TriggerType = automation.TriggerType
	// Trigger defines when an automation job fires.
	Trigger = automation.Trigger
	// AgentConfig defines the agent that executes a job.
	// When Slug is set the daemon resolves the full agent definition from seshat-ai
	// and uses it as the base configuration; all other fields act as overrides.
	AgentConfig = automation.AgentConfig
	// JobStatus is the state of a job: active, paused, or inactive (a once-trigger that already fired).
	JobStatus = automation.JobStatus
	// Job is a persisted automation task: trigger + agent config + task description.
	Job = automation.Job
	// RunStatus is the state of one execution of a job: running, success or error.
	RunStatus = automation.RunStatus
	// JobRun records a single execution of a Job.
	JobRun = automation.JobRun
	// JobStore is the persistence interface for automation jobs and their runs.
	JobStore = automation.JobStore
	// DBJobStore implements JobStore backed by a seshat DB instance.
	DBJobStore = automation.DBJobStore
	// JobScheduler manages the lifecycle of persisted automation jobs.
	// It ticks every 10 seconds, checks due jobs, and fires them as goroutines.
	// All job state is persisted via JobStore so the scheduler survives restarts.
	JobScheduler = automation.JobScheduler
)

const (
	TriggerTypeCron     = automation.TriggerTypeCron
	TriggerTypeInterval = automation.TriggerTypeInterval
	TriggerTypeOnce     = automation.TriggerTypeOnce
	TriggerTypeEvent    = automation.TriggerTypeEvent
	JobStatusActive     = automation.JobStatusActive
	JobStatusPaused     = automation.JobStatusPaused
	JobStatusInactive   = automation.JobStatusInactive
	RunStatusRunning    = automation.RunStatusRunning
	RunStatusSuccess    = automation.RunStatusSuccess
	RunStatusError      = automation.RunStatusError
)

// NewJobScheduler builds a JobScheduler backed by store and runner.
// To create a DBJobStore, use internal/automation.NewDBJobStore with a *db.DB.
func NewJobScheduler(store JobStore, runner *Runner) *JobScheduler {
	return automation.NewJobScheduler(store, runner)
}

// ─── ModelIdentifier re-export ────────────────────────────────────────────────

// ModelIdentifier uniquely identifies a model: a provider and a model name.
type ModelIdentifier = sdk.ModelIdentifier

// ─── Convenience: build RunnerConfig from environment ────────────────────────

// RunnerConfigFromEnv resolves provider credentials from the environment and
// seshat config file. modelRaw follows "provider:model" format.
func RunnerConfigFromEnv(modelRaw string) (RunnerConfig, error) {
	cfg, err := engineconfig.Load()
	if err != nil {
		return RunnerConfig{}, err
	}

	model := engineconfig.ParseModelIdentifier(modelRaw)
	if !engineconfig.HasExplicitProviderPrefix(modelRaw) {
		if p := engineconfig.DetectProviderFromModel(modelRaw); p != "" {
			model.Provider = p
		}
	}

	apiKey := engineconfig.ResolveAPIKey(cfg, model.Provider)
	providerCfg := providers.GetProviderConfig(model.Provider)
	if providerCfg == nil {
		providerCfg = &providers.Config{Provider: model.Provider}
	}
	providerCfg.APIKey = apiKey
	if cfg.ProviderBaseURL != "" {
		providerCfg.BaseURL = cfg.ProviderBaseURL
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	return RunnerConfig{
		Model:          model,
		ProviderConfig: providerCfg,
		MaxTokens:      maxTokens,
	}, nil
}
