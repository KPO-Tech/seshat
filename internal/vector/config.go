package vector

import (
	"context"
	"fmt"
	"strings"

	dbpkg "github.com/KPO-Tech/seshat/internal/db"
)

// StoreKind identifies which vector store implementation to use.
type StoreKind string

const (
	StoreSQLite     StoreKind = "sqlite"
	StorePgVector   StoreKind = "pgvector"
	StoreQdrant     StoreKind = "qdrant"
	StoreOpenSearch StoreKind = "opensearch"
	StoreChroma     StoreKind = "chroma"
	StoreMemory     StoreKind = "memory" // in-process, for tests and dev
	StoreHNSW       StoreKind = "hnsw"   // embedded HNSW, no CGO, no external service

	// Deprecated: use StoreKind and Store* constants.
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

// Config describes how to open a vector store.
// Not all fields are used by all backends — see field comments.
type Config struct {
	// StoreKind selects the implementation.
	StoreKind StoreKind

	// Backend selects the implementation.
	//
	// Deprecated: use StoreKind.
	Backend StoreKind

	// DB is used by the SQLite and pgvector backends.
	// For SQLite:   must be a SQLite DB (DriverSQLite).
	// For pgvector: must be a Postgres DB (DriverPostgres).
	DB *dbpkg.DB

	// Dim is the vector dimension required by pgvector and Qdrant when
	// creating a new collection/table. Defaults to 1536 (OpenAI ada-002).
	Dim int

	// PgVectorCreateExtension controls whether the pgvector extension is created
	// automatically at store initialization time.
	PgVectorCreateExtension *bool

	// PgVectorIndexMethod selects the ANN index type for pgvector. Supported:
	// "hnsw" and "ivfflat". Empty defaults to "hnsw".
	PgVectorIndexMethod string

	// PgVectorHNSWM tunes HNSW index creation when PgVectorIndexMethod=hnsw.
	PgVectorHNSWM int

	// PgVectorHNSWEfConstruction tunes HNSW ef_construction.
	PgVectorHNSWEfConstruction int

	// PgVectorIVFFlatLists tunes IVFFlat index creation when
	// PgVectorIndexMethod=ivfflat.
	PgVectorIVFFlatLists int

	// QdrantHost / QdrantPort / QdrantAPIKey for the Qdrant gRPC client.
	QdrantHost   string
	QdrantPort   int // defaults to 6334 (gRPC)
	QdrantAPIKey string

	// QdrantPrefix is prepended to every collection name (useful for
	// multi-tenant deployments sharing one Qdrant instance).
	QdrantPrefix string

	// OpenSearchAddresses is the list of OpenSearch HTTP endpoints.
	// Defaults to http://localhost:9200.
	OpenSearchAddresses []string

	// OpenSearchUsername / OpenSearchPassword configure HTTP Basic auth.
	OpenSearchUsername string
	OpenSearchPassword string

	// OpenSearchAPIKey configures API key auth through the Authorization header.
	OpenSearchAPIKey string

	// OpenSearchIndexPrefix is prepended to every namespace index.
	OpenSearchIndexPrefix string

	// OpenSearchCreateIndex controls whether missing namespace indices are
	// created automatically during Upsert. Nil defaults to true.
	OpenSearchCreateIndex *bool

	// OpenSearchKNN enables knn_vector mappings and vector queries.
	OpenSearchKNN bool

	// OpenSearchInsecureSkipVerify disables TLS certificate verification.
	OpenSearchInsecureSkipVerify bool

	// OpenSearchBulkSize controls how many records are sent per _bulk request.
	// Values <= 0 use the OpenSearch store default.
	OpenSearchBulkSize int

	// ChromaURL is the base URL of the Chroma HTTP API (e.g. "http://localhost:8000").
	ChromaURL string

	// ChromaAPIKey is sent as "Authorization: Bearer <key>" when non-empty.
	ChromaAPIKey string

	// ChromaTenant and ChromaDatabase are used with the Chroma v2 API.
	// Defaults to "default_tenant" / "default_database".
	ChromaTenant   string
	ChromaDatabase string

	// HNSWDir is the directory where HNSW index files are stored.
	// Each namespace gets its own pair of files (<slug>.hnsw + <slug>.meta.json).
	// Defaults to <runtime_root>/data/hnsw when not set.
	HNSWDir string
}

// NewStore creates and returns a ready-to-use Store from cfg.
// The caller is responsible for closing any resources (e.g. Qdrant gRPC connection).
func NewStore(ctx context.Context, cfg Config) (Store, error) {
	if cfg.Dim <= 0 {
		cfg.Dim = 1536
	}
	kind := cfg.StoreKind
	if kind == "" {
		kind = cfg.Backend
	}
	switch kind {
	case StoreMemory:
		return NewMemoryStore(), nil

	case StoreSQLite, "":
		if cfg.DB == nil {
			return nil, fmt.Errorf("vector.NewStore: DB is required for sqlite backend")
		}
		return NewSQLiteStore(cfg.DB)

	case StorePgVector:
		if cfg.DB == nil {
			return nil, fmt.Errorf("vector.NewStore: DB is required for pgvector backend")
		}
		createExtension := true
		if cfg.PgVectorCreateExtension != nil {
			createExtension = *cfg.PgVectorCreateExtension
		}
		return NewPgVectorStore(ctx, cfg.DB, PgVectorOptions{
			Dim:                cfg.Dim,
			CreateExtension:    createExtension,
			IndexMethod:        cfg.PgVectorIndexMethod,
			HNSWM:              cfg.PgVectorHNSWM,
			HNSWEfConstruction: cfg.PgVectorHNSWEfConstruction,
			IVFFlatLists:       cfg.PgVectorIVFFlatLists,
		})

	case StoreQdrant:
		host := strings.TrimSpace(cfg.QdrantHost)
		if host == "" {
			host = "localhost"
		}
		port := cfg.QdrantPort
		if port <= 0 {
			port = 6334
		}
		return NewQdrantStore(ctx, QdrantConfig{
			Host:       host,
			Port:       port,
			APIKey:     cfg.QdrantAPIKey,
			CollPrefix: cfg.QdrantPrefix,
			DefaultDim: cfg.Dim,
		})

	case StoreOpenSearch:
		createIndex := true
		if cfg.OpenSearchCreateIndex != nil {
			createIndex = *cfg.OpenSearchCreateIndex
		}
		return NewOpenSearchStore(ctx, OpenSearchConfig{
			Addresses:          cfg.OpenSearchAddresses,
			Username:           cfg.OpenSearchUsername,
			Password:           cfg.OpenSearchPassword,
			APIKey:             cfg.OpenSearchAPIKey,
			IndexPrefix:        cfg.OpenSearchIndexPrefix,
			DefaultDim:         cfg.Dim,
			CreateIndex:        createIndex,
			KNN:                cfg.OpenSearchKNN,
			InsecureSkipVerify: cfg.OpenSearchInsecureSkipVerify,
			BulkSize:           cfg.OpenSearchBulkSize,
		})

	case StoreChroma:
		url := strings.TrimSpace(cfg.ChromaURL)
		if url == "" {
			url = "http://localhost:8000"
		}
		tenant := cfg.ChromaTenant
		if tenant == "" {
			tenant = "default_tenant"
		}
		database := cfg.ChromaDatabase
		if database == "" {
			database = "default_database"
		}
		return NewChromaStore(ChromaConfig{
			BaseURL:  url,
			APIKey:   cfg.ChromaAPIKey,
			Tenant:   tenant,
			Database: database,
		}), nil

	case StoreHNSW:
		dir := strings.TrimSpace(cfg.HNSWDir)
		if dir == "" {
			return nil, fmt.Errorf("vector.NewStore: HNSWDir is required for hnsw backend")
		}
		return NewHNSWStore(dir)

	default:
		return nil, fmt.Errorf("vector.NewStore: unknown vector store %q", kind)
	}
}
