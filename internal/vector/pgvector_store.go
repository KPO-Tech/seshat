package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
)

// PgVectorStore is a persistent vector.Store backed by PostgreSQL + pgvector extension.
//
// Schema: a single `vector_chunks` table shared by all namespaces.
// Each row: (collection_name TEXT, id TEXT, text TEXT, embedding VECTOR(dim), metadata JSONB)
// Primary key: (collection_name, id).
// An HNSW cosine index is created on `embedding` at initialization.
//
// Vectors are passed as formatted strings (e.g. "[0.1,0.2,0.3]") and cast
// with ::vector, avoiding the need for any codec registration.
type PgVectorStore struct {
	db      *sql.DB
	options PgVectorOptions
}

type PgVectorOptions struct {
	Dim                int
	CreateExtension    bool
	IndexMethod        string
	HNSWM              int
	HNSWEfConstruction int
	IVFFlatLists       int
}

// NewPgVectorStore opens a pgvector store using an already-open Postgres DB.
// It creates the vector extension and the vector_chunks table if they don't exist.
func NewPgVectorStore(ctx context.Context, database *dbpkg.DB, options PgVectorOptions) (*PgVectorStore, error) {
	if database == nil {
		return nil, fmt.Errorf("pgvector store: database is required")
	}
	if database.Driver() != dbpkg.DriverPostgres {
		return nil, fmt.Errorf("pgvector store requires postgres, got %q", database.Driver())
	}
	options = normalizePgVectorOptions(options)
	s := &PgVectorStore{db: database.SQL(), options: options}
	if err := s.initialize(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PgVectorStore) initialize(ctx context.Context) error {
	if s.options.CreateExtension {
		if _, err := s.db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			return fmt.Errorf("pgvector init extension: %w", err)
		}
	}

	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS vector_chunks (
			collection_name TEXT NOT NULL,
			id              TEXT NOT NULL,
			text            TEXT NOT NULL DEFAULT '',
			embedding       VECTOR(%d),
			metadata        JSONB NOT NULL DEFAULT '{}',
			PRIMARY KEY (collection_name, id)
		)`, s.options.Dim),
		`CREATE INDEX IF NOT EXISTS idx_vector_chunks_collection ON vector_chunks(collection_name)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("pgvector init: %w", err)
		}
	}
	return s.ensureVectorIndex(ctx)
}

func (s *PgVectorStore) Upsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pgvector upsert: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO vector_chunks (collection_name, id, text, embedding, metadata)
		VALUES ($1, $2, $3, $4::vector, $5::jsonb)
		ON CONFLICT (collection_name, id) DO UPDATE SET
			text      = EXCLUDED.text,
			embedding = EXCLUDED.embedding,
			metadata  = EXCLUDED.metadata
	`)
	if err != nil {
		return fmt.Errorf("pgvector upsert: prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		if r.Namespace == "" {
			return fmt.Errorf("vector namespace is required")
		}
		if r.Key == "" {
			return fmt.Errorf("vector key is required")
		}
		if len(r.Vector) == 0 {
			return fmt.Errorf("vector values are required")
		}
		meta, err := json.Marshal(r.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for %q: %w", r.Key, err)
		}
		if _, err := stmt.ExecContext(ctx, r.Namespace, r.Key, r.Text, formatVector(r.Vector), string(meta)); err != nil {
			return fmt.Errorf("pgvector upsert record %q: %w", r.Key, err)
		}
	}
	return tx.Commit()
}

func (s *PgVectorStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	if query.Namespace == "" {
		return nil, fmt.Errorf("vector query namespace is required")
	}
	if len(query.Vector) == 0 {
		return nil, fmt.Errorf("vector query values are required")
	}
	topK := query.TopK
	if topK <= 0 {
		topK = 5
	}

	hw := query.HybridWeight
	if hw > 0 && strings.TrimSpace(query.QueryText) != "" {
		return s.searchHybrid(ctx, query, topK, hw)
	}
	return s.searchVector(ctx, query, topK)
}

// searchVector is the pure cosine-similarity path.
func (s *PgVectorStore) searchVector(ctx context.Context, query Query, topK int) ([]SearchResult, error) {
	args := []any{query.Namespace, formatVector(query.Vector), topK}
	where := "collection_name = $1"
	where += pgFilterClause(query.Filter, &args)

	q := fmt.Sprintf(`
		SELECT id, text, metadata,
		       1 - (embedding <=> $2::vector) AS score
		FROM   vector_chunks
		WHERE  %s
		ORDER  BY embedding <=> $2::vector
		LIMIT  $3`, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgvector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id, text, metaJSON string
		var score float32
		if err := rows.Scan(&id, &text, &metaJSON, &score); err != nil {
			return nil, fmt.Errorf("pgvector scan: %w", err)
		}
		var meta map[string]string
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		results = append(results, SearchResult{
			Record: Record{Namespace: query.Namespace, Key: id, Text: text, Metadata: meta},
			Score:  score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector iterate: %w", err)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

// searchHybrid blends cosine similarity with ts_rank full-text scoring.
// final_score = (1-hw)*vector_score + hw*ts_rank
func (s *PgVectorStore) searchHybrid(ctx context.Context, query Query, topK int, hw float32) ([]SearchResult, error) {
	args := []any{
		query.Namespace,
		formatVector(query.Vector),
		query.QueryText,
		1 - hw,
		hw,
		topK,
	}
	baseArgIdx := 6
	where := "collection_name = $1"
	where += pgFilterClause(query.Filter, &args)

	q := fmt.Sprintf(`
		SELECT id, text, metadata,
		    $4::float4 * (1 - (embedding <=> $2::vector)) +
		    $5::float4 * COALESCE(ts_rank(to_tsvector('simple', text), plainto_tsquery('simple', $3)), 0)
		    AS blended_score
		FROM vector_chunks
		WHERE %s
		ORDER BY blended_score DESC
		LIMIT $%d`, where, baseArgIdx)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		// ts_rank may not be available in all pg setups — fall back to pure vector.
		return s.searchVector(ctx, query, topK)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id, text, metaJSON string
		var score float32
		if err := rows.Scan(&id, &text, &metaJSON, &score); err != nil {
			return nil, fmt.Errorf("pgvector hybrid scan: %w", err)
		}
		var meta map[string]string
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		results = append(results, SearchResult{
			Record: Record{Namespace: query.Namespace, Key: id, Text: text, Metadata: meta},
			Score:  score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector hybrid iterate: %w", err)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

func (s *PgVectorStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if len(keys) == 0 {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, text, metadata FROM vector_chunks WHERE collection_name = $1`, namespace)
	} else {
		placeholders := make([]string, len(keys))
		args := make([]any, 0, len(keys)+1)
		args = append(args, namespace)
		for i, k := range keys {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, k)
		}
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, text, metadata FROM vector_chunks WHERE collection_name = $1 AND id IN (`+
				strings.Join(placeholders, ",")+`)`,
			args...)
	}
	if err != nil {
		return nil, fmt.Errorf("pgvector get: %w", err)
	}
	defer rows.Close()

	var results []Record
	for rows.Next() {
		var id, text, metaJSON string
		if err := rows.Scan(&id, &text, &metaJSON); err != nil {
			return nil, fmt.Errorf("pgvector get scan: %w", err)
		}
		var meta map[string]string
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		results = append(results, Record{Namespace: namespace, Key: id, Text: text, Metadata: meta})
	}
	return results, rows.Err()
}

func (s *PgVectorStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vector_chunks WHERE collection_name = $1 LIMIT 1`, namespace).Scan(&count)
	return count > 0, err
}

func (s *PgVectorStore) DeleteNamespace(ctx context.Context, namespace string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM vector_chunks WHERE collection_name = $1`, namespace)
	return err
}

func (s *PgVectorStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	placeholders := make([]string, len(keys))
	args := make([]any, 0, len(keys)+1)
	args = append(args, namespace)
	for i, k := range keys {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, k)
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM vector_chunks WHERE collection_name = $1 AND id IN (`+
			strings.Join(placeholders, ",")+`)`, args...)
	return err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// formatVector converts []float32 to the pgvector literal format "[f1,f2,...]".
func formatVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// pgFilterClause appends JSONB predicate fragments to the WHERE clause.
// args is extended in-place; the next placeholder index is len(*args)+1.
func pgFilterClause(filter map[string]any, args *[]any) string {
	if len(filter) == 0 {
		return ""
	}
	var sb strings.Builder
	idx := len(*args) + 1
	for k, v := range filter {
		switch t := v.(type) {
		case string:
			idx++
			*args = append(*args, t)
			sb.WriteString(fmt.Sprintf(` AND metadata->>'%s' = $%d`, pgEscape(k), idx))
		case map[string]any:
			if ins, ok := t["$in"]; ok {
				var vals []string
				switch sl := ins.(type) {
				case []string:
					vals = sl
				case []any:
					for _, a := range sl {
						if s, ok := a.(string); ok {
							vals = append(vals, s)
						}
					}
				}
				if len(vals) > 0 {
					ph := make([]string, len(vals))
					for i, val := range vals {
						idx++
						*args = append(*args, val)
						ph[i] = fmt.Sprintf("$%d", idx)
					}
					sb.WriteString(fmt.Sprintf(` AND metadata->>'%s' IN (%s)`,
						pgEscape(k), strings.Join(ph, ",")))
				}
			}
		}
	}
	return sb.String()
}

func normalizePgVectorOptions(options PgVectorOptions) PgVectorOptions {
	if options.Dim <= 0 {
		options.Dim = 1536
	}
	if options.HNSWM <= 0 {
		options.HNSWM = 16
	}
	if options.HNSWEfConstruction <= 0 {
		options.HNSWEfConstruction = 64
	}
	if options.IVFFlatLists <= 0 {
		options.IVFFlatLists = 100
	}
	if !strings.EqualFold(options.IndexMethod, "ivfflat") {
		options.IndexMethod = "hnsw"
	} else {
		options.IndexMethod = "ivfflat"
	}
	return options
}

func (s *PgVectorStore) ensureVectorIndex(ctx context.Context) error {
	const indexName = "idx_vector_chunks_embedding"

	existingDef, err := s.existingVectorIndexDefinition(ctx, indexName)
	if err != nil {
		return err
	}
	if existingMethod := extractPgIndexMethod(existingDef); existingMethod != "" && existingMethod != s.options.IndexMethod {
		return fmt.Errorf(
			"pgvector existing index %q uses %q but configuration requires %q",
			indexName,
			existingMethod,
			s.options.IndexMethod,
		)
	}
	if existingDef != "" {
		return nil
	}

	indexSQL := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s ON vector_chunks USING %s (embedding vector_cosine_ops)%s`,
		indexName,
		s.options.IndexMethod,
		s.indexOptionsSQL(),
	)
	if _, err := s.db.ExecContext(ctx, indexSQL); err != nil {
		return fmt.Errorf("pgvector init index: %w", err)
	}
	return nil
}

func (s *PgVectorStore) existingVectorIndexDefinition(ctx context.Context, indexName string) (string, error) {
	var indexDef sql.NullString
	err := s.db.QueryRowContext(
		ctx,
		`SELECT indexdef
		 FROM pg_indexes
		 WHERE schemaname = current_schema()
		   AND tablename = 'vector_chunks'
		   AND indexname = $1`,
		indexName,
	).Scan(&indexDef)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query pgvector index metadata: %w", err)
	}
	return indexDef.String, nil
}

func (s *PgVectorStore) indexOptionsSQL() string {
	switch s.options.IndexMethod {
	case "ivfflat":
		return fmt.Sprintf(" WITH (lists = %d)", s.options.IVFFlatLists)
	default:
		return fmt.Sprintf(" WITH (m = %d, ef_construction = %d)", s.options.HNSWM, s.options.HNSWEfConstruction)
	}
}

func extractPgIndexMethod(indexDef string) string {
	indexDef = strings.TrimSpace(strings.ToLower(indexDef))
	if indexDef == "" {
		return ""
	}
	parts := strings.SplitN(indexDef, " using ", 2)
	if len(parts) != 2 {
		return ""
	}
	afterUsing := parts[1]
	methodParts := strings.Fields(afterUsing)
	if len(methodParts) == 0 {
		return ""
	}
	return methodParts[0]
}

// pgEscape performs minimal escaping of field names for JSONB access.
// Only alphanumeric + underscore names are trusted; others are rejected.
func pgEscape(s string) string {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return "invalid_field"
		}
	}
	return s
}
