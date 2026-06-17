package longterm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SQLiteStore implements Store using a SQLite database.
// Schema is initialized via migration 20260615_012_longterm_memory.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore wraps an open *sql.DB as a longterm memory Store.
// The caller is responsible for closing the underlying DB.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

var _ Store = (*SQLiteStore)(nil)

// UpsertEntities creates entities that do not yet exist (deduplicates by name per user).
// Returns only the entities that were actually inserted.
func (s *SQLiteStore) UpsertEntities(ctx context.Context, userID string, inputs []EntityInput) ([]Entity, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	nowUnix := now.Unix()

	var created []Entity
	for _, inp := range inputs {
		name := strings.TrimSpace(inp.Name)
		if name == "" {
			continue
		}
		obs := inp.Observations
		if obs == nil {
			obs = []string{}
		}
		obsJSON, _ := json.Marshal(obs)
		id := uuid.NewString()

		res, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO memory_entities
			 (id, user_id, name, entity_type, observations_json, created_at_unix, updated_at_unix)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, userID, name, strings.TrimSpace(inp.EntityType), string(obsJSON), nowUnix, nowUnix,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert entity %q: %w", name, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			created = append(created, Entity{
				ID:           id,
				UserID:       userID,
				Name:         name,
				EntityType:   strings.TrimSpace(inp.EntityType),
				Observations: obs,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	return created, nil
}

// AddObservations appends new observations to existing entities.
// Duplicate observation strings (per entity) are silently skipped.
func (s *SQLiteStore) AddObservations(ctx context.Context, userID string, inputs []ObservationInput) ([]ObservationResult, error) {
	results := make([]ObservationResult, 0, len(inputs))
	for _, inp := range inputs {
		var obsJSON string
		err := s.db.QueryRowContext(ctx,
			`SELECT observations_json FROM memory_entities WHERE user_id = ? AND name = ?`,
			userID, inp.EntityName,
		).Scan(&obsJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load entity %q: %w", inp.EntityName, err)
		}

		var current []string
		_ = json.Unmarshal([]byte(obsJSON), &current)

		existing := make(map[string]bool, len(current))
		for _, o := range current {
			existing[o] = true
		}
		var added []string
		for _, o := range inp.Contents {
			if o = strings.TrimSpace(o); o != "" && !existing[o] {
				current = append(current, o)
				existing[o] = true
				added = append(added, o)
			}
		}
		if len(added) > 0 {
			newJSON, _ := json.Marshal(current)
			if _, err := s.db.ExecContext(ctx,
				`UPDATE memory_entities SET observations_json = ?, updated_at_unix = ? WHERE user_id = ? AND name = ?`,
				string(newJSON), time.Now().UTC().Unix(), userID, inp.EntityName,
			); err != nil {
				return nil, fmt.Errorf("update observations for %q: %w", inp.EntityName, err)
			}
		}
		if added == nil {
			added = []string{}
		}
		results = append(results, ObservationResult{EntityName: inp.EntityName, AddedObservations: added})
	}
	return results, nil
}

// SearchNodes performs case-insensitive substring search across entity names,
// types, and observation content.
func (s *SQLiteStore) SearchNodes(ctx context.Context, userID, query string) (*Graph, error) {
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, entity_type, observations_json, created_at_unix, updated_at_unix
		 FROM memory_entities
		 WHERE user_id = ?
		   AND (lower(name) LIKE ? OR lower(entity_type) LIKE ? OR lower(observations_json) LIKE ?)`,
		userID, q, q, q,
	)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	entities, err := scanEntities(rows)
	if err != nil {
		return nil, err
	}
	relations, err := s.relationsForEntities(ctx, userID, entities)
	if err != nil {
		return nil, err
	}
	return &Graph{Entities: entities, Relations: relations}, nil
}

// OpenNodes retrieves specific entities by exact name and their relations.
func (s *SQLiteStore) OpenNodes(ctx context.Context, userID string, names []string) (*Graph, error) {
	if len(names) == 0 {
		return &Graph{}, nil
	}
	ph := strings.Repeat("?,", len(names))
	ph = ph[:len(ph)-1]
	args := make([]any, 0, len(names)+1)
	args = append(args, userID)
	for _, n := range names {
		args = append(args, n)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT id, user_id, name, entity_type, observations_json, created_at_unix, updated_at_unix
			 FROM memory_entities WHERE user_id = ? AND name IN (%s)`, ph,
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("open nodes: %w", err)
	}
	entities, err := scanEntities(rows)
	if err != nil {
		return nil, err
	}
	relations, err := s.relationsForEntities(ctx, userID, entities)
	if err != nil {
		return nil, err
	}
	return &Graph{Entities: entities, Relations: relations}, nil
}

// RetrieveForContext searches memory and formats results as a Markdown block.
func (s *SQLiteStore) RetrieveForContext(ctx context.Context, userID, query string, maxTokens int) (string, error) {
	graph, err := s.SearchNodes(ctx, userID, query)
	if err != nil || graph == nil || len(graph.Entities) == 0 {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("## Memory\n\n")
	for _, e := range graph.Entities {
		sb.WriteString(fmt.Sprintf("**%s** (%s)\n", e.Name, e.EntityType))
		for _, obs := range e.Observations {
			sb.WriteString("- " + obs + "\n")
		}
		sb.WriteString("\n")
	}
	result := sb.String()
	if maxTokens > 0 && len(result) > maxTokens*4 {
		result = result[:maxTokens*4]
	}
	return result, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func scanEntities(rows *sql.Rows) ([]Entity, error) {
	defer rows.Close()
	var entities []Entity
	for rows.Next() {
		var e Entity
		var obsJSON string
		var createdUnix, updatedUnix int64
		if err := rows.Scan(&e.ID, &e.UserID, &e.Name, &e.EntityType, &obsJSON, &createdUnix, &updatedUnix); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		_ = json.Unmarshal([]byte(obsJSON), &e.Observations)
		if e.Observations == nil {
			e.Observations = []string{}
		}
		e.CreatedAt = time.Unix(createdUnix, 0).UTC()
		e.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (s *SQLiteStore) relationsForEntities(ctx context.Context, userID string, entities []Entity) ([]Relation, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(entities))
	seen := make(map[string]bool, len(entities))
	for _, e := range entities {
		if !seen[e.Name] {
			names = append(names, e.Name)
			seen[e.Name] = true
		}
	}

	ph := strings.Repeat("?,", len(names))
	ph = ph[:len(ph)-1]

	// Build args: userID + names (×2 for FROM and TO clauses)
	args := make([]any, 0, 1+len(names)*2)
	args = append(args, userID)
	for _, n := range names {
		args = append(args, n)
	}
	for _, n := range names {
		args = append(args, n)
	}

	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT id, user_id, from_name, to_name, relation_type, created_at_unix
			 FROM memory_relations
			 WHERE user_id = ? AND (from_name IN (%s) OR to_name IN (%s))`, ph, ph,
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query relations: %w", err)
	}
	defer rows.Close()

	var relations []Relation
	for rows.Next() {
		var r Relation
		var createdUnix int64
		if err := rows.Scan(&r.ID, &r.UserID, &r.From, &r.To, &r.RelationType, &createdUnix); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		r.CreatedAt = time.Unix(createdUnix, 0).UTC()
		relations = append(relations, r)
	}
	return relations, rows.Err()
}
