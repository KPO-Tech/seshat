package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (t *listPagesTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameListPages,
		DisplayName:        "BrowserListPages",
		SearchHint:         "list pages in the current browser session",
		Description:        "List all browser pages associated with the current Nexus session. Use this to inspect the current browser tab set before selecting or closing a page.",
		Category:           "browser",
		InputSchema:        schema.FromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *listPagesTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "list_pages")
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	pages, err := t.ensureManager().ListPages(ctx, sessionID)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pages, formatPages(pages)), nil
}

func (t *listPagesTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *listPagesTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if len(input) > 0 {
		return nil, fmt.Errorf("browser_list_pages takes no input")
	}
	return input, nil
}
func (t *listPagesTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "list_pages")
}
func (t *listPagesTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *listPagesTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *listPagesTool) IsEnabled() bool                             { return true }
func (t *listPagesTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *listPagesTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "list_pages")
}
func (t *listPagesTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "list_pages")), nil
}

func (t *networkListTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameNetwork,
		DisplayName: "BrowserNetworkList",
		SearchHint:  "list recent browser network activity for the current session or page",
		Description: "List recent browser network activity captured for the current session. Optionally filter by page_id and cap the number of entries. Use this after navigation or interaction when you need to inspect requests, responses, or failures that the page triggered.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID. When omitted, returns recent activity across the current browser session.",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Optional maximum number of entries to return. Defaults to 25.",
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		MaxResultSize:      12000,
	}
}

func (t *networkListTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "network_list", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	entries, err := t.ensureManager().ListNetwork(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), readOptionalInt(input.Parsed, "limit"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(entries, formatNetwork(entries)), nil
}

func (t *networkListTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *networkListTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if value, ok := input["limit"]; ok {
		if _, isNumber := value.(float64); !isNumber {
			return nil, fmt.Errorf("limit must be a number")
		}
	}
	return validateOptionalString(input, "page_id")
}
func (t *networkListTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "network_list")
}
func (t *networkListTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *networkListTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *networkListTool) IsEnabled() bool                             { return true }
func (t *networkListTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *networkListTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "network_list")
}
func (t *networkListTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "network_list")), nil
}

func (t *downloadListTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameDownloads,
		DisplayName: "BrowserDownloadList",
		SearchHint:  "list recent browser downloads for the current session or page",
		Description: "List recent browser downloads captured for the current session. Optionally filter by page_id and cap the number of entries. Use this after clicks or navigations that may have triggered a file download.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID. When omitted, returns downloads across the current browser session.",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Optional maximum number of entries to return. Defaults to 25.",
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		MaxResultSize:      12000,
	}
}

func (t *downloadListTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "list_downloads", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	entries, err := t.ensureManager().ListDownloads(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), readOptionalInt(input.Parsed, "limit"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(entries, formatDownloads(entries)), nil
}

func (t *downloadListTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *downloadListTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if value, ok := input["limit"]; ok {
		if _, isNumber := value.(float64); !isNumber {
			return nil, fmt.Errorf("limit must be a number")
		}
	}
	return validateOptionalString(input, "page_id")
}
func (t *downloadListTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "list_downloads")
}
func (t *downloadListTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *downloadListTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *downloadListTool) IsEnabled() bool                             { return true }
func (t *downloadListTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *downloadListTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "list_downloads")
}
func (t *downloadListTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "list_downloads")), nil
}

func (t *selectPageTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameSelect,
		DisplayName: "BrowserSelectPage",
		SearchHint:  "select the active browser page by page id",
		Description: "Select the active browser page to target subsequent page-less operations. Use this after browser_list_pages when multiple tabs or pages exist and you want to switch focus.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Page ID to mark active.",
				},
			},
			"required": []string{"page_id"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *selectPageTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "select_page", readRequiredString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	pageID := readRequiredString(input.Parsed, "page_id")
	if pageID == "" {
		return tool.NewErrorResult(fmt.Errorf("page_id is required")), nil
	}
	pageInfo, err := t.ensureManager().SelectPage(ctx, sessionID, pageID)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Selected browser page %s", pageInfo.ID)), nil
}

func (t *selectPageTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *selectPageTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return validateRequiredString(input, "page_id")
}
func (t *selectPageTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "select_page")
}
func (t *selectPageTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *selectPageTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *selectPageTool) IsEnabled() bool                             { return true }
func (t *selectPageTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *selectPageTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "select_page")
}
func (t *selectPageTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "select_page")), nil
}

func (t *closePageTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameClose,
		DisplayName: "BrowserClosePage",
		SearchHint:  "close a browser page by page id",
		Description: "Close a browser page in the current Nexus session. If page_id is omitted, the active page is closed. Use this to reduce browser session state when a tab is no longer needed.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional page ID to close. Defaults to the active page.",
				},
			},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *closePageTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "close_page", readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	sessionState, err := t.ensureManager().ClosePage(ctx, sessionID, readOptionalString(input.Parsed, "page_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(sessionState, fmt.Sprintf("Closed browser page. Active page: %s", sessionState.ActivePageID)), nil
}

func (t *closePageTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *closePageTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return validateOptionalString(input, "page_id")
}
func (t *closePageTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "close_page")
}
func (t *closePageTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *closePageTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *closePageTool) IsEnabled() bool                             { return true }
func (t *closePageTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *closePageTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "close_page")
}
func (t *closePageTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "close_page")), nil
}
