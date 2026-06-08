package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SessionFile represents one file operation recorded against a session.
// ToolUseID is the tool_use_id from the transcript — use it to look up the
// full ToolResultContent.Metadata (structured_patch, git_diff, content, …)
// in session_transcript_entries without scanning the whole transcript.
type SessionFile struct {
	ID            int64
	SessionID     string
	ToolUseID     string // foreign key into session_transcript_entries JSON
	FilePath      string
	Operation     string // "create" | "update" | "edit" | "patch"
	TimestampUnix int64
	LinesAdded    int
	LinesRemoved  int
}

// UpsertSessionFile records a file operation for a session.
// When tool_use_id is non-empty, INSERT OR IGNORE silently skips duplicates
// (the unique index on tool_use_id WHERE tool_use_id != ” enforces this).
func (db *DB) UpsertSessionFile(ctx context.Context, sf SessionFile) error {
	if sf.TimestampUnix == 0 {
		sf.TimestampUnix = time.Now().Unix()
	}
	_, err := db.SQL().ExecContext(ctx, `
		INSERT OR IGNORE INTO session_files
			(session_id, tool_use_id, file_path, operation, timestamp_unix, lines_added, lines_removed)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sf.SessionID, sf.ToolUseID, sf.FilePath, sf.Operation,
		sf.TimestampUnix, sf.LinesAdded, sf.LinesRemoved,
	)
	if err != nil {
		return fmt.Errorf("upsert session file: %w", err)
	}
	return nil
}

// GetSessionFiles returns all file operations recorded for a session,
// ordered by timestamp ascending.
func (db *DB) GetSessionFiles(ctx context.Context, sessionID string) ([]SessionFile, error) {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT id, session_id, tool_use_id, file_path, operation, timestamp_unix, lines_added, lines_removed
		FROM session_files
		WHERE session_id = ?
		ORDER BY timestamp_unix ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get session files: %w", err)
	}
	defer rows.Close()

	var files []SessionFile
	for rows.Next() {
		var f SessionFile
		if err := rows.Scan(
			&f.ID, &f.SessionID, &f.ToolUseID, &f.FilePath, &f.Operation,
			&f.TimestampUnix, &f.LinesAdded, &f.LinesRemoved,
		); err != nil {
			return nil, fmt.Errorf("scan session file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetFileSessions returns all sessions that touched a given file path,
// ordered by most recent first.
func (db *DB) GetFileSessions(ctx context.Context, filePath string) ([]SessionFile, error) {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT id, session_id, tool_use_id, file_path, operation, timestamp_unix, lines_added, lines_removed
		FROM session_files
		WHERE file_path = ?
		ORDER BY timestamp_unix DESC, id DESC`,
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("get file sessions: %w", err)
	}
	defer rows.Close()

	var files []SessionFile
	for rows.Next() {
		var f SessionFile
		if err := rows.Scan(
			&f.ID, &f.SessionID, &f.ToolUseID, &f.FilePath, &f.Operation,
			&f.TimestampUnix, &f.LinesAdded, &f.LinesRemoved,
		); err != nil {
			return nil, fmt.Errorf("scan file session: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// HasSessionFileEntry returns true if session_files already has at least one
// row for the given session. Used to detect whether backfill is needed.
func (db *DB) HasSessionFileEntry(ctx context.Context, sessionID string) (bool, error) {
	var exists bool
	err := db.SQL().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM session_files WHERE session_id = ? LIMIT 1)`,
		sessionID,
	).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return exists, nil
}
