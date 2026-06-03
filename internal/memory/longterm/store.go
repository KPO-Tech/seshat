package longterm

import "context"

// Store is the persistence interface for the long-term memory knowledge graph.
// Concrete persistence implementations are injected by the embedding runtime.
// All operations are scoped to a single user via userID.
type Store interface {
	// UpsertEntities creates entities that do not exist yet (deduplicates by name).
	// Returns only the entities that were actually inserted.
	UpsertEntities(ctx context.Context, userID string, inputs []EntityInput) ([]Entity, error)

	// AddObservations appends new observations to existing entities.
	// Duplicate observation strings are silently skipped.
	AddObservations(ctx context.Context, userID string, inputs []ObservationInput) ([]ObservationResult, error)

	// SearchNodes performs case-insensitive substring search across entity names,
	// entity types, and observation content. Returns matching entities plus all
	// relations where at least one endpoint is in the result set.
	SearchNodes(ctx context.Context, userID, query string) (*Graph, error)

	// OpenNodes returns specific entities by exact name and the relations that
	// touch them (at least one endpoint in the requested set).
	OpenNodes(ctx context.Context, userID string, names []string) (*Graph, error)

	// RetrieveForContext retrieves memory relevant to the current query and
	// formats it as a Markdown block capped to approximately maxTokens.
	// Returns an empty string when there is nothing relevant.
	RetrieveForContext(ctx context.Context, userID, query string, maxTokens int) (string, error)
}
