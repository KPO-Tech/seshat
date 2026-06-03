package ragtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/rag"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// SearchTool implements the rag_search tool.
type SearchTool struct {
	svc *rag.Service
}

// NewSearchTool creates a new rag_search tool backed by the given RAG service.
func NewSearchTool(svc *rag.Service) *SearchTool {
	return &SearchTool{svc: svc}
}

func (t *SearchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolSearchName,
		DisplayName: "RAG Search",
		SearchHint:  SearchHint,
		Description: "Search a document corpus using semantic similarity. Provide a corpus_id (namespace) and a natural-language query. Returns the top matching text chunks ranked by relevance score.",
		Category:    "rag",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"corpus_id": map[string]any{
					"type":        "string",
					"description": "The corpus namespace to search (same id used during ingest).",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Natural-language search query.",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "Maximum number of chunks to return (default 5).",
					"minimum":     1,
					"maximum":     20,
				},
				"hybrid_weight": map[string]any{
					"type":        "number",
					"description": "Blend BM25 keyword scoring with vector similarity. 0 (default) = pure vector; 1 = pure BM25; 0.5 = equal blend. Only effective when the backend supports FTS (SQLite/pgvector).",
					"minimum":     0,
					"maximum":     1,
				},
				"filter": map[string]any{
					"type":                 "object",
					"description":          "Optional metadata filter. Keys are metadata field names; values are strings (equality) or {\"$in\": [...]} objects (membership). Example: {\"filename\": \"readme.md\"}.",
					"additionalProperties": true,
				},
			},
			"required": []string{"corpus_id", "query"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *SearchTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	corpusID, _ := input.Parsed["corpus_id"].(string)
	query, _ := input.Parsed["query"].(string)

	topK := DefaultTopK
	if v, ok := input.Parsed["top_k"]; ok {
		switch n := v.(type) {
		case float64:
			topK = int(n)
		case int:
			topK = n
		}
	}

	var hybridWeight float32
	if v, ok := input.Parsed["hybrid_weight"]; ok {
		switch n := v.(type) {
		case float64:
			hybridWeight = float32(n)
		case float32:
			hybridWeight = n
		}
		if hybridWeight < 0 {
			hybridWeight = 0
		}
		if hybridWeight > 1 {
			hybridWeight = 1
		}
	}

	var filter map[string]any
	if v, ok := input.Parsed["filter"]; ok {
		if m, ok := v.(map[string]any); ok {
			filter = m
		}
	}

	corpusID = strings.TrimSpace(corpusID)
	query = strings.TrimSpace(query)
	if corpusID == "" {
		return tool.NewErrorResult(fmt.Errorf("corpus_id is required")), nil
	}
	if query == "" {
		return tool.NewErrorResult(fmt.Errorf("query is required")), nil
	}
	if topK < 1 {
		topK = DefaultTopK
	}

	resp, err := t.svc.Search(ctx, rag.SearchRequest{
		CorpusID:     corpusID,
		Query:        query,
		TopK:         topK,
		HybridWeight: hybridWeight,
		Filter:       filter,
	})
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("rag search failed: %w", err)), nil
	}

	result := tool.NewJSONResult(resp)
	result.Content = formatSearchResponse(resp)
	result.Metadata = &tool.ResultMetadata{
		Additional: map[string]any{
			"corpus_id":    resp.CorpusID,
			"result_count": len(resp.Results),
		},
	}
	return result, nil
}

func (t *SearchTool) Description(_ context.Context) (string, error) {
	return t.Definition().Description, nil
}

func (t *SearchTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (t *SearchTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.AllowWithInput("", input)
}

func (t *SearchTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *SearchTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *SearchTool) IsEnabled() bool                         { return t.svc != nil }

func (t *SearchTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	if resp, ok := data.(rag.SearchResponse); ok {
		return formatSearchResponse(resp)
	}
	b, _ := json.Marshal(data)
	return string(b)
}

func (t *SearchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

func formatSearchResponse(resp rag.SearchResponse) string {
	if len(resp.Results) == 0 {
		return fmt.Sprintf("No results found in corpus %q.", resp.CorpusID)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Corpus: %s — %d result(s)\n\n", resp.CorpusID, len(resp.Results))
	for i, r := range resp.Results {
		fmt.Fprintf(&sb, "[%d] score=%.4f", i+1, r.Score)
		if fn := r.Metadata["filename"]; fn != "" {
			fmt.Fprintf(&sb, "  file=%s", fn)
		}
		if pos := r.Metadata["position"]; pos != "" {
			fmt.Fprintf(&sb, "  chunk=%s", pos)
		}
		sb.WriteString("\n")
		sb.WriteString(r.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
