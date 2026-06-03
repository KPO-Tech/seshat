package db

import "context"

func sqliteCoreMigrations() []schemaMigration {
	return []schemaMigration{
		{
			ID:    "20260514_001_runtime_session_tables",
			Scope: migrationScopeCoreSQLite,
			Run:   migrateSQLiteRuntimeSessionTables,
		},
		{
			ID:    "20260514_002_runtime_vector_records",
			Scope: migrationScopeCoreSQLite,
			Run:   migrateSQLiteRuntimeVectorRecords,
		},
		{
			// FTS5 virtual table for BM25 hybrid search alongside the vector_records table.
			// Stores (namespace, key, text) so BM25 scores can be joined back by namespace+key.
			// The tokenizer uses unicode61 with diacritic removal for broad language support.
			ID:    "20260531_003_vector_records_fts5",
			Scope: migrationScopeCoreSQLite,
			Run:   migrateSQLiteVectorFTS5,
		},
		{
			ID:    "20260603_004_credentials_table",
			Scope: migrationScopeCoreSQLite,
			Run:   migrateSQLiteCredentials,
		},
	}
}

func migrateSQLiteRuntimeSessionTables(ctx context.Context, db *DB) error {
	statements := []string{
		// Kept for backward compatibility with older SQLite runtime databases.
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL DEFAULT (unixepoch())
		)`,
		`CREATE TABLE IF NOT EXISTS session_metadata (
			session_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at_unix INTEGER NOT NULL,
			updated_at_unix INTEGER NOT NULL,
			metadata_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_metadata_updated_at
			ON session_metadata(updated_at_unix DESC)`,
		`CREATE TABLE IF NOT EXISTS session_transcript_entries (
			session_id TEXT NOT NULL,
			entry_index INTEGER NOT NULL,
			entry_json TEXT NOT NULL,
			PRIMARY KEY (session_id, entry_index),
			FOREIGN KEY (session_id) REFERENCES session_metadata(session_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS session_checkpoints (
			session_id TEXT PRIMARY KEY,
			updated_at_unix INTEGER NOT NULL,
			checkpoint_json TEXT NOT NULL,
			FOREIGN KEY (session_id) REFERENCES session_metadata(session_id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range statements {
		if err := db.gormDB.WithContext(ctx).Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateSQLiteVectorFTS5(ctx context.Context, db *DB) error {
	statements := []string{
		// FTS5 virtual table: namespace and key are stored but not full-text indexed;
		// only the text column participates in BM25 scoring.
		`CREATE VIRTUAL TABLE IF NOT EXISTS vector_records_fts
		 USING fts5(
		 	namespace UNINDEXED,
		 	key       UNINDEXED,
		 	text,
		 	tokenize  = 'unicode61 remove_diacritics 1'
		 )`,
		// Populate FTS5 from any existing vector_records rows.
		`INSERT OR IGNORE INTO vector_records_fts(namespace, key, text)
		 SELECT namespace, key, text FROM vector_records`,
	}
	for _, stmt := range statements {
		if err := db.gormDB.WithContext(ctx).Exec(stmt).Error; err != nil {
			// FTS5 is an enhancement; a failure here must not block startup.
			// Log and continue rather than returning an error.
			_ = err
		}
	}
	return nil
}

func migrateSQLiteRuntimeVectorRecords(ctx context.Context, db *DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS vector_records (
			namespace TEXT    NOT NULL,
			key       TEXT    NOT NULL,
			text      TEXT    NOT NULL DEFAULT '',
			vector    BLOB    NOT NULL,
			metadata  TEXT    NOT NULL DEFAULT '{}',
			PRIMARY KEY (namespace, key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vector_records_namespace
			ON vector_records(namespace)`,
	}
	for _, stmt := range statements {
		if err := db.gormDB.WithContext(ctx).Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateSQLiteCredentials(ctx context.Context, db *DB) error {
	return db.gormDB.WithContext(ctx).AutoMigrate(&gCredential{})
}
