package vector

import (
	"context"
	"fmt"
	"strings"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
)

// Backend identifies which vector store implementation to use.
type Backend string

const (
	BackendSQLite   Backend = "sqlite"
	BackendPgVector Backend = "pgvector"
	BackendQdrant   Backend = "qdrant"
	BackendChroma   Backend = "chroma"
	BackendMemory   Backend = "memory" // in-process, for tests and dev
)

// Config describes how to open a vector store.
// Not all fields are used by all backends — see field comments.
type Config struct {
	// Backend selects the implementation.
	Backend Backend

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

	// ChromaURL is the base URL of the Chroma HTTP API (e.g. "http://localhost:8000").
	ChromaURL string

	// ChromaAPIKey is sent as "Authorization: Bearer <key>" when non-empty.
	ChromaAPIKey string

	// ChromaTenant and ChromaDatabase are used with the Chroma v2 API.
	// Defaults to "default_tenant" / "default_database".
	ChromaTenant   string
	ChromaDatabase string
}

// NewStore creates and returns a ready-to-use Store from cfg.
// The caller is responsible for closing any resources (e.g. Qdrant gRPC connection).
func NewStore(ctx context.Context, cfg Config) (Store, error) {
	if cfg.Dim <= 0 {
		cfg.Dim = 1536
	}
	switch cfg.Backend {
	case BackendMemory:
		return NewMemoryStore(), nil

	case BackendSQLite, "":
		if cfg.DB == nil {
			return nil, fmt.Errorf("vector.NewStore: DB is required for sqlite backend")
		}
		return NewSQLiteStore(cfg.DB)

	case BackendPgVector:
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

	case BackendQdrant:
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

	case BackendChroma:
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

	default:
		return nil, fmt.Errorf("vector.NewStore: unknown backend %q", cfg.Backend)
	}
}
