package db

import "context"

// Initialize applies only the public engine's core persistence migrations.
// Product/backend schema lives in the private product repository.
func (db *DB) Initialize(ctx context.Context) error {
	if db.driver != DriverSQLite {
		return nil
	}
	return db.runSQLiteCoreMigrations(ctx)
}

// InitializeAutomationDaemon applies daemon-specific migrations on top of the
// core migrations. Call this in the seshat-automation binary after Initialize.
func (db *DB) InitializeAutomationDaemon(ctx context.Context) error {
	if db.driver != DriverSQLite {
		return nil
	}
	return db.runSQLiteAutomationDaemonMigrations(ctx)
}
