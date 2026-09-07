package vector

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	opensearch "github.com/opensearch-project/opensearch-go/v5"
	"github.com/opensearch-project/opensearch-go/v5/opensearchapi"
)

const (
	defaultOpenSearchAddress     = "http://localhost:9200"
	defaultOpenSearchIndexPrefix = "seshat-rag"
	defaultOpenSearchTimeout     = 10 * time.Second
	defaultOpenSearchBulkSize    = 500
)

// OpenSearchConfig holds connection and indexing parameters for OpenSearch.
type OpenSearchConfig struct {
	Addresses          []string
	Username           string
	Password           string
	APIKey             string
	IndexPrefix        string
	DefaultDim         int
	CreateIndex        bool
	KNN                bool
	InsecureSkipVerify bool
	RequestTimeout     time.Duration
	BulkSize           int
}

// OpenSearchStore is a vector.Store backed by OpenSearch.
//
// Each Seshat namespace is mapped to a dedicated OpenSearch index. This keeps
// destructive operations such as DeleteNamespace cheap and avoids accidental
// cross-tenant search when hosts use namespaces as product or organization
// boundaries.
type OpenSearchStore struct {
	client *opensearchapi.Client
	cfg    OpenSearchConfig
}

func NewOpenSearchStore(_ context.Context, cfg OpenSearchConfig) (*OpenSearchStore, error) {
	cfg = normalizeOpenSearchConfig(cfg)
	header := http.Header{}
	if cfg.APIKey != "" {
		header.Set("Authorization", "ApiKey "+cfg.APIKey)
	}
	client, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses:          cfg.Addresses,
			Username:           cfg.Username,
			Password:           cfg.Password,
			Header:             header,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
			RequestTimeout:     cfg.RequestTimeout,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch client: %w", err)
	}
	return &OpenSearchStore{client: client, cfg: cfg}, nil
}

func (s *OpenSearchStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *OpenSearchStore) Upsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	byIndex := make(map[string][]Record)
	for _, record := range records {
		if strings.TrimSpace(record.Namespace) == "" {
			return fmt.Errorf("opensearch upsert: namespace is required")
		}
		if strings.TrimSpace(record.Key) == "" {
			return fmt.Errorf("opensearch upsert: key is required")
		}
		if len(record.Vector) > 0 && s.cfg.KNN && len(record.Vector) != s.cfg.DefaultDim {
			return fmt.Errorf("opensearch upsert: vector dimension %d does not match configured dimension %d", len(record.Vector), s.cfg.DefaultDim)
		}
		if s.cfg.CreateIndex {
			if err := s.ensureIndex(ctx, record.Namespace); err != nil {
				return err
			}
		}
		index := s.indexName(record.Namespace)
		byIndex[index] = append(byIndex[index], record)
	}
	indices := make([]string, 0, len(byIndex))
	for index := range byIndex {
		indices = append(indices, index)
	}
	sort.Strings(indices)
	for _, index := range indices {
		batch := byIndex[index]
		for start := 0; start < len(batch); start += s.cfg.BulkSize {
			end := start + s.cfg.BulkSize
			if end > len(batch) {
				end = len(batch)
			}
			if err := s.bulkIndex(ctx, index, batch[start:end]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *OpenSearchStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	topK := query.TopK
	if topK <= 0 {
		topK = 5
	}
	hw := query.HybridWeight
	if hw < 0 {
		hw = 0
	}
	if hw > 1 {
		hw = 1
	}

	hasText := strings.TrimSpace(query.QueryText) != ""
	hasVector := len(query.Vector) > 0
	switch {
	case hasVector && hw < 1 && hasText && hw > 0:
		vectorResults, err := s.searchVector(ctx, query, topK*2)
		if err != nil {
			return nil, err
		}
		textResults, err := s.searchText(ctx, query, topK*2)
		if err != nil {
			return nil, err
		}
		return blendOpenSearchResults(vectorResults, textResults, hw, topK), nil
	case hasVector && hw < 1:
		return s.searchVector(ctx, query, topK)
	case hasText:
		return s.searchText(ctx, query, topK)
	default:
		return nil, fmt.Errorf("opensearch search requires QueryText or Vector")
	}
}

func (s *OpenSearchStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	index := s.indexName(namespace)
	if len(keys) == 0 {
		results, err := s.searchRaw(ctx, namespace, map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
			"size":  10000,
		})
		if err != nil {
			return nil, err
		}
		return recordsFromSearchResults(results), nil
	}

	results, err := s.searchRaw(ctx, namespace, map[string]any{
		"query": map[string]any{
			"ids": map[string]any{"values": keys},
		},
		"size": len(keys),
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch get from %q: %w", index, err)
	}
	return recordsFromSearchResults(results), nil
}

func (s *OpenSearchStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	resp, err := s.client.Indices.Exists(ctx, &opensearchapi.IndicesExistsReq{
		Indices: []string{s.indexName(namespace)},
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return false, nil
		}
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("opensearch exists %q: unexpected status %s", namespace, resp.Status())
	}
}

func (s *OpenSearchStore) DeleteNamespace(ctx context.Context, namespace string) error {
	missingOK := true
	_, err := s.client.Indices.Delete(ctx, &opensearchapi.IndicesDeleteReq{
		Indices: []string{s.indexName(namespace)},
		Params: &opensearchapi.IndicesDeleteParams{
			IgnoreUnavailable: &missingOK,
		},
	})
	if err != nil {
		return fmt.Errorf("opensearch delete namespace %q: %w", namespace, err)
	}
	return nil
}

func (s *OpenSearchStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	index := s.indexName(namespace)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if _, err := s.client.Doc.Delete(ctx, opensearchapi.DeleteReq{
			Index: index,
			ID:    key,
			Params: &opensearchapi.DeleteParams{
				Refresh: "false",
			},
		}); err != nil {
			return fmt.Errorf("opensearch delete key %q: %w", key, err)
		}
	}
	return nil
}

func (s *OpenSearchStore) ensureIndex(ctx context.Context, namespace string) error {
	exists, err := s.HasNamespace(ctx, namespace)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	body, err := json.Marshal(s.indexMapping())
	if err != nil {
		return fmt.Errorf("opensearch marshal index mapping: %w", err)
	}
	if _, err := s.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      s.indexName(namespace),
		BodyReader: bytes.NewReader(body),
	}); err != nil {
		return fmt.Errorf("opensearch create index %q: %w", namespace, err)
	}
	return nil
}

func (s *OpenSearchStore) bulkIndex(ctx context.Context, index string, records []Record) error {
	body, err := openSearchBulkBody(index, records)
	if err != nil {
		return err
	}
	resp, err := s.client.Doc.Bulk(ctx, opensearchapi.BulkReq{
		Body: bytes.NewReader(body),
		Params: &opensearchapi.BulkParams{
			Refresh: "wait_for",
		},
	})
	if err != nil {
		return fmt.Errorf("opensearch bulk index %q: %w", index, err)
	}
	if failure := resp.BulkItemFailures(); failure != nil {
		return fmt.Errorf("opensearch bulk index %q: %w", index, failure)
	}
	return nil
}

func openSearchBulkBody(index string, records []Record) ([]byte, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		action := map[string]any{
			"index": map[string]string{
				"_index": index,
				"_id":    record.Key,
			},
		}
		if err := encoder.Encode(action); err != nil {
			return nil, fmt.Errorf("opensearch marshal bulk action %q: %w", record.Key, err)
		}
		if err := encoder.Encode(openSearchDocumentFromRecord(record)); err != nil {
			return nil, fmt.Errorf("opensearch marshal bulk record %q: %w", record.Key, err)
		}
	}
	return body.Bytes(), nil
}

func (s *OpenSearchStore) searchText(ctx context.Context, query Query, topK int) ([]SearchResult, error) {
	body := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"match": map[string]any{
							"text": map[string]any{"query": query.QueryText},
						},
					},
				},
				"filter": openSearchFilterClauses(query.Filter),
			},
		},
		"size": topK,
	}
	return s.searchRaw(ctx, query.Namespace, body)
}

func (s *OpenSearchStore) searchVector(ctx context.Context, query Query, topK int) ([]SearchResult, error) {
	if !s.cfg.KNN {
		return nil, fmt.Errorf("opensearch vector search requires OpenSearchKNN=true")
	}
	body := map[string]any{
		"query": map[string]any{
			"knn": map[string]any{
				"vector": map[string]any{
					"vector": query.Vector,
					"k":      topK,
					"filter": map[string]any{
						"bool": map[string]any{"filter": openSearchFilterClauses(query.Filter)},
					},
				},
			},
		},
		"size": topK,
	}
	return s.searchRaw(ctx, query.Namespace, body)
}

func (s *OpenSearchStore) searchRaw(ctx context.Context, namespace string, body map[string]any) ([]SearchResult, error) {
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("opensearch search: namespace is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("opensearch marshal search: %w", err)
	}
	resp, err := s.client.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{s.indexName(namespace)},
		BodyReader: bytes.NewReader(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch search namespace %q: %w", namespace, err)
	}
	results := make([]SearchResult, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var doc openSearchDocument
		if err := json.Unmarshal(hit.Source, &doc); err != nil {
			return nil, fmt.Errorf("opensearch decode hit: %w", err)
		}
		score := float32(0)
		if hit.Score != nil {
			score = float32(*hit.Score)
		}
		results = append(results, SearchResult{
			Record: doc.toRecord(),
			Score:  score,
		})
	}
	return results, nil
}

func (s *OpenSearchStore) indexMapping() map[string]any {
	properties := map[string]any{
		"namespace": map[string]any{"type": "keyword"},
		"key":       map[string]any{"type": "keyword"},
		"text":      map[string]any{"type": "text"},
		"metadata":  map[string]any{"type": "object", "dynamic": true},
	}
	settings := map[string]any{}
	if s.cfg.KNN {
		settings["index"] = map[string]any{"knn": true}
		properties["vector"] = map[string]any{
			"type":      "knn_vector",
			"dimension": s.cfg.DefaultDim,
		}
	}
	return map[string]any{
		"settings": settings,
		"mappings": map[string]any{
			"dynamic_templates": []any{
				map[string]any{
					"metadata_strings": map[string]any{
						"path_match": "metadata.*",
						"mapping":    map[string]any{"type": "keyword"},
					},
				},
			},
			"properties": properties,
		},
	}
}

func (s *OpenSearchStore) indexName(namespace string) string {
	return openSearchIndexName(s.cfg.IndexPrefix, namespace)
}

type openSearchDocument struct {
	Namespace string         `json:"namespace"`
	Key       string         `json:"key"`
	Text      string         `json:"text"`
	Vector    []float32      `json:"vector,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func openSearchDocumentFromRecord(record Record) openSearchDocument {
	meta := make(map[string]any, len(record.Metadata))
	for key, value := range record.Metadata {
		meta[key] = openSearchMetadataValue(value)
	}
	return openSearchDocument{
		Namespace: record.Namespace,
		Key:       record.Key,
		Text:      record.Text,
		Vector:    record.Vector,
		Metadata:  meta,
	}
}

func (d openSearchDocument) toRecord() Record {
	meta := make(map[string]string, len(d.Metadata))
	for key, value := range d.Metadata {
		meta[key] = stringifyOpenSearchMetadataValue(value)
	}
	return Record{
		Namespace: d.Namespace,
		Key:       d.Key,
		Text:      d.Text,
		Vector:    d.Vector,
		Metadata:  meta,
	}
}

func openSearchMetadataValue(value string) any {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err == nil {
		return values
	}
	return value
}

func stringifyOpenSearchMetadataValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		if len(values) == len(v) {
			data, _ := json.Marshal(values)
			return string(data)
		}
	case []string:
		data, _ := json.Marshal(v)
		return string(data)
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func openSearchFilterClauses(filter map[string]any) []any {
	if len(filter) == 0 {
		return nil
	}
	clauses := make([]any, 0, len(filter))
	keys := make([]string, 0, len(filter))
	for key := range filter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		field := "metadata." + key
		switch value := filter[key].(type) {
		case string:
			clauses = append(clauses, map[string]any{"term": map[string]any{field: value}})
		case []string:
			clauses = append(clauses, map[string]any{"terms": map[string]any{field: value}})
		case []any:
			clauses = append(clauses, map[string]any{"terms": map[string]any{field: value}})
		case map[string]any:
			if in, ok := value["$in"]; ok {
				clauses = append(clauses, map[string]any{"terms": map[string]any{field: in}})
			}
		default:
			clauses = append(clauses, map[string]any{"term": map[string]any{field: value}})
		}
	}
	return clauses
}

func blendOpenSearchResults(vectorResults, textResults []SearchResult, hybridWeight float32, topK int) []SearchResult {
	merged := map[string]SearchResult{}
	vectorMax := maxOpenSearchScore(vectorResults)
	textMax := maxOpenSearchScore(textResults)
	for _, result := range vectorResults {
		score := normalizeOpenSearchScore(result.Score, vectorMax) * (1 - hybridWeight)
		result.Score = score
		merged[result.Record.Key] = result
	}
	for _, result := range textResults {
		score := normalizeOpenSearchScore(result.Score, textMax) * hybridWeight
		if existing, ok := merged[result.Record.Key]; ok {
			existing.Score += score
			merged[result.Record.Key] = existing
			continue
		}
		result.Score = score
		merged[result.Record.Key] = result
	}
	out := make([]SearchResult, 0, len(merged))
	for _, result := range merged {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Record.Key < out[j].Record.Key
		}
		return out[i].Score > out[j].Score
	})
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}

func maxOpenSearchScore(results []SearchResult) float32 {
	max := float32(0)
	for _, result := range results {
		if result.Score > max {
			max = result.Score
		}
	}
	return max
}

func normalizeOpenSearchScore(score, max float32) float32 {
	if max <= 0 {
		return 0
	}
	return score / max
}

func recordsFromSearchResults(results []SearchResult) []Record {
	records := make([]Record, 0, len(results))
	for _, result := range results {
		records = append(records, result.Record)
	}
	return records
}

func normalizeOpenSearchConfig(cfg OpenSearchConfig) OpenSearchConfig {
	if len(cfg.Addresses) == 0 {
		cfg.Addresses = []string{defaultOpenSearchAddress}
	}
	if strings.TrimSpace(cfg.IndexPrefix) == "" {
		cfg.IndexPrefix = defaultOpenSearchIndexPrefix
	}
	if cfg.DefaultDim <= 0 {
		cfg.DefaultDim = 1536
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultOpenSearchTimeout
	}
	if cfg.BulkSize <= 0 {
		cfg.BulkSize = defaultOpenSearchBulkSize
	}
	return cfg
}

var openSearchIndexSlugRe = regexp.MustCompile(`[^a-z0-9_-]+`)

func openSearchIndexName(prefix, namespace string) string {
	prefix = openSearchIndexSlug(prefix, defaultOpenSearchIndexPrefix)
	namespaceSlug := openSearchIndexSlug(namespace, "default")
	sum := sha1.Sum([]byte(namespace))
	return fmt.Sprintf("%s-%s-%x", prefix, namespaceSlug, sum[:4])
}

func openSearchIndexSlug(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = openSearchIndexSlugRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if value == "" {
		value = fallback
	}
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-_")
	}
	return value
}

var _ Store = (*OpenSearchStore)(nil)
