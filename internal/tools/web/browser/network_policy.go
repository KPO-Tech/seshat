package browser

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func (t *getNetworkPolicyTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameGetPolicy,
		DisplayName:        "BrowserGetNetworkPolicy",
		SearchHint:         "inspect the current browser session network blocking policy",
		Description:        "Return the current browser session request blocking policy, including blocked URL patterns and resource policy preset. Use this to inspect whether the current session is blocking images, media, analytics, or custom URL patterns.",
		Category:           "browser",
		InputSchema:        schema.FromMap(map[string]any{"type": "object", "properties": map[string]any{}}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *getNetworkPolicyTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "network_policy")
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	policy, err := t.ensureManager().GetNetworkPolicy(ctx, sessionID)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(policy, formatJSONish(policy)), nil
}

func (t *getNetworkPolicyTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *getNetworkPolicyTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if len(input) > 0 {
		return nil, fmt.Errorf("browser_get_network_policy takes no input")
	}
	return input, nil
}
func (t *getNetworkPolicyTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "network_policy")
}
func (t *getNetworkPolicyTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *getNetworkPolicyTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *getNetworkPolicyTool) IsEnabled() bool                             { return true }
func (t *getNetworkPolicyTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *getNetworkPolicyTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "network_policy")
}
func (t *getNetworkPolicyTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "network_policy")), nil
}

func (t *setNetworkPolicyTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameSetPolicy,
		DisplayName: "BrowserSetNetworkPolicy",
		SearchHint:  "set browser session request blocking rules",
		Description: "Replace the current browser session request blocking policy. This affects existing and future pages in the same browser session and can reduce browser noise or bandwidth by blocking images, fonts, media, analytics, or explicit URL patterns.",
		Category:    "browser",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blocked_urls": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of URL glob patterns to block, such as *.png* or *analytics.example.com*.",
				},
				"resource_policy": map[string]any{
					"type":        "string",
					"description": "Optional preset: default, text_only, or aggressive.",
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

func (t *setNetworkPolicyTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	_ = permissionCheck
	sessionID, err := t.prepareAction(input, "set_network_policy")
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	policy := browsercore.NetworkPolicy{
		BlockedURLs:    readStringArray(input.Parsed, "blocked_urls"),
		ResourcePolicy: readOptionalString(input.Parsed, "resource_policy"),
	}
	updated, err := t.ensureManager().SetNetworkPolicy(ctx, sessionID, policy)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return jsonResult(updated, formatJSONish(updated)), nil
}

func (t *setNetworkPolicyTool) Description(ctx context.Context) (string, error) {
	return t.Definition().Description, nil
}
func (t *setNetworkPolicyTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if raw := readOptionalString(input, "resource_policy"); raw != "" {
		switch raw {
		case "default", "text_only", "aggressive":
		default:
			return nil, fmt.Errorf("resource_policy must be default, text_only, or aggressive")
		}
	}
	if _, ok := input["blocked_urls"]; ok {
		if _, ok := input["blocked_urls"].([]any); !ok {
			if _, ok := input["blocked_urls"].([]string); !ok {
				return nil, fmt.Errorf("blocked_urls must be an array of strings")
			}
		}
	}
	return input, nil
}
func (t *setNetworkPolicyTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return browserPermissionResult(ctx, t.ensureManager(), input, toolCtx, "set_network_policy")
}
func (t *setNetworkPolicyTool) IsConcurrencySafe(input map[string]any) bool { return false }
func (t *setNetworkPolicyTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *setNetworkPolicyTool) IsEnabled() bool                             { return true }
func (t *setNetworkPolicyTool) FormatResult(data any) string                { return formatJSONish(data) }
func (t *setNetworkPolicyTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "set_network_policy")
}
func (t *setNetworkPolicyTool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	return browserPermissionMatcher(browserPermissionInput(ctx, nil, input, tool.ToolUseContext{}, "set_network_policy")), nil
}

func readStringArray(input map[string]any, key string) []string {
	switch values := input[key].(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
