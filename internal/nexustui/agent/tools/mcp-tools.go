package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools/mcp"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/config"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/permission"
	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// whitelistDockerTools contains Docker MCP tools that don't require permission.
var whitelistDockerTools = []string{
	"mcp_docker_mcp-find",
	"mcp_docker_mcp-add",
	"mcp_docker_mcp-remove",
	"mcp_docker_mcp-config-set",
	"mcp_docker_code-mode",
}

// GetMCPTools returns all currently available nexustui MCP tools as SDK tools.
func GetMCPTools(permissions permission.Service, cfg *config.ConfigStore, wd string) []tool.Tool {
	var result []tool.Tool
	for mcpName, mcpTools := range mcp.Tools() {
		for _, t := range mcpTools {
			result = append(result, &MCPTool{
				mcpName:     mcpName,
				mcpTool:     t,
				permissions: permissions,
				workingDir:  wd,
				cfg:         cfg,
			})
		}
	}
	return result
}

// MCPTool wraps a nexustui MCP tool as a contract.Tool so it can be
// registered with the SDK and called through the standard tool pipeline.
type MCPTool struct {
	mcpName     string
	mcpTool     *mcp.Tool
	cfg         *config.ConfigStore
	permissions permission.Service
	workingDir  string
}

func (m *MCPTool) toolName() string {
	return fmt.Sprintf("mcp_%s_%s", m.mcpName, m.mcpTool.Name)
}

func (m *MCPTool) Definition() tool.Definition {
	var inputSchema schema.JSONSchema
	if m.mcpTool.InputSchema != nil {
		if raw, err := json.Marshal(m.mcpTool.InputSchema); err == nil {
			_ = json.Unmarshal(raw, &inputSchema)
		}
	}
	return tool.Definition{
		Name:               m.toolName(),
		DisplayName:        m.mcpTool.Name,
		Description:        m.mcpTool.Description,
		InputSchema:        inputSchema,
		RequiresPermission: !slices.Contains(whitelistDockerTools, m.toolName()),
		IsMCP:              true,
	}
}

func (m *MCPTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	inputJSON, err := json.Marshal(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	result, err := mcp.RunTool(ctx, m.cfg, m.mcpName, m.mcpTool.Name, string(inputJSON))
	if err != nil {
		return tool.NewTextResult(fmt.Sprintf("error: %s", err)), nil
	}

	return tool.NewTextResult(result.Content), nil
}

func (m *MCPTool) Description(_ context.Context) (string, error) {
	return m.mcpTool.Description, nil
}

func (m *MCPTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (m *MCPTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	if slices.Contains(whitelistDockerTools, m.toolName()) {
		return types.AllowWithInput("", input)
	}
	return types.Passthrough(input)
}

func (m *MCPTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (m *MCPTool) IsReadOnly(_ map[string]any) bool         { return false }
func (m *MCPTool) IsEnabled() bool                          { return true }

func (m *MCPTool) FormatResult(data any) string {
	return fmt.Sprintf("%v", data)
}

func (m *MCPTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// compile-time check
var _ contract.Tool = (*MCPTool)(nil)
