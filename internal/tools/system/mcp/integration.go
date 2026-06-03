package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

// ServerResult captures the outcome of integrating one MCP server.
type ServerResult struct {
	// Name is the server name from ServerConfig.
	Name string
	// ToolsRegistered is the number of tools successfully registered.
	ToolsRegistered int
	// Error is non-nil if the server could not be connected, initialized, or wrapped.
	Error error
}

// IntegrationResult represents the result of MCP integration
type IntegrationResult struct {
	// MCPTools are the wrapped MCP tools (across all servers that succeeded).
	MCPTools []tool.Tool

	// ServerResults holds per-server outcome — one entry per configured server.
	// Inspect this to diagnose which servers loaded and which failed.
	ServerResults []ServerResult

	// Error is set only when zero tools were registered across all configured
	// servers. For partial failures, inspect ServerResults instead.
	Error error

	clients   []*Client
	closeOnce sync.Once
	closeErr  error
}

// IntegrateMCPServers takes a tool registry and a list of MCP server configurations,
// creates MCP clients, and registers all wrapped tools (tools, resources, prompts,
// plus listMcpResources and readMcpResource) in the registry.
func IntegrateMCPServers(ctx context.Context, registry *tool.Registry, serverConfigs []ServerConfig) *IntegrationResult {
	return IntegrateMCPServersWithOptions(ctx, registry, serverConfigs, nil)
}

func IntegrateMCPServersWithOptions(ctx context.Context, registry *tool.Registry, serverConfigs []ServerConfig, options *IntegrationOptions) *IntegrationResult {
	result := &IntegrationResult{
		MCPTools:      make([]tool.Tool, 0),
		ServerResults: make([]ServerResult, 0, len(serverConfigs)),
	}

	for _, serverConfig := range serverConfigs {
		sr := ServerResult{Name: serverConfig.Name}

		client, err := NewClient(serverConfig)
		if err != nil {
			sr.Error = fmt.Errorf("create client: %w", err)
			log.Printf("[mcp] server %q: %v", serverConfig.Name, sr.Error)
			result.ServerResults = append(result.ServerResults, sr)
			continue
		}

		if err := client.Start(ctx); err != nil {
			sr.Error = fmt.Errorf("start: %w", err)
			log.Printf("[mcp] server %q: %v", serverConfig.Name, sr.Error)
			result.ServerResults = append(result.ServerResults, sr)
			continue
		}

		_, err = client.Initialize(ctx)
		if err != nil {
			sr.Error = fmt.Errorf("initialize: %w", err)
			log.Printf("[mcp] server %q: %v", serverConfig.Name, sr.Error)
			client.Close()
			result.ServerResults = append(result.ServerResults, sr)
			continue
		}

		wrapper := NewWrapper(client, serverConfig.Name, options)

		mcpTools, err := wrapper.WrapAll(ctx)
		if err != nil {
			sr.Error = fmt.Errorf("wrap tools: %w", err)
			log.Printf("[mcp] server %q: %v", serverConfig.Name, sr.Error)
			client.Close()
			result.ServerResults = append(result.ServerResults, sr)
			continue
		}

		for _, mcpTool := range mcpTools {
			if err := registry.Register(mcpTool); err != nil {
				log.Printf("[mcp] server %q: skip tool %q: %v", serverConfig.Name, mcpTool.Definition().Name, err)
				continue
			}
			result.MCPTools = append(result.MCPTools, mcpTool)
			sr.ToolsRegistered++
		}
		result.clients = append(result.clients, client)

		log.Printf("[mcp] server %q: registered %d/%d tools", serverConfig.Name, sr.ToolsRegistered, len(mcpTools))
		result.ServerResults = append(result.ServerResults, sr)
	}

	if len(serverConfigs) > 0 && len(result.MCPTools) == 0 {
		result.Error = fmt.Errorf("no MCP tools registered from %d configured server(s)", len(serverConfigs))
	}

	return result
}

// Close releases all successfully integrated MCP clients. It is safe to call
// multiple times.
func (r *IntegrationResult) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		var errs []error
		for _, client := range r.clients {
			if client == nil {
				continue
			}
			if err := client.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		r.closeErr = errors.Join(errs...)
	})
	return r.closeErr
}
