package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func (t *screenshotTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameScreenshot,
		DisplayName: "BrowserScreenshot",
		SearchHint:  "capture a browser screenshot",
		Description: "Capture a PNG screenshot of the active browser page and return it as base64 data. Use this when a visual snapshot is needed beyond text extraction.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID. Defaults to the active page.",
				},
				"full_page": map[string]any{
					"type":        "boolean",
					"description": "Whether to capture the full page instead of only the viewport.",
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *screenshotTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "screenshot", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	screenshot, err := t.ensureManager().Screenshot(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), browsercore.ScreenshotOptions{
		FullPage: readOptionalBool(input.Parsed, "full_page"),
	})
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(screenshot, formatScreenshot(screenshot)), nil
}

func (t *screenshotTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *screenshotTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateOptionalString(input, "page_id"); err != nil {
		return nil, err
	}
	if value, ok := input["full_page"]; ok {
		if _, isBool := value.(bool); !isBool {
			return nil, fmt.Errorf("full_page must be a boolean")
		}
	}
	return input, nil
}
func (t *screenshotTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "screenshot")
}
func (t *screenshotTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *screenshotTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *screenshotTool) IsEnabled() bool                             { return true }
func (t *screenshotTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *screenshotTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "screenshot")
}
func (t *screenshotTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "screenshot")), nil
}
