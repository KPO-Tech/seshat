package contract

import (
	"encoding/json"

	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
)

// ToolSurface represents the tool surface exposed to the model.
// This contains all the information needed to display and use a tool
// in the AI interface.
type ToolSurface struct {
	// Name is the unique identifier for this tool
	Name string `json:"name"`

	// DisplayName is a human-readable name
	DisplayName string `json:"display_name,omitempty"`

	// Description explains what the tool does
	Description string `json:"description"`

	// Prompt provides detailed guidance on when and how to use the tool
	// This is used by the AI to understand the tool's usage patterns
	Prompt string `json:"prompt,omitempty"`

	// SearchHint provides a hint for tool search functionality
	SearchHint string `json:"search_hint,omitempty"`

	// Category groups related tools together
	Category string `json:"category,omitempty"`

	// InputSchema defines the expected input structure
	InputSchema schema.JSONSchema `json:"input_schema"`

	// IsReadOnly indicates if the tool doesn't modify state
	IsReadOnly bool `json:"is_read_only"`

	// IsDestructive indicates if the tool can cause destructive changes
	IsDestructive bool `json:"is_destructive"`

	// IsConcurrencySafe indicates if the tool can run concurrently with others
	IsConcurrencySafe bool `json:"is_concurrency_safe"`

	// RequiresPermission indicates if the tool requires permission checks
	RequiresPermission bool `json:"requires_permission"`

	// Aliases are alternative names for this tool
	Aliases []string `json:"aliases,omitempty"`

	// MaxResultSize is the maximum number of characters allowed in the tool
	// result content. When exceeded the runtime truncates and records a
	// ContentReplacementState (equivalent to OpenClaude's maxResultSizeChars).
	// A value of 0 means unlimited.
	MaxResultSize int `json:"max_result_size,omitempty"`

	// Metadata contains additional tool metadata
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewToolSurfaceFromDefinition creates a ToolSurface from a Tool Definition.
func NewToolSurfaceFromDefinition(def Definition) ToolSurface {
	return ToolSurface{
		Name:               def.Name,
		DisplayName:        def.DisplayName,
		Description:        def.Description,
		Prompt:             def.Prompt,
		SearchHint:         def.SearchHint,
		Category:           def.Category,
		InputSchema:        def.InputSchema,
		IsReadOnly:         def.IsReadOnly,
		IsDestructive:      def.IsDestructive,
		IsConcurrencySafe:  def.IsConcurrencySafe,
		RequiresPermission: def.RequiresPermission,
		Aliases:            def.Aliases,
		MaxResultSize:      def.MaxResultSize,
		Metadata:           def.Metadata,
	}
}

// ToJSON converts the ToolSurface to JSON format.
func (s ToolSurface) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// FromJSON creates a ToolSurface from JSON format.
func FromJSON(data []byte) (*ToolSurface, error) {
	var surface ToolSurface
	err := json.Unmarshal(data, &surface)
	if err != nil {
		return nil, err
	}
	return &surface, nil
}

// ToMap converts the ToolSurface to a map representation.
func (s ToolSurface) ToMap() map[string]any {
	data, _ := json.Marshal(s)
	var result map[string]any
	json.Unmarshal(data, &result)
	return result
}

// MatchesName checks if this tool matches the given name or any alias.
func (s ToolSurface) MatchesName(name string) bool {
	if s.Name == name {
		return true
	}
	for _, alias := range s.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

// IsEnabled returns whether the tool is enabled (always true for surfaces).
func (s ToolSurface) IsEnabled() bool {
	return true
}
