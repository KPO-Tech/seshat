package db

import "context"

func automationDaemonMigrations() []schemaMigration {
	return []schemaMigration{
		{
			ID:    "20260629_001_api_keys",
			Scope: migrationScopeAutomationDaemon,
			Run:   migrateSQLiteAutomationAPIKeys,
		},
		{
			ID:    "20260629_002_api_keys_expiry",
			Scope: migrationScopeAutomationDaemon,
			Run:   migrateSQLiteAutomationAPIKeysExpiry,
		},
	}
}

func migrateSQLiteAutomationAPIKeysExpiry(ctx context.Context, db *DB) error {
	// expires_at = 0 means no expiry. Any positive value is a Unix timestamp.
	return db.gormDB.WithContext(ctx).Exec(
		`ALTER TABLE automation_api_keys ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0`,
	).Error
}

func migrateSQLiteAutomationAPIKeys(ctx context.Context, db *DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS automation_api_keys (
			id          TEXT    PRIMARY KEY,
			key_hash    TEXT    NOT NULL UNIQUE,
			key_prefix  TEXT    NOT NULL DEFAULT '',
			label       TEXT    NOT NULL DEFAULT '',
			owner_id    TEXT    NOT NULL DEFAULT '',
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_api_keys_owner
			ON automation_api_keys(owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_api_keys_hash
			ON automation_api_keys(key_hash)`,
	}
	for _, stmt := range statements {
		if err := db.gormDB.WithContext(ctx).Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
