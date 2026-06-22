package longterm_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	longterm "github.com/EngineerProjects/seshat/internal/memory/longterm"
	"github.com/EngineerProjects/seshat/internal/types"
)

// ─── Fake LLM caller ─────────────────────────────────────────────────────────

type fakeLLMCaller struct {
	response string
	err      error
}

func (f *fakeLLMCaller) CreateMessage(_ context.Context, _ types.APIRequest) (*types.APIResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &types.APIResponse{
		Content: []types.ContentBlock{types.TextContent{Text: f.response}},
	}, nil
}

// ─── In-memory Store ─────────────────────────────────────────────────────────

type memoryStore struct {
	entities map[string]map[string]*longterm.Entity
}

func newTestStore(t *testing.T) longterm.Store {
	t.Helper()
	return &memoryStore{entities: make(map[string]map[string]*longterm.Entity)}
}

func (s *memoryStore) UpsertEntities(_ context.Context, userID string, inputs []longterm.EntityInput) ([]longterm.Entity, error) {
	if s.entities[userID] == nil {
		s.entities[userID] = make(map[string]*longterm.Entity)
	}
	created := make([]longterm.Entity, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		if _, exists := s.entities[userID][name]; exists {
			continue
		}
		entityType := strings.TrimSpace(input.EntityType)
		if entityType == "" {
			entityType = "entity"
		}
		now := time.Now().UTC()
		obs := dedupe(input.Observations)
		entity := &longterm.Entity{
			ID:           fmt.Sprintf("%s:%s", userID, name),
			UserID:       userID,
			Name:         name,
			EntityType:   entityType,
			Observations: obs,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		s.entities[userID][name] = entity
		created = append(created, cloneEntity(*entity))
	}
	return created, nil
}

func (s *memoryStore) AddObservations(_ context.Context, userID string, inputs []longterm.ObservationInput) ([]longterm.ObservationResult, error) {
	results := make([]longterm.ObservationResult, 0, len(inputs))
	for _, input := range inputs {
		entity := s.entities[userID][strings.TrimSpace(input.EntityName)]
		if entity == nil {
			continue
		}
		existing := make(map[string]bool, len(entity.Observations))
		for _, obs := range entity.Observations {
			existing[obs] = true
		}
		added := make([]string, 0, len(input.Contents))
		for _, obs := range input.Contents {
			obs = strings.TrimSpace(obs)
			if obs == "" || existing[obs] {
				continue
			}
			entity.Observations = append(entity.Observations, obs)
			existing[obs] = true
			added = append(added, obs)
		}
		if len(added) > 0 {
			entity.UpdatedAt = time.Now().UTC()
		}
		results = append(results, longterm.ObservationResult{
			EntityName:        entity.Name,
			AddedObservations: added,
		})
	}
	return results, nil
}

func (s *memoryStore) SearchNodes(_ context.Context, userID, query string) (*longterm.Graph, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	graph := &longterm.Graph{}
	for _, entity := range s.entities[userID] {
		if query == "" || strings.Contains(strings.ToLower(entity.Name), query) || strings.Contains(strings.ToLower(entity.EntityType), query) || containsObservation(entity.Observations, query) {
			graph.Entities = append(graph.Entities, cloneEntity(*entity))
		}
	}
	return graph, nil
}

func (s *memoryStore) OpenNodes(_ context.Context, userID string, names []string) (*longterm.Graph, error) {
	graph := &longterm.Graph{}
	for _, name := range names {
		entity := s.entities[userID][strings.TrimSpace(name)]
		if entity != nil {
			graph.Entities = append(graph.Entities, cloneEntity(*entity))
		}
	}
	return graph, nil
}

func (s *memoryStore) RetrieveForContext(_ context.Context, _ string, _ string, _ int) (string, error) {
	return "", nil
}

func cloneEntity(entity longterm.Entity) longterm.Entity {
	entity.Observations = append([]string(nil), entity.Observations...)
	return entity
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func containsObservation(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

// ─── Conversation builders ────────────────────────────────────────────────────

func conversation(pairs ...string) []types.Message {
	var msgs []types.Message
	for i := 0; i+1 < len(pairs); i += 2 {
		msgs = append(msgs,
			types.UserMessage(fmt.Sprintf("u%d", i), pairs[i]),
			types.AssistantMessage(fmt.Sprintf("a%d", i), []types.ContentBlock{
				types.TextContent{Text: pairs[i+1]},
			}),
		)
	}
	return msgs
}

// ─── ParseLLMResponse ─────────────────────────────────────────────────────────

func TestParseLLMResponse_ValidJSON(t *testing.T) {
	raw := `{"entities":[{"name":"seshat","entity_type":"project","observations":["Written in Go","Uses RAG pipeline"]}]}`
	entities, err := longterm.ParseLLMResponse(raw)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	require.Equal(t, "seshat", entities[0].Name)
	require.Equal(t, "project", entities[0].EntityType)
	require.Len(t, entities[0].Observations, 2)
}

func TestParseLLMResponse_MarkdownFence(t *testing.T) {
	raw := "```json\n{\"entities\":[{\"name\":\"alice\",\"entity_type\":\"person\",\"observations\":[\"Prefers Go\"]}]}\n```"
	entities, err := longterm.ParseLLMResponse(raw)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	require.Equal(t, "alice", entities[0].Name)
}

func TestParseLLMResponse_EmptyEntities(t *testing.T) {
	raw := `{"entities":[]}`
	entities, err := longterm.ParseLLMResponse(raw)
	require.NoError(t, err)
	require.Empty(t, entities)
}

func TestParseLLMResponse_JSONEmbeddedInProse(t *testing.T) {
	raw := `Here is the extraction:\n{"entities":[{"name":"bob","entity_type":"person","observations":["Likes Python"]}]}\nDone.`
	entities, err := longterm.ParseLLMResponse(raw)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	require.Equal(t, "bob", entities[0].Name)
}

func TestParseLLMResponse_NoJSON(t *testing.T) {
	_, err := longterm.ParseLLMResponse("no json here at all")
	require.Error(t, err)
}

func TestParseLLMResponse_Empty(t *testing.T) {
	entities, err := longterm.ParseLLMResponse("")
	require.NoError(t, err)
	require.Empty(t, entities)
}

// ─── BuildTranscript ──────────────────────────────────────────────────────────

func TestBuildTranscript_Basic(t *testing.T) {
	msgs := conversation(
		"Hello, I prefer Go over Python.",
		"Got it! Go is a great choice.",
		"Can you help me with RAG?",
		"Sure, RAG stands for Retrieval-Augmented Generation.",
	)
	transcript := longterm.BuildTranscript(msgs, 8000)
	require.Contains(t, transcript, "User:")
	require.Contains(t, transcript, "Assistant:")
	require.Contains(t, transcript, "Go")
	require.Contains(t, transcript, "RAG")
}

func TestBuildTranscript_TruncatesLongConversation(t *testing.T) {
	msgs := conversation(
		strings.Repeat("A", 2000),
		strings.Repeat("B", 2000),
	)
	transcript := longterm.BuildTranscript(msgs, 1000)
	require.LessOrEqual(t, len([]rune(transcript)), 1100, "transcript should be approximately capped")
}

func TestBuildTranscript_SkipsSystemMessages(t *testing.T) {
	msgs := []types.Message{
		{Role: types.RoleSystem, Content: []types.ContentBlock{types.TextContent{Text: "System context"}}},
		types.UserMessage("u1", "Hello"),
		types.AssistantMessage("a1", []types.ContentBlock{types.TextContent{Text: "Hi"}}),
	}
	transcript := longterm.BuildTranscript(msgs, 8000)
	require.NotContains(t, transcript, "System context")
	require.Contains(t, transcript, "Hello")
}

// ─── Extractor.Extract ────────────────────────────────────────────────────────

func TestExtractor_SkipsTooFewTurns(t *testing.T) {
	store := newTestStore(t)
	caller := &fakeLLMCaller{response: `{"entities":[{"name":"foo","entity_type":"concept","observations":["bar"]}]}`}
	ex := longterm.NewExtractor(store, caller, longterm.ExtractorConfig{MinTurns: 3})

	msgs := conversation("Turn 1", "Response 1", "Turn 2", "Response 2")
	err := ex.Extract(context.Background(), "user-skip", msgs)
	require.NoError(t, err)

	graph, err := store.SearchNodes(context.Background(), "user-skip", "foo")
	require.NoError(t, err)
	require.Empty(t, graph.Entities)
}

func TestExtractor_StoresExtractedEntities(t *testing.T) {
	store := newTestStore(t)
	caller := &fakeLLMCaller{
		response: `{"entities":[` +
			`{"name":"seshat","entity_type":"project","observations":["backend in Go","uses FTS5 hybrid search"]},` +
			`{"name":"user","entity_type":"person","observations":["prefers Go over Python"]}` +
			`]}`,
	}
	cfg := longterm.DefaultExtractorConfig()
	cfg.MinTurns = 1
	ex := longterm.NewExtractor(store, caller, cfg)

	msgs := conversation(
		"I prefer Go over Python for the seshat backend.",
		"Understood. Go is a great choice for performance-critical backends.",
		"We're also adding FTS5 hybrid search to the RAG pipeline.",
		"Excellent improvement! Hybrid search combines vector and BM25 scoring.",
	)
	err := ex.Extract(context.Background(), "user-store", msgs)
	require.NoError(t, err)

	graph, err := store.SearchNodes(context.Background(), "user-store", "seshat")
	require.NoError(t, err)
	require.Len(t, graph.Entities, 1)
	require.Equal(t, "seshat", graph.Entities[0].Name)
	require.Contains(t, graph.Entities[0].Observations, "backend in Go")
}

func TestExtractor_IsIdempotent(t *testing.T) {
	store := newTestStore(t)
	caller := &fakeLLMCaller{response: `{"entities":[{"name":"go-lang","entity_type":"language","observations":["compiled","statically typed"]}]}`}
	cfg := longterm.DefaultExtractorConfig()
	cfg.MinTurns = 1
	ex := longterm.NewExtractor(store, caller, cfg)

	msgs := conversation("What about Go?", "Go is great.")

	err := ex.Extract(context.Background(), "user-idem", msgs)
	require.NoError(t, err)
	err = ex.Extract(context.Background(), "user-idem", msgs)
	require.NoError(t, err)

	graph, err := store.SearchNodes(context.Background(), "user-idem", "go-lang")
	require.NoError(t, err)
	require.Len(t, graph.Entities, 1)
	seen := make(map[string]int)
	for _, o := range graph.Entities[0].Observations {
		seen[o]++
	}
	for obs, count := range seen {
		require.Equal(t, 1, count, "duplicate observation %q", obs)
	}
}

func TestExtractor_GracefulOnLLMError(t *testing.T) {
	store := newTestStore(t)
	caller := &fakeLLMCaller{err: fmt.Errorf("LLM service unavailable")}
	cfg := longterm.DefaultExtractorConfig()
	cfg.MinTurns = 1
	ex := longterm.NewExtractor(store, caller, cfg)

	msgs := conversation("Can you help me?", "Of course!", "Tell me more.", "Sure.")
	err := ex.Extract(context.Background(), "user-err", msgs)
	require.NoError(t, err)
}

func TestExtractor_NilCallerIsNoop(t *testing.T) {
	store := newTestStore(t)
	ex := longterm.NewExtractor(store, nil, longterm.DefaultExtractorConfig())
	msgs := conversation("Hello", "Hi", "How are you", "Fine", "Good", "Thanks")
	err := ex.Extract(context.Background(), "user-nil", msgs)
	require.NoError(t, err)
}

func TestExtractor_EmptyUserIDIsNoop(t *testing.T) {
	store := newTestStore(t)
	caller := &fakeLLMCaller{response: `{"entities":[]}`}
	ex := longterm.NewExtractor(store, caller, longterm.DefaultExtractorConfig())
	msgs := conversation("Hello", "Hi", "How are you", "Fine", "Good", "Thanks")
	err := ex.Extract(context.Background(), "", msgs)
	require.NoError(t, err)
}
