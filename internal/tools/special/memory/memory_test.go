package memory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	longterm "github.com/EngineerProjects/nexus-engine/internal/memory/longterm"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	memtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/memory"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── fakeStore ────────────────────────────────────────────────────────────────

// fakeStore is an in-memory Store for tool-level tests; no DB required.
type fakeStore struct {
	entities []longterm.Entity
}

func (s *fakeStore) UpsertEntities(_ context.Context, userID string, inputs []longterm.EntityInput) ([]longterm.Entity, error) {
	created := make([]longterm.Entity, 0)
	for _, inp := range inputs {
		dup := false
		for _, e := range s.entities {
			if e.UserID == userID && e.Name == inp.Name {
				dup = true
				break
			}
		}
		if !dup {
			e := longterm.Entity{
				ID:           "e-" + inp.Name,
				UserID:       userID,
				Name:         inp.Name,
				EntityType:   inp.EntityType,
				Observations: inp.Observations,
			}
			if e.Observations == nil {
				e.Observations = []string{}
			}
			s.entities = append(s.entities, e)
			created = append(created, e)
		}
	}
	return created, nil
}

func (s *fakeStore) AddObservations(_ context.Context, userID string, inputs []longterm.ObservationInput) ([]longterm.ObservationResult, error) {
	results := make([]longterm.ObservationResult, 0, len(inputs))
	for _, inp := range inputs {
		for i := range s.entities {
			if s.entities[i].UserID == userID && s.entities[i].Name == inp.EntityName {
				s.entities[i].Observations = append(s.entities[i].Observations, inp.Contents...)
				results = append(results, longterm.ObservationResult{
					EntityName:        inp.EntityName,
					AddedObservations: inp.Contents,
				})
				break
			}
		}
	}
	return results, nil
}

func (s *fakeStore) SearchNodes(_ context.Context, userID, query string) (*longterm.Graph, error) {
	q := strings.ToLower(query)
	matched := make([]longterm.Entity, 0)
	for _, e := range s.entities {
		if e.UserID != userID {
			continue
		}
		if strings.Contains(strings.ToLower(e.Name), q) ||
			strings.Contains(strings.ToLower(e.EntityType), q) {
			matched = append(matched, e)
		}
	}
	return &longterm.Graph{Entities: matched}, nil
}

func (s *fakeStore) OpenNodes(_ context.Context, userID string, names []string) (*longterm.Graph, error) {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	matched := make([]longterm.Entity, 0)
	for _, e := range s.entities {
		if e.UserID == userID && nameSet[e.Name] {
			matched = append(matched, e)
		}
	}
	return &longterm.Graph{Entities: matched}, nil
}

func (s *fakeStore) RetrieveForContext(_ context.Context, userID, query string, _ int) (string, error) {
	g, _ := s.SearchNodes(context.Background(), userID, query)
	if g == nil || len(g.Entities) == 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.WriteString("## Memory\n\n")
	for _, e := range g.Entities {
		sb.WriteString("**" + e.Name + "**\n")
	}
	return sb.String(), nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func ctxWithUser(userID string) context.Context {
	return types.WithAgentUserID(context.Background(), userID)
}

func callTool(t *testing.T, tl tool.Tool, ctx context.Context, params map[string]any) tool.CallResult {
	t.Helper()
	result, err := tl.Call(ctx, tool.CallInput{Parsed: params}, nil)
	require.NoError(t, err)
	return result
}

// ─── Tool tests ───────────────────────────────────────────────────────────────

func TestCreateEntitiesTool_BasicFlow(t *testing.T) {
	store := &fakeStore{}
	tl := memtool.NewCreateEntitiesTool(store)
	ctx := ctxWithUser("user-1")

	result := callTool(t, tl, ctx, map[string]any{
		"entities": []any{
			map[string]any{"name": "nexus-engine", "entity_type": "project", "observations": []any{"Written in Go"}},
			map[string]any{"name": "user", "entity_type": "person"},
		},
	})
	require.False(t, result.IsError(), "got: %s", result.Content)

	var created []longterm.Entity
	require.NoError(t, json.Unmarshal([]byte(result.Content), &created))
	require.Len(t, created, 2)
	require.Equal(t, "nexus-engine", created[0].Name)
	require.Equal(t, []string{"Written in Go"}, created[0].Observations)
}

func TestCreateEntitiesTool_Idempotent(t *testing.T) {
	store := &fakeStore{}
	tl := memtool.NewCreateEntitiesTool(store)
	ctx := ctxWithUser("user-1")
	params := map[string]any{
		"entities": []any{map[string]any{"name": "go", "entity_type": "language"}},
	}
	callTool(t, tl, ctx, params)

	result := callTool(t, tl, ctx, params) // second call — must not duplicate
	var created []longterm.Entity
	require.NoError(t, json.Unmarshal([]byte(result.Content), &created))
	require.Empty(t, created, "second upsert must not create a duplicate entity")
}

func TestCreateEntitiesTool_RequiresUserID(t *testing.T) {
	tl := memtool.NewCreateEntitiesTool(&fakeStore{})
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{
		"entities": []any{map[string]any{"name": "x", "entity_type": "y"}},
	}}, nil)
	require.NoError(t, err)
	require.True(t, result.IsError(), "must fail when user ID is absent from context")
}

func TestAddObservationsTool_AppendsToEntity(t *testing.T) {
	store := &fakeStore{}
	ctx := ctxWithUser("user-2")

	callTool(t, memtool.NewCreateEntitiesTool(store), ctx, map[string]any{
		"entities": []any{map[string]any{"name": "go", "entity_type": "language"}},
	})

	result := callTool(t, memtool.NewAddObservationsTool(store), ctx, map[string]any{
		"observations": []any{
			map[string]any{"entity_name": "go", "contents": []any{"Compiled", "Statically typed"}},
		},
	})
	require.False(t, result.IsError())

	var results []longterm.ObservationResult
	require.NoError(t, json.Unmarshal([]byte(result.Content), &results))
	require.Len(t, results, 1)
	require.Equal(t, []string{"Compiled", "Statically typed"}, results[0].AddedObservations)
}

func TestSearchNodesTool_FindsEntityByName(t *testing.T) {
	store := &fakeStore{}
	ctx := ctxWithUser("user-3")
	callTool(t, memtool.NewCreateEntitiesTool(store), ctx, map[string]any{
		"entities": []any{
			map[string]any{"name": "nexus", "entity_type": "project"},
			map[string]any{"name": "other", "entity_type": "misc"},
		},
	})

	result := callTool(t, memtool.NewSearchNodesTool(store), ctx, map[string]any{"query": "nexus"})
	require.False(t, result.IsError())

	var graph longterm.Graph
	require.NoError(t, json.Unmarshal([]byte(result.Content), &graph))
	require.Len(t, graph.Entities, 1)
	require.Equal(t, "nexus", graph.Entities[0].Name)
}

func TestOpenNodesTool_ExactLookup(t *testing.T) {
	store := &fakeStore{}
	ctx := ctxWithUser("user-4")
	callTool(t, memtool.NewCreateEntitiesTool(store), ctx, map[string]any{
		"entities": []any{
			map[string]any{"name": "alice", "entity_type": "person"},
			map[string]any{"name": "bob", "entity_type": "person"},
		},
	})

	result := callTool(t, memtool.NewOpenNodesTool(store), ctx, map[string]any{"names": []any{"alice"}})
	require.False(t, result.IsError())

	var graph longterm.Graph
	require.NoError(t, json.Unmarshal([]byte(result.Content), &graph))
	require.Len(t, graph.Entities, 1)
	require.Equal(t, "alice", graph.Entities[0].Name)
}

func TestTools_NilStore_Disabled(t *testing.T) {
	ctx := ctxWithUser("u")
	for _, tl := range []tool.Tool{
		memtool.NewCreateEntitiesTool(nil),
		memtool.NewAddObservationsTool(nil),
		memtool.NewSearchNodesTool(nil),
		memtool.NewOpenNodesTool(nil),
	} {
		require.False(t, tl.IsEnabled(), "%s should be disabled when store is nil", tl.Definition().Name)
		result, _ := tl.Call(ctx, tool.CallInput{Parsed: map[string]any{
			"query": "x", "names": []any{"x"}, "entities": []any{}, "observations": []any{},
		}}, nil)
		require.True(t, result.IsError(), "%s should return error when nil", tl.Definition().Name)
	}
}

func TestSearchNodesTool_UserIsolation(t *testing.T) {
	store := &fakeStore{}
	ctxA := ctxWithUser("user-A")
	ctxB := ctxWithUser("user-B")
	createTl := memtool.NewCreateEntitiesTool(store)

	callTool(t, createTl, ctxA, map[string]any{
		"entities": []any{map[string]any{"name": "secret", "entity_type": "data"}},
	})
	callTool(t, createTl, ctxB, map[string]any{
		"entities": []any{map[string]any{"name": "other", "entity_type": "data"}},
	})

	// user-B must not see user-A's entity.
	result := callTool(t, memtool.NewSearchNodesTool(store), ctxB, map[string]any{"query": "secret"})
	var graph longterm.Graph
	require.NoError(t, json.Unmarshal([]byte(result.Content), &graph))
	require.Empty(t, graph.Entities, "user isolation must be enforced")
}
