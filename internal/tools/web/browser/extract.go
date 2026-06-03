package browser

import (
	"context"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func (t *snapshotTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameSnapshot,
		DisplayName: "BrowserSnapshot",
		SearchHint:  "capture a compact text snapshot of the current browser page",
		Description: "Capture a compact agent-facing snapshot of the current browser page including visible text and indexed interactive elements. Use this after navigation or interaction to inspect page state before taking another action.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID. Defaults to the active page.",
				},
				"max_text": map[string]any{
					"type":        "number",
					"description": "Optional maximum number of text characters to return.",
				},
				"max_elements": map[string]any{
					"type":        "number",
					"description": "Optional maximum number of interactive elements to enumerate.",
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		MaxResultSize:      12000,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *snapshotTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "snapshot", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	options := browsercore.SnapshotOptions{
		MaxText:     readOptionalInt(input.Parsed, "max_text"),
		MaxElements: readOptionalInt(input.Parsed, "max_elements"),
	}
	snapshot, err := t.ensureManager().Snapshot(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), options)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(snapshot, formatSnapshot(snapshot)), nil
}

func (t *snapshotTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *snapshotTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *snapshotTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "snapshot")
}
func (t *snapshotTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *snapshotTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *snapshotTool) IsEnabled() bool                             { return true }
func (t *snapshotTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *snapshotTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "snapshot")
}
func (t *snapshotTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "snapshot")), nil
}

func (t *extractTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameExtract,
		DisplayName: "BrowserExtract",
		SearchHint:  "extract page text from the current browser page",
		Description: "Extract a richer text-oriented view of the current browser page, including semantic headings when available. Use this when you want readable page content without the full interactive snapshot payload.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID. Defaults to the active page.",
				},
				"max_text": map[string]any{
					"type":        "number",
					"description": "Optional maximum number of text characters to return. Defaults higher than browser_snapshot.",
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		MaxResultSize:      16000,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *extractTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "extract", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	maxText := readOptionalInt(input.Parsed, "max_text")
	if maxText <= 0 {
		maxText = 12000
	}
	snapshot, err := t.ensureManager().Snapshot(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), browsercore.SnapshotOptions{
		MaxText:     maxText,
		MaxElements: 0,
	})
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(snapshot, formatExtract(snapshot)), nil
}

func (t *extractTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *extractTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *extractTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "extract")
}
func (t *extractTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *extractTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *extractTool) IsEnabled() bool                             { return true }
func (t *extractTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *extractTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "extract")
}
func (t *extractTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "extract")), nil
}
