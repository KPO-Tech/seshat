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
	Workflow       = automation.Workflow
	Result         = automation.Result
	Options        = automation.Options
	ExecuteFunc    = automation.ExecuteFunc
	Middleware     = automation.Middleware
	Sink           = automation.Sink
	StateStore     = automation.StateStore
	ExecutionState = automation.ExecutionState
	ScheduledRun   = automation.ScheduledRun
	Schedule       = automation.Schedule
)

// ─── Runner ───────────────────────────────────────────────────────────────────

type (
	Runner         = automation.Runner
	RunnerConfig   = automation.RunnerConfig
	ExecuteConfig  = automation.ExecuteConfig
	SystemPrompter = automation.SystemPrompter
)

func NewRunner(cfg RunnerConfig) (*Runner, error) { return automation.NewRunner(cfg) }

// ─── Executor ─────────────────────────────────────────────────────────────────

type (
	Executor       = automation.Executor
	ExecutorConfig = automation.ExecutorConfig
)

func NewExecutor(cfg ExecutorConfig) (*Executor, error) { return automation.NewExecutor(cfg) }

// ─── Registry ─────────────────────────────────────────────────────────────────

type Registry = automation.Registry

func NewRegistry() *Registry { return automation.NewRegistry() }

// ─── Scheduler ────────────────────────────────────────────────────────────────

type Scheduler = automation.Scheduler

func NewScheduler(e *Executor) *Scheduler { return automation.NewScheduler(e) }

// ─── Schedules ────────────────────────────────────────────────────────────────

type (
	IntervalSchedule = automation.IntervalSchedule
	OnceSchedule     = automation.OnceSchedule
	CronSchedule     = automation.CronSchedule
)

func Every(d time.Duration) *IntervalSchedule { return automation.Every(d) }
func Once(at time.Time) *OnceSchedule         { return automation.Once(at) }
func Cron(expr string) (*CronSchedule, error) { return automation.Cron(expr) }
func MustCron(expr string) *CronSchedule      { return automation.MustCron(expr) }

// ─── Sinks ────────────────────────────────────────────────────────────────────

type (
	StdoutSink  = automation.StdoutSink
	FileSink    = automation.FileSink
	WebhookSink = automation.WebhookSink
	MultiSink   = automation.MultiSink
	DiscardSink = automation.DiscardSink
)

func NewWebhookSink(url string) *WebhookSink { return automation.NewWebhookSink(url) }
func NewMultiSink(sinks ...Sink) *MultiSink  { return automation.NewMultiSink(sinks...) }

// ─── State stores ─────────────────────────────────────────────────────────────

type (
	MemoryStateStore = automation.MemoryStateStore
	FileStateStore   = automation.FileStateStore
)

func NewMemoryStateStore() *MemoryStateStore                { return automation.NewMemoryStateStore() }
func NewFileStateStore(dir string) (*FileStateStore, error) { return automation.NewFileStateStore(dir) }

// ─── Middleware constructors ──────────────────────────────────────────────────

func Chain(mw ...Middleware) Middleware           { return automation.Chain(mw...) }
func WithRetry(n int, b time.Duration) Middleware { return automation.WithRetry(n, b) }
func WithTimeout(d time.Duration) Middleware      { return automation.WithTimeout(d) }
func WithLogging(l *log.Logger) Middleware        { return automation.WithLogging(l) }
func WithRecovery() Middleware                    { return automation.WithRecovery() }
func WithMetrics(fn func(Result)) Middleware      { return automation.WithMetrics(fn) }

// ─── Options ─────────────────────────────────────────────────────────────────

func DefaultOptions() Options { return automation.DefaultOptions() }

// ─── Jobs & persistent scheduler ─────────────────────────────────────────────

type (
	TriggerType  = automation.TriggerType
	Trigger      = automation.Trigger
	AgentConfig  = automation.AgentConfig
	JobStatus    = automation.JobStatus
	Job          = automation.Job
	RunStatus    = automation.RunStatus
	JobRun       = automation.JobRun
	JobStore     = automation.JobStore
	DBJobStore   = automation.DBJobStore
	JobScheduler = automation.JobScheduler
)

const (
	TriggerTypeCron     = automation.TriggerTypeCron
	TriggerTypeInterval = automation.TriggerTypeInterval
	TriggerTypeOnce     = automation.TriggerTypeOnce
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
