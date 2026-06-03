package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (t *searchContentTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameSearch,
		DisplayName: "BrowserSearchContent",
		SearchHint:  "search across previously captured browser snapshots in the current session",
		Description: "Search across the text, titles, URLs, and headings of snapshots previously captured in the current browser session. Use this after taking snapshots on multiple pages or tabs when you need to find which page already contains a topic before navigating again.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query to match against stored snapshot content."},
				"limit": map[string]any{"type": "number", "description": "Optional maximum number of matching pages to return."},
			},
			"required": []string{"query"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		MaxResultSize:      12000,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *searchContentTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "search_content", readRequiredString(input.Parsed, "query"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	query := readRequiredString(input.Parsed, "query")
	if query == "" {
		return tool.NewErrorResult(fmt.Errorf("query is required")), nil
	}
	results, err := t.ensureManager().SearchSnapshots(ctx, sessionID, query, readOptionalInt(input.Parsed, "limit"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(results, formatJSONish(results)), nil
}

func (t *searchContentTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *searchContentTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if query := readRequiredString(input, "query"); query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if value, ok := input["limit"]; ok {
		if _, isNumber := value.(float64); !isNumber {
			return nil, fmt.Errorf("limit must be a number")
		}
	}
	return input, nil
}
func (t *searchContentTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "search_content")
}
func (t *searchContentTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *searchContentTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *searchContentTool) IsEnabled() bool                             { return true }
func (t *searchContentTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *searchContentTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "search_content")
}
func (t *searchContentTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "search_content")), nil
}
