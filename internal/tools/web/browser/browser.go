// Package browser exposes native browser tools on top of the shared browser runtime.
package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

const (
	ToolNameOpen       = "browser_open"
	ToolNameNavigate   = "browser_navigate"
	ToolNameSnapshot   = "browser_snapshot"
	ToolNameExtract    = "browser_extract"
	ToolNameListPages  = "browser_list_pages"
	ToolNameNetwork    = "browser_network_list"
	ToolNameDownloads  = "browser_list_downloads"
	ToolNameSearch     = "browser_search_content"
	ToolNameGetPolicy  = "browser_get_network_policy"
	ToolNameSetPolicy  = "browser_set_network_policy"
	ToolNameSelect     = "browser_select_page"
	ToolNameClose      = "browser_close_page"
	ToolNameClick      = "browser_click"
	ToolNameType       = "browser_type"
	ToolNamePress      = "browser_press"
	ToolNameScroll     = "browser_scroll"
	ToolNameWait       = "browser_wait"
	ToolNameScreenshot = "browser_screenshot"
)

// managerTool wires one tool instance to the shared browser runtime.
type managerTool struct {
	manager browsercore.Manager
}

func newManagerTool(manager browsercore.Manager) managerTool {
	if manager == nil {
		manager = browsercore.DefaultManager()
	}
	return managerTool{manager: manager}
}

func (t managerTool) ensureManager() browsercore.Manager {
	if t.manager == nil {
		return browsercore.DefaultManager()
	}
	return t.manager
}

func (t managerTool) sessionID(input tool.CallInput) (types.SessionID, error) {
	toolCtx := input.ToolContextValue()
	if toolCtx.SessionID != "" {
		return toolCtx.SessionID, nil
	}
	if input.SessionID != "" {
		return input.SessionID, nil
	}
	return "", fmt.Errorf("browser tools require a session ID")
}

func (t managerTool) prepareAction(input tool.CallInput, action string, details ...string) (types.SessionID, error) {
	sessionID, err := t.sessionID(input)
	if err != nil {
		return "", err
	}
	if err := enforceBrowserTurnBudget(input, action, details...); err != nil {
		return "", err
	}
	return sessionID, nil
}

type openTool struct{ managerTool }
type navigateTool struct{ managerTool }
type snapshotTool struct{ managerTool }
type extractTool struct{ managerTool }
type listPagesTool struct{ managerTool }
type networkListTool struct{ managerTool }
type downloadListTool struct{ managerTool }
type searchContentTool struct{ managerTool }
type getNetworkPolicyTool struct{ managerTool }
type setNetworkPolicyTool struct{ managerTool }
type selectPageTool struct{ managerTool }
type closePageTool struct{ managerTool }
type clickTool struct{ managerTool }
type typeTool struct{ managerTool }
type pressTool struct{ managerTool }
type scrollTool struct{ managerTool }
type waitTool struct{ managerTool }
type screenshotTool struct{ managerTool }

func NewOpenTool(manager browsercore.Manager) tool.Tool { return &openTool{newManagerTool(manager)} }
func NewNavigateTool(manager browsercore.Manager) tool.Tool {
	return &navigateTool{newManagerTool(manager)}
}
func NewSnapshotTool(manager browsercore.Manager) tool.Tool {
	return &snapshotTool{newManagerTool(manager)}
}
func NewExtractTool(manager browsercore.Manager) tool.Tool {
	return &extractTool{newManagerTool(manager)}
}
func NewListPagesTool(manager browsercore.Manager) tool.Tool {
	return &listPagesTool{newManagerTool(manager)}
}
func NewNetworkListTool(manager browsercore.Manager) tool.Tool {
	return &networkListTool{newManagerTool(manager)}
}
func NewDownloadListTool(manager browsercore.Manager) tool.Tool {
	return &downloadListTool{newManagerTool(manager)}
}
func NewSearchContentTool(manager browsercore.Manager) tool.Tool {
	return &searchContentTool{newManagerTool(manager)}
}
func NewGetNetworkPolicyTool(manager browsercore.Manager) tool.Tool {
	return &getNetworkPolicyTool{newManagerTool(manager)}
}
func NewSetNetworkPolicyTool(manager browsercore.Manager) tool.Tool {
	return &setNetworkPolicyTool{newManagerTool(manager)}
}
func NewSelectPageTool(manager browsercore.Manager) tool.Tool {
	return &selectPageTool{newManagerTool(manager)}
}
func NewClosePageTool(manager browsercore.Manager) tool.Tool {
	return &closePageTool{newManagerTool(manager)}
}
func NewClickTool(manager browsercore.Manager) tool.Tool { return &clickTool{newManagerTool(manager)} }
func NewTypeTool(manager browsercore.Manager) tool.Tool  { return &typeTool{newManagerTool(manager)} }
func NewPressTool(manager browsercore.Manager) tool.Tool { return &pressTool{newManagerTool(manager)} }
func NewScrollTool(manager browsercore.Manager) tool.Tool {
	return &scrollTool{newManagerTool(manager)}
}
func NewWaitTool(manager browsercore.Manager) tool.Tool { return &waitTool{newManagerTool(manager)} }
func NewScreenshotTool(manager browsercore.Manager) tool.Tool {
	return &screenshotTool{newManagerTool(manager)}
}

func (t *openTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameOpen,
		DisplayName: "BrowserOpen",
		SearchHint:  "open a browser page in an isolated browser session",
		Description: "Open a new browser page in the current Nexus session. Use this to start browser-based exploration or interaction. Prefer it before browser_navigate when no page exists yet.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Optional URL to open. Defaults to about:blank.",
				},
			},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *openTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "open", readOptionalString(input.Parsed, "url"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	pageInfo, err := t.ensureManager().OpenPage(ctx, sessionID, readOptionalString(input.Parsed, "url"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Opened browser page %s at %s", pageInfo.ID, pageInfo.URL)), nil
}

func (t *openTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *openTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return validateOptionalString(input, "url")
}
func (t *openTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "open")
}
func (t *openTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *openTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *openTool) IsEnabled() bool                             { return true }
func (t *openTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *openTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "open")
}
func (t *openTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "open")), nil
}

func (t *navigateTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameNavigate,
		DisplayName: "BrowserNavigate",
		SearchHint:  "navigate the active browser page to a URL",
		Description: "Navigate an existing browser page to a URL. If page_id is omitted, the active page is used. Use this after browser_open or browser_select_page.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Target URL to navigate to.",
				},
				"page_id": map[string]any{
					"type":        "string",
					"description": "Optional target page ID. Defaults to the active page.",
				},
			},
			"required": []string{"url"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *navigateTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "navigate", readOptionalString(input.Parsed, "page_id"), readOptionalString(input.Parsed, "url"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	rawURL := readRequiredString(input.Parsed, "url")
	if rawURL == "" {
		return tool.NewErrorResult(fmt.Errorf("url is required")), nil
	}
	pageInfo, err := t.ensureManager().Navigate(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), rawURL)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Navigated browser page %s to %s", pageInfo.ID, pageInfo.URL)), nil
}

func (t *navigateTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *navigateTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateRequiredString(input, "url"); err != nil {
		return nil, err
	}
	return validateOptionalString(input, "page_id")
}
func (t *navigateTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "navigate")
}
func (t *navigateTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *navigateTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *navigateTool) IsEnabled() bool                             { return true }
func (t *navigateTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *navigateTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "navigate")
}
func (t *navigateTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "navigate")), nil
}
