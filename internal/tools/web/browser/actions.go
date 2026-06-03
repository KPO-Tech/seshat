package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func (t *clickTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameClick,
		DisplayName: "BrowserClick",
		SearchHint:  "click an indexed interactive browser element",
		Description: "Click an interactive element from the latest browser snapshot using its element_id. Use this after browser_snapshot and prefer stable element IDs from the snapshot instead of CSS selectors.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"element_id": map[string]any{"type": "string", "description": "Element ID from browser_snapshot, such as e1."},
				"revision":   map[string]any{"type": "string", "description": "Snapshot revision returned by browser_snapshot. Required to guard against stale DOM actions."},
				"page_id":    map[string]any{"type": "string", "description": "Optional page ID. Defaults to the active page."},
			},
			"required": []string{"element_id", "revision"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *clickTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "click", readOptionalString(input.Parsed, "page_id"), readRequiredString(input.Parsed, "element_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	elementID := readRequiredString(input.Parsed, "element_id")
	revision := readRequiredString(input.Parsed, "revision")
	pageInfo, err := t.ensureManager().Click(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), elementID, revision)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Clicked %s on browser page %s", elementID, pageInfo.ID)), nil
}

func (t *clickTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *clickTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateRequiredString(input, "element_id"); err != nil {
		return nil, err
	}
	if _, err := validateRequiredString(input, "revision"); err != nil {
		return nil, err
	}
	return validateOptionalString(input, "page_id")
}
func (t *clickTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "click")
}
func (t *clickTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *clickTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *clickTool) IsEnabled() bool                             { return true }
func (t *clickTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *clickTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "click")
}
func (t *clickTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "click")), nil
}

func (t *typeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameType,
		DisplayName: "BrowserType",
		SearchHint:  "type text into an indexed browser input element",
		Description: "Type text into an editable element from the latest browser snapshot using its element_id. Use this after browser_snapshot to fill an input or textarea by element ID.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"element_id": map[string]any{"type": "string", "description": "Element ID from browser_snapshot, such as e1."},
				"revision":   map[string]any{"type": "string", "description": "Snapshot revision returned by browser_snapshot. Required to guard against stale DOM actions."},
				"text":       map[string]any{"type": "string", "description": "Text to type into the element."},
				"clear":      map[string]any{"type": "boolean", "description": "Whether to clear existing text first."},
				"page_id":    map[string]any{"type": "string", "description": "Optional page ID. Defaults to the active page."},
			},
			"required": []string{"element_id", "revision", "text"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *typeTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "type", readOptionalString(input.Parsed, "page_id"), readRequiredString(input.Parsed, "element_id"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	elementID := readRequiredString(input.Parsed, "element_id")
	revision := readRequiredString(input.Parsed, "revision")
	text := readRequiredString(input.Parsed, "text")
	clear := readOptionalBool(input.Parsed, "clear")
	pageInfo, err := t.ensureManager().Type(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), elementID, revision, text, clear)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Typed into %s on browser page %s", elementID, pageInfo.ID)), nil
}

func (t *typeTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *typeTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateRequiredString(input, "element_id"); err != nil {
		return nil, err
	}
	if _, err := validateRequiredString(input, "revision"); err != nil {
		return nil, err
	}
	if _, err := validateRequiredString(input, "text"); err != nil {
		return nil, err
	}
	if value, ok := input["clear"]; ok {
		if _, isBool := value.(bool); !isBool {
			return nil, fmt.Errorf("clear must be a boolean")
		}
	}
	return validateOptionalString(input, "page_id")
}
func (t *typeTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "type")
}
func (t *typeTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *typeTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *typeTool) IsEnabled() bool                             { return true }
func (t *typeTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *typeTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "type")
}
func (t *typeTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "type")), nil
}

func (t *pressTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNamePress,
		DisplayName: "BrowserPress",
		SearchHint:  "press a keyboard key in the active browser page",
		Description: "Send a keyboard key such as Enter, Tab, Escape, ArrowDown, or PageDown to the active browser page. Use this when a page requires keyboard interaction, such as submitting a form or moving focus.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":     map[string]any{"type": "string", "description": "Keyboard key to press, such as Enter, Tab, Escape, ArrowDown, or PageDown."},
				"page_id": map[string]any{"type": "string", "description": "Optional page ID. Defaults to the active page."},
			},
			"required": []string{"key"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *pressTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "press", readOptionalString(input.Parsed, "page_id"), readRequiredString(input.Parsed, "key"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	key := readRequiredString(input.Parsed, "key")
	pageInfo, err := t.ensureManager().Press(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), key)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Pressed %s on browser page %s", key, pageInfo.ID)), nil
}

func (t *pressTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *pressTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateRequiredString(input, "key"); err != nil {
		return nil, err
	}
	return validateOptionalString(input, "page_id")
}
func (t *pressTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "press")
}
func (t *pressTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *pressTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *pressTool) IsEnabled() bool                             { return true }
func (t *pressTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *pressTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "press")
}
func (t *pressTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "press")), nil
}

func (t *scrollTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameScroll,
		DisplayName: "BrowserScroll",
		SearchHint:  "scroll the active browser page",
		Description: "Scroll the active browser page up, down, left, or right by a configurable amount. Use this before a new snapshot when content is below or above the current viewport.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string", "description": "Scroll direction: down, up, left, or right. Defaults to down."},
				"amount":    map[string]any{"type": "number", "description": "Optional scroll amount in pixels. Defaults to 600."},
				"page_id":   map[string]any{"type": "string", "description": "Optional page ID. Defaults to the active page."},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *scrollTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "scroll", readOptionalString(input.Parsed, "page_id"), readOptionalString(input.Parsed, "direction"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	pageInfo, err := t.ensureManager().Scroll(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), browsercore.ScrollOptions{
		Direction: readOptionalString(input.Parsed, "direction"),
		Amount:    readOptionalInt(input.Parsed, "amount"),
	})
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Scrolled browser page %s", pageInfo.ID)), nil
}

func (t *scrollTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *scrollTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateOptionalString(input, "direction"); err != nil {
		return nil, err
	}
	if _, err := validateOptionalString(input, "page_id"); err != nil {
		return nil, err
	}
	if value, ok := input["amount"]; ok {
		if _, isNumber := value.(float64); !isNumber {
			return nil, fmt.Errorf("amount must be a number")
		}
	}
	return input, nil
}
func (t *scrollTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "scroll")
}
func (t *scrollTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *scrollTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *scrollTool) IsEnabled() bool                             { return true }
func (t *scrollTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *scrollTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "scroll")
}
func (t *scrollTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "scroll")), nil
}

func (t *waitTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameWait,
		DisplayName: "BrowserWait",
		SearchHint:  "wait for a browser page to stabilize or contain specific text",
		Description: "Wait for the active browser page to stabilize, or until page text contains a requested string. Use this between navigation and snapshot, or before another interaction when the page needs time to update.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page_id":    map[string]any{"type": "string", "description": "Optional page ID. Defaults to the active page."},
				"text":       map[string]any{"type": "string", "description": "Optional text to wait for in the page body."},
				"timeout_ms": map[string]any{"type": "number", "description": "Optional timeout in milliseconds."},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *waitTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "wait", readOptionalString(input.Parsed, "page_id"), readOptionalString(input.Parsed, "text"))
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	pageInfo, err := t.ensureManager().Wait(ctx, sessionID, readOptionalString(input.Parsed, "page_id"), browsercore.WaitOptions{
		Text:      readOptionalString(input.Parsed, "text"),
		TimeoutMs: readOptionalInt(input.Parsed, "timeout_ms"),
	})
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(pageInfo, fmt.Sprintf("Browser page %s is ready", pageInfo.ID)), nil
}

func (t *waitTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *waitTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if _, err := validateOptionalString(input, "page_id"); err != nil {
		return nil, err
	}
	if _, err := validateOptionalString(input, "text"); err != nil {
		return nil, err
	}
	if value, ok := input["timeout_ms"]; ok {
		if _, isNumber := value.(float64); !isNumber {
			return nil, fmt.Errorf("timeout_ms must be a number")
		}
	}
	return input, nil
}
func (t *waitTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "wait")
}
func (t *waitTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *waitTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *waitTool) IsEnabled() bool                             { return true }
func (t *waitTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *waitTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "wait")
}
func (t *waitTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "wait")), nil
}
