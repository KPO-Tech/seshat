package vector

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qdrant/go-client/qdrant"
)

const (
	qdrantPayloadKeyText     = "_text"
	qdrantPayloadKeySeshatNS = "_seshat_ns"
	qdrantPayloadKeySeshatID = "_seshat_id"
)

// QdrantConfig holds connection parameters for the Qdrant gRPC client.
type QdrantConfig struct {
	Host       string
	Port       int
	APIKey     string
	CollPrefix string // prefix prepended to every collection name
	DefaultDim int    // vector dimension used when creating new collections
}

// QdrantStore is a vector.Store backed by Qdrant (gRPC).
//
// Namespace mapping: each Seshat namespace becomes a Qdrant collection named
// "{prefix}{namespace}". Seshat string keys are converted to deterministic
// uint64 point IDs via SHA-256 (first 8 bytes), with the original key stored
// in the point payload under _seshat_id so it can be round-tripped.
//
// Metadata is stored in the point payload as individual string fields (Qdrant
// does not support nested JSONB natively in gRPC filters).
type QdrantStore struct {
	client *qdrant.Client
	cfg    QdrantConfig
}

// NewQdrantStore dials Qdrant and returns a ready QdrantStore.
func NewQdrantStore(_ context.Context, cfg QdrantConfig) (*QdrantStore, error) {
	if cfg.DefaultDim <= 0 {
		cfg.DefaultDim = 1536
	}
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant connect: %w", err)
	}
	return &QdrantStore{client: client, cfg: cfg}, nil
}

// Close releases the underlying gRPC connection.
func (s *QdrantStore) Close() error {
	return s.client.Close()
}

func (s *QdrantStore) collName(namespace string) string {
	return s.cfg.CollPrefix + namespace
}

// ensureCollection creates the Qdrant collection if it does not yet exist.
func (s *QdrantStore) ensureCollection(ctx context.Context, namespace string) error {
	name := s.collName(namespace)
	exists, err := s.client.CollectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("qdrant check collection %q: %w", name, err)
	}
	if exists {
		return nil
	}
	return s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(s.cfg.DefaultDim),
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

func (s *QdrantStore) Upsert(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	// Group by namespace so we do one Qdrant upsert per collection.
	byNS := make(map[string][]Record)
	for _, r := range records {
		byNS[r.Namespace] = append(byNS[r.Namespace], r)
	}
	for ns, recs := range byNS {
		if err := s.ensureCollection(ctx, ns); err != nil {
			return err
		}
		points := make([]*qdrant.PointStruct, 0, len(recs))
		for _, r := range recs {
			if r.Namespace == "" || r.Key == "" || len(r.Vector) == 0 {
				return fmt.Errorf("qdrant upsert: namespace, key and vector are required")
			}
			payload := map[string]*qdrant.Value{
				qdrantPayloadKeyText:     qdrant.NewValueString(r.Text),
				qdrantPayloadKeySeshatNS: qdrant.NewValueString(r.Namespace),
				qdrantPayloadKeySeshatID: qdrant.NewValueString(r.Key),
			}
			for k, v := range r.Metadata {
				payload[k] = qdrant.NewValueString(v)
			}
			points = append(points, &qdrant.PointStruct{
				Id:      keyToQdrantID(r.Namespace, r.Key),
				Vectors: qdrant.NewVectors(r.Vector...),
				Payload: payload,
			})
		}
		if _, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: s.collName(ns),
			Points:         points,
		}); err != nil {
			return fmt.Errorf("qdrant upsert collection %q: %w", ns, err)
		}
	}
	return nil
}

func (s *QdrantStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	if query.Namespace == "" {
		return nil, fmt.Errorf("qdrant search: namespace is required")
	}
	if len(query.Vector) == 0 {
		return nil, fmt.Errorf("qdrant search: vector is required")
	}
	topK := query.TopK
	if topK <= 0 {
		topK = 5
	}

	filter := buildQdrantFilter(query.Filter)
	req := &qdrant.QueryPoints{
		CollectionName: s.collName(query.Namespace),
		Query:          qdrant.NewQueryDense(query.Vector),
		Limit:          qdrant.PtrOf(uint64(topK)),
		WithPayload:    qdrant.NewWithPayload(true),
		Filter:         filter,
	}

	scored, err := s.client.Query(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "Not found") {
			return nil, nil // empty namespace
		}
		return nil, fmt.Errorf("qdrant query: %w", err)
	}

	results := make([]SearchResult, 0, len(scored))
	for _, pt := range scored {
		r := Record{
			Namespace: query.Namespace,
			Key:       stringPayload(pt.Payload, qdrantPayloadKeySeshatID),
			Text:      stringPayload(pt.Payload, qdrantPayloadKeyText),
			Metadata:  make(map[string]string),
		}
		for k, v := range pt.Payload {
			if k == qdrantPayloadKeyText || k == qdrantPayloadKeySeshatID || k == qdrantPayloadKeySeshatNS {
				continue
			}
			if sv := v.GetStringValue(); sv != "" {
				r.Metadata[k] = sv
			}
		}
		results = append(results, SearchResult{Record: r, Score: pt.Score})
	}
	return results, nil
}

func (s *QdrantStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	name := s.collName(namespace)
	var pts []*qdrant.RetrievedPoint
	var err error
	if len(keys) == 0 {
		// Scroll all points in the collection.
		var all []*qdrant.RetrievedPoint
		all, err = s.client.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: name,
			WithPayload:    qdrant.NewWithPayload(true),
			Limit:          qdrant.PtrOf(uint32(10000)),
		})
		pts = all
	} else {
		ids := make([]*qdrant.PointId, len(keys))
		for i, k := range keys {
			ids[i] = keyToQdrantID(namespace, k)
		}
		pts, err = s.client.Get(ctx, &qdrant.GetPoints{
			CollectionName: name,
			Ids:            ids,
			WithPayload:    qdrant.NewWithPayload(true),
		})
	}
	if err != nil {
		if strings.Contains(err.Error(), "Not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("qdrant get: %w", err)
	}

	results := make([]Record, 0, len(pts))
	for _, pt := range pts {
		r := Record{
			Namespace: namespace,
			Key:       stringPayload(pt.Payload, qdrantPayloadKeySeshatID),
			Text:      stringPayload(pt.Payload, qdrantPayloadKeyText),
			Metadata:  make(map[string]string),
		}
		for k, v := range pt.Payload {
			if k == qdrantPayloadKeyText || k == qdrantPayloadKeySeshatID || k == qdrantPayloadKeySeshatNS {
				continue
			}
			if sv := v.GetStringValue(); sv != "" {
				r.Metadata[k] = sv
			}
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *QdrantStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	return s.client.CollectionExists(ctx, s.collName(namespace))
}

func (s *QdrantStore) DeleteNamespace(ctx context.Context, namespace string) error {
	exists, err := s.client.CollectionExists(ctx, s.collName(namespace))
	if err != nil || !exists {
		return err
	}
	return s.client.DeleteCollection(ctx, s.collName(namespace))
}

func (s *QdrantStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	ids := make([]*qdrant.PointId, len(keys))
	for i, k := range keys {
		ids[i] = keyToQdrantID(namespace, k)
	}
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collName(namespace),
		Points:         qdrant.NewPointsSelectorIDs(ids),
	})
	if err != nil && strings.Contains(err.Error(), "Not found") {
		return nil
	}
	return err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// keyToQdrantID derives a stable uint64 point ID from (namespace, key) via SHA-256.
func keyToQdrantID(namespace, key string) *qdrant.PointId {
	h := sha256.Sum256([]byte(namespace + "\x00" + key))
	n := binary.LittleEndian.Uint64(h[:8])
	return qdrant.NewIDNum(n)
}

func stringPayload(payload map[string]*qdrant.Value, key string) string {
	if v, ok := payload[key]; ok {
		return v.GetStringValue()
	}
	return ""
}

// buildQdrantFilter converts our generic filter map to a Qdrant Filter.
func buildQdrantFilter(filter map[string]any) *qdrant.Filter {
	if len(filter) == 0 {
		return nil
	}
	must := make([]*qdrant.Condition, 0, len(filter))
	for k, v := range filter {
		switch t := v.(type) {
		case string:
			must = append(must, qdrant.NewMatchKeyword(k, t))
		case map[string]any:
			if ins, ok := t["$in"]; ok {
				var keywords []string
				switch sl := ins.(type) {
				case []string:
					keywords = sl
				case []any:
					for _, a := range sl {
						if s, ok := a.(string); ok {
							keywords = append(keywords, s)
						}
					}
				}
				if len(keywords) > 0 {
					must = append(must, qdrant.NewMatchKeywords(k, keywords...))
				}
			}
		}
	}
	if len(must) == 0 {
		return nil
	}
	return &qdrant.Filter{Must: must}
}

// Ensure json is used (for potential future payload marshal helpers).
var _ = json.Marshal
