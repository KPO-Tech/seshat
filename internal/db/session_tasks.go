package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type SessionTask struct {
	ID            int64
	SessionID     string
	TaskID        string
	Position      int
	Subject       string
	Description   string
	Status        string
	ActiveForm    string
	Owner         string
	Blocks        []string
	BlockedBy     []string
	Metadata      map[string]any
	CreatedAtUnix int64
	UpdatedAtUnix int64
}

func (db *DB) NextSessionTaskPosition(ctx context.Context, sessionID string) (int, error) {
	var next int
	err := db.SQL().QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), 0) + 1
		FROM session_tasks
		WHERE session_id = ?`, sessionID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next session task position: %w", err)
	}
	return next, nil
}

func (db *DB) CreateSessionTask(ctx context.Context, task SessionTask) error {
	blocksJSON, err := marshalTaskJSON(task.Blocks, "[]")
	if err != nil {
		return err
	}
	blockedByJSON, err := marshalTaskJSON(task.BlockedBy, "[]")
	if err != nil {
		return err
	}
	metadataJSON, err := marshalTaskJSON(task.Metadata, "{}")
	if err != nil {
		return err
	}
	_, err = db.SQL().ExecContext(ctx, `
		INSERT INTO session_tasks (
			session_id, task_id, position, subject, description, status,
			active_form, owner, blocks_json, blocked_by_json, metadata_json,
			created_at_unix, updated_at_unix
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.SessionID, task.TaskID, task.Position, task.Subject, task.Description, task.Status,
		task.ActiveForm, task.Owner, blocksJSON, blockedByJSON, metadataJSON,
		task.CreatedAtUnix, task.UpdatedAtUnix,
	)
	if err != nil {
		return fmt.Errorf("create session task: %w", err)
	}
	return nil
}

func (db *DB) GetSessionTask(ctx context.Context, sessionID, taskID string) (SessionTask, error) {
	row := db.SQL().QueryRowContext(ctx, `
		SELECT id, session_id, task_id, position, subject, description, status,
		       active_form, owner, blocks_json, blocked_by_json, metadata_json,
		       created_at_unix, updated_at_unix
		FROM session_tasks
		WHERE session_id = ? AND task_id = ?`, sessionID, taskID,
	)
	return scanSessionTask(row)
}

func (db *DB) ListSessionTasks(ctx context.Context, sessionID string) ([]SessionTask, error) {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT id, session_id, task_id, position, subject, description, status,
		       active_form, owner, blocks_json, blocked_by_json, metadata_json,
		       created_at_unix, updated_at_unix
		FROM session_tasks
		WHERE session_id = ?
		ORDER BY position ASC, created_at_unix ASC, id ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list session tasks: %w", err)
	}
	defer rows.Close()

	var tasks []SessionTask
	for rows.Next() {
		task, err := scanSessionTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (db *DB) UpdateSessionTask(ctx context.Context, task SessionTask) error {
	blocksJSON, err := marshalTaskJSON(task.Blocks, "[]")
	if err != nil {
		return err
	}
	blockedByJSON, err := marshalTaskJSON(task.BlockedBy, "[]")
	if err != nil {
		return err
	}
	metadataJSON, err := marshalTaskJSON(task.Metadata, "{}")
	if err != nil {
		return err
	}
	result, err := db.SQL().ExecContext(ctx, `
		UPDATE session_tasks
		SET position = ?, subject = ?, description = ?, status = ?, active_form = ?, owner = ?,
		    blocks_json = ?, blocked_by_json = ?, metadata_json = ?, updated_at_unix = ?
		WHERE session_id = ? AND task_id = ?`,
		task.Position, task.Subject, task.Description, task.Status, task.ActiveForm, task.Owner,
		blocksJSON, blockedByJSON, metadataJSON, task.UpdatedAtUnix,
		task.SessionID, task.TaskID,
	)
	if err != nil {
		return fmt.Errorf("update session task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (db *DB) DeleteSessionTask(ctx context.Context, sessionID, taskID string) error {
	_, err := db.SQL().ExecContext(ctx, `
		DELETE FROM session_tasks
		WHERE session_id = ? AND task_id = ?`, sessionID, taskID,
	)
	if err != nil {
		return fmt.Errorf("delete session task: %w", err)
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSessionTask(row scannable) (SessionTask, error) {
	var task SessionTask
	var blocksJSON, blockedByJSON, metadataJSON string
	if err := row.Scan(
		&task.ID, &task.SessionID, &task.TaskID, &task.Position, &task.Subject, &task.Description,
		&task.Status, &task.ActiveForm, &task.Owner, &blocksJSON, &blockedByJSON, &metadataJSON,
		&task.CreatedAtUnix, &task.UpdatedAtUnix,
	); err != nil {
		return SessionTask{}, err
	}
	if err := unmarshalTaskStrings(blocksJSON, &task.Blocks); err != nil {
		return SessionTask{}, err
	}
	if err := unmarshalTaskStrings(blockedByJSON, &task.BlockedBy); err != nil {
		return SessionTask{}, err
	}
	if err := unmarshalTaskMap(metadataJSON, &task.Metadata); err != nil {
		return SessionTask{}, err
	}
	return task, nil
}

func marshalTaskJSON(value any, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return fallback, nil
	}
	return string(data), nil
}

func unmarshalTaskStrings(raw string, dest *[]string) error {
	if raw == "" {
		raw = "[]"
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("unmarshal task string list: %w", err)
	}
	if *dest == nil {
		*dest = []string{}
	}
	return nil
}

func unmarshalTaskMap(raw string, dest *map[string]any) error {
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("unmarshal task metadata: %w", err)
	}
	if *dest == nil {
		*dest = map[string]any{}
	}
	return nil
}
