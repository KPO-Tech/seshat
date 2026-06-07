package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SessionFile represents one file operation recorded against a session.
type SessionFile struct {
	ID            int64
	SessionID     string
	FilePath      string
	Operation     string // "create" | "update" | "edit" | "patch" | "read"
	TimestampUnix int64
	LinesAdded    int
	LinesRemoved  int
}

// UpsertSessionFile records (or updates) a file operation for a session.
// Multiple operations on the same file within the same second are coalesced
// by incrementing the counters rather than inserting duplicates.
func (db *DB) UpsertSessionFile(ctx context.Context, sf SessionFile) error {
	if sf.TimestampUnix == 0 {
		sf.TimestampUnix = time.Now().Unix()
	}
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO session_files
			(session_id, file_path, operation, timestamp_unix, lines_added, lines_removed)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sf.SessionID, sf.FilePath, sf.Operation,
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
		SELECT id, session_id, file_path, operation, timestamp_unix, lines_added, lines_removed
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
			&f.ID, &f.SessionID, &f.FilePath, &f.Operation,
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
		SELECT id, session_id, file_path, operation, timestamp_unix, lines_added, lines_removed
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
			&f.ID, &f.SessionID, &f.FilePath, &f.Operation,
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
	var n int
	err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM session_files WHERE session_id = ? LIMIT 1`,
		sessionID,
	).Scan(&n)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return n > 0, nil
}
