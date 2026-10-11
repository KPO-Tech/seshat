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
