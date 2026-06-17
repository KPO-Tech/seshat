package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	longterm "github.com/EngineerProjects/nexus-engine/internal/memory/longterm"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	memtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/memory"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// fakeStore is an in-memory Store for testing; no DB required.
type fakeStore struct {
	entities  []longterm.Entity
	relations []longterm.Relation
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
			obs := inp.Observations
			if obs == nil {
				obs = []string{}
			}
			e := longterm.Entity{
				ID:           "e-" + inp.Name,
				UserID:       userID,
				Name:         inp.Name,
				EntityType:   inp.EntityType,
				Observations: obs,
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

func (s *fakeStore) SearchNodes(_ context.Context, _ string, _ string) (*longterm.Graph, error) {
	return &longterm.Graph{Entities: s.entities, Relations: s.relations}, nil
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

func (s *fakeStore) RetrieveForContext(_ context.Context, _ string, _ string, _ int) (string, error) {
	return "", nil
}

var permFn types.CanUseToolFn = func(_ context.Context, _ types.ToolPermissionRequest) types.PermissionResult {
	return types.AllowWithInput("", nil)
}

func mustJSON[T any](content string) T {
	var v T
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		log.Fatalf("json unmarshal %T: %v — raw: %s", v, err, content)
	}
	return v
}

func main() {
	ctx := types.WithAgentUserID(context.Background(), "test-user")
	store := &fakeStore{}

	// ── Step 1: memory_create_entities ───────────────────────────────────────
	fmt.Println("=== Step 1: memory_create_entities ===")
	createTool := memtool.NewCreateEntitiesTool(store)
	createResult, err := createTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{
			"entities": []any{
				map[string]any{"name": "nexus-engine", "entity_type": "project",
					"observations": []any{"TUI terminal en Go avec bubbletea", "Supporte les agents multi-modaux"}},
				map[string]any{"name": "Alice", "entity_type": "person",
					"observations": []any{"Développeuse principale"}},
				map[string]any{"name": "bubbletea", "entity_type": "library"},
			},
		},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	created := mustJSON[[]longterm.Entity](createResult.Content)
	fmt.Printf("Create Memory  %d entities\n", len(created))
	for _, e := range created {
		fmt.Printf("  %s (%s) · %d obs\n", e.Name, e.EntityType, len(e.Observations))
	}

	// ── Step 2: memory_add_observations ──────────────────────────────────────
	fmt.Println("\n=== Step 2: memory_add_observations ===")
	addTool := memtool.NewAddObservationsTool(store)
	addResult, err := addTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{
			"observations": []any{
				map[string]any{"entity_name": "bubbletea",
					"contents": []any{"Framework TUI Charmbracelet", "Utilisé par nexus-engine"}},
				map[string]any{"entity_name": "Alice",
					"contents": []any{"Préfère les interfaces CLI"}},
			},
		},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	obsResults := mustJSON[[]longterm.ObservationResult](addResult.Content)
	fmt.Printf("Memory Observe  %d entities\n", len(obsResults))
	for _, r := range obsResults {
		fmt.Printf("  %s · +%d observations\n", r.EntityName, len(r.AddedObservations))
	}

	// ── Step 3: memory_search_nodes (with results) ────────────────────────────
	fmt.Println("\n=== Step 3: memory_search_nodes — with results ===")
	searchTool := memtool.NewSearchNodesTool(store)
	searchResult, err := searchTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{"query": "Go"},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	graph := mustJSON[longterm.Graph](searchResult.Content)
	fmt.Printf("Memory Search  Go  %d results\n", len(graph.Entities))
	for _, e := range graph.Entities {
		fmt.Printf("  %s (%s)\n", e.Name, e.EntityType)
	}

	// ── Step 4: memory_search_nodes (no results) ─────────────────────────────
	fmt.Println("\n=== Step 4: memory_search_nodes — no results ===")
	searchResult2, err := searchTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{"query": "rust programming language xyz"},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	graph2 := mustJSON[longterm.Graph](searchResult2.Content)
	if len(graph2.Entities) == 0 {
		fmt.Println("Memory Search  rust programming language xyz  no results ✓")
	} else {
		fmt.Printf("UNEXPECTED: got %d results\n", len(graph2.Entities))
	}

	// ── Step 5: memory_open_nodes (known names) ───────────────────────────────
	fmt.Println("\n=== Step 5: memory_open_nodes — 2 known nodes ===")
	openTool := memtool.NewOpenNodesTool(store)
	openResult, err := openTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{"names": []any{"nexus-engine", "Alice"}},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	graph3 := mustJSON[longterm.Graph](openResult.Content)
	fmt.Printf("Memory Open  2 nodes  %d found\n", len(graph3.Entities))
	for _, e := range graph3.Entities {
		fmt.Printf("  %s (%s) · %d obs\n", e.Name, e.EntityType, len(e.Observations))
	}

	// ── Step 6: memory_open_nodes (unknown name) ──────────────────────────────
	fmt.Println("\n=== Step 6: memory_open_nodes — unknown node ===")
	openResult2, err := openTool.Call(ctx, tool.CallInput{
		Parsed: map[string]any{"names": []any{"entite-inconnue-xyz"}},
	}, permFn)
	if err != nil {
		log.Fatal(err)
	}
	graph4 := mustJSON[longterm.Graph](openResult2.Content)
	if len(graph4.Entities) == 0 {
		fmt.Println("Memory Open  1 node  not found ✓")
	} else {
		fmt.Printf("UNEXPECTED: got %d entities\n", len(graph4.Entities))
	}
}
