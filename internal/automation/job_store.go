package automation

import (
	"context"
	"fmt"
	"time"

	"github.com/EngineerProjects/seshat/internal/db"
)

// DBJobStore implements JobStore backed by a seshat DB instance.
type DBJobStore struct {
	db *db.DB
}

// NewDBJobStore wraps a DB instance as a JobStore.
func NewDBJobStore(database *db.DB) *DBJobStore {
	return &DBJobStore{db: database}
}

// ─── Job CRUD ─────────────────────────────────────────────────────────────────

func (s *DBJobStore) CreateJob(ctx context.Context, job *Job) error {
	return s.db.CreateAutomationJob(ctx, jobToRow(job))
}

func (s *DBJobStore) GetJob(ctx context.Context, id string) (*Job, error) {
	row, err := s.db.GetAutomationJob(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return rowToJob(row), nil
}

func (s *DBJobStore) ListJobs(ctx context.Context, ownerID string) ([]*Job, error) {
	rows, err := s.db.ListAutomationJobs(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	jobs := make([]*Job, len(rows))
	for i, r := range rows {
		jobs[i] = rowToJob(r)
	}
	return jobs, nil
}

func (s *DBJobStore) UpdateJob(ctx context.Context, job *Job) error {
	return s.db.UpdateAutomationJob(ctx, jobToRow(job))
}

func (s *DBJobStore) DeleteJob(ctx context.Context, id, ownerID string) error {
	return s.db.DeleteAutomationJob(ctx, id, ownerID)
}

// ─── Run CRUD ─────────────────────────────────────────────────────────────────

func (s *DBJobStore) CreateRun(ctx context.Context, run *JobRun) error {
	return s.db.CreateAutomationRun(ctx, runToRow(run))
}

func (s *DBJobStore) UpdateRun(ctx context.Context, run *JobRun) error {
	return s.db.UpdateAutomationRun(ctx, runToRow(run))
}

func (s *DBJobStore) ListRuns(ctx context.Context, jobID string, limit int) ([]*JobRun, error) {
	rows, err := s.db.ListAutomationRuns(ctx, jobID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	runs := make([]*JobRun, len(rows))
	for i, r := range rows {
		runs[i] = rowToRun(r)
	}
	return runs, nil
}

func (s *DBJobStore) GetRun(ctx context.Context, id string) (*JobRun, error) {
	row, err := s.db.GetAutomationRun(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return rowToRun(row), nil
}

// ─── Conversion helpers ───────────────────────────────────────────────────────

func jobToRow(job *Job) db.AutomationJobRow {
	var runAt *int64
	if job.Trigger.RunAt != nil {
		u := job.Trigger.RunAt.UTC().Unix()
		runAt = &u
	}
	return db.AutomationJobRow{
		ID:              job.ID,
		OwnerID:         job.OwnerID,
		Name:            job.Name,
		Description:     job.Description,
		TriggerType:     string(job.Trigger.Type),
		TriggerCron:     job.Trigger.Cron,
		TriggerInterval: int64(job.Trigger.Interval),
		TriggerRunAt:    runAt,
		AgentSlug:       job.Agent.Slug,
		AgentBaseType:   job.Agent.BaseType,
		AgentToolsJSON:  db.AutomationStringsToJSON(job.Agent.Tools),
		AgentSkillsJSON: db.AutomationStringsToJSON(job.Agent.Skills),
		AgentModel:      job.Agent.Model,
		AgentMaxTurns:   job.Agent.MaxTurns,
		AgentSysPrompt:  job.Agent.SystemPrompt,
		Task:            job.Task,
		Status:          string(job.Status),
		LastRunAt:       db.TimePtrToUnix(job.LastRunAt),
		NextRunAt:       db.TimePtrToUnix(job.NextRunAt),
		LastRunStatus:   job.LastRunStatus,
		CreatedAt:       job.CreatedAt.UTC().Unix(),
		UpdatedAt:       job.UpdatedAt.UTC().Unix(),
	}
}

func rowToJob(r *db.AutomationJobRow) *Job {
	var runAt *time.Time
	if r.TriggerRunAt != nil {
		t := time.Unix(*r.TriggerRunAt, 0).UTC()
		runAt = &t
	}
	return &Job{
		ID:          r.ID,
		OwnerID:     r.OwnerID,
		Name:        r.Name,
		Description: r.Description,
		Trigger: Trigger{
			Type:     TriggerType(r.TriggerType),
			Cron:     r.TriggerCron,
			Interval: time.Duration(r.TriggerInterval),
			RunAt:    runAt,
		},
		Agent: AgentConfig{
			Slug:         r.AgentSlug,
			BaseType:     r.AgentBaseType,
			Tools:        db.AutomationJobRowToStrings(r.AgentToolsJSON),
			Skills:       db.AutomationJobRowToStrings(r.AgentSkillsJSON),
			Model:        r.AgentModel,
			MaxTurns:     r.AgentMaxTurns,
			SystemPrompt: r.AgentSysPrompt,
		},
		Task:          r.Task,
		Status:        JobStatus(r.Status),
		LastRunAt:     db.UnixPtrToTime(r.LastRunAt),
		NextRunAt:     db.UnixPtrToTime(r.NextRunAt),
		LastRunStatus: r.LastRunStatus,
		CreatedAt:     time.Unix(r.CreatedAt, 0).UTC(),
		UpdatedAt:     time.Unix(r.UpdatedAt, 0).UTC(),
	}
}

func runToRow(run *JobRun) db.AutomationRunRow {
	return db.AutomationRunRow{
		ID:        run.ID,
		JobID:     run.JobID,
		StartedAt: run.StartedAt.UTC().Unix(),
		EndedAt:   db.TimePtrToUnix(run.EndedAt),
		Status:    string(run.Status),
		Output:    run.Output,
		Error:     run.Error,
		CreatedAt: run.StartedAt.UTC().Unix(),
	}
}

func rowToRun(r *db.AutomationRunRow) *JobRun {
	return &JobRun{
		ID:        r.ID,
		JobID:     r.JobID,
		StartedAt: time.Unix(r.StartedAt, 0).UTC(),
		EndedAt:   db.UnixPtrToTime(r.EndedAt),
		Status:    RunStatus(r.Status),
		Output:    r.Output,
		Error:     r.Error,
	}
}
