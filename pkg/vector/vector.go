package vector

import (
	"context"
	"fmt"
	"strings"

	coredb "github.com/KPO-Tech/seshat/internal/db"
	internalvector "github.com/KPO-Tech/seshat/internal/vector"
)

type (
	StoreKind    = internalvector.StoreKind
	Query        = internalvector.Query
	Record       = internalvector.Record
	SearchResult = internalvector.SearchResult
	Store        = internalvector.Store
)

const (
	StoreSQLite     = internalvector.StoreSQLite
	StorePgVector   = internalvector.StorePgVector
	StoreQdrant     = internalvector.StoreQdrant
	StoreOpenSearch = internalvector.StoreOpenSearch
	StoreChroma     = internalvector.StoreChroma
	StoreMemory     = internalvector.StoreMemory
	StoreHNSW       = internalvector.StoreHNSW

	// Deprecated: use Store* constants.
	BackendSQLite     = StoreSQLite
	BackendPgVector   = StorePgVector
	BackendQdrant     = StoreQdrant
	BackendOpenSearch = StoreOpenSearch
	BackendChroma     = StoreChroma
	BackendMemory     = StoreMemory
	BackendHNSW       = StoreHNSW
)

// Deprecated: use StoreKind.
type Backend = StoreKind

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
	StoreKind StoreKind
	DB        *DBHandle

	// Deprecated: use StoreKind.
	Backend StoreKind

	Dim                          int
	PgVectorCreateExtension      *bool
	PgVectorIndexMethod          string
	PgVectorHNSWM                int
	PgVectorHNSWEfConstruction   int
	PgVectorIVFFlatLists         int
	QdrantHost                   string
	QdrantPort                   int
	QdrantAPIKey                 string
	QdrantPrefix                 string
	OpenSearchAddresses          []string
	OpenSearchUsername           string
	OpenSearchPassword           string
	OpenSearchAPIKey             string
	OpenSearchIndexPrefix        string
	OpenSearchCreateIndex        *bool
	OpenSearchKNN                bool
	OpenSearchInsecureSkipVerify bool
	OpenSearchBulkSize           int
	ChromaURL                    string
	ChromaAPIKey                 string
	ChromaTenant                 string
	ChromaDatabase               string
	// HNSWDir is the directory for HNSW index files (StoreHNSW only).
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
		StoreKind:                    cfg.StoreKind,
		Backend:                      cfg.Backend,
		DB:                           coreHandle,
		Dim:                          cfg.Dim,
		PgVectorCreateExtension:      cfg.PgVectorCreateExtension,
		PgVectorIndexMethod:          cfg.PgVectorIndexMethod,
		PgVectorHNSWM:                cfg.PgVectorHNSWM,
		PgVectorHNSWEfConstruction:   cfg.PgVectorHNSWEfConstruction,
		PgVectorIVFFlatLists:         cfg.PgVectorIVFFlatLists,
		QdrantHost:                   cfg.QdrantHost,
		QdrantPort:                   cfg.QdrantPort,
		QdrantAPIKey:                 cfg.QdrantAPIKey,
		QdrantPrefix:                 cfg.QdrantPrefix,
		OpenSearchAddresses:          cfg.OpenSearchAddresses,
		OpenSearchUsername:           cfg.OpenSearchUsername,
		OpenSearchPassword:           cfg.OpenSearchPassword,
		OpenSearchAPIKey:             cfg.OpenSearchAPIKey,
		OpenSearchIndexPrefix:        cfg.OpenSearchIndexPrefix,
		OpenSearchCreateIndex:        cfg.OpenSearchCreateIndex,
		OpenSearchKNN:                cfg.OpenSearchKNN,
		OpenSearchInsecureSkipVerify: cfg.OpenSearchInsecureSkipVerify,
		OpenSearchBulkSize:           cfg.OpenSearchBulkSize,
		ChromaURL:                    cfg.ChromaURL,
		ChromaAPIKey:                 cfg.ChromaAPIKey,
		ChromaTenant:                 cfg.ChromaTenant,
		ChromaDatabase:               cfg.ChromaDatabase,
		HNSWDir:                      cfg.HNSWDir,
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
