package ragtool

import (
	"context"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/rag"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
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
		Description: "Chunk a text document into a named corpus for later search. Embeds each chunk for semantic search when an embedding provider is configured; otherwise chunks are indexed for keyword/BM25 search instead (still works, no embedder required). Re-ingesting the same file_id (or, absent that, the same filename) replaces its chunks in-place instead of duplicating them. Returns the artifact key and chunk count.",
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
				"file_id": map[string]any{
					"type":        "string",
					"description": "Stable identifier for this document within the corpus. Re-ingesting with the same file_id replaces the previous version's chunks instead of creating duplicates. Defaults to filename when omitted.",
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
	fileID, _ := input.Parsed["file_id"].(string)

	corpusID = strings.TrimSpace(corpusID)
	filename = strings.TrimSpace(filename)
	text = strings.TrimSpace(text)
	fileID = strings.TrimSpace(fileID)

	if corpusID == "" {
		return tool.NewErrorResult(fmt.Errorf("corpus_id is required")), nil
	}
	if filename == "" {
		return tool.NewErrorResult(fmt.Errorf("filename is required")), nil
	}
	if text == "" {
		return tool.NewErrorResult(fmt.Errorf("text is required")), nil
	}
	// Default file_id to filename so re-ingesting the same filename is
	// idempotent by default - the caller only needs to pass file_id
	// explicitly when the same filename legitimately refers to different
	// documents within one corpus.
	if fileID == "" {
		fileID = filename
	}

	result, err := t.svc.Ingest(ctx, rag.IngestRequest{
		CorpusID: corpusID,
		FileID:   fileID,
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
		"file_id":      fileID,
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
