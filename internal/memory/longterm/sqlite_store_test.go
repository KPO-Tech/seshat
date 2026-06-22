package longterm

import (
	"context"
	"testing"

	dbpkg "github.com/EngineerProjects/seshat/internal/db"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(":memory:"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteStore(db.SQL())
}

func TestSQLiteStore_UpsertEntities(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	userID := "u1"

	created, err := s.UpsertEntities(ctx, userID, []EntityInput{
		{Name: "seshat", EntityType: "project", Observations: []string{"a TUI", "written in Go"}},
		{Name: "alice", EntityType: "person"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("want 2 created, got %d", len(created))
	}

	// Upsert again: duplicates must be ignored
	created2, err := s.UpsertEntities(ctx, userID, []EntityInput{
		{Name: "seshat", EntityType: "project"},
		{Name: "bob", EntityType: "person"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created2) != 1 || created2[0].Name != "bob" {
		t.Fatalf("want only 'bob' as new, got %v", created2)
	}
}

func TestSQLiteStore_AddObservations(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	userID := "u1"

	if _, err := s.UpsertEntities(ctx, userID, []EntityInput{
		{Name: "seshat", EntityType: "project", Observations: []string{"initial"}},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := s.AddObservations(ctx, userID, []ObservationInput{
		{EntityName: "seshat", Contents: []string{"second", "initial"}}, // "initial" is duplicate
		{EntityName: "ghost", Contents: []string{"should be skipped"}},  // entity doesn't exist
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only "seshat" should appear (ghost silently skipped)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].EntityName != "seshat" {
		t.Fatalf("want entity 'seshat', got %q", results[0].EntityName)
	}
	// Only "second" should be added ("initial" is duplicate)
	if len(results[0].AddedObservations) != 1 || results[0].AddedObservations[0] != "second" {
		t.Fatalf("want [second], got %v", results[0].AddedObservations)
	}
}

func TestSQLiteStore_SearchNodes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	userID := "u1"

	if _, err := s.UpsertEntities(ctx, userID, []EntityInput{
		{Name: "seshat", EntityType: "project", Observations: []string{"TUI terminal en Go"}},
		{Name: "alice", EntityType: "person", Observations: []string{"developer"}},
		{Name: "bubbletea", EntityType: "library"},
	}); err != nil {
		t.Fatal(err)
	}

	// Search matching "Go" in observations
	g, err := s.SearchNodes(ctx, userID, "Go")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Entities) != 1 || g.Entities[0].Name != "seshat" {
		t.Fatalf("want [seshat], got %v", entityNames(g.Entities))
	}

	// Search matching entity type "library"
	g2, err := s.SearchNodes(ctx, userID, "library")
	if err != nil {
		t.Fatal(err)
	}
	if len(g2.Entities) != 1 || g2.Entities[0].Name != "bubbletea" {
		t.Fatalf("want [bubbletea], got %v", entityNames(g2.Entities))
	}

	// Search with no match
	g3, err := s.SearchNodes(ctx, userID, "rust-lang-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if len(g3.Entities) != 0 {
		t.Fatalf("want 0 results, got %d", len(g3.Entities))
	}
}

func TestSQLiteStore_OpenNodes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	userID := "u1"

	if _, err := s.UpsertEntities(ctx, userID, []EntityInput{
		{Name: "seshat", EntityType: "project"},
		{Name: "alice", EntityType: "person"},
	}); err != nil {
		t.Fatal(err)
	}

	// Open known nodes
	g, err := s.OpenNodes(ctx, userID, []string{"seshat", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Entities) != 2 {
		t.Fatalf("want 2 entities, got %d", len(g.Entities))
	}

	// Open unknown node
	g2, err := s.OpenNodes(ctx, userID, []string{"ghost-xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(g2.Entities) != 0 {
		t.Fatalf("want 0 entities for unknown node, got %d", len(g2.Entities))
	}
}

func entityNames(entities []Entity) []string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = e.Name
	}
	return names
}
