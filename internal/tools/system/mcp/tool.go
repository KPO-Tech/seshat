package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// MCPClientManager manages MCP server connections
type MCPClientManager struct {
	clients map[string]*Client
}

// Global MCP client manager
var globalMCPManager = &MCPClientManager{
	clients: make(map[string]*Client),
}

// GlobalMCPManager returns the global MCP client manager
func GlobalMCPManager() *MCPClientManager {
	return globalMCPManager
}

// GetClient returns an MCP client by server name
func (m *MCPClientManager) GetClient(serverName string) (*Client, bool) {
	client, ok := m.clients[serverName]
	return client, ok
}

// ListServers returns all connected server names
func (m *MCPClientManager) ListServers() []string {
	servers := make([]string, 0, len(m.clients))
	for name := range m.clients {
		servers = append(servers, name)
	}
	return servers
}

// MCPTool implements a template MCP tool that forwards to connected MCP servers
type MCPTool struct {
	manager *MCPClientManager
}

// NewMCPTool creates a new MCP tool
func NewMCPTool(manager *MCPClientManager) *MCPTool {
	if manager == nil {
		manager = GlobalMCPManager()
	}

	toolInstance := &MCPTool{
		manager: manager,
	}

	// Auto-connect MCP servers on tool creation
	go func() {
		ctx := context.Background()
		cwd, _ := os.Getwd()
		if cwd == "" {
			cwd = "."
		}
		ConnectMcpServers(ctx, manager, cwd) //nolint:errcheck // background MCP setup
	}()

	return toolInstance
}

// Definition returns the tool definition
func (t *MCPTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameMCP,
		DisplayName:        "mcp",
		SearchHint:         SearchHintMCP,
		Description:        DescriptionMCP,
		Category:           "mcp",
		IsReadOnly:         true,
		IsDestructive:      false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		Metadata: map[string]any{
			"is_mcp":           true,
			"surface_profiles": []string{"mono_run"},
		},
	}
}

// Call executes an MCP tool call
func (t *MCPTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	// Parse input to extract tool name and arguments
	var parsedInput map[string]any
	if err := json.Unmarshal([]byte(input.Raw), &parsedInput); err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("Failed to parse input: %v", err)},
			Content: fmt.Sprintf("Failed to parse input: %v", err),
		}, nil
	}

	// Extract tool name (the tool itself provides this via call name)
	toolName := ""
	args := make(map[string]any)

	for k, v := range parsedInput {
		if k == "_tool" || k == "tool" {
			toolName, _ = v.(string)
		} else if k == "_server" || k == "server" {
			// server is handled separately
		} else {
			args[k] = v
		}
	}

	// Parse tool name to extract server and tool
	// Format: mcp__server__toolname or just toolname
	serverName := ""
	localToolName := toolName

	if strings.HasPrefix(toolName, "mcp__") {
		parts := strings.SplitN(strings.TrimPrefix(toolName, "mcp__"), "__", 2)
		if len(parts) == 2 {
			serverName = parts[0]
			localToolName = parts[1]
		}
	}

	// Find the client
	client, ok := t.manager.GetClient(serverName)
	if !ok {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("MCP server '%s' not connected", serverName)},
			Content: fmt.Sprintf("MCP server '%s' not connected", serverName),
		}, nil
	}

	// Call the MCP tool
	result, err := client.CallTool(ctx, localToolName, args)
	if err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("MCP tool call failed: %v", err)},
			Content: fmt.Sprintf("MCP tool call failed: %v", err),
		}, nil
	}

	// Format result
	resultBytes, _ := json.Marshal(result)

	return tool.CallResult{
		Data:    result,
		Content: string(resultBytes),
	}, nil
}

// ValidateInput validates tool input
func (t *MCPTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks tool-specific permissions
func (t *MCPTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	// MCP tool access requires explicit permission
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

// IsConcurrencySafe returns whether the tool can run concurrently
func (t *MCPTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether the tool is read-only
func (t *MCPTool) IsReadOnly(input map[string]any) bool {
	// MCP tools may modify data - be conservative
	return false
}

// IsEnabled returns whether the tool is enabled
func (t *MCPTool) IsEnabled() bool {
	return true
}

// FormatResult formats the tool output
func (t *MCPTool) FormatResult(data any) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(bytes)
}

// BackfillInput enriches input with derived fields
func (t *MCPTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// Description returns a human-readable description of the tool
func (t *MCPTool) Description(ctx context.Context) (string, error) {
	return DescriptionMCP, nil
}
