package webfetch

import (
	"context"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
)

// permissionInput enriches the tool payload with stable derived fields used by the permission layer.
func permissionInput(input map[string]any, executionMode string) map[string]any {
	enriched := webcore.EnrichFetchPermissionInput(input)
	if executionMode != "" {
		enriched["execution_mode"] = executionMode
	}
	if normalizedURL, parsedURL, err := fetchcore.NormalizeURL(readOptionalString(input, "url")); err == nil {
		enriched["url"] = normalizedURL
		enriched["domain"] = parsedURL.Hostname()
		enriched["host"] = webcore.NormalizeHost(parsedURL.Hostname())
		enriched["path"] = parsedURL.EscapedPath()
	}
	if mode, err := fetchcore.NormalizeRenderMode(readOptionalString(input, "render_mode")); err == nil {
		enriched["render_mode"] = mode
	}
	return enriched
}

func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	_ = ctx
	return webcore.EvaluatePermission(permissionInput(input, toolCtx.ExecutionMode))
}

// BackfillInput adds derived fields so downstream permission and transcript code sees normalized values.
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	_ = ctx
	return permissionInput(input, "")
}

// PreparePermissionMatcher compiles content-specific permission matching for fetch URL, host, and render mode rules.
func (t *Tool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	_ = ctx
	return webcore.PermissionMatcher(permissionInput(input, "")), nil
}
