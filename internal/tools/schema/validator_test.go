package schema

import "testing"

func TestFromMapBuildsStructuredJSONSchema(t *testing.T) {
	converted := FromMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"filters": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"query"},
	})

	if converted.Type != "object" {
		t.Fatalf("expected top-level type object, got %q", converted.Type)
	}
	if len(converted.Required) != 1 || converted.Required[0] != "query" {
		t.Fatalf("expected required query, got %#v", converted.Required)
	}
	querySchema, ok := converted.Properties["query"]
	if !ok {
		t.Fatal("expected query property")
	}
	if querySchema.Type != "string" {
		t.Fatalf("expected query type string, got %q", querySchema.Type)
	}
	filtersSchema, ok := converted.Properties["filters"]
	if !ok || filtersSchema.Items == nil {
		t.Fatalf("expected filters array items, got %#v", filtersSchema)
	}
	if filtersSchema.Items.Type != "string" {
		t.Fatalf("expected filters item type string, got %q", filtersSchema.Items.Type)
	}
}
