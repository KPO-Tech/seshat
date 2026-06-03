package db

// Driver identifies a database driver supported by the shared DB module.
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
	DriverMySQL    Driver = "mysql"
)

// Config describes how to open and initialize an application database.
type Config struct {
	Driver        Driver
	DSN           string
	AutoMigrate   bool // when true, runs versioned schema migrations on Open
	BusyTimeoutMS int  // SQLite only
}

// DefaultSQLiteConfig returns a pragmatic default SQLite configuration for
// local Nexus persistence.
func DefaultSQLiteConfig(path string) Config {
	return Config{
		Driver:        DriverSQLite,
		DSN:           path,
		AutoMigrate:   true,
		BusyTimeoutMS: 5000,
	}
}

// DefaultPostgresConfig returns a default PostgreSQL configuration.
func DefaultPostgresConfig(dsn string) Config {
	return Config{Driver: DriverPostgres, DSN: dsn, AutoMigrate: true}
}

// DefaultMySQLConfig returns a default MySQL configuration.
func DefaultMySQLConfig(dsn string) Config {
	return Config{Driver: DriverMySQL, DSN: dsn, AutoMigrate: true}
}
