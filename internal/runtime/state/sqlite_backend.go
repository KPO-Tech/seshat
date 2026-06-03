package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// SQLiteBackend persists session state in a shared application database.
type SQLiteBackend struct {
	db *dbpkg.DB
}

// Close releases the owned database handle.
func (b *SQLiteBackend) Close() error {
	if b == nil || b.db == nil {
		return nil
	}
	return b.db.Close()
}

// NewSQLiteBackend creates a session-state backend on top of a shared DB
// handle. The DB schema must already be initialized, which db.Open handles when
// AutoMigrate is enabled.
func NewSQLiteBackend(database *dbpkg.DB) (*SQLiteBackend, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if database.Driver() != dbpkg.DriverSQLite {
		return nil, fmt.Errorf("sqlite backend requires sqlite database, got %q", database.Driver())
	}
	return &SQLiteBackend{db: database}, nil
}

// OpenSQLiteBackend opens a SQLite database with the shared DB module and wraps
// it as a session-state backend.
func OpenSQLiteBackend(path string) (*SQLiteBackend, error) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(path))
	if err != nil {
		return nil, err
	}
	return NewSQLiteBackend(database)
}

func (b *SQLiteBackend) SaveSession(sessionID types.SessionID, metadata *types.SessionMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	createdAt := metadata.CreatedAt.UTC().Unix()
	updatedAt := metadata.UpdatedAt.UTC().Unix()
	_, err = b.db.SQL().Exec(
		`INSERT INTO session_metadata (session_id, status, created_at_unix, updated_at_unix, metadata_json)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
			status = excluded.status,
			created_at_unix = excluded.created_at_unix,
			updated_at_unix = excluded.updated_at_unix,
			metadata_json = excluded.metadata_json`,
		sessionID.String(),
		string(metadata.Status),
		createdAt,
		updatedAt,
		string(data),
	)
	if err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

func (b *SQLiteBackend) LoadSession(sessionID types.SessionID) (*types.SessionMetadata, error) {
	var data string
	err := b.db.SQL().QueryRow(
		`SELECT metadata_json FROM session_metadata WHERE session_id = ?`,
		sessionID.String(),
	).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("failed to read metadata: session %s not found", sessionID)
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata types.SessionMetadata
	if err := json.Unmarshal([]byte(data), &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &metadata, nil
}

func (b *SQLiteBackend) DeleteSession(sessionID types.SessionID) error {
	tx, err := b.db.SQL().BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	statements := []string{
		`DELETE FROM session_transcript_entries WHERE session_id = ?`,
		`DELETE FROM session_checkpoints WHERE session_id = ?`,
		`DELETE FROM session_metadata WHERE session_id = ?`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement, sessionID.String()); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessionID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete session %s: %w", sessionID, err)
	}
	return nil
}

func (b *SQLiteBackend) ListSessions() ([]types.SessionID, error) {
	rows, err := b.db.SQL().Query(
		`SELECT session_id FROM session_metadata ORDER BY updated_at_unix DESC, session_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]types.SessionID, 0)
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, fmt.Errorf("failed to scan session id: %w", err)
		}
		sessions = append(sessions, types.SessionID(sessionID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sessions: %w", err)
	}
	return sessions, nil
}

func (b *SQLiteBackend) AppendTranscriptEntries(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := b.db.SQL().BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to begin transcript append transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	startIndex, err := currentTranscriptLength(tx, sessionID)
	if err != nil {
		return err
	}
	if err := insertTranscriptEntries(tx, sessionID, startIndex, entries); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transcript append: %w", err)
	}
	return nil
}

func (b *SQLiteBackend) ReplaceTranscript(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	tx, err := b.db.SQL().BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to begin transcript replace transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`DELETE FROM session_transcript_entries WHERE session_id = ?`, sessionID.String()); err != nil {
		return fmt.Errorf("failed to clear transcript: %w", err)
	}
	if err := insertTranscriptEntries(tx, sessionID, 0, entries); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transcript replace: %w", err)
	}
	return nil
}

// SearchTranscriptsByContent returns IDs of sessions whose stored transcript
// JSON contains needle. Uses a SQL LIKE to avoid loading full transcripts.
func (b *SQLiteBackend) SearchTranscriptsByContent(needle string, limit int) ([]types.SessionID, error) {
	pattern := "%" + needle + "%"
	var query string
	var args []any
	if limit > 0 {
		query = `SELECT DISTINCT session_id FROM session_transcript_entries WHERE entry_json LIKE ? LIMIT ?`
		args = []any{pattern, limit}
	} else {
		query = `SELECT DISTINCT session_id FROM session_transcript_entries WHERE entry_json LIKE ?`
		args = []any{pattern}
	}
	rows, err := b.db.SQL().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search transcripts by content: %w", err)
	}
	defer rows.Close()
	var ids []types.SessionID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan session id: %w", err)
		}
		ids = append(ids, types.SessionID(id))
	}
	return ids, rows.Err()
}

func (b *SQLiteBackend) LoadTranscript(sessionID types.SessionID) ([]types.TranscriptEntry, error) {
	rows, err := b.db.SQL().Query(
		`SELECT entry_json FROM session_transcript_entries WHERE session_id = ? ORDER BY entry_index ASC`,
		sessionID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load transcript: %w", err)
	}
	defer rows.Close()

	entries := make([]types.TranscriptEntry, 0)
	lineNumber := 0
	for rows.Next() {
		lineNumber++
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("failed to scan transcript entry: %w", err)
		}
		var entry types.TranscriptEntry
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			return nil, fmt.Errorf("%w for session %s at row %d: %v", ErrMalformedTranscriptEntry, sessionID, lineNumber, err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transcript: %w", err)
	}
	return entries, nil
}

func (b *SQLiteBackend) SaveCheckpoint(sessionID types.SessionID, checkpoint *Checkpoint) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	_, err = b.db.SQL().Exec(
		`INSERT INTO session_checkpoints (session_id, updated_at_unix, checkpoint_json)
		 VALUES (?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
			updated_at_unix = excluded.updated_at_unix,
			checkpoint_json = excluded.checkpoint_json`,
		sessionID.String(),
		time.Now().UTC().Unix(),
		string(data),
	)
	if err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}
	return nil
}

func (b *SQLiteBackend) LoadCheckpoint(sessionID types.SessionID) (*Checkpoint, error) {
	var data string
	err := b.db.SQL().QueryRow(
		`SELECT checkpoint_json FROM session_checkpoints WHERE session_id = ?`,
		sessionID.String(),
	).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal([]byte(data), &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}
	return &checkpoint, nil
}

func currentTranscriptLength(tx *sql.Tx, sessionID types.SessionID) (int, error) {
	var count int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM session_transcript_entries WHERE session_id = ?`,
		sessionID.String(),
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to query transcript length: %w", err)
	}
	return count, nil
}

func insertTranscriptEntries(tx *sql.Tx, sessionID types.SessionID, startIndex int, entries []types.TranscriptEntry) error {
	for idx, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal transcript entry: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO session_transcript_entries (session_id, entry_index, entry_json) VALUES (?, ?, ?)`,
			sessionID.String(),
			startIndex+idx,
			string(data),
		); err != nil {
			return fmt.Errorf("failed to write transcript entry: %w", err)
		}
	}
	return nil
}
