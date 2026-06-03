package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ChromaConfig holds connection parameters for the Chroma HTTP API.
type ChromaConfig struct {
	// BaseURL is the root URL of the Chroma server, e.g. "http://localhost:8000".
	BaseURL string

	// APIKey is sent as "Authorization: Bearer <key>" when non-empty.
	APIKey string

	// Tenant and Database are used by the v2 API.
	// Defaults: "default_tenant" / "default_database".
	Tenant   string
	Database string
}

// ChromaStore is a vector.Store backed by the Chroma HTTP API (v2).
//
// Namespace mapping: each Nexus namespace becomes a Chroma collection.
// Collection IDs (UUIDs) are cached in-memory after first use.
//
// Chroma stores the original Nexus key as the document id.
// Text and metadata are stored as Chroma document + metadata fields.
type ChromaStore struct {
	cfg    ChromaConfig
	client *http.Client

	mu      sync.Mutex
	collIDs map[string]string // namespace → Chroma collection UUID
}

// NewChromaStore creates a ChromaStore. No network call is made at construction time.
func NewChromaStore(cfg ChromaConfig) *ChromaStore {
	if cfg.Tenant == "" {
		cfg.Tenant = "default_tenant"
	}
	if cfg.Database == "" {
		cfg.Database = "default_database"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &ChromaStore{
		cfg:     cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		collIDs: make(map[string]string),
	}
}

// ─── Store interface ──────────────────────────────────────────────────────────

func (s *ChromaStore) Upsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	// Group by namespace.
	byNS := make(map[string][]Record)
	for _, r := range records {
		byNS[r.Namespace] = append(byNS[r.Namespace], r)
	}
	for ns, recs := range byNS {
		collID, err := s.ensureCollection(ctx, ns)
		if err != nil {
			return err
		}
		ids := make([]string, len(recs))
		texts := make([]string, len(recs))
		embeddings := make([][]float32, len(recs))
		metas := make([]map[string]any, len(recs))
		for i, r := range recs {
			ids[i] = r.Key
			texts[i] = r.Text
			embeddings[i] = r.Vector
			m := make(map[string]any, len(r.Metadata)+1)
			for k, v := range r.Metadata {
				m[k] = v
			}
			m["_nexus_ns"] = r.Namespace
			metas[i] = m
		}
		body := map[string]any{
			"ids":        ids,
			"documents":  texts,
			"embeddings": embeddings,
			"metadatas":  metas,
		}
		_, err = s.doJSON(ctx, http.MethodPost,
			s.collPath(collID)+"/upsert", body)
		if err != nil {
			return fmt.Errorf("chroma upsert %q: %w", ns, err)
		}
	}
	return nil
}

func (s *ChromaStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	if query.Namespace == "" {
		return nil, fmt.Errorf("chroma search: namespace is required")
	}
	if len(query.Vector) == 0 {
		return nil, fmt.Errorf("chroma search: vector is required")
	}
	topK := query.TopK
	if topK <= 0 {
		topK = 5
	}
	collID, err := s.ensureCollection(ctx, query.Namespace)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"query_embeddings": [][]float32{query.Vector},
		"n_results":        topK,
		"include":          []string{"documents", "metadatas", "distances"},
	}
	if where := buildChromaWhere(query.Filter); where != nil {
		body["where"] = where
	}

	resp, err := s.doJSON(ctx, http.MethodPost, s.collPath(collID)+"/query", body)
	if err != nil {
		return nil, fmt.Errorf("chroma query %q: %w", query.Namespace, err)
	}

	return parseChromaQueryResponse(query.Namespace, resp)
}

func (s *ChromaStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	collID, err := s.ensureCollection(ctx, namespace)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"include": []string{"documents", "metadatas"},
	}
	if len(keys) > 0 {
		body["ids"] = keys
	}
	resp, err := s.doJSON(ctx, http.MethodPost, s.collPath(collID)+"/get", body)
	if err != nil {
		return nil, fmt.Errorf("chroma get %q: %w", namespace, err)
	}
	return parseChromaGetResponse(namespace, resp)
}

func (s *ChromaStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	_, err := s.getCollectionID(ctx, namespace)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *ChromaStore) DeleteNamespace(ctx context.Context, namespace string) error {
	collID, err := s.getCollectionID(ctx, namespace)
	if err != nil {
		return nil // already gone
	}
	_, err = s.doJSON(ctx, http.MethodDelete, s.collPath(collID), nil)
	if err != nil && strings.Contains(err.Error(), "404") {
		return nil
	}
	if err == nil {
		s.mu.Lock()
		delete(s.collIDs, namespace)
		s.mu.Unlock()
	}
	return err
}

func (s *ChromaStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	collID, err := s.getCollectionID(ctx, namespace)
	if err != nil {
		return nil
	}
	_, err = s.doJSON(ctx, http.MethodPost, s.collPath(collID)+"/delete",
		map[string]any{"ids": keys})
	if err != nil && strings.Contains(err.Error(), "404") {
		return nil
	}
	return err
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *ChromaStore) collPath(collectionID string) string {
	return fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections/%s",
		s.cfg.Tenant, s.cfg.Database, collectionID)
}

func (s *ChromaStore) collListPath() string {
	return fmt.Sprintf("/api/v2/tenants/%s/databases/%s/collections",
		s.cfg.Tenant, s.cfg.Database)
}

// ensureCollection returns the Chroma collection UUID for namespace, creating
// it if it does not exist. The result is cached in-memory.
func (s *ChromaStore) ensureCollection(ctx context.Context, namespace string) (string, error) {
	s.mu.Lock()
	if id, ok := s.collIDs[namespace]; ok {
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()

	// Try GET first.
	id, err := s.getCollectionID(ctx, namespace)
	if err == nil {
		return id, nil
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		return "", err
	}

	// Create.
	resp, err := s.doJSON(ctx, http.MethodPost, s.collListPath(), map[string]any{
		"name":     namespace,
		"metadata": map[string]any{"hnsw:space": "cosine"},
	})
	if err != nil {
		return "", fmt.Errorf("chroma create collection %q: %w", namespace, err)
	}
	cid, ok := resp["id"].(string)
	if !ok || cid == "" {
		return "", fmt.Errorf("chroma create collection %q: no id in response", namespace)
	}
	s.mu.Lock()
	s.collIDs[namespace] = cid
	s.mu.Unlock()
	return cid, nil
}

func (s *ChromaStore) getCollectionID(ctx context.Context, namespace string) (string, error) {
	s.mu.Lock()
	if id, ok := s.collIDs[namespace]; ok {
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()

	// GET /api/v2/.../collections/{name}
	resp, err := s.doJSON(ctx, http.MethodGet,
		s.collListPath()+"/"+namespace, nil)
	if err != nil {
		return "", err
	}
	id, _ := resp["id"].(string)
	if id == "" {
		return "", fmt.Errorf("chroma: collection %q not found", namespace)
	}
	s.mu.Lock()
	s.collIDs[namespace] = id
	s.mu.Unlock()
	return id, nil
}

func (s *ChromaStore) doJSON(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("chroma HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var result map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &result)
	}
	return result, nil
}

// buildChromaWhere converts our filter format to Chroma's where clause.
func buildChromaWhere(filter map[string]any) map[string]any {
	if len(filter) == 0 {
		return nil
	}
	conditions := make([]map[string]any, 0, len(filter))
	for k, v := range filter {
		switch t := v.(type) {
		case string:
			conditions = append(conditions, map[string]any{k: map[string]any{"$eq": t}})
		case map[string]any:
			if ins, ok := t["$in"]; ok {
				conditions = append(conditions, map[string]any{k: map[string]any{"$in": ins}})
			}
		}
	}
	switch len(conditions) {
	case 0:
		return nil
	case 1:
		return conditions[0]
	default:
		return map[string]any{"$and": conditions}
	}
}

func parseChromaQueryResponse(namespace string, resp map[string]any) ([]SearchResult, error) {
	idsRaw, _ := resp["ids"].([]any)
	docsRaw, _ := resp["documents"].([]any)
	metasRaw, _ := resp["metadatas"].([]any)
	distsRaw, _ := resp["distances"].([]any)
	if len(idsRaw) == 0 {
		return nil, nil
	}
	// Chroma returns per-query batches; we always send one query.
	ids, _ := idsRaw[0].([]any)
	docs, _ := docsRaw[0].([]any)
	metas, _ := metasRaw[0].([]any)
	dists, _ := distsRaw[0].([]any)

	results := make([]SearchResult, 0, len(ids))
	for i, id := range ids {
		r := Record{
			Namespace: namespace,
			Key:       fmt.Sprintf("%v", id),
			Metadata:  make(map[string]string),
		}
		if i < len(docs) {
			r.Text = fmt.Sprintf("%v", docs[i])
		}
		if i < len(metas) {
			if m, ok := metas[i].(map[string]any); ok {
				for k, v := range m {
					if k == "_nexus_ns" {
						continue
					}
					r.Metadata[k] = fmt.Sprintf("%v", v)
				}
			}
		}
		// Chroma returns cosine distance [0,2]; convert to similarity [0,1].
		score := float32(0)
		if i < len(dists) {
			if d, ok := dists[i].(float64); ok {
				score = float32((2.0 - d) / 2.0)
			}
		}
		results = append(results, SearchResult{Record: r, Score: score})
	}
	return results, nil
}

func parseChromaGetResponse(namespace string, resp map[string]any) ([]Record, error) {
	idsRaw, _ := resp["ids"].([]any)
	docsRaw, _ := resp["documents"].([]any)
	metasRaw, _ := resp["metadatas"].([]any)
	if len(idsRaw) == 0 {
		return nil, nil
	}
	results := make([]Record, 0, len(idsRaw))
	for i, id := range idsRaw {
		r := Record{
			Namespace: namespace,
			Key:       fmt.Sprintf("%v", id),
			Metadata:  make(map[string]string),
		}
		if i < len(docsRaw) {
			r.Text = fmt.Sprintf("%v", docsRaw[i])
		}
		if i < len(metasRaw) {
			if m, ok := metasRaw[i].(map[string]any); ok {
				for k, v := range m {
					if k == "_nexus_ns" {
						continue
					}
					r.Metadata[k] = fmt.Sprintf("%v", v)
				}
			}
		}
		results = append(results, r)
	}
	return results, nil
}
