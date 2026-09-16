package ragtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/rag"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
)

// DeleteTool implements the rag_delete tool.
type DeleteTool struct {
	svc *rag.Service
}

// NewDeleteTool creates a new rag_delete tool backed by the given RAG service.
func NewDeleteTool(svc *rag.Service) *DeleteTool {
	return &DeleteTool{svc: svc}
}

func (t *DeleteTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolDeleteName,
		DisplayName: "RAG Delete",
		SearchHint:  DeleteHint,
		Description: "Delete an entire corpus (omit file_id/filename), or just one file's chunks within a corpus (provide file_id or filename). " +
			"Use this to clean up a corpus, or before re-ingesting a file under a different file_id so the old chunks don't linger.",
		Category: "rag",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"corpus_id": map[string]any{
					"type":        "string",
					"description": "The corpus namespace to delete from.",
				},
				"file_id": map[string]any{
					"type":        "string",
					"description": "Delete only this file's chunks instead of the whole corpus. Matches the file_id (or filename, if file_id was omitted) used at ingest time.",
				},
				"filename": map[string]any{
					"type":        "string",
					"description": "Alias for file_id — use whichever value was used as file_id at ingest time (they default to the same thing).",
				},
			},
			"required": []string{"corpus_id"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
	}
}

func (t *DeleteTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	corpusID, _ := input.Parsed["corpus_id"].(string)
	fileID, _ := input.Parsed["file_id"].(string)
	filename, _ := input.Parsed["filename"].(string)

	corpusID = strings.TrimSpace(corpusID)
	fileID = strings.TrimSpace(fileID)
	filename = strings.TrimSpace(filename)
	if fileID == "" {
		fileID = filename
	}

	if corpusID == "" {
		return tool.NewErrorResult(fmt.Errorf("corpus_id is required")), nil
	}

	if fileID == "" {
		if err := t.svc.DeleteNamespace(ctx, corpusID); err != nil {
			return tool.NewErrorResult(fmt.Errorf("rag delete failed: %w", err)), nil
		}
		msg := fmt.Sprintf("Deleted corpus %q.", corpusID)
		res := tool.NewJSONResult(map[string]any{
			"corpus_id": corpusID,
			"scope":     "corpus",
		})
		res.Content = msg
		return res, nil
	}

	artifactKey := rag.ArtifactKey(corpusID, fileID)
	if err := t.svc.DeleteFileChunks(ctx, corpusID, artifactKey, 0); err != nil {
		return tool.NewErrorResult(fmt.Errorf("rag delete failed: %w", err)), nil
	}
	msg := fmt.Sprintf("Deleted chunks for %q in corpus %q (artifact: %s).", fileID, corpusID, artifactKey)
	res := tool.NewJSONResult(map[string]any{
		"corpus_id":    corpusID,
		"file_id":      fileID,
		"artifact_key": artifactKey,
		"scope":        "file",
	})
	res.Content = msg
	return res, nil
}

func (t *DeleteTool) Description(_ context.Context) (string, error) {
	return t.Definition().Description, nil
}

func (t *DeleteTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (t *DeleteTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.AllowWithInput("", input)
}

func (t *DeleteTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *DeleteTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *DeleteTool) IsEnabled() bool                         { return t.svc != nil }

func (t *DeleteTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	b, _ := json.Marshal(data)
	return string(b)
}

func (t *DeleteTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
