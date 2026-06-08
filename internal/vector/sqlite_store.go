package vector

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
)

// SQLiteStore is a persistent vector.Store backed by SQLite.
//
// Vectors are stored as raw IEEE-754 little-endian float32 BLOBs for compact
// storage and fast decode. Cosine similarity search runs in Go after loading
// the namespace's vectors from SQLite — adequate for typical RAG corpora
// (thousands of chunks). The table schema is registered as migration
// 005_rag_vector_store in internal/db/schema.go.
type SQLiteStore struct {
	db *dbpkg.DB
}

// NewSQLiteStore wraps an already-open, already-migrated DB handle.
func NewSQLiteStore(database *dbpkg.DB) (*SQLiteStore, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if database.Driver() != dbpkg.DriverSQLite {
		return nil, fmt.Errorf("sqlite vector store requires sqlite database, got %q", database.Driver())
	}
	return &SQLiteStore{db: database}, nil
}

// OpenSQLiteStore opens (or creates) a SQLite file at path and initializes the
// schema, returning a ready-to-use SQLiteStore.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite vector store: %w", err)
	}
	return NewSQLiteStore(database)
}

// Close releases the underlying database connection.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Upsert inserts or replaces records in the vector_records table.
func (s *SQLiteStore) Upsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO vector_records (namespace, key, text, vector, metadata)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(namespace, key) DO UPDATE SET
		   text     = excluded.text,
		   vector   = excluded.vector,
		   metadata = excluded.metadata`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
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
		blob, err := encodeVector(r.Vector)
		if err != nil {
			return fmt.Errorf("encode vector for key %q: %w", r.Key, err)
		}
		meta, err := json.Marshal(r.Metadata)
		if err != nil {
			return fmt.Errorf("encode metadata for key %q: %w", r.Key, err)
		}
		if _, err := stmt.ExecContext(ctx, r.Namespace, r.Key, r.Text, blob, string(meta)); err != nil {
			return fmt.Errorf("upsert record %q: %w", r.Key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert: %w", err)
	}

	// Sync FTS5 index (best-effort; the table may not exist on old DBs).
	for _, r := range records {
		_, _ = s.db.SQL().ExecContext(ctx,
			`DELETE FROM vector_records_fts WHERE namespace = ? AND key = ?`,
			r.Namespace, r.Key)
		_, _ = s.db.SQL().ExecContext(ctx,
			`INSERT INTO vector_records_fts(namespace, key, text) VALUES(?, ?, ?)`,
			r.Namespace, r.Key, r.Text)
	}
	return nil
}

// Search performs cosine similarity search over the given namespace.
// When query.HybridWeight > 0 and query.QueryText is set, BM25 scores from the
// FTS5 index are blended with vector scores using linear interpolation.
func (s *SQLiteStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
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

	// Load all vector records for the namespace.
	rows, err := s.db.SQL().QueryContext(ctx,
		`SELECT key, text, vector, metadata FROM vector_records WHERE namespace = ?`,
		query.Namespace)
	if err != nil {
		return nil, fmt.Errorf("query vector_records: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var key, text, metaJSON string
		var blob []byte
		if err := rows.Scan(&key, &text, &blob, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan vector record: %w", err)
		}
		vec, err := decodeVector(blob)
		if err != nil {
			return nil, fmt.Errorf("decode vector for key %q: %w", key, err)
		}
		var meta map[string]string
		if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
			meta = nil
		}
		r := Record{
			Namespace: query.Namespace,
			Key:       key,
			Text:      text,
			Vector:    vec,
			Metadata:  meta,
		}
		if len(query.Filter) > 0 && !matchesFilter(r, query.Filter) {
			continue
		}
		score := cosineSimilarity(query.Vector, vec)
		results = append(results, SearchResult{Record: r, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vector_records: %w", err)
	}

	// Hybrid: blend with BM25 from FTS5 when requested.
	hw := query.HybridWeight
	if hw > 0 && strings.TrimSpace(query.QueryText) != "" && len(results) > 0 {
		results = s.blendBM25(ctx, query.Namespace, query.QueryText, hw, results)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// blendBM25 retrieves FTS5 BM25 scores and blends them with the vector scores.
// bm25 raw scores from SQLite are negative (more negative = better match).
// We normalize them to [0,1] then blend: final = (1-hw)*vector + hw*bm25_norm.
func (s *SQLiteStore) blendBM25(ctx context.Context, namespace, queryText string, hw float32, results []SearchResult) []SearchResult {
	ftsQuery := sanitizeFTSQuery(queryText)
	if ftsQuery == "" {
		return results
	}

	ftsRows, err := s.db.SQL().QueryContext(ctx,
		`SELECT key, bm25(vector_records_fts) FROM vector_records_fts
		 WHERE namespace = ? AND vector_records_fts MATCH ?`,
		namespace, ftsQuery)
	if err != nil {
		// FTS5 unavailable or query error — fall back to pure vector scores.
		return results
	}
	defer ftsRows.Close()

	bm25Raw := make(map[string]float64)
	for ftsRows.Next() {
		var key string
		var score float64
		if err := ftsRows.Scan(&key, &score); err == nil {
			bm25Raw[key] = score
		}
	}
	_ = ftsRows.Err()

	if len(bm25Raw) == 0 {
		return results
	}

	// Normalize BM25 scores: most-negative → 1.0, least-negative (0) → 0.0.
	minRaw := 0.0
	for _, v := range bm25Raw {
		if v < minRaw {
			minRaw = v
		}
	}
	span := -minRaw // span > 0 if there are any matches
	if span == 0 {
		return results
	}

	for i := range results {
		raw, hit := bm25Raw[results[i].Record.Key]
		var bm25Norm float32
		if hit {
			bm25Norm = float32(-raw / span) // normalize to [0,1]
		}
		results[i].Score = (1-hw)*results[i].Score + hw*bm25Norm
	}
	return results
}

// sanitizeFTSQuery removes FTS5-special characters that could cause parse errors.
func sanitizeFTSQuery(q string) string {
	var sb strings.Builder
	for _, r := range q {
		switch r {
		case '"', '(', ')', '*', '^', '-', '+':
			sb.WriteByte(' ')
		default:
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

// Get retrieves records by key. If keys is nil/empty, all records in the namespace
// are returned (without their vector blobs decoded, which would be expensive).
func (s *SQLiteStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	var (
		sqlRows *sql.Rows
		err     error
	)
	if len(keys) == 0 {
		sqlRows, err = s.db.SQL().QueryContext(ctx,
			`SELECT key, text, metadata FROM vector_records WHERE namespace = ?`, namespace)
	} else {
		// Build parameterized IN clause.
		placeholders := make([]string, len(keys))
		args := make([]any, 0, len(keys)+1)
		args = append(args, namespace)
		for i, k := range keys {
			placeholders[i] = "?"
			args = append(args, k)
		}
		sqlRows, err = s.db.SQL().QueryContext(ctx,
			`SELECT key, text, metadata FROM vector_records WHERE namespace = ? AND key IN (`+
				strings.Join(placeholders, ",")+`)`,
			args...)
	}
	if err != nil {
		return nil, fmt.Errorf("get vector records: %w", err)
	}
	defer sqlRows.Close()

	var results []Record
	for sqlRows.Next() {
		var key, text, metaJSON string
		if err := sqlRows.Scan(&key, &text, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan vector record: %w", err)
		}
		var meta map[string]string
		if metaJSON != "" && metaJSON != "{}" {
			if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
				log.Printf("[vector/sqlite] metadata unmarshal warning for key %q in namespace %q: %v", key, namespace, err)
			}
		}
		results = append(results, Record{
			Namespace: namespace,
			Key:       key,
			Text:      text,
			Metadata:  meta,
		})
	}
	if err := sqlRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vector_records: %w", err)
	}
	return results, nil
}

// HasNamespace reports whether at least one record exists in the namespace.
func (s *SQLiteStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	var exists bool
	err := s.db.SQL().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM vector_records WHERE namespace = ? LIMIT 1)`, namespace).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has namespace: %w", err)
	}
	return exists, nil
}

// DeleteNamespace removes all records for a namespace.
func (s *SQLiteStore) DeleteNamespace(ctx context.Context, namespace string) error {
	_, err := s.db.SQL().ExecContext(ctx,
		`DELETE FROM vector_records WHERE namespace = ?`, namespace)
	if err != nil {
		return fmt.Errorf("delete namespace %q: %w", namespace, err)
	}
	// Sync FTS5 (best-effort).
	_, _ = s.db.SQL().ExecContext(ctx,
		`DELETE FROM vector_records_fts WHERE namespace = ?`, namespace)
	return nil
}

// DeleteKeys removes specific keys within a namespace.
func (s *SQLiteStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := s.db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`DELETE FROM vector_records WHERE namespace = ? AND key = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete: %w", err)
	}
	defer stmt.Close()

	for _, key := range keys {
		if _, err := stmt.ExecContext(ctx, namespace, key); err != nil {
			return fmt.Errorf("delete key %q: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	// Sync FTS5 (best-effort; outside main transaction to avoid FTS5 locking issues).
	for _, key := range keys {
		_, _ = s.db.SQL().ExecContext(ctx,
			`DELETE FROM vector_records_fts WHERE namespace = ? AND key = ?`, namespace, key)
	}
	return nil
}

// CountNamespace returns the number of records stored under a namespace.
// Not part of Store interface — useful for tests and diagnostics.
func (s *SQLiteStore) CountNamespace(ctx context.Context, namespace string) (int, error) {
	var count int
	err := s.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vector_records WHERE namespace = ?`, namespace).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return count, nil
}

// encodeVector serialises []float32 as a little-endian IEEE 754 BLOB.
func encodeVector(v []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeVector deserialises a little-endian IEEE 754 BLOB into []float32.
func decodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid vector blob length %d (not a multiple of 4)", len(b))
	}
	v := make([]float32, len(b)/4)
	if err := binary.Read(bytes.NewReader(b), binary.LittleEndian, v); err != nil {
		return nil, err
	}
	return v, nil
}
