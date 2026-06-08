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

// dbCtx returns a context with a timeout for short DB operations.
// This is a pragmatic guard until the Backend interface accepts ctx directly (see L-A in audit).
func dbCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

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
	// A single DELETE on the parent row is sufficient: FOREIGN KEY ON DELETE CASCADE
	// propagates to session_transcript_entries, session_checkpoints, and session_files.
	// The trg_transcript_fts_delete trigger keeps session_transcript_fts in sync for
	// every cascade-deleted transcript row.
	ctx, cancel := dbCtx(10 * time.Second)
	defer cancel()
	_, err := b.db.SQL().ExecContext(ctx,
		`DELETE FROM session_metadata WHERE session_id = ?`,
		sessionID.String(),
	)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
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
	ctx, cancel := dbCtx(30 * time.Second)
	defer cancel()
	tx, err := b.db.SQL().BeginTx(ctx, nil)
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
	ctx, cancel := dbCtx(30 * time.Second)
	defer cancel()
	tx, err := b.db.SQL().BeginTx(ctx, nil)
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
// JSON contains needle. Uses the session_transcript_fts FTS5 index when available
// (O(log n)); falls back to a LIKE full scan on older databases.
func (b *SQLiteBackend) SearchTranscriptsByContent(needle string, limit int) ([]types.SessionID, error) {
	scan := func(rows *sql.Rows) ([]types.SessionID, error) {
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

	// Try FTS5 first — MATCH is word-token based, fast on large datasets.
	ftsQuery := `SELECT DISTINCT session_id FROM session_transcript_fts WHERE entry_json MATCH ?`
	if limit > 0 {
		ftsQuery += fmt.Sprintf(" LIMIT %d", limit)
	}
	if rows, err := b.db.SQL().Query(ftsQuery, needle); err == nil {
		if ids, err := scan(rows); err == nil {
			return ids, nil
		}
	}

	// Fallback: LIKE scan (O(n)) for databases without the FTS5 table.
	pattern := "%" + needle + "%"
	likeQuery := `SELECT DISTINCT session_id FROM session_transcript_entries WHERE entry_json LIKE ?`
	if limit > 0 {
		likeQuery += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := b.db.SQL().Query(likeQuery, pattern)
	if err != nil {
		return nil, fmt.Errorf("search transcripts by content: %w", err)
	}
	return scan(rows)
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
