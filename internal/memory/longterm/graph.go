package longterm

import "time"

// Entity is a named thing with a type and a list of observations.
// Modelled after the MCP memory server reference (servers/src/memory).
type Entity struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Name         string    `json:"name"`
	EntityType   string    `json:"entity_type"`
	Observations []string  `json:"observations"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EntityInput is used for upsert operations.
type EntityInput struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entity_type"`
	Observations []string `json:"observations"`
}

// Relation is a directed edge between two entities (active-voice convention).
type Relation struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	From         string    `json:"from"` // entity name
	To           string    `json:"to"`   // entity name
	RelationType string    `json:"relation_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// RelationInput is used for create operations.
type RelationInput struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relation_type"`
}

// ObservationInput appends observations to an existing entity.
type ObservationInput struct {
	EntityName string   `json:"entity_name"`
	Contents   []string `json:"contents"`
}

// ObservationResult reports what was actually added after deduplication.
type ObservationResult struct {
	EntityName        string   `json:"entity_name"`
	AddedObservations []string `json:"added_observations"`
}

// Graph is a result set containing entities and the relations that touch them.
type Graph struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}
