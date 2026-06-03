package lsp

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// CheckPermissions checks if the LSP operation is allowed
func CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	operation, ok := input["operation"].(string)
	if !ok || operation == "" {
		return types.Deny("operation is required")
	}

	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return types.Deny("file_path is required")
	}

	// Check if the operation is valid
	validOperations := map[string]bool{
		"definition":       true,
		"references":       true,
		"hover":            true,
		"symbols":          true,
		"implementations":  true,
		"incoming_calls":   true,
		"outgoing_calls":   true,
		"workspace_symbol": true,
	}

	if !validOperations[operation] {
		return types.Deny(fmt.Sprintf("invalid operation: %s", operation))
	}

	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}
