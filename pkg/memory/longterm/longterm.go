package longterm

import internallongterm "github.com/KPO-Tech/seshat/internal/memory/longterm"

type (
	// Entity is a named thing with a type and a list of observations.
	// Modelled after the MCP memory server reference (servers/src/memory).
	Entity = internallongterm.Entity
	// EntityInput is used for upsert operations.
	EntityInput = internallongterm.EntityInput
	// Extractor runs LLM-powered extraction of entities and observations from
	// session transcripts and persists them in the long-term memory graph.
	//
	// Extraction is designed to be asynchronous and best-effort: any error is
	// logged at DEBUG level and not surfaced to the user.
	Extractor = internallongterm.Extractor
	// ExtractorConfig controls extraction behaviour.
	ExtractorConfig = internallongterm.ExtractorConfig
	// Graph is a result set containing entities and the relations that touch them.
	Graph = internallongterm.Graph
	// ObservationInput appends observations to an existing entity.
	ObservationInput = internallongterm.ObservationInput
	// ObservationResult reports what was actually added after deduplication.
	ObservationResult = internallongterm.ObservationResult
	// Relation is a directed edge between two entities (active-voice convention).
	Relation = internallongterm.Relation
	// RelationInput is used for create operations.
	RelationInput = internallongterm.RelationInput
	// Store is the persistence interface for the long-term memory knowledge graph.
	// Concrete persistence implementations are injected by the embedding runtime.
	// All operations are scoped to a single user via userID.
	Store = internallongterm.Store
)

// DefaultExtractorConfig returns sensible defaults.
func DefaultExtractorConfig() ExtractorConfig {
	return internallongterm.DefaultExtractorConfig()
}

// NewExtractor creates an Extractor. Zero-value config fields are replaced with
// defaults.
func NewExtractor(store Store, caller internallongterm.LLMCaller, cfg ExtractorConfig) *Extractor {
	return internallongterm.NewExtractor(store, caller, cfg)
}
