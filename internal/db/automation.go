package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ─── Row types ────────────────────────────────────────────────────────────────

// AutomationJobRow is the DB representation of a persisted automation job.
type AutomationJobRow struct {
	ID              string
	OwnerID         string // user ID from the calling app; "" = system/unowned
	Name            string
	Description     string
	TriggerType     string
	TriggerCron     string
	TriggerInterval int64 // nanoseconds
	TriggerRunAt    *int64
	AgentSlug       string // optional: slug of a named agent in seshat-ai
	AgentBaseType   string
	AgentToolsJSON  string
	AgentSkillsJSON string
	AgentModel      string
	AgentMaxTurns   int
	AgentSysPrompt  string
	Task            string
	Status          string
	LastRunAt       *int64
	NextRunAt       *int64
	LastRunStatus   string
	CreatedAt       int64
	UpdatedAt       int64
}

// AutomationRunRow is the DB representation of a single job execution.
type AutomationRunRow struct {
	ID        string
	JobID     string
	StartedAt int64
	EndedAt   *int64
	Status    string
	Output    string
	Error     string
	CreatedAt int64
}

// ─── Job CRUD ─────────────────────────────────────────────────────────────────

func (db *DB) CreateAutomationJob(ctx context.Context, row AutomationJobRow) error {
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO automation_jobs (
			id, owner_id, name, description,
			trigger_type, trigger_cron, trigger_interval_ns, trigger_run_at,
			agent_slug, agent_base_type, agent_tools_json, agent_skills_json, agent_model, agent_max_turns, agent_system_prompt,
			task, status, last_run_at, next_run_at, last_run_status,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.ID, row.OwnerID, row.Name, row.Description,
		row.TriggerType, row.TriggerCron, row.TriggerInterval, row.TriggerRunAt,
		row.AgentSlug, row.AgentBaseType, row.AgentToolsJSON, row.AgentSkillsJSON, row.AgentModel, row.AgentMaxTurns, row.AgentSysPrompt,
		row.Task, row.Status, row.LastRunAt, row.NextRunAt, row.LastRunStatus,
		row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create automation job: %w", err)
	}
	return nil
}

func (db *DB) GetAutomationJob(ctx context.Context, id string) (*AutomationJobRow, error) {
	row := db.SQL().QueryRowContext(ctx, `
		SELECT id, owner_id, name, description,
		       trigger_type, trigger_cron, trigger_interval_ns, trigger_run_at,
		       agent_slug, agent_base_type, agent_tools_json, agent_skills_json, agent_model, agent_max_turns, agent_system_prompt,
		       task, status, last_run_at, next_run_at, last_run_status,
		       created_at, updated_at
		FROM automation_jobs WHERE id = ?`, id)
	r, err := scanAutomationJobRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ListAutomationJobs returns all jobs, optionally filtered by owner.
// Pass ownerID = "" to list all jobs (system/admin use).
func (db *DB) ListAutomationJobs(ctx context.Context, ownerID string) ([]*AutomationJobRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if ownerID == "" {
		rows, err = db.SQL().QueryContext(ctx, `
			SELECT id, owner_id, name, description,
			       trigger_type, trigger_cron, trigger_interval_ns, trigger_run_at,
			       agent_slug, agent_base_type, agent_tools_json, agent_skills_json, agent_model, agent_max_turns, agent_system_prompt,
			       task, status, last_run_at, next_run_at, last_run_status,
			       created_at, updated_at
			FROM automation_jobs
			ORDER BY created_at DESC`)
	} else {
		rows, err = db.SQL().QueryContext(ctx, `
			SELECT id, owner_id, name, description,
			       trigger_type, trigger_cron, trigger_interval_ns, trigger_run_at,
			       agent_slug, agent_base_type, agent_tools_json, agent_skills_json, agent_model, agent_max_turns, agent_system_prompt,
			       task, status, last_run_at, next_run_at, last_run_status,
			       created_at, updated_at
			FROM automation_jobs
			WHERE owner_id = ?
			ORDER BY created_at DESC`, ownerID)
	}
	if err != nil {
		return nil, fmt.Errorf("list automation jobs: %w", err)
	}
	defer rows.Close()

	var result []*AutomationJobRow
	for rows.Next() {
		r, err := scanAutomationJobRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) UpdateAutomationJob(ctx context.Context, row AutomationJobRow) error {
	res, err := db.SQL().ExecContext(ctx, `
		UPDATE automation_jobs SET
			name = ?, description = ?,
			trigger_type = ?, trigger_cron = ?, trigger_interval_ns = ?, trigger_run_at = ?,
			agent_slug = ?, agent_base_type = ?, agent_tools_json = ?, agent_skills_json = ?,
			agent_model = ?, agent_max_turns = ?, agent_system_prompt = ?,
			task = ?, status = ?, last_run_at = ?, next_run_at = ?, last_run_status = ?,
			updated_at = ?
		WHERE id = ? AND (owner_id = ? OR owner_id = '')`,
		row.Name, row.Description,
		row.TriggerType, row.TriggerCron, row.TriggerInterval, row.TriggerRunAt,
		row.AgentSlug, row.AgentBaseType, row.AgentToolsJSON, row.AgentSkillsJSON,
		row.AgentModel, row.AgentMaxTurns, row.AgentSysPrompt,
		row.Task, row.Status, row.LastRunAt, row.NextRunAt, row.LastRunStatus,
		row.UpdatedAt, row.ID, row.OwnerID,
	)
	if err != nil {
		return fmt.Errorf("update automation job: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) DeleteAutomationJob(ctx context.Context, id, ownerID string) error {
	var err error
	if ownerID == "" {
		_, err = db.SQL().ExecContext(ctx, `DELETE FROM automation_jobs WHERE id = ?`, id)
	} else {
		_, err = db.SQL().ExecContext(ctx, `DELETE FROM automation_jobs WHERE id = ? AND owner_id = ?`, id, ownerID)
	}
	if err != nil {
		return fmt.Errorf("delete automation job: %w", err)
	}
	return nil
}

// ─── Run CRUD ─────────────────────────────────────────────────────────────────

func (db *DB) CreateAutomationRun(ctx context.Context, row AutomationRunRow) error {
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO automation_runs (id, job_id, started_at, ended_at, status, output, error, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		row.ID, row.JobID, row.StartedAt, row.EndedAt, row.Status, row.Output, row.Error, row.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create automation run: %w", err)
	}
	return nil
}

func (db *DB) UpdateAutomationRun(ctx context.Context, row AutomationRunRow) error {
	res, err := db.SQL().ExecContext(ctx, `
		UPDATE automation_runs SET ended_at = ?, status = ?, output = ?, error = ?
		WHERE id = ?`,
		row.EndedAt, row.Status, row.Output, row.Error, row.ID,
	)
	if err != nil {
		return fmt.Errorf("update automation run: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListAutomationRuns(ctx context.Context, jobID string, limit int) ([]*AutomationRunRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT id, job_id, started_at, ended_at, status, output, error, created_at
		FROM automation_runs
		WHERE job_id = ?
		ORDER BY started_at DESC
		LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, fmt.Errorf("list automation runs: %w", err)
	}
	defer rows.Close()

	var result []*AutomationRunRow
	for rows.Next() {
		r, err := scanAutomationRunRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) GetAutomationRun(ctx context.Context, id string) (*AutomationRunRow, error) {
	row := db.SQL().QueryRowContext(ctx, `
		SELECT id, job_id, started_at, ended_at, status, output, error, created_at
		FROM automation_runs WHERE id = ?`, id)
	r, err := scanAutomationRunRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// ─── Scanners ─────────────────────────────────────────────────────────────────

func scanAutomationJobRow(s scannable) (*AutomationJobRow, error) {
	var r AutomationJobRow
	err := s.Scan(
		&r.ID, &r.OwnerID, &r.Name, &r.Description,
		&r.TriggerType, &r.TriggerCron, &r.TriggerInterval, &r.TriggerRunAt,
		&r.AgentSlug, &r.AgentBaseType, &r.AgentToolsJSON, &r.AgentSkillsJSON, &r.AgentModel, &r.AgentMaxTurns, &r.AgentSysPrompt,
		&r.Task, &r.Status, &r.LastRunAt, &r.NextRunAt, &r.LastRunStatus,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanAutomationRunRow(s scannable) (*AutomationRunRow, error) {
	var r AutomationRunRow
	err := s.Scan(&r.ID, &r.JobID, &r.StartedAt, &r.EndedAt, &r.Status, &r.Output, &r.Error, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ─── Helpers: row ↔ domain conversion ────────────────────────────────────────

func AutomationJobRowToStrings(jsonStr string) []string {
	var out []string
	if jsonStr == "" || jsonStr == "[]" {
		return out
	}
	_ = json.Unmarshal([]byte(jsonStr), &out)
	return out
}

func AutomationStringsToJSON(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(ss)
	return string(data)
}

func UnixPtrToTime(u *int64) *time.Time {
	if u == nil {
		return nil
	}
	t := time.Unix(*u, 0).UTC()
	return &t
}

func TimePtrToUnix(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	u := t.UTC().Unix()
	return &u
}
