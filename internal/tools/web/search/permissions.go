package websearch

import (
	"context"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// permissionInput enriches search inputs with normalized domain and provider metadata for the permission pipeline.
func (t *Tool) permissionInput(input map[string]any, executionMode string) map[string]any {
	enriched := webcore.EnrichSearchPermissionInput(input)
	if executionMode != "" {
		enriched["execution_mode"] = executionMode
	}
	if mode := t.service.ProviderMode(); mode != "" {
		enriched["provider_mode"] = mode
	}
	return enriched
}

// CheckPermissions applies shared web policy before delegating any remaining decision to the global permission pipeline.
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	_ = ctx
	return webcore.EvaluatePermission(t.permissionInput(input, toolCtx.ExecutionMode))
}

// BackfillInput enriches a shallow clone of the parsed input with derived permission fields.
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	_ = ctx
	return t.permissionInput(input, "")
}

// PreparePermissionMatcher compiles content-specific permission matching for domain- and provider-aware search rules.
func (t *Tool) PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(string) bool, error) {
	_ = ctx
	return webcore.PermissionMatcher(t.permissionInput(input, "")), nil
}
