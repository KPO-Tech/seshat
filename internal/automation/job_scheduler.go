package automation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/seshat/pkg/sdk"
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
		if job.Status != JobStatusActive || job.NextRunAt != nil {
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

	wf := &jobWorkflow{job: job}
	execErr := runner.Execute(ctx, wf, ec)

	endedAt := time.Now()
	run.EndedAt = &endedAt
	run.Output = buf.String()
	if execErr != nil {
		run.Status = RunStatusError
		run.Error = execErr.Error()
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
	current.LastRunAt = &endedAt
	current.LastRunStatus = string(run.Status)

	sched, err := job.Trigger.ToSchedule()
	if err == nil {
		next := sched.Next(endedAt)
		if next.IsZero() {
			current.Status = JobStatusInactive // once-trigger done
		} else {
			current.NextRunAt = &next
		}
	}
	current.UpdatedAt = endedAt
	_ = s.store.UpdateJob(ctx, current)
}

// ─── Management API ───────────────────────────────────────────────────────────

// AddJob persists a new job and computes its initial NextRunAt.
func (s *JobScheduler) AddJob(ctx context.Context, job *Job) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
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
	job.Status = JobStatusActive
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now
	return s.store.CreateJob(ctx, job)
}

// UpdateJob re-persists a job and recomputes its next run time.
func (s *JobScheduler) UpdateJob(ctx context.Context, job *Job) error {
	sched, err := job.Trigger.ToSchedule()
	if err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}
	next := sched.Next(time.Now())
	if !next.IsZero() {
		job.NextRunAt = &next
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
	sched, err := job.Trigger.ToSchedule()
	if err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}
	next := sched.Next(time.Now())
	if !next.IsZero() {
		job.NextRunAt = &next
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

		wf := &jobWorkflow{job: job}
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
}

func (w *jobWorkflow) Name() string         { return w.job.ID }
func (w *jobWorkflow) Description() string  { return w.job.Description }
func (w *jobWorkflow) SystemPrompt() string { return w.job.Agent.SystemPrompt }

func (w *jobWorkflow) Run(ctx context.Context, session *sdk.Session) error {
	_, err := session.SubmitMessage(ctx, w.job.Task)
	return err
}
