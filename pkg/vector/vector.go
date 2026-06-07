package vector

import (
	"context"
	"fmt"
	"strings"

	coredb "github.com/EngineerProjects/nexus-engine/internal/db"
	internalvector "github.com/EngineerProjects/nexus-engine/internal/vector"
)

type (
	Backend      = internalvector.Backend
	Query        = internalvector.Query
	Record       = internalvector.Record
	SearchResult = internalvector.SearchResult
	Store        = internalvector.Store
)

const (
	BackendSQLite   = internalvector.BackendSQLite
	BackendPgVector = internalvector.BackendPgVector
	BackendQdrant   = internalvector.BackendQdrant
	BackendChroma   = internalvector.BackendChroma
	BackendMemory   = internalvector.BackendMemory
	BackendHNSW     = internalvector.BackendHNSW
)

// DBHandle is the public vector-facing database descriptor.
// It intentionally avoids leaking the engine's internal DB type while keeping
// enough information to reopen a compatible low-level handle when needed.
type DBHandle struct {
	DriverName    string
	DSN           string
	BusyTimeoutMS int
}

func NewDBHandle(driverName, dsn string) *DBHandle {
	handle := &DBHandle{
		DriverName: strings.TrimSpace(driverName),
		DSN:        strings.TrimSpace(dsn),
	}
	if handle.DriverName == string(coredb.DriverSQLite) {
		handle.BusyTimeoutMS = 5000
	}
	return handle
}

type Config struct {
	Backend Backend
	DB      *DBHandle

	Dim                        int
	PgVectorCreateExtension    *bool
	PgVectorIndexMethod        string
	PgVectorHNSWM              int
	PgVectorHNSWEfConstruction int
	PgVectorIVFFlatLists       int
	QdrantHost                 string
	QdrantPort                 int
	QdrantAPIKey               string
	QdrantPrefix               string
	ChromaURL                  string
	ChromaAPIKey               string
	ChromaTenant               string
	ChromaDatabase             string
	// HNSWDir is the directory for HNSW index files (BackendHNSW only).
	// Defaults to <runtime_root>/data/hnsw via pkg/config helpers.
	HNSWDir string
}

func NewMemoryStore() *internalvector.MemoryStore {
	return internalvector.NewMemoryStore()
}

func NewStore(ctx context.Context, cfg Config) (Store, error) {
	coreHandle, err := openCoreDB(ctx, cfg.DB)
	if err != nil {
		return nil, err
	}
	return internalvector.NewStore(ctx, internalvector.Config{
		Backend:                    cfg.Backend,
		DB:                         coreHandle,
		Dim:                        cfg.Dim,
		PgVectorCreateExtension:    cfg.PgVectorCreateExtension,
		PgVectorIndexMethod:        cfg.PgVectorIndexMethod,
		PgVectorHNSWM:              cfg.PgVectorHNSWM,
		PgVectorHNSWEfConstruction: cfg.PgVectorHNSWEfConstruction,
		PgVectorIVFFlatLists:       cfg.PgVectorIVFFlatLists,
		QdrantHost:                 cfg.QdrantHost,
		QdrantPort:                 cfg.QdrantPort,
		QdrantAPIKey:               cfg.QdrantAPIKey,
		QdrantPrefix:               cfg.QdrantPrefix,
		ChromaURL:                  cfg.ChromaURL,
		ChromaAPIKey:               cfg.ChromaAPIKey,
		ChromaTenant:               cfg.ChromaTenant,
		ChromaDatabase:             cfg.ChromaDatabase,
		HNSWDir:                    cfg.HNSWDir,
	})
}

func openCoreDB(ctx context.Context, handle *DBHandle) (*coredb.DB, error) {
	if handle == nil {
		return nil, nil
	}
	driver := coredb.Driver(strings.TrimSpace(handle.DriverName))
	dsn := strings.TrimSpace(handle.DSN)
	if driver == "" || dsn == "" {
		return nil, fmt.Errorf("vector DB handle requires non-empty driver and DSN")
	}
	cfg := coredb.Config{
		Driver:        driver,
		DSN:           dsn,
		AutoMigrate:   false,
		BusyTimeoutMS: handle.BusyTimeoutMS,
	}
	if cfg.Driver == coredb.DriverSQLite && cfg.BusyTimeoutMS <= 0 {
		cfg.BusyTimeoutMS = 5000
	}
	return coredb.Open(ctx, cfg)
}
