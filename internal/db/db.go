package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the shared low-level application database handle. Runtime subsystems
// such as session storage can depend on it now, and future app domains like
// users/roles can reuse the same connection and migration layer later.
type DB struct {
	gormDB *gorm.DB
	driver Driver
	dsn    string
}

// Open creates a database handle and initializes the shared schema when
// AutoMigrate is enabled.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.Driver == "" {
		return nil, fmt.Errorf("database driver is required")
	}
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	dialector, normalizedDSN, err := buildDialector(cfg)
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	}

	gdb, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{
		gormDB: gdb,
		driver: cfg.Driver,
		dsn:    normalizedDSN,
	}

	// Apply driver-specific pool settings and pragmas
	if err := db.configure(ctx, cfg); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Ping
	sqlDB, err := gdb.DB()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if cfg.AutoMigrate {
		if err := db.Initialize(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return db, nil
}

// SQL exposes the underlying sql.DB for backend implementations that need
// transactions or driver-specific operations (e.g. sqlite_backend.go).
func (db *DB) SQL() *sql.DB {
	sqlDB, _ := db.gormDB.DB()
	return sqlDB
}

// GormDB returns the underlying *gorm.DB.
func (db *DB) GormDB() *gorm.DB {
	return db.gormDB
}

// Driver returns the configured driver kind.
func (db *DB) Driver() Driver {
	return db.driver
}

// DSN returns the normalized data source name used to open the database.
func (db *DB) DSN() string {
	return db.dsn
}

// Ping checks that the database connection is alive.
func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.gormDB == nil {
		return fmt.Errorf("database not initialized")
	}
	sqlDB, err := db.gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the underlying database handle.
// For SQLite, PRAGMA optimize is run first to update the query planner statistics.
func (db *DB) Close() error {
	if db == nil || db.gormDB == nil {
		return nil
	}
	sqlDB, err := db.gormDB.DB()
	if err != nil {
		return err
	}
	if db.driver == DriverSQLite {
		_, _ = sqlDB.Exec("PRAGMA optimize")
	}
	return sqlDB.Close()
}

func (db *DB) configure(ctx context.Context, cfg Config) error {
	sqlDB, err := db.gormDB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB for configure: %w", err)
	}

	switch db.driver {
	case DriverSQLite:
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)

		pragmaStatements := []string{
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA cache_size = -20000",       // 20 MB page cache (default ~2 MB)
			"PRAGMA mmap_size = 134217728",     // 128 MB memory-mapped I/O
			"PRAGMA temp_store = MEMORY",       // temp tables in RAM, never disk
			"PRAGMA wal_autocheckpoint = 1000", // checkpoint every 1 000 WAL pages
		}
		if cfg.BusyTimeoutMS > 0 {
			pragmaStatements = append(pragmaStatements, fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.BusyTimeoutMS))
		}
		for _, stmt := range pragmaStatements {
			if err := db.gormDB.WithContext(ctx).Exec(stmt).Error; err != nil {
				return fmt.Errorf("configure sqlite database: %w", err)
			}
		}

	case DriverPostgres:
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)

	case DriverMySQL:
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)

	default:
		return fmt.Errorf("unsupported database driver %q", db.driver)
	}
	return nil
}

func buildDialector(cfg Config) (gorm.Dialector, string, error) {
	switch cfg.Driver {
	case DriverSQLite:
		if err := ensureSQLitePath(cfg.DSN); err != nil {
			return nil, "", err
		}
		dsn := cfg.DSN
		if dsn != ":memory:" {
			if !strings.HasPrefix(dsn, "file:") {
				abs, err := filepath.Abs(dsn)
				if err == nil {
					dsn = abs
				}
				dsn = "file:" + dsn
			}
			if !strings.Contains(dsn, "_txlock=") {
				if strings.Contains(dsn, "?") {
					dsn += "&_txlock=immediate"
				} else {
					dsn += "?_txlock=immediate"
				}
			}
		}
		return glebarez.Open(dsn), dsn, nil

	case DriverPostgres:
		return postgres.Open(cfg.DSN), cfg.DSN, nil

	case DriverMySQL:
		dsn := cfg.DSN
		if !strings.Contains(dsn, "parseTime=") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		return mysql.Open(dsn), dsn, nil

	default:
		return nil, "", fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func ensureSQLitePath(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("sqlite dsn is required")
	}
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sqlite directory: %w", err)
	}
	return nil
}
