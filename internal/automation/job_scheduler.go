package automation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
	"github.com/google/uuid"
)

// RunnerResolver resolves a per-owner Runner at execution time.
// agentSlug is the named agent to resolve (empty = no named agent).
// modelOverride is the job-level model override (may be empty).
// The second return value carries the resolved base AgentConfig from the named
// agent definition; the scheduler merges it with the job's inline AgentConfig
// (inline fields take precedence over the resolved base).
type RunnerResolver func(ctx context.Context, ownerID string, agentSlug string, modelOverride string) (*Runner, AgentConfig, error)

// JobScheduler manages the lifecycle of persisted automation jobs.
// It ticks every 10 seconds, checks due jobs, and fires them as goroutines.
// All job state is persisted via JobStore so the scheduler survives restarts.
type JobScheduler struct {
	store    JobStore
	runner   *Runner
	resolver RunnerResolver // optional; takes precedence over runner when set
	logger   *log.Logger
	mu       sync.Mutex
	running  map[string]context.CancelFunc // jobID → cancel for in-flight executions
}

// NewJobScheduler builds a JobScheduler backed by store and runner.
func NewJobScheduler(store JobStore, runner *Runner) *JobScheduler {
	return &JobScheduler{
		store:   store,
		runner:  runner,
		logger:  log.Default(),
		running: make(map[string]context.CancelFunc),
	}
}

// SetLogger replaces the default logger.
func (s *JobScheduler) SetLogger(l *log.Logger) { s.logger = l }

// SetRunnerResolver installs a dynamic runner resolver that fetches per-user
// LLM credentials at execution time. When set, it overrides the static runner.
func (s *JobScheduler) SetRunnerResolver(resolver RunnerResolver) { s.resolver = resolver }

// resolveRunner returns the runner and resolved base agent config for a job.
// If a resolver is set it is called first; it falls back to the static runner
// with an empty AgentConfig (job inline config is used as-is).
func (s *JobScheduler) resolveRunner(ctx context.Context, job *Job) (*Runner, AgentConfig, error) {
	if s.resolver != nil {
		return s.resolver(ctx, job.OwnerID, job.Agent.Slug, job.Agent.Model)
	}
	return s.runner, AgentConfig{}, nil
}

// mergeAgentConfig merges a resolved base agent config with job inline overrides.
// Non-zero inline fields take precedence over the resolved base.
func mergeAgentConfig(base, inline AgentConfig) AgentConfig {
	result := base
	if inline.SystemPrompt != "" {
		result.SystemPrompt = inline.SystemPrompt
	}
	if inline.Model != "" {
		result.Model = inline.Model
	}
	if inline.MaxTurns != 0 {
		result.MaxTurns = inline.MaxTurns
	}
	if len(inline.Tools) > 0 {
		result.Tools = inline.Tools
	}
	if len(inline.Skills) > 0 {
		result.Skills = inline.Skills
	}
	return result
}

// formatExecError turns a runner.Execute error into the JobRun.Error text,
// giving a timeout its own clear message instead of the raw
// "context deadline exceeded" — pulled out of execute() as a pure function
// so it's testable without a real Runner (Runner always makes a real LLM
// call, so it can't stand in for a "long-running" test double).
func formatExecError(err error, maxDuration time.Duration) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("execution timed out after %s", maxDuration)
	}
	return err.Error()
}

// applyRunOutcome updates job in place after a run finishes: records the
// outcome, increments RunCount, and decides whether the job goes Inactive
// (MaxRuns budget exhausted, or a "once" trigger that already fired) or gets
// rescheduled. Pulled out of execute() as a pure function (no store/runner
// dependency) so the MaxRuns/reschedule decision is unit-testable directly.
func applyRunOutcome(job *Job, run *JobRun, endedAt time.Time) {
	job.LastRunAt = &endedAt
	job.LastRunStatus = string(run.Status)
	job.RunCount++

	if job.MaxRuns > 0 && job.RunCount >= job.MaxRuns {
		job.Status = JobStatusInactive // run-count budget exhausted
		return
	}
	if job.Trigger.IsEvent() {
		return // dispatched on demand, never has a NextRunAt to compute
	}
	sched, err := job.Trigger.ToSchedule()
	if err != nil {
		return
	}
	next := sched.Next(endedAt)
	if next.IsZero() {
		job.Status = JobStatusInactive // once-trigger done
	} else {
		job.NextRunAt = &next
	}
}

// Run blocks and ticks the scheduler until ctx is cancelled.
func (s *JobScheduler) Run(ctx context.Context) error {
	// compute NextRunAt for any jobs that don't have it yet (e.g. after restart)
	if err := s.rehydrate(ctx); err != nil {
		s.logger.Printf("[automation] rehydrate error: %v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}

// rehydrate ensures all active jobs have a valid NextRunAt computed.
func (s *JobScheduler) rehydrate(ctx context.Context) error {
	jobs, err := s.store.ListJobs(ctx, "") // "" = all owners
	if err != nil {
		return err
	}
	now := time.Now()
	for _, job := range jobs {
		if job.Status != JobStatusActive || job.NextRunAt != nil || job.Trigger.IsEvent() {
			continue
		}
		sched, err := job.Trigger.ToSchedule()
		if err != nil {
			continue
		}
		next := sched.Next(now)
		if next.IsZero() {
			job.Status = JobStatusInactive
		} else {
			job.NextRunAt = &next
		}
		job.UpdatedAt = now
		_ = s.store.UpdateJob(ctx, job)
	}
	return nil
}

func (s *JobScheduler) tick(ctx context.Context, now time.Time) {
	jobs, err := s.store.ListJobs(ctx, "") // "" = all owners for scheduler loop
	if err != nil {
		s.logger.Printf("[automation] tick list error: %v", err)
		return
	}
	for _, job := range jobs {
		if job.Status != JobStatusActive {
			continue
		}
		if job.NextRunAt == nil || now.Before(*job.NextRunAt) {
			continue
		}
		s.mu.Lock()
		if _, running := s.running[job.ID]; running {
			s.mu.Unlock()
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		s.running[job.ID] = cancel
		s.mu.Unlock()

		go s.execute(jobCtx, job)
	}
}

// execute runs a single job, records the run, and updates the job state.
func (s *JobScheduler) execute(ctx context.Context, job *Job) {
	defer func() {
		s.mu.Lock()
		delete(s.running, job.ID)
		s.mu.Unlock()
	}()

	run := &JobRun{
		ID:        uuid.New().String(),
		JobID:     job.ID,
		StartedAt: time.Now(),
		Status:    RunStatusRunning,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		s.logger.Printf("[automation] create run error for job %s: %v", job.ID, err)
		return
	}

	s.logger.Printf("[automation] starting job %q (%s)", job.Name, job.ID)

	runner, baseAgent, resolveErr := s.resolveRunner(ctx, job)
	if resolveErr != nil {
		endedAt := time.Now()
		run.EndedAt = &endedAt
		run.Status = RunStatusError
		run.Error = fmt.Sprintf("resolve runner: %v", resolveErr)
		s.logger.Printf("[automation] job %q runner resolve failed: %v", job.Name, resolveErr)
		if err := s.store.UpdateRun(ctx, run); err != nil {
			s.logger.Printf("[automation] update run error for job %s: %v", job.ID, err)
		}
		return
	}

	// Merge: resolved agent definition is the base; job inline fields override.
	effectiveAgent := mergeAgentConfig(baseAgent, job.Agent)

	var buf strings.Builder
	ec := ExecuteConfig{
		StreamFn: func(delta string) { buf.WriteString(delta) },
	}
	if effectiveAgent.SystemPrompt != "" {
		ec.SystemPrompt = effectiveAgent.SystemPrompt
	}
	if effectiveAgent.Model != "" {
		ec.ModelOverride = effectiveAgent.Model
	}

	// MaxDuration caps a single execution's wall-clock time so a hung agent
	// turn can't run forever; 0 = unlimited (see Job.MaxDuration doc).
	execCtx := ctx
	if job.MaxDuration > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, job.MaxDuration)
		defer cancel()
	}

	wf := &jobWorkflow{job: job, registry: runner.cfg.NodeRegistry, secrets: runner.cfg.Secrets}
	execErr := runner.Execute(execCtx, wf, ec)

	endedAt := time.Now()
	run.EndedAt = &endedAt
	run.Output = buf.String()
	if execErr != nil {
		run.Status = RunStatusError
		run.Error = formatExecError(execErr, job.MaxDuration)
		s.logger.Printf("[automation] job %q failed: %v", job.Name, execErr)
	} else {
		run.Status = RunStatusSuccess
		s.logger.Printf("[automation] job %q completed in %s", job.Name, endedAt.Sub(run.StartedAt))
	}

	if err := s.store.UpdateRun(ctx, run); err != nil {
		s.logger.Printf("[automation] update run error for job %s: %v", job.ID, err)
	}

	// Reload job to avoid overwriting concurrent updates.
	current, err := s.store.GetJob(ctx, job.ID)
	if err != nil || current == nil {
		return
	}
	applyRunOutcome(current, run, endedAt)
	_ = s.store.UpdateJob(ctx, current)
}

// ─── Management API ───────────────────────────────────────────────────────────

// AddJob persists a new job and computes its initial NextRunAt.
// A TriggerTypeEvent job has no schedule (see Trigger.IsEvent) and is
// persisted with NextRunAt left nil - it only ever runs via RunEvent.
func (s *JobScheduler) AddJob(ctx context.Context, job *Job) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if !job.Trigger.IsEvent() {
		sched, err := job.Trigger.ToSchedule()
		if err != nil {
			return fmt.Errorf("invalid trigger: %w", err)
		}
		next := sched.Next(time.Now())
		if next.IsZero() && job.Trigger.Type == TriggerTypeOnce {
			return fmt.Errorf("once trigger RunAt is in the past")
		}
		if !next.IsZero() {
			job.NextRunAt = &next
		}
	}
	job.Status = JobStatusActive
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now
	return s.store.CreateJob(ctx, job)
}

// UpdateJob re-persists a job and recomputes its next run time (skipped for
// a TriggerTypeEvent job - see AddJob).
func (s *JobScheduler) UpdateJob(ctx context.Context, job *Job) error {
	if !job.Trigger.IsEvent() {
		sched, err := job.Trigger.ToSchedule()
		if err != nil {
			return fmt.Errorf("invalid trigger: %w", err)
		}
		next := sched.Next(time.Now())
		if !next.IsZero() {
			job.NextRunAt = &next
		}
	}
	job.UpdatedAt = time.Now()
	return s.store.UpdateJob(ctx, job)
}

// RemoveJob deletes a job and cancels any in-flight execution.
// ownerID="" bypasses ownership check (admin/system use).
func (s *JobScheduler) RemoveJob(ctx context.Context, id, ownerID string) error {
	s.mu.Lock()
	if cancel, running := s.running[id]; running {
		cancel()
		delete(s.running, id)
	}
	s.mu.Unlock()
	return s.store.DeleteJob(ctx, id, ownerID)
}

// PauseJob marks a job as paused so the scheduler skips it.
func (s *JobScheduler) PauseJob(ctx context.Context, id string) error {
	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("job %q not found", id)
	}
	job.Status = JobStatusPaused
	job.UpdatedAt = time.Now()
	return s.store.UpdateJob(ctx, job)
}

// ResumeJob re-activates a paused job and recomputes its next run time.
func (s *JobScheduler) ResumeJob(ctx context.Context, id string) error {
	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("job %q not found", id)
	}
	if !job.Trigger.IsEvent() {
		sched, err := job.Trigger.ToSchedule()
		if err != nil {
			return fmt.Errorf("invalid trigger: %w", err)
		}
		next := sched.Next(time.Now())
		if !next.IsZero() {
			job.NextRunAt = &next
		}
	}
	job.Status = JobStatusActive
	job.UpdatedAt = time.Now()
	return s.store.UpdateJob(ctx, job)
}

// RunNow immediately fires a job outside of its schedule.
// Returns the JobRun created for this execution.
func (s *JobScheduler) RunNow(ctx context.Context, id string) (*JobRun, error) {
	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("job %q not found", id)
	}

	run := &JobRun{
		ID:        uuid.New().String(),
		JobID:     job.ID,
		StartedAt: time.Now(),
		Status:    RunStatusRunning,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	go func() {
		runner, baseAgent, resolveErr := s.resolveRunner(ctx, job)
		if resolveErr != nil {
			endedAt := time.Now()
			run.EndedAt = &endedAt
			run.Status = RunStatusError
			run.Error = fmt.Sprintf("resolve runner: %v", resolveErr)
			_ = s.store.UpdateRun(ctx, run)
			return
		}

		effectiveAgent := mergeAgentConfig(baseAgent, job.Agent)
		var buf strings.Builder
		ec := ExecuteConfig{
			StreamFn: func(delta string) { buf.WriteString(delta) },
		}
		if effectiveAgent.SystemPrompt != "" {
			ec.SystemPrompt = effectiveAgent.SystemPrompt
		}
		if effectiveAgent.Model != "" {
			ec.ModelOverride = effectiveAgent.Model
		}

		wf := &jobWorkflow{job: job, registry: runner.cfg.NodeRegistry, secrets: runner.cfg.Secrets}
		execErr := runner.Execute(ctx, wf, ec)

		endedAt := time.Now()
		run.EndedAt = &endedAt
		run.Output = buf.String()
		if execErr != nil {
			run.Status = RunStatusError
			run.Error = execErr.Error()
		} else {
			run.Status = RunStatusSuccess
		}
		_ = s.store.UpdateRun(ctx, run)

		current, _ := s.store.GetJob(ctx, job.ID)
		if current != nil {
			current.LastRunAt = &endedAt
			current.LastRunStatus = string(run.Status)
			current.UpdatedAt = endedAt
			_ = s.store.UpdateJob(ctx, current)
		}
	}()

	return run, nil
}

// RunEvent fires job in response to an external event, bypassing the store
// lookup RunNow does (the caller already has the job in hand from its own
// event-matching pass - e.g. seshat-backend listing active
// TriggerTypeEvent jobs and evaluating each one's EventFilter against the
// firing event, entirely outside this package - see Trigger.EventType's
// doc comment). contextText is appended to job.Task as-is (typically the
// triggering event's fields formatted as readable text); pass "" for none.
//
// Mirrors RunNow's body (create run, resolve runner, merge agent config,
// execute, update run+job state via applyRunOutcome) but skips the
// by-ID reload afterward - job is already the caller's own in-memory
// value, and reloading here would only be useful for concurrent-update
// safety, which RunNow needs (many tick-driven executions racing a
// concurrent API update) but a single on-demand event dispatch does not.
func (s *JobScheduler) RunEvent(ctx context.Context, job *Job, contextText string) (*JobRun, error) {
	if job == nil {
		return nil, fmt.Errorf("job is nil")
	}

	run := &JobRun{
		ID:        uuid.New().String(),
		JobID:     job.ID,
		StartedAt: time.Now(),
		Status:    RunStatusRunning,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	runner, baseAgent, resolveErr := s.resolveRunner(ctx, job)
	if resolveErr != nil {
		endedAt := time.Now()
		run.EndedAt = &endedAt
		run.Status = RunStatusError
		run.Error = fmt.Sprintf("resolve runner: %v", resolveErr)
		if err := s.store.UpdateRun(ctx, run); err != nil {
			s.logger.Printf("[automation] update run error for job %s: %v", job.ID, err)
		}
		return run, nil
	}

	effectiveAgent := mergeAgentConfig(baseAgent, job.Agent)
	var buf strings.Builder
	ec := ExecuteConfig{
		StreamFn: func(delta string) { buf.WriteString(delta) },
	}
	if effectiveAgent.SystemPrompt != "" {
		ec.SystemPrompt = effectiveAgent.SystemPrompt
	}
	if effectiveAgent.Model != "" {
		ec.ModelOverride = effectiveAgent.Model
	}

	execCtx := ctx
	if job.MaxDuration > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, job.MaxDuration)
		defer cancel()
	}

	wf := &jobWorkflow{job: job, eventContext: contextText, registry: runner.cfg.NodeRegistry, secrets: runner.cfg.Secrets}
	execErr := runner.Execute(execCtx, wf, ec)

	endedAt := time.Now()
	run.EndedAt = &endedAt
	run.Output = buf.String()
	if execErr != nil {
		run.Status = RunStatusError
		run.Error = formatExecError(execErr, job.MaxDuration)
		s.logger.Printf("[automation] event job %q failed: %v", job.Name, execErr)
	} else {
		run.Status = RunStatusSuccess
		s.logger.Printf("[automation] event job %q completed in %s", job.Name, endedAt.Sub(run.StartedAt))
	}

	if err := s.store.UpdateRun(ctx, run); err != nil {
		s.logger.Printf("[automation] update run error for job %s: %v", job.ID, err)
	}

	applyRunOutcome(job, run, endedAt)
	if err := s.store.UpdateJob(ctx, job); err != nil {
		s.logger.Printf("[automation] update job error for job %s: %v", job.ID, err)
	}

	return run, nil
}

// GetJob returns a single job by ID.
func (s *JobScheduler) GetJob(ctx context.Context, id string) (*Job, error) {
	return s.store.GetJob(ctx, id)
}

// ListJobs returns jobs for ownerID; "" returns all (admin/scheduler use).
func (s *JobScheduler) ListJobs(ctx context.Context, ownerID string) ([]*Job, error) {
	return s.store.ListJobs(ctx, ownerID)
}

// ListRuns returns the most recent runs for a job (newest first).
func (s *JobScheduler) ListRuns(ctx context.Context, jobID string, limit int) ([]*JobRun, error) {
	return s.store.ListRuns(ctx, jobID, limit)
}

// GetRun returns a single run by ID.
func (s *JobScheduler) GetRun(ctx context.Context, id string) (*JobRun, error) {
	return s.store.GetRun(ctx, id)
}

// ─── jobWorkflow adapts Job to the Workflow interface ─────────────────────────

type jobWorkflow struct {
	job *Job
	// eventContext, when non-empty, is appended below job.Task - set by
	// RunEvent to give the agent the triggering event's details; empty for
	// every other execution path (RunNow, the time-based ticker). Unused
	// when job.Graph is set — a graph's own trigger-context wiring is left
	// to whichever node needs it (e.g. an "agent" node's prompt), not
	// injected implicitly the way the flat Task path does.
	eventContext string
	// registry/secrets are the resolved Runner's RunnerConfig.NodeRegistry/
	// Secrets, threaded through at jobWorkflow construction time (see
	// execute/RunNow/RunEvent) — only used when job.Graph is set.
	registry *dataflow.Registry
	secrets  dataflow.SecretResolver
}

func (w *jobWorkflow) Name() string         { return w.job.ID }
func (w *jobWorkflow) Description() string  { return w.job.Description }
func (w *jobWorkflow) SystemPrompt() string { return w.job.Agent.SystemPrompt }

func (w *jobWorkflow) Run(ctx context.Context, session *sdk.Session) error {
	return w.run(ctx, session)
}

// run does the actual work against the narrower messageSubmitter interface
// rather than the concrete *sdk.Session, so the Graph-vs-Task branch is
// unit-testable with a fake session (see dataflow_adapter_test.go) — Run
// itself stays a one-line forward, since jobWorkflow must still satisfy the
// Workflow interface's *sdk.Session signature for Runner.Execute to call it.
func (w *jobWorkflow) run(ctx context.Context, session messageSubmitter) error {
	if w.job.Graph != nil {
		return runGraph(ctx, w.job.Graph, w.registry, w.secrets, session)
	}
	task := w.job.Task
	if w.eventContext != "" {
		task = task + "\n\n" + w.eventContext
	}
	_, err := session.SubmitMessage(ctx, task)
	return err
}
