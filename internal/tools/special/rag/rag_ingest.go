package ragtool

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/rag"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// IngestTool implements the rag_ingest tool.
type IngestTool struct {
	svc *rag.Service
}

// NewIngestTool creates a new rag_ingest tool backed by the given RAG service.
func NewIngestTool(svc *rag.Service) *IngestTool {
	return &IngestTool{svc: svc}
}

func (t *IngestTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolIngestName,
		DisplayName: "RAG Ingest",
		SearchHint:  IngestHint,
		Description: "Chunk and embed a text document into a named corpus for later semantic search. Returns the artifact key and chunk count.",
		Category:    "rag",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"corpus_id": map[string]any{
					"type":        "string",
					"description": "Namespace for the corpus (e.g. \"project-docs\"). Used as the search target in rag_search.",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Logical filename or identifier for the document (e.g. \"readme.md\").",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Full text content to ingest.",
				},
			},
			"required": []string{"corpus_id", "filename", "text"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
	}
}

func (t *IngestTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	corpusID, _ := input.Parsed["corpus_id"].(string)
	filename, _ := input.Parsed["filename"].(string)
	text, _ := input.Parsed["text"].(string)

	corpusID = strings.TrimSpace(corpusID)
	filename = strings.TrimSpace(filename)
	text = strings.TrimSpace(text)

	if corpusID == "" {
		return tool.NewErrorResult(fmt.Errorf("corpus_id is required")), nil
	}
	if filename == "" {
		return tool.NewErrorResult(fmt.Errorf("filename is required")), nil
	}
	if text == "" {
		return tool.NewErrorResult(fmt.Errorf("text is required")), nil
	}

	result, err := t.svc.Ingest(ctx, rag.IngestRequest{
		CorpusID: corpusID,
		Filename: filename,
		Text:     text,
	})
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("rag ingest failed: %w", err)), nil
	}

	msg := fmt.Sprintf("Ingested %q into corpus %q: %d chunk(s) indexed (artifact: %s).",
		filename, corpusID, result.Chunks, result.Artifact.Key)

	res := tool.NewJSONResult(map[string]any{
		"corpus_id":    corpusID,
		"filename":     filename,
		"artifact_key": result.Artifact.Key,
		"chunks":       result.Chunks,
	})
	res.Content = msg
	return res, nil
}

func (t *IngestTool) Description(_ context.Context) (string, error) {
	return t.Definition().Description, nil
}

func (t *IngestTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (t *IngestTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.AllowWithInput("", input)
}

func (t *IngestTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *IngestTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *IngestTool) IsEnabled() bool                         { return t.svc != nil }

func (t *IngestTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

func (t *IngestTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
