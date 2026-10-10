package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const migrationTableName = "seshat_schema_migrations"

const migrationScopeCoreSQLite = "core_sqlite"

type schemaMigration struct {
	ID    string
	Scope string
	Run   func(context.Context, *DB) error
}

type gSchemaMigration struct {
	ID            string `gorm:"primaryKey;size:128;column:id"`
	Scope         string `gorm:"primaryKey;size:32;column:scope"`
	AppliedAtUnix int64  `gorm:"column:applied_at_unix;not null"`
}

func (gSchemaMigration) TableName() string { return migrationTableName }

func (db *DB) runSQLiteCoreMigrations(ctx context.Context) error {
	return db.applyMigrations(ctx, sqliteCoreMigrations())
}

func (db *DB) applyMigrations(ctx context.Context, migrations []schemaMigration) error {
	if len(migrations) == 0 {
		return nil
	}
	if err := db.ensureMigrationTable(ctx); err != nil {
		return err
	}

	applied, err := db.appliedMigrationSet(ctx)
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		key := migration.Scope + ":" + migration.ID
		if applied[key] {
			continue
		}
		err := db.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txDB := db.withGorm(tx)
			if err := migration.Run(ctx, txDB); err != nil {
				return wrapErr("apply migration "+key, err)
			}
			record := gSchemaMigration{
				ID:            migration.ID,
				Scope:         migration.Scope,
				AppliedAtUnix: time.Now().UTC().Unix(),
			}
			if err := tx.WithContext(ctx).Create(&record).Error; err != nil {
				return wrapErr("record migration "+key, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		applied[key] = true
	}
	return nil
}

func (db *DB) ensureMigrationTable(ctx context.Context) error {
	if err := db.gormDB.WithContext(ctx).AutoMigrate(&gSchemaMigration{}); err != nil {
		return wrapErr("automigrate migration table", err)
	}
	return nil
}

func (db *DB) appliedMigrationSet(ctx context.Context) (map[string]bool, error) {
	var rows []gSchemaMigration
	if err := db.gormDB.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, wrapErr("list applied migrations", err)
	}
	applied := make(map[string]bool, len(rows))
	for _, row := range rows {
		applied[row.Scope+":"+row.ID] = true
	}
	return applied, nil
}

func (db *DB) withGorm(gormDB *gorm.DB) *DB {
	if db == nil {
		return nil
	}
	cloned := *db
	cloned.gormDB = gormDB
	return &cloned
}

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
